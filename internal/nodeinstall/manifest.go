package nodeinstall

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	internalwireguard "deploycrate-ce/internal/wireguard"
	"deploycrate-ce/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

const ManifestVersion = 3

type Manifest struct {
	ManifestVersion       int                       `json:"manifest_version"`
	ServerID              string                    `json:"server_id"`
	NodeName              string                    `json:"node_name"`
	PrivateAddress        string                    `json:"private_address"`
	ListenPort            int                       `json:"listen_port"`
	SSHPort               int                       `json:"ssh_port"`
	ControlPlanePublicKey string                    `json:"control_plane_public_key"`
	ControlPlaneAddress   string                    `json:"control_plane_address"`
	ControlPlaneEndpoint  string                    `json:"control_plane_endpoint"`
	NodePeers             []internalwireguard.Peer  `json:"node_peers"`
	SSHUserCAPublicKey    string                    `json:"ssh_user_ca_public_key"`
	OTLPEndpoint          string                    `json:"otlp_endpoint"`
	Capabilities          models.ServerCapabilities `json:"capabilities"`
}

type Result struct {
	ServerID            string                    `json:"server_id"`
	WireGuardPublicKey  string                    `json:"wireguard_public_key"`
	SSHHostPublicKey    string                    `json:"ssh_host_public_key"`
	OperatingSystem     string                    `json:"operating_system"`
	Distribution        string                    `json:"distribution"`
	DistributionVersion string                    `json:"distribution_version"`
	Architecture        string                    `json:"architecture"`
	Capabilities        models.ServerCapabilities `json:"capabilities"`
}

func DecodeManifest(value []byte) (Manifest, error) {
	decoder := json.NewDecoder(strings.NewReader(string(value)))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode node installation manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (manifest Manifest) Validate() error {
	var errs []error
	if manifest.ManifestVersion != ManifestVersion {
		errs = append(errs, fmt.Errorf("manifest version must be %d", ManifestVersion))
	}
	if _, err := uuid.Parse(manifest.ServerID); err != nil {
		errs = append(errs, errors.New("server ID must be a UUID"))
	}
	if strings.TrimSpace(manifest.NodeName) == "" || strings.ContainsAny(manifest.NodeName, "\r\n\x00") {
		errs = append(errs, errors.New("node name is invalid"))
	}
	address, err := netip.ParseAddr(strings.TrimSpace(manifest.PrivateAddress))
	if err != nil || !netip.MustParsePrefix(internalwireguard.NodeCIDR).Contains(address) || address.String() == internalwireguard.ControlPlaneAddress || address == netip.MustParsePrefix(internalwireguard.NodeCIDR).Addr() {
		errs = append(errs, errors.New("private address must be an allocatable Node address"))
	}
	if manifest.ListenPort < 1 || manifest.ListenPort > 65535 {
		errs = append(errs, errors.New("WireGuard listen port is invalid"))
	}
	if manifest.SSHPort < 1 || manifest.SSHPort > 65535 {
		errs = append(errs, errors.New("SSH port is invalid"))
	}
	key, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(manifest.ControlPlanePublicKey))
	if decodeErr != nil || len(key) != 32 {
		errs = append(errs, errors.New("control-plane WireGuard public key is invalid"))
	}
	controlAddress, addressErr := netip.ParseAddr(strings.TrimSpace(manifest.ControlPlaneAddress))
	if addressErr != nil || controlAddress.String() != internalwireguard.ControlPlaneAddress {
		errs = append(errs, errors.New("control-plane WireGuard address is invalid"))
	}
	host, port, endpointErr := net.SplitHostPort(strings.TrimSpace(manifest.ControlPlaneEndpoint))
	portNumber, portErr := strconv.Atoi(port)
	if endpointErr != nil || strings.TrimSpace(host) == "" || portErr != nil || portNumber < 1 || portNumber > 65535 {
		errs = append(errs, errors.New("control-plane WireGuard endpoint is invalid"))
	}
	if _, _, _, remainder, caErr := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(manifest.SSHUserCAPublicKey))); caErr != nil || len(strings.TrimSpace(string(remainder))) != 0 {
		errs = append(errs, errors.New("SSH user CA public key is invalid"))
	}
	otlp, otlpErr := netip.ParseAddrPort(strings.TrimSpace(manifest.OTLPEndpoint))
	if otlpErr != nil || otlp.Addr().String() != internalwireguard.ControlPlaneAddress {
		errs = append(errs, errors.New("OTLP endpoint must be on the control-plane WireGuard address"))
	}
	for _, peer := range manifest.NodePeers {
		if len(peer.AllowedIPs) != 1 {
			errs = append(errs, errors.New("each Node peer must own exactly one WireGuard address"))
			continue
		}
		prefix, prefixErr := netip.ParsePrefix(strings.TrimSpace(peer.AllowedIPs[0]))
		if prefixErr != nil || prefix.Bits() != 32 || !netip.MustParsePrefix(internalwireguard.NodeCIDR).Contains(prefix.Addr()) || prefix.Addr() == address || prefix.Addr().String() == internalwireguard.ControlPlaneAddress {
			errs = append(errs, errors.New("Node peer allowed IP must identify another Node"))
		}
	}
	if _, peerErr := manifest.PeerConfiguration(); peerErr != nil {
		errs = append(errs, peerErr)
	}
	if err := manifest.Capabilities.Validate(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (manifest Manifest) PeerConfiguration() (string, error) {
	peers := make([]internalwireguard.Peer, 0, len(manifest.NodePeers)+1)
	peers = append(peers, internalwireguard.Peer{
		PublicKey: manifest.ControlPlanePublicKey,
		AllowedIPs: []string{
			strings.TrimSpace(manifest.ControlPlaneAddress) + "/32",
			internalwireguard.DeviceCIDR,
		},
		Endpoint:            manifest.ControlPlaneEndpoint,
		PersistentKeepalive: true,
	})
	peers = append(peers, manifest.NodePeers...)
	state, err := internalwireguard.BuildPeerConfiguration(peers)
	return state.Configuration, err
}
