package wireguard

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	MeshCIDR            = "10.99.0.0/16"
	NodeCIDR            = "10.99.0.0/17"
	DeviceCIDR          = "10.99.128.0/17"
	ControlPlaneAddress = "10.99.0.1"
)

type Peer struct {
	PublicKey           string   `json:"public_key"`
	AllowedIPs          []string `json:"allowed_ips"`
	Endpoint            string   `json:"endpoint"`
	PersistentKeepalive bool     `json:"persistent_keepalive"`
}

type DesiredState struct {
	Configuration string
	Revision      string
}

func BuildPeerConfiguration(peers []Peer) (DesiredState, error) {
	mesh := netip.MustParsePrefix(MeshCIDR)
	seenKeys := make(map[string]struct{}, len(peers))
	seenAllowedIPs := make(map[netip.Prefix]struct{})
	normalized := make([]Peer, 0, len(peers))
	for _, peer := range peers {
		key := strings.TrimSpace(peer.PublicKey)
		decodedKey, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(decodedKey) != 32 {
			return DesiredState{}, errors.New(
				"WireGuard peer public key must be a base64-encoded 32-byte key",
			)
		}
		if _, exists := seenKeys[key]; exists {
			return DesiredState{}, errors.New("duplicate WireGuard peer public key")
		}
		allowedIPs := make([]string, 0, len(peer.AllowedIPs))
		for _, value := range peer.AllowedIPs {
			prefix, prefixErr := netip.ParsePrefix(strings.TrimSpace(value))
			if prefixErr != nil || !prefix.Addr().Is4() || !mesh.Contains(prefix.Addr()) ||
				prefix.Bits() < mesh.Bits() {
				return DesiredState{}, fmt.Errorf("invalid WireGuard peer allowed IP %q", value)
			}
			prefix = prefix.Masked()
			if _, exists := seenAllowedIPs[prefix]; exists {
				return DesiredState{}, fmt.Errorf("duplicate WireGuard peer allowed IP %s", prefix)
			}
			seenAllowedIPs[prefix] = struct{}{}
			allowedIPs = append(allowedIPs, prefix.String())
		}
		if len(allowedIPs) == 0 {
			return DesiredState{}, errors.New("WireGuard peer requires at least one allowed IP")
		}
		slices.Sort(allowedIPs)
		host, port, endpointErr := net.SplitHostPort(strings.TrimSpace(peer.Endpoint))
		portNumber, portErr := strconv.Atoi(port)
		if endpointErr != nil || strings.TrimSpace(host) == "" || portErr != nil ||
			portNumber < 1 ||
			portNumber > 65535 {
			return DesiredState{}, fmt.Errorf("invalid WireGuard peer endpoint %q", peer.Endpoint)
		}
		seenKeys[key] = struct{}{}
		normalized = append(normalized, Peer{
			PublicKey: key, AllowedIPs: allowedIPs, Endpoint: net.JoinHostPort(host, port),
			PersistentKeepalive: peer.PersistentKeepalive,
		})
	}
	slices.SortFunc(normalized, func(left, right Peer) int {
		return strings.Compare(
			strings.Join(left.AllowedIPs, ","),
			strings.Join(right.AllowedIPs, ","),
		)
	})
	var builder strings.Builder
	for _, peer := range normalized {
		builder.WriteString("\n[Peer]\nPublicKey = ")
		builder.WriteString(peer.PublicKey)
		builder.WriteString("\nAllowedIPs = ")
		builder.WriteString(strings.Join(peer.AllowedIPs, ", "))
		builder.WriteString("\nEndpoint = ")
		builder.WriteString(peer.Endpoint)
		builder.WriteByte('\n')
		if peer.PersistentKeepalive {
			builder.WriteString("PersistentKeepalive = 25\n")
		}
	}
	configuration := builder.String()
	digest := sha256.Sum256([]byte(configuration))
	return DesiredState{Configuration: configuration, Revision: hex.EncodeToString(digest[:])}, nil
}

func Apply(ctx context.Context, interfaceName, configurationPath string, desired []byte) error {
	if interfaceName == "" || strings.ContainsAny(interfaceName, "/\\\r\n\x00") {
		return errors.New("WireGuard interface name is invalid")
	}
	if len(desired) == 0 {
		return errors.New("WireGuard desired configuration is empty")
	}
	directory := filepath.Dir(configurationPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create WireGuard configuration directory: %w", err)
	}
	staged, err := os.CreateTemp(directory, ".wg0.conf.")
	if err != nil {
		return fmt.Errorf("stage WireGuard configuration: %w", err)
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if err := staged.Chmod(0o600); err != nil {
		staged.Close()
		return fmt.Errorf("protect staged WireGuard configuration: %w", err)
	}
	if _, err := staged.Write(desired); err != nil {
		staged.Close()
		return fmt.Errorf("write staged WireGuard configuration: %w", err)
	}
	if err := staged.Sync(); err != nil {
		staged.Close()
		return fmt.Errorf("sync staged WireGuard configuration: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged WireGuard configuration: %w", err)
	}

	stripped, err := exec.CommandContext(ctx, "wg-quick", "strip", stagedPath).Output()
	if err != nil {
		return fmt.Errorf("strip staged WireGuard configuration: %w", err)
	}
	live, err := os.CreateTemp(directory, ".wg0.syncconf.")
	if err != nil {
		return fmt.Errorf("stage live WireGuard configuration: %w", err)
	}
	livePath := live.Name()
	defer os.Remove(livePath)
	if err := live.Chmod(0o600); err != nil {
		live.Close()
		return fmt.Errorf("protect live WireGuard configuration: %w", err)
	}
	if _, err := live.Write(stripped); err != nil {
		live.Close()
		return fmt.Errorf("write live WireGuard configuration: %w", err)
	}
	if err := live.Close(); err != nil {
		return fmt.Errorf("close live WireGuard configuration: %w", err)
	}
	if output, err := exec.CommandContext(ctx, "wg", "syncconf", interfaceName, livePath).
		CombinedOutput(); err != nil {
		return fmt.Errorf(
			"apply live WireGuard peers: %w: %s",
			err,
			strings.TrimSpace(string(output)),
		)
	}
	if err := os.Rename(stagedPath, configurationPath); err != nil {
		return fmt.Errorf("persist WireGuard configuration after live apply: %w", err)
	}
	return nil
}
