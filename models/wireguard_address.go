package models

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"deploycrate-ce/internal/storage"
	internalwireguard "deploycrate-ce/internal/wireguard"
)

const wireGuardAddressAllocationLock = "deploycrate-wireguard-address-allocation"

func (wireGuardAddress) Next(pool string, existing []string) (string, error) {
	network := netip.MustParsePrefix(pool)
	mesh := netip.MustParsePrefix(internalwireguard.MeshCIDR)
	used := make(map[netip.Addr]struct{}, len(existing)+1)
	used[netip.MustParseAddr(internalwireguard.ControlPlaneAddress)] = struct{}{}
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

func (address wireGuardAddress) Allocate(
	ctx context.Context,
	db storage.Executor,
	pool string,
) (string, error) {
	if _, err := db.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtext(?))",
		wireGuardAddressAllocationLock,
	); err != nil {
		return "", fmt.Errorf("lock WireGuard address allocation: %w", err)
	}
	allocated := make([]string, 0)
	if err := db.NewSelect().
		TableExpr("wireguard_address_reservations").
		Column("private_address").
		OrderExpr("private_address").
		Scan(ctx, &allocated); err != nil {
		return "", fmt.Errorf("load WireGuard address reservations: %w", err)
	}
	return address.Next(pool, allocated)
}
