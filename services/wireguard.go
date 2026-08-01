package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"deploycrate-ce/internal/storage"
)

const WireGuardMeshCIDR = "10.99.0.0/16"
const WireGuardPrivateAddress = "10.99.0.1"
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
	network := netip.MustParsePrefix(WireGuardMeshCIDR)
	seenAddresses := make(map[netip.Addr]struct{}, len(peers))
	seenKeys := make(map[string]struct{}, len(peers))
	normalized := make([]WireGuardDesiredPeer, 0, len(peers))
	for _, peer := range peers {
		address, err := netip.ParseAddr(strings.TrimSpace(peer.PrivateAddress))
		if err != nil || !address.Is4() || !network.Contains(address) || address == network.Addr() || address.String() == "10.99.0.1" {
			return WireGuardDesiredState{}, fmt.Errorf("invalid managed peer address %q", peer.PrivateAddress)
		}
		if _, exists := seenAddresses[address]; exists {
			return WireGuardDesiredState{}, fmt.Errorf("duplicate managed peer address %s", address)
		}
		key := strings.TrimSpace(peer.PublicKey)
		decodedKey, keyErr := base64.StdEncoding.DecodeString(key)
		if keyErr != nil || len(decodedKey) != 32 {
			return WireGuardDesiredState{}, errors.New("managed peer public key must be a base64-encoded 32-byte key")
		}
		if _, exists := seenKeys[key]; exists {
			return WireGuardDesiredState{}, errors.New("duplicate managed peer public key")
		}
		host, port, err := net.SplitHostPort(strings.TrimSpace(peer.Endpoint))
		portNumber, portErr := strconv.Atoi(port)
		if err != nil || strings.TrimSpace(host) == "" || portErr != nil || portNumber < 1 || portNumber > 65535 {
			return WireGuardDesiredState{}, fmt.Errorf("invalid managed peer endpoint %q", peer.Endpoint)
		}
		seenAddresses[address] = struct{}{}
		seenKeys[key] = struct{}{}
		normalized = append(normalized, WireGuardDesiredPeer{
			PublicKey: key, PrivateAddress: address.String(), Endpoint: net.JoinHostPort(host, port),
			PersistentKeepalive: peer.PersistentKeepalive,
		})
	}
	slices.SortFunc(normalized, func(left, right WireGuardDesiredPeer) int {
		return strings.Compare(left.PrivateAddress, right.PrivateAddress)
	})
	var builder strings.Builder
	for _, peer := range normalized {
		builder.WriteString("\n[Peer]\nPublicKey = ")
		builder.WriteString(peer.PublicKey)
		builder.WriteString("\nAllowedIPs = ")
		builder.WriteString(peer.PrivateAddress)
		builder.WriteString("/32\nEndpoint = ")
		builder.WriteString(peer.Endpoint)
		builder.WriteByte('\n')
		if peer.PersistentKeepalive {
			builder.WriteString("PersistentKeepalive = 25\n")
		}
	}
	configuration := builder.String()
	digest := sha256.Sum256([]byte(configuration))
	return WireGuardDesiredState{Configuration: configuration, Revision: hex.EncodeToString(digest[:])}, nil
}

func NextWireGuardPrivateAddress(existing []string) (string, error) {
	network := netip.MustParsePrefix(WireGuardMeshCIDR)
	used := make(map[netip.Addr]struct{}, len(existing)+1)
	used[netip.MustParseAddr("10.99.0.1")] = struct{}{}
	for _, value := range existing {
		address, err := netip.ParseAddr(strings.TrimSpace(value))
		if err != nil || !address.Is4() || !network.Contains(address) {
			return "", fmt.Errorf("invalid allocated WireGuard address %q", value)
		}
		used[address] = struct{}{}
	}
	last := netip.MustParseAddr("10.99.255.255")
	for address := netip.MustParseAddr("10.99.0.2"); address.Compare(last) < 0; address = address.Next() {
		if _, exists := used[address]; !exists {
			return address.String(), nil
		}
	}
	return "", errors.New("WireGuard address pool is exhausted")
}

func AllocateWireGuardPrivateAddress(ctx context.Context, db storage.Executor) (string, error) {
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext(?))", wireGuardAddressAllocationLock); err != nil {
		return "", fmt.Errorf("lock WireGuard address allocation: %w", err)
	}
	allocated := make([]string, 0)
	if err := db.NewSelect().TableExpr("wireguard_address_reservations").
		Column("private_address").OrderExpr("private_address").Scan(ctx, &allocated); err != nil {
		return "", fmt.Errorf("load WireGuard address reservations: %w", err)
	}
	return NextWireGuardPrivateAddress(allocated)
}
