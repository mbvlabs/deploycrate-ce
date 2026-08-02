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

	"deploycrate-ce/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

const ManifestVersion = 2

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
	if err != nil || !netip.MustParsePrefix("10.99.0.0/16").Contains(address) || address.String() == "10.99.0.1" {
		errs = append(errs, errors.New("private address must be a worker address in 10.99.0.0/16"))
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
	if addressErr != nil || controlAddress.String() != "10.99.0.1" {
		errs = append(errs, errors.New("control-plane WireGuard address must be 10.99.0.1"))
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
	if otlpErr != nil || otlp.Addr().String() != "10.99.0.1" {
		errs = append(errs, errors.New("OTLP endpoint must be on the control-plane WireGuard address"))
	}
	if err := manifest.Capabilities.Validate(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
