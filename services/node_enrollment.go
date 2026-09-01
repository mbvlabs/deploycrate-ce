package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	sshclient "deploycrate-ce/clients/ssh"
	wireguardclient "deploycrate-ce/clients/wireguard"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/hostcommand"
	"deploycrate-ce/internal/nodeinstall"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	internalwireguard "deploycrate-ce/internal/wireguard"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"golang.org/x/crypto/ssh"
)

const nodeInstallerURL = "https://ce-stable.deploycrate.com/ce"

type NodeEnrollment struct {
	db        storage.Pool
	queue     storage.InsertQueue
	config    config.Config
	version   CurrentVersion
	ssh       sshclient.Client
	sshCA     SSHCAService
	wireguard wireguardclient.Client
	telemetry *TelemetryIdentity
}

type CreateNodeInput struct {
	Name         string
	Address      string
	Port         int
	Username     string
	PrivateKey   string
	Passphrase   string
	Capabilities models.ServerCapabilities
}

type NodeEnrollmentDetail struct {
	Server     models.ServerEntity
	Enrollment models.NodeEnrollmentEntity
	Credential models.ServerSSHCredentialEntity
}

func NewNodeEnrollment(
	db storage.Pool,
	queue storage.InsertQueue,
	configuration config.Config,
	version CurrentVersion,
	sshCA SSHCAService,
	telemetry *TelemetryIdentity,
) *NodeEnrollment {
	return &NodeEnrollment{
		db: db, queue: queue, config: configuration, version: version,
		ssh: sshclient.New(), sshCA: sshCA, wireguard: wireguardclient.New(), telemetry: telemetry,
	}
}

func (service *NodeEnrollment) List(ctx context.Context) ([]NodeEnrollmentDetail, error) {
	servers, err := models.Server.ActiveWorkers(ctx, service.db.Executor())
	if err != nil {
		return nil, err
	}
	items := make([]NodeEnrollmentDetail, 0, len(servers))
	for _, server := range servers {
		enrollment, err := models.NodeEnrollment.LatestForServer(
			ctx,
			service.db.Executor(),
			server.ID,
		)
		if err != nil {
			return nil, err
		}
		credential, err := models.ServerSSHCredential.FindForServer(
			ctx,
			service.db.Executor(),
			server.ID,
		)
		if err != nil {
			return nil, err
		}
		items = append(
			items,
			NodeEnrollmentDetail{Server: server, Enrollment: enrollment, Credential: credential},
		)
	}
	return items, nil
}

func (service *NodeEnrollment) Get(
	ctx context.Context,
	id uuid.UUID,
) (NodeEnrollmentDetail, error) {
	enrollment, err := models.NodeEnrollment.Find(ctx, service.db.Executor(), id)
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}
	server, err := models.Server.Find(ctx, service.db.Executor(), enrollment.ServerID)
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}
	credential, err := models.ServerSSHCredential.FindForServer(
		ctx,
		service.db.Executor(),
		server.ID,
	)
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}
	return NodeEnrollmentDetail{Server: server, Enrollment: enrollment, Credential: credential}, nil
}

