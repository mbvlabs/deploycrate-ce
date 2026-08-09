package services

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"deploycrate-ce/internal/storage"
	internalwireguard "deploycrate-ce/internal/wireguard"
	"deploycrate-ce/models"
)

const WireGuardMeshCIDR = internalwireguard.MeshCIDR
const WireGuardNodeCIDR = internalwireguard.NodeCIDR
const WireGuardDeviceCIDR = internalwireguard.DeviceCIDR
const WireGuardPrivateAddress = internalwireguard.ControlPlaneAddress

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
		if err != nil || !address.Is4() ||
			!netip.MustParsePrefix(WireGuardMeshCIDR).Contains(address) ||
			address == netip.MustParsePrefix(WireGuardMeshCIDR).Addr() ||
			address.String() == WireGuardPrivateAddress {
			return WireGuardDesiredState{}, fmt.Errorf(
				"invalid managed peer address %q",
				peer.PrivateAddress,
			)
		}
		desired = append(desired, internalwireguard.Peer{
			PublicKey: strings.TrimSpace(
				peer.PublicKey,
			),
			AllowedIPs:          []string{address.String() + "/32"},
			Endpoint:            peer.Endpoint,
			PersistentKeepalive: peer.PersistentKeepalive,
		})
	}
	state, err := internalwireguard.BuildPeerConfiguration(desired)
	return WireGuardDesiredState(state), err
}

func nextWireGuardPrivateAddress(pool string, existing []string) (string, error) {
	return models.WireGuardAddress.Next(pool, existing)
}

func NextWireGuardNodeAddress(existing []string) (string, error) {
	return nextWireGuardPrivateAddress(WireGuardNodeCIDR, existing)
}

func NextWireGuardDeviceAddress(existing []string) (string, error) {
	return nextWireGuardPrivateAddress(WireGuardDeviceCIDR, existing)
}

func allocateWireGuardPrivateAddress(
	ctx context.Context,
	db storage.Executor,
	pool string,
) (string, error) {
	return models.WireGuardAddress.Allocate(ctx, db, pool)
}

func AllocateWireGuardNodeAddress(ctx context.Context, db storage.Executor) (string, error) {
	return allocateWireGuardPrivateAddress(ctx, db, WireGuardNodeCIDR)
}

func AllocateWireGuardDeviceAddress(ctx context.Context, db storage.Executor) (string, error) {
	return allocateWireGuardPrivateAddress(ctx, db, WireGuardDeviceCIDR)
}
