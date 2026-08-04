package services

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"deploycrate-ce/internal/storage"
	internalwireguard "deploycrate-ce/internal/wireguard"
)

const WireGuardMeshCIDR = internalwireguard.MeshCIDR
const WireGuardNodeCIDR = internalwireguard.NodeCIDR
const WireGuardDeviceCIDR = internalwireguard.DeviceCIDR
const WireGuardPrivateAddress = internalwireguard.ControlPlaneAddress
const ControlPlaneOTLPEndpoint = "http://" + WireGuardPrivateAddress + ":4318"
const wireGuardAddressAllocationLock = "deploycrate-wireguard-address-allocation"

type WireGuardDesiredPeer struct {
	PublicKey           string
	PrivateAddress      string
	Endpoint            string
	PersistentKeepalive bool
}

type WireGuardDesiredState struct {
	Configuration string
	Revision      string
}

func BuildWireGuardDesiredState(peers []WireGuardDesiredPeer) (WireGuardDesiredState, error) {
	desired := make([]internalwireguard.Peer, 0, len(peers))
	for _, peer := range peers {
		address, err := netip.ParseAddr(strings.TrimSpace(peer.PrivateAddress))
		if err != nil || !address.Is4() || !netip.MustParsePrefix(WireGuardMeshCIDR).Contains(address) || address == netip.MustParsePrefix(WireGuardMeshCIDR).Addr() || address.String() == WireGuardPrivateAddress {
			return WireGuardDesiredState{}, fmt.Errorf("invalid managed peer address %q", peer.PrivateAddress)
		}
		desired = append(desired, internalwireguard.Peer{
			PublicKey: strings.TrimSpace(peer.PublicKey), AllowedIPs: []string{address.String() + "/32"}, Endpoint: peer.Endpoint,
			PersistentKeepalive: peer.PersistentKeepalive,
		})
	}
	state, err := internalwireguard.BuildPeerConfiguration(desired)
	return WireGuardDesiredState(state), err
}

func nextWireGuardPrivateAddress(pool string, existing []string) (string, error) {
	network := netip.MustParsePrefix(pool)
	mesh := netip.MustParsePrefix(WireGuardMeshCIDR)
	used := make(map[netip.Addr]struct{}, len(existing)+1)
	used[netip.MustParseAddr(WireGuardPrivateAddress)] = struct{}{}
	for _, value := range existing {
		address, err := netip.ParseAddr(strings.TrimSpace(value))
		if err != nil || !address.Is4() || !mesh.Contains(address) {
			return "", fmt.Errorf("invalid allocated WireGuard address %q", value)
		}
		used[address] = struct{}{}
	}
	for address := network.Addr().Next(); network.Contains(address) && network.Contains(address.Next()); address = address.Next() {
		if _, exists := used[address]; !exists {
			return address.String(), nil
		}
	}
	return "", errors.New("WireGuard address pool is exhausted")
}

func NextWireGuardNodeAddress(existing []string) (string, error) {
	return nextWireGuardPrivateAddress(WireGuardNodeCIDR, existing)
}

func NextWireGuardDeviceAddress(existing []string) (string, error) {
	return nextWireGuardPrivateAddress(WireGuardDeviceCIDR, existing)
}

func allocateWireGuardPrivateAddress(ctx context.Context, db storage.Executor, pool string) (string, error) {
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext(?))", wireGuardAddressAllocationLock); err != nil {
		return "", fmt.Errorf("lock WireGuard address allocation: %w", err)
	}
	allocated := make([]string, 0)
	if err := db.NewSelect().TableExpr("wireguard_address_reservations").
		Column("private_address").OrderExpr("private_address").Scan(ctx, &allocated); err != nil {
		return "", fmt.Errorf("load WireGuard address reservations: %w", err)
	}
	return nextWireGuardPrivateAddress(pool, allocated)
}

func AllocateWireGuardNodeAddress(ctx context.Context, db storage.Executor) (string, error) {
	return allocateWireGuardPrivateAddress(ctx, db, WireGuardNodeCIDR)
}

func AllocateWireGuardDeviceAddress(ctx context.Context, db storage.Executor) (string, error) {
	return allocateWireGuardPrivateAddress(ctx, db, WireGuardDeviceCIDR)
}