func (service *NodeEnrollment) Create(
	ctx context.Context,
	input CreateNodeInput,
) (NodeEnrollmentDetail, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Address = strings.TrimSpace(input.Address)
	input.Username = strings.TrimSpace(input.Username)
	if err := validateCreateNodeInput(input); err != nil {
		return NodeEnrollmentDetail{}, errors.Join(models.ErrDomainValidation, err)
	}
	if err := validateSSHPrivateKey(
		[]byte(input.PrivateKey),
		[]byte(input.Passphrase),
	); err != nil {
		return NodeEnrollmentDetail{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{Field: "privateKey", Code: "invalid", Message: err.Error()},
			},
		)
	}
	input.Capabilities.Telemetry = true
	capabilities, err := input.Capabilities.JSON()
	if err != nil {
		return NodeEnrollmentDetail{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{Field: "capabilities", Code: "invalid", Message: err.Error()},
			},
		)
	}
	var ipv4Address, ipv6Address string
	if address, parseErr := netip.ParseAddr(input.Address); parseErr == nil {
		if address.Is4() {
			ipv4Address = address.String()
		} else {
			ipv6Address = address.String()
		}
	}
	sshAddress := net.JoinHostPort(input.Address, strconv.Itoa(input.Port))
	hostKey, fingerprint, err := service.ssh.ProbeHostKey(ctx, sshAddress)
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}

	tx, err := service.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}
	defer tx.Rollback()
	allocated, err := AllocateWireGuardNodeAddress(ctx, tx)
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}
	server, err := models.Server.Create(ctx, tx, models.CreateServerData{
		Name:         input.Name,
		Slug:         slug.Make(input.Name),
		Kind:         "worker",
		Capabilities: capabilities,
		Ipv4Address:  ipv4Address,
		Ipv6Address:  ipv6Address,
		IsConfigured: false,
		Address:      input.Address,
	})
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}
	privateKey, err := secretcrypto.EncryptForPurpose(
		[]byte(input.PrivateKey),
		service.config.App.SessionEncryptionKey,
		"node-enrollment/"+server.ID.String()+"/private-key",
	)
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}
	var passphrase []byte
	if input.Passphrase != "" {
		passphrase, err = secretcrypto.EncryptForPurpose(
			[]byte(input.Passphrase),
			service.config.App.SessionEncryptionKey,
			"node-enrollment/"+server.ID.String()+"/private-key-passphrase",
		)
		if err != nil {
			return NodeEnrollmentDetail{}, err
		}
	}
	credential, err := models.ServerSSHCredential.Create(
		ctx,
		tx,
		models.CreateServerSSHCredentialData{
			Username: input.Username, Port: int32(input.Port), EncPrivateKey: privateKey,
			EncPrivateKeyPassphrase: passphrase, KnownHostKey: hostKey, ServerID: server.ID,
		},
	)
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}
	enrollment, err := models.NodeEnrollment.Create(ctx, tx, models.CreateNodeEnrollmentData{
		HostFingerprint: fingerprint, AllocatedAddress: allocated,
		InstallerVersion: string(service.version), ServerID: server.ID,
	})
	if err != nil {
		return NodeEnrollmentDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return NodeEnrollmentDetail{}, err
	}
	return NodeEnrollmentDetail{Server: server, Enrollment: enrollment, Credential: credential}, nil
}

