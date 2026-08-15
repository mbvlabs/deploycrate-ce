package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/telemetry"
)

func resolveDNSServerPublicIPv4(
	ctx context.Context,
	db storage.Executor,
	server models.DNSServerAddress,
) (string, error) {
	for _, candidate := range []string{server.IPv4, server.Addr} {
		if parsed, err := netip.ParseAddr(strings.TrimSpace(candidate)); err == nil &&
			isPublicIPv4(parsed) {
			return parsed.String(), nil
		}
	}
	if server.Kind != "self_hosted" {
		return "", errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Cloudflare-managed DNS requires a public IPv4 address on runtime Server %s; set one in the Server settings",
				server.ID,
			),
		)
	}
	detected, err := detectHostPublicIPv4(ctx)
	if err != nil {
		return "", errors.Join(models.ErrDomainValidation, err)
	}
	if err := models.Server.UpdateIPv4Address(
		ctx,
		db,
		server.ID,
		detected,
		time.Now().UTC(),
	); err != nil {
		return "", err
	}
	return detected, nil
}

func detectHostPublicIPv4(ctx context.Context) (string, error) {
	var lastErr error
	for _, endpoint := range []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"} {
		connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(
			ctx,
			"tcp4",
			endpoint,
		)
		if err != nil {
			lastErr = err
			continue
		}
		local := connection.LocalAddr()
		_ = connection.Close()
		if tcpAddress, ok := local.(*net.TCPAddr); ok {
			if address, ok := netip.AddrFromSlice(tcpAddress.IP); ok && isPublicIPv4(address) {
				return tcpAddress.IP.String(), nil
			}
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api4.ipify.org", nil)
	if err != nil {
		return "", lastErr
	}
	response, err := telemetry.NewHTTPClient(5 * time.Second).Do(request)
	if err != nil {
		return "", errors.Join(lastErr, err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 64))
	if readErr != nil {
		return "", errors.Join(lastErr, readErr)
	}
	value := strings.TrimSpace(string(body))
	if parsed, err := netip.ParseAddr(value); err == nil && isPublicIPv4(parsed) {
		return parsed.String(), nil
	}
	return "", errors.Join(lastErr, errors.New("could not detect the host's public IPv4 address"))
}

func isPublicIPv4(address netip.Addr) bool {
	return address.Is4() && address.IsGlobalUnicast() && !address.IsPrivate() &&
		!address.IsLoopback() &&
		!address.IsLinkLocalUnicast()
}