func (service *NodeEnrollment) Confirm(
	ctx context.Context,
	id uuid.UUID,
	fingerprint string,
) error {
	detail, err := service.Get(ctx, id)
	if err != nil {
		return err
	}
	if detail.Enrollment.State != models.NodeEnrollmentAwaitingConfirmation {
		return errors.New("node enrollment is not awaiting host-key confirmation")
	}
	if strings.TrimSpace(fingerprint) != detail.Enrollment.HostFingerprint {
		return errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "fingerprint",
					Code:    "mismatch",
					Message: "fingerprint does not match the discovered SSH host key",
				},
			},
		)
	}
	tx, err := service.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := models.ServerSSHCredential.ConfirmHostKey(ctx, tx, detail.Server.ID); err != nil {
		return err
	}
	if err := models.NodeEnrollment.Transition(
		ctx,
		tx,
		id,
		models.NodeEnrollmentQueued,
		"queued",
		nil,
	); err != nil {
		return err
	}
	inserted, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.NodeEnrollmentArgs{EnrollmentID: id},
		jobs.NodeEnrollmentInsertOpts(id),
	)
	if err != nil {
		return err
	}
	if err := models.NodeEnrollment.SetJob(ctx, tx, id, inserted.Job.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *NodeEnrollment) Retry(ctx context.Context, id uuid.UUID) error {
	detail, err := service.Get(ctx, id)
	if err != nil {
		return err
	}
	if detail.Enrollment.State != models.NodeEnrollmentFailed ||
		!detail.Credential.HostKeyConfirmedAt.Valid ||
		len(detail.Credential.EncPrivateKey) == 0 {
		return errors.New("node enrollment cannot be retried")
	}
	tx, err := service.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := models.NodeEnrollment.Transition(
		ctx,
		tx,
		id,
		models.NodeEnrollmentQueued,
		"queued",
		nil,
	); err != nil {
		return err
	}
	inserted, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.NodeEnrollmentArgs{EnrollmentID: id},
		jobs.NodeEnrollmentInsertOpts(id),
	)
	if err != nil {
		return err
	}
	if err := models.NodeEnrollment.SetJob(ctx, tx, id, inserted.Job.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *NodeEnrollment) Execute(ctx context.Context, id uuid.UUID) (returnErr error) {
	defer func() {
		if returnErr != nil {
			_ = models.NodeEnrollment.Transition(
				context.WithoutCancel(ctx),
				service.db.Executor(),
				id,
				models.NodeEnrollmentFailed,
				"failed",
				returnErr,
			)
		}
	}()
	detail, err := service.Get(ctx, id)
	if err != nil {
		return err
	}
	if !detail.Credential.HostKeyConfirmedAt.Valid {
		return errors.New("SSH host key has not been confirmed")
	}
	if err := models.NodeEnrollment.Transition(
		ctx,
		service.db.Executor(),
		id,
		models.NodeEnrollmentInstalling,
		"remote_install",
		nil,
	); err != nil {
		return err
	}
	peer, peerErr := models.WireGuardPeer.FindActiveForServer(
		ctx,
		service.db.Executor(),
		detail.Server.ID,
	)
	if peerErr != nil && !errors.Is(peerErr, sql.ErrNoRows) {
		return peerErr
	}
	if errors.Is(peerErr, sql.ErrNoRows) {
		privateKey, passphrase, err := service.decryptBootstrapCredential(detail)
		if err != nil {
			return err
		}
		manifest, err := service.manifest(ctx, detail)
		if err != nil {
			return err
		}
		manifestJSON, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		publicAddress := net.JoinHostPort(
			detail.Server.Address,
			strconv.Itoa(int(detail.Credential.Port)),
		)
		command := fmt.Sprintf(`set -eu
if [ "$(id -u)" -eq 0 ]; then dc_sudo=""; else dc_sudo="sudo -n"; fi
curl -fsSL %s | ${dc_sudo} env DEPLOYCRATE_INSTALL_ONLY=1 bash
${dc_sudo} /usr/local/bin/bootstrap node install --manifest-stdin`, nodeInstallerURL)
		remote, err := service.ssh.Run(ctx, publicAddress, sshclient.Credentials{
			Username: detail.Credential.Username, PrivateKey: privateKey, Passphrase: passphrase,
			HostKey: detail.Credential.KnownHostKey,
		}, command, bytes.NewReader(manifestJSON))
		if err != nil {
			return err
		}
		result, err := parseNodeInstallResult(remote.Stdout)
		if err != nil {
			return err
		}
		if result.ServerID != detail.Server.ID.String() {
			return errors.New("node installer returned the wrong Server identity")
		}
		installedHostKey, _, _, remainder, parseErr := ssh.ParseAuthorizedKey(
			[]byte(result.SSHHostPublicKey),
		)
		if parseErr != nil || len(bytes.TrimSpace(remainder)) != 0 {
			return errors.New("node installer returned an invalid SSH host key")
		}
		if ssh.FingerprintSHA256(installedHostKey) != detail.Enrollment.HostFingerprint {
			return errors.New("node SSH host key changed during installation")
		}
		detail.Server.OperatingSystem = sql.NullString{String: result.OperatingSystem, Valid: true}
		detail.Server.Distribution = sql.NullString{String: result.Distribution, Valid: true}
		detail.Server.DistributionVersion = sql.NullString{
			String: result.DistributionVersion,
			Valid:  true,
		}
		detail.Server.Architecture = sql.NullString{String: result.Architecture, Valid: true}
		detail.Server.PackageManager = sql.NullString{String: "apt", Valid: true}
		detail.Server.InitSystem = sql.NullString{String: "systemd", Valid: true}
		if _, err := service.updateServer(
			ctx,
			service.db.Executor(),
			detail.Server,
			false,
		); err != nil {
			return err
		}
		peer, err = models.WireGuardPeer.Create(
			ctx,
			service.db.Executor(),
			models.CreateWireGuardPeerData{
				PublicKey:      result.WireGuardPublicKey,
				PrivateAddress: detail.Enrollment.AllocatedAddress,
				Endpoint: sql.NullString{
					String: net.JoinHostPort(detail.Server.Address, "51820"),
					Valid:  true,
				},
				ListenPort:  51820,
				ActivatedAt: time.Now().UTC(),
				ServerID:    detail.Server.ID,
			},
		)
		if err != nil {
			return err
		}
	}
	if err := service.wireguard.ApplyPeer(ctx, peer.PublicKey, peer.PrivateAddress); err != nil {
		return err
	}
	if _, err := hostcommand.Run(
		ctx,
		"node-telemetry-target",
		detail.Server.ID.String(),
		detail.Enrollment.AllocatedAddress,
	); err != nil {
		return err
	}
	if err := models.NodeEnrollment.Transition(
		ctx,
		service.db.Executor(),
		id,
		models.NodeEnrollmentVerifying,
		"ssh_ca_verification",
		nil,
	); err != nil {
		return err
	}
	certificate, err := service.sshCA.GenerateUserCertificate(5 * time.Minute)
	if err != nil {
		return err
	}
	privateAddress := net.JoinHostPort(
		detail.Enrollment.AllocatedAddress,
		strconv.Itoa(int(detail.Credential.Port)),
	)
	verifyCommand := fmt.Sprintf(
		"sudo -n true && curl -fsS http://%s:9100/metrics >/dev/null && curl -fsS http://%s:9101/healthz >/dev/null && curl -fsS http://127.0.0.1:13133/ >/dev/null",
		detail.Enrollment.AllocatedAddress,
		detail.Enrollment.AllocatedAddress,
	)
	if _, err := service.ssh.RunWithCertificate(
		ctx,
		privateAddress,
		"admin",
		detail.Credential.KnownHostKey,
		certificate.PrivateKey,
		certificate.Certificate,
		verifyCommand,
		nil,
	); err != nil {
		return fmt.Errorf("verify permanent SSH access: %w", err)
	}
	harden := `sudo -n sh -c 'printf "%s\n" "PermitRootLogin no" "PasswordAuthentication no" > /etc/ssh/sshd_config.d/99-deploycrate-node-hardening.conf && systemctl reload ssh.service'`
	if _, err := service.ssh.RunWithCertificate(
		ctx,
		privateAddress,
		"admin",
		detail.Credential.KnownHostKey,
		certificate.PrivateKey,
		certificate.Certificate,
		harden,
		nil,
	); err != nil {
		return fmt.Errorf("complete SSH trust transition: %w", err)
	}
	if err := service.reconcileNodeMesh(ctx, detail.Server.ID); err != nil {
		return fmt.Errorf("reconcile Node WireGuard mesh: %w", err)
	}
	tx, err := service.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	refreshedServer, err := models.Server.Find(ctx, tx, detail.Server.ID)
	if err != nil {
		return err
	}
	if _, err := service.updateServer(ctx, tx, refreshedServer, true); err != nil {
		return err
	}
	if err := service.ensureServerNetwork(
		ctx,
		tx,
		detail.Server.ID,
		detail.Enrollment.AllocatedAddress,
	); err != nil {
		return err
	}
	if _, err := models.ServerStatus.Create(ctx, tx, models.CreateServerStatusData{
		State: "ready", ObservedAt: time.Now().UTC(), ServerID: detail.Server.ID,
	}); err != nil {
		return err
	}
	if err := models.ServerSSHCredential.CompleteTrustTransition(
		ctx,
		tx,
		detail.Server.ID,
	); err != nil {
		return err
	}
	if err := models.NodeEnrollment.Transition(
		ctx,
		tx,
		id,
		models.NodeEnrollmentReady,
		"complete",
		nil,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *NodeEnrollment) ensureServerNetwork(
	ctx context.Context,
	db storage.Executor,
	serverID uuid.UUID,
	privateAddress string,
) error {
	networkID, err := models.EnvironmentNetwork.SystemPrivateNetworkID(ctx, db)
	if err != nil {
		return fmt.Errorf("load control-plane private network: %w", err)
	}
	exists, err := models.ServerNetwork.ActiveExists(ctx, db, serverID, networkID)
	if err != nil || exists {
		return err
	}
	now := sql.NullTime{Time: time.Now().UTC(), Valid: true}
	configuration, err := json.Marshal(map[string]any{
		"address": privateAddress, "cidr": WireGuardMeshCIDR, "interface": "wg0",
	})
	if err != nil {
		return err
	}
	_, err = models.ServerNetwork.Create(ctx, db, models.CreateServerNetworkData{
		Driver: "wireguard", ExternalID: sql.NullString{String: "wg0", Valid: true},
		Configuration: configuration, State: "applied", AppliedAt: now, ObservedAt: now,
		ServerID: serverID, PrivateNetworkID: networkID,
	})
	return err
}

func (service *NodeEnrollment) updateServer(
	ctx context.Context,
	db storage.Executor,
	server models.ServerEntity,
	configured bool,
) (models.ServerEntity, error) {
	return models.Server.Update(ctx, db, models.UpdateServerData{
		ID:                  server.ID,
		ArchivedAt:          server.ArchivedAt,
		Name:                server.Name,
		Slug:                server.Slug,
		Kind:                server.Kind,
		Capabilities:        server.Capabilities,
		OperatingSystem:     server.OperatingSystem,
		Distribution:        server.Distribution,
		DistributionVersion: server.DistributionVersion,
		Architecture:        server.Architecture,
		PackageManager:      server.PackageManager,
		InitSystem:          server.InitSystem,
		Ipv4Address:         server.Ipv4Address,
		Ipv6Address:         server.Ipv6Address,
		IsConfigured:        configured,
		Address:             server.Address,
	})
}

func (service *NodeEnrollment) manifest(
	ctx context.Context,
	detail NodeEnrollmentDetail,
) (nodeinstall.Manifest, error) {
	control, err := models.WireGuardPeer.FindActiveByPrivateAddress(
		ctx,
		service.db.Executor(),
		WireGuardPrivateAddress,
	)
	if err != nil {
		return nodeinstall.Manifest{}, fmt.Errorf("load control-plane WireGuard peer: %w", err)
	}
	if !control.Endpoint.Valid {
		return nodeinstall.Manifest{}, errors.New("control-plane WireGuard endpoint is unavailable")
	}
	telemetryEndpoint, err := models.ResourceEndpoint.FindSystemEnvironmentEndpoint(
		ctx,
		service.db.Executor(),
		"opentelemetry",
	)
	if err != nil {
		return nodeinstall.Manifest{}, fmt.Errorf(
			"load control-plane OpenTelemetry Resource endpoint: %w",
			err,
		)
	}
	if telemetryEndpoint.Address != control.PrivateAddress {
		return nodeinstall.Manifest{}, errors.New(
			"control-plane OpenTelemetry Resource endpoint does not match the WireGuard peer",
		)
	}
	if err = telemetryEndpoint.ValidateForKind("opentelemetry"); err != nil {
		return nodeinstall.Manifest{}, fmt.Errorf(
			"validate control-plane OpenTelemetry Resource endpoint: %w",
			err,
		)
	}
	telemetryToken, err := service.telemetry.NodeToken(detail.Server.ID)
	if err != nil {
		return nodeinstall.Manifest{}, fmt.Errorf("issue Node telemetry identity: %w", err)
	}
	telemetryJWKSet, err := service.telemetry.PublicJWKSet()
	if err != nil {
		return nodeinstall.Manifest{}, fmt.Errorf("encode telemetry identity keys: %w", err)
	}
	userCA, err := os.ReadFile(service.config.SSHCA.UserPrivateKeyPath + ".pub")
	if err != nil {
		return nodeinstall.Manifest{}, fmt.Errorf("read SSH user CA public key: %w", err)
	}
	capabilities, err := models.ParseServerCapabilities(detail.Server.Capabilities)
	if err != nil {
		return nodeinstall.Manifest{}, fmt.Errorf("parse node capabilities: %w", err)
	}
	nodePeers, err := service.activeNodeMeshPeers(ctx, detail.Server.ID, uuid.Nil)
	if err != nil {
		return nodeinstall.Manifest{}, err
	}
	return nodeinstall.Manifest{
		ManifestVersion:       nodeinstall.ManifestVersion,
		ServerID:              detail.Server.ID.String(),
		NodeName:              detail.Server.Name,
		PrivateAddress:        detail.Enrollment.AllocatedAddress,
		ListenPort:            51820,
		SSHPort:               int(detail.Credential.Port),
		ControlPlanePublicKey: control.PublicKey,
		ControlPlaneAddress:   WireGuardPrivateAddress,
		ControlPlaneEndpoint:  control.Endpoint.String,
		NodePeers:             nodePeers,
		SSHUserCAPublicKey:    strings.TrimSpace(string(userCA)),
		OTLPEndpoint:          telemetryEndpoint.URL(),
		TelemetryIssuer:       service.telemetry.Issuer(),
		TelemetryJWKSet:       telemetryJWKSet,
		TelemetryNodeToken:    telemetryToken,
		Capabilities:          capabilities,
	}, nil
}

func (service *NodeEnrollment) activeNodeMeshPeers(
	ctx context.Context,
	excludeServerID, includeServerID uuid.UUID,
) ([]internalwireguard.Peer, error) {
	rows, err := models.WireGuardPeer.ActiveWorkerPeers(
		ctx,
		service.db.Executor(),
		excludeServerID,
		includeServerID,
	)
	if err != nil {
		return nil, fmt.Errorf("load active Node WireGuard peers: %w", err)
	}
	peers := make([]internalwireguard.Peer, 0, len(rows))
	for _, row := range rows {
		peers = append(peers, internalwireguard.Peer{
			PublicKey: row.PublicKey, AllowedIPs: []string{row.PrivateAddress + "/32"},
			Endpoint: row.Endpoint, PersistentKeepalive: true,
		})
	}
	return peers, nil
}

func (service *NodeEnrollment) reconcileNodeMesh(
	ctx context.Context,
	enrollingServerID uuid.UUID,
) error {
	control, err := models.WireGuardPeer.FindActiveByPrivateAddress(
		ctx,
		service.db.Executor(),
		WireGuardPrivateAddress,
	)
	if err != nil {
		return fmt.Errorf("load control-plane WireGuard peer: %w", err)
	}
	if !control.Endpoint.Valid {
		return errors.New("control-plane WireGuard endpoint is unavailable")
	}
	targets, err := models.Server.ActiveWireGuardMeshTargets(
		ctx,
		service.db.Executor(),
		enrollingServerID,
	)
	if err != nil {
		return fmt.Errorf("load Node mesh targets: %w", err)
	}
	certificate, err := service.sshCA.GenerateUserCertificate(5 * time.Minute)
	if err != nil {
		return err
	}
	for _, target := range targets {
		peers, err := service.activeNodeMeshPeers(ctx, target.ServerID, enrollingServerID)
		if err != nil {
			return err
		}
		peers = append(peers, internalwireguard.Peer{
			PublicKey: control.PublicKey,
			AllowedIPs: []string{
				WireGuardPrivateAddress + "/32",
				WireGuardDeviceCIDR,
			},
			Endpoint: control.Endpoint.String, PersistentKeepalive: true,
		})
		desired, err := internalwireguard.BuildPeerConfiguration(peers)
		if err != nil {
			return fmt.Errorf("build Node WireGuard configuration: %w", err)
		}
		configuration := "[Interface]\nListenPort = 51820\n" + desired.Configuration
		address := net.JoinHostPort(target.PrivateAddress, strconv.Itoa(int(target.SSHPort)))
		command := "sudo -n /usr/bin/wg syncconf wg0 /dev/stdin && sudo -n /usr/bin/wg-quick save wg0"
		if _, err := service.ssh.RunWithCertificate(
			ctx,
			address,
			"admin",
			target.KnownHostKey,
			certificate.PrivateKey,
			certificate.Certificate,
			command,
			strings.NewReader(configuration),
		); err != nil {
			return fmt.Errorf(
				"apply Node WireGuard configuration to %s: %w",
				target.PrivateAddress,
				err,
			)
		}
	}
	return nil
}

func (service *NodeEnrollment) decryptBootstrapCredential(
	detail NodeEnrollmentDetail,
) ([]byte, []byte, error) {
	privateKey, err := secretcrypto.DecryptForPurpose(
		detail.Credential.EncPrivateKey,
		service.config.App.SessionEncryptionKey,
		"node-enrollment/"+detail.Server.ID.String()+"/private-key",
	)
	if err != nil {
		return nil, nil, err
	}
	var passphrase []byte
	if len(detail.Credential.EncPrivateKeyPassphrase) > 0 {
		passphrase, err = secretcrypto.DecryptForPurpose(
			detail.Credential.EncPrivateKeyPassphrase,
			service.config.App.SessionEncryptionKey,
			"node-enrollment/"+detail.Server.ID.String()+"/private-key-passphrase",
		)
	}
	return privateKey, passphrase, err
}

func parseNodeInstallResult(output string) (nodeinstall.Result, error) {
	const prefix = "DEPLOYCRATE_NODE_RESULT="
	for line := range strings.SplitSeq(output, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(line, prefix))
		if err != nil {
			return nodeinstall.Result{}, errors.New("decode node installation result")
		}
		var result nodeinstall.Result
		if err := json.Unmarshal(decoded, &result); err != nil {
			return nodeinstall.Result{}, errors.New("parse node installation result")
		}
		return result, nil
	}
	return nodeinstall.Result{}, errors.New("node installer did not return a result")
}

func validateCreateNodeInput(input CreateNodeInput) error {
	var errs validation.ValidationErrors
	if input.Name == "" {
		errs = append(
			errs,
			validation.Error{Field: "name", Code: "required", Message: "name is required"},
		)
	}
	if input.Address == "" || strings.ContainsAny(input.Address, "\r\n\x00 /\\") {
		errs = append(
			errs,
			validation.Error{Field: "address", Code: "invalid", Message: "address is invalid"},
		)
	}
	if input.Port < 1 || input.Port > 65535 {
		errs = append(
			errs,
			validation.Error{
				Field:   "port",
				Code:    "invalid",
				Message: "port must be between 1 and 65535",
			},
		)
	}
	if input.Username == "" {
		errs = append(
			errs,
			validation.Error{Field: "username", Code: "required", Message: "username is required"},
		)
	}
	if strings.TrimSpace(input.PrivateKey) == "" {
		errs = append(
			errs,
			validation.Error{
				Field:   "privateKey",
				Code:    "required",
				Message: "private key is required",
			},
		)
	}
	return errs
}

func validateSSHPrivateKey(privateKey, passphrase []byte) error {
	if len(passphrase) == 0 {
		_, err := ssh.ParsePrivateKey(privateKey)
		return err
	}
	_, err := ssh.ParsePrivateKeyWithPassphrase(privateKey, passphrase)
	return err
}
