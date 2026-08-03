package seeds

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models/factories"
)

// UI creates a complete, factory-only dataset for visually developing the UI.
// It is intentionally direct and non-idempotent. Run it against a fresh database.
func UI(ctx context.Context, exec storage.Executor) error {
	now := time.Now()

	edgePrimary, err := factories.CreateServer(
		ctx,
		exec,
		factories.WithServersName("Edge Primary"),
		factories.WithServersSlug("edge-primary"),
		factories.WithServersKind("worker"),
		factories.WithServersOperatingSystem(sql.NullString{String: "linux", Valid: true}),
		factories.WithServersDistribution(sql.NullString{String: "ubuntu", Valid: true}),
		factories.WithServersDistributionVersion(sql.NullString{String: "24.04", Valid: true}),
		factories.WithServersArchitecture(sql.NullString{String: "amd64", Valid: true}),
		factories.WithServersPackageManager(sql.NullString{String: "apt", Valid: true}),
		factories.WithServersInitSystem(sql.NullString{String: "systemd", Valid: true}),
		factories.WithServersCapabilities(
			json.RawMessage(`{"runtime":true,"resource":true,"database":true,"repository":true,"telemetry":true}`),
		),
		factories.WithServersIPv4Address("203.0.113.10"),
		factories.WithServersIPv6Address("2001:db8::10"),
		factories.WithServersIsConfigured(true),
		factories.WithServersAddress("203.0.113.10"),
		factories.WithServersArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create edge primary server: %w", err)
	}

	_, err = factories.CreateServerSSHCredential(
		ctx,
		exec,
		edgePrimary.ID,
		factories.WithServerSshCredentialsUsername("deploy"),
		factories.WithServerSshCredentialsPort(22),
		factories.WithServerSshCredentialsEncPrivateKey(
			[]byte("encrypted-edge-primary-private-key"),
		),
		factories.WithServerSshCredentialsKnownHostKey("ssh-ed25519 AAAA-edge-primary"),
	)
	if err != nil {
		return fmt.Errorf("create edge primary SSH credential: %w", err)
	}

	_, err = factories.CreateServerStatus(
		ctx,
		exec,
		edgePrimary.ID,
		factories.WithServerStatusesState("ready"),
		factories.WithServerStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create edge primary status: %w", err)
	}

	edgePrimaryPeer, err := factories.CreateWireGuardPeer(
		ctx,
		exec,
		edgePrimary.ID,
		factories.WithWireguardPeersPublicKey("A01HgChlFDmNeerZkEsg0ssMLfrWzAaN+YQU17E6jsA="),
		factories.WithWireguardPeersEncPrivateKey(
			[]byte("encrypted-edge-primary-wireguard-private-key"),
		),
		factories.WithWireguardPeersPrivateAddress("10.99.0.10"),
		factories.WithWireguardPeersEndpoint(
			sql.NullString{String: "203.0.113.10:51820", Valid: true},
		),
		factories.WithWireguardPeersListenPort(51820),
		factories.WithWireguardPeersActivatedAt(now),
		factories.WithWireguardPeersRetiredAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create edge primary WireGuard peer: %w", err)
	}

	_, err = factories.CreateWireGuardPeerStatus(ctx, exec, edgePrimaryPeer.ID,
		factories.WithWireguardPeerStatusesState("connected"),
		factories.WithWireguardPeerStatusesLatestHandshakeAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithWireguardPeerStatusesError(sql.NullString{}),
		factories.WithWireguardPeerStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create edge primary WireGuard status: %w", err)
	}

	edgeSecondary, err := factories.CreateServer(
		ctx,
		exec,
		factories.WithServersName("Edge Secondary"),
		factories.WithServersSlug("edge-secondary"),
		factories.WithServersKind("worker"),
		factories.WithServersOperatingSystem(sql.NullString{String: "linux", Valid: true}),
		factories.WithServersDistribution(sql.NullString{String: "debian", Valid: true}),
		factories.WithServersDistributionVersion(sql.NullString{String: "13", Valid: true}),
		factories.WithServersArchitecture(sql.NullString{String: "arm64", Valid: true}),
		factories.WithServersPackageManager(sql.NullString{String: "apt", Valid: true}),
		factories.WithServersInitSystem(sql.NullString{String: "systemd", Valid: true}),
		factories.WithServersCapabilities(
			json.RawMessage(`{"runtime":true,"telemetry":true}`),
		),
		factories.WithServersIPv4Address("203.0.113.11"),
		factories.WithServersIPv6Address("2001:db8::11"),
		factories.WithServersIsConfigured(true),
		factories.WithServersAddress("203.0.113.11"),
		factories.WithServersArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create edge secondary server: %w", err)
	}

	_, err = factories.CreateServerSSHCredential(
		ctx,
		exec,
		edgeSecondary.ID,
		factories.WithServerSshCredentialsUsername("deploy"),
		factories.WithServerSshCredentialsPort(22),
		factories.WithServerSshCredentialsEncPrivateKey(
			[]byte("encrypted-edge-secondary-private-key"),
		),
		factories.WithServerSshCredentialsKnownHostKey("ssh-ed25519 AAAA-edge-secondary"),
	)
	if err != nil {
		return fmt.Errorf("create edge secondary SSH credential: %w", err)
	}

	_, err = factories.CreateServerStatus(
		ctx,
		exec,
		edgeSecondary.ID,
		factories.WithServerStatusesState("ready"),
		factories.WithServerStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create edge secondary status: %w", err)
	}

	edgeSecondaryPeer, err := factories.CreateWireGuardPeer(
		ctx,
		exec,
		edgeSecondary.ID,
		factories.WithWireguardPeersPublicKey("Hmya7lvwcIhy9WNC+GnIWRZlT9OY7XLzK8LSQbByl8Y="),
		factories.WithWireguardPeersEncPrivateKey(
			[]byte("encrypted-edge-secondary-wireguard-private-key"),
		),
		factories.WithWireguardPeersPrivateAddress("10.99.0.11"),
		factories.WithWireguardPeersEndpoint(
			sql.NullString{String: "203.0.113.11:51820", Valid: true},
		),
		factories.WithWireguardPeersListenPort(51820),
		factories.WithWireguardPeersActivatedAt(now),
		factories.WithWireguardPeersRetiredAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create edge secondary WireGuard peer: %w", err)
	}

	_, err = factories.CreateWireGuardPeerStatus(ctx, exec, edgeSecondaryPeer.ID,
		factories.WithWireguardPeerStatusesState("connected"),
		factories.WithWireguardPeerStatusesLatestHandshakeAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithWireguardPeerStatusesError(sql.NullString{}),
		factories.WithWireguardPeerStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create edge secondary WireGuard status: %w", err)
	}

	worker, err := factories.CreateServer(
		ctx,
		exec,
		factories.WithServersName("Build Worker"),
		factories.WithServersSlug("build-worker"),
		factories.WithServersKind("worker"),
		factories.WithServersOperatingSystem(sql.NullString{String: "linux", Valid: true}),
		factories.WithServersDistribution(sql.NullString{String: "alpine", Valid: true}),
		factories.WithServersDistributionVersion(sql.NullString{String: "3.22", Valid: true}),
		factories.WithServersArchitecture(sql.NullString{String: "amd64", Valid: true}),
		factories.WithServersPackageManager(sql.NullString{String: "apk", Valid: true}),
		factories.WithServersInitSystem(sql.NullString{String: "openrc", Valid: true}),
		factories.WithServersCapabilities(
			json.RawMessage(`{"build":true,"runtime":true,"telemetry":true}`),
		),
		factories.WithServersIPv4Address("198.51.100.20"),
		factories.WithServersIPv6Address("2001:db8::20"),
		factories.WithServersIsConfigured(true),
		factories.WithServersAddress("198.51.100.20"),
		factories.WithServersArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create build worker server: %w", err)
	}

	_, err = factories.CreateServerSSHCredential(
		ctx,
		exec,
		worker.ID,
		factories.WithServerSshCredentialsUsername("deploy"),
		factories.WithServerSshCredentialsPort(2222),
		factories.WithServerSshCredentialsEncPrivateKey(
			[]byte("encrypted-build-worker-private-key"),
		),
		factories.WithServerSshCredentialsKnownHostKey("ssh-ed25519 AAAA-build-worker"),
	)
	if err != nil {
		return fmt.Errorf("create build worker SSH credential: %w", err)
	}

	_, err = factories.CreateServerStatus(
		ctx,
		exec,
		worker.ID,
		factories.WithServerStatusesState("ready"),
		factories.WithServerStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create build worker status: %w", err)
	}

	workerPeer, err := factories.CreateWireGuardPeer(
		ctx,
		exec,
		worker.ID,
		factories.WithWireguardPeersPublicKey("7Z0N2UKFv0cG4B6Je+TlfB0xzY5K0mnwx6Wh2Sa2Hbw="),
		factories.WithWireguardPeersEncPrivateKey(
			[]byte("encrypted-build-worker-wireguard-private-key"),
		),
		factories.WithWireguardPeersPrivateAddress("10.99.0.20"),
		factories.WithWireguardPeersEndpoint(
			sql.NullString{String: "198.51.100.20:51820", Valid: true},
		),
		factories.WithWireguardPeersListenPort(51820),
		factories.WithWireguardPeersActivatedAt(now),
		factories.WithWireguardPeersRetiredAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create build worker WireGuard peer: %w", err)
	}

	_, err = factories.CreateWireGuardPeerStatus(ctx, exec, workerPeer.ID,
		factories.WithWireguardPeerStatusesState("connected"),
		factories.WithWireguardPeerStatusesLatestHandshakeAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithWireguardPeerStatusesError(sql.NullString{}),
		factories.WithWireguardPeerStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create build worker WireGuard status: %w", err)
	}

	sharedNetwork, err := factories.CreatePrivateNetwork(ctx, exec, nil,
		factories.WithPrivateNetworksName("Shared Services"),
		factories.WithPrivateNetworksArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create shared services network: %w", err)
	}

	productionNetwork, err := factories.CreatePrivateNetwork(ctx, exec, nil,
		factories.WithPrivateNetworksName("Production Mesh"),
		factories.WithPrivateNetworksArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production network: %w", err)
	}

	stagingNetwork, err := factories.CreatePrivateNetwork(ctx, exec, nil,
		factories.WithPrivateNetworksName("Staging Mesh"),
		factories.WithPrivateNetworksArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create staging network: %w", err)
	}

	_, err = factories.CreateServerNetwork(
		ctx,
		exec,
		sql.NullString{String: "wg-shared-edge-primary", Valid: true},
		edgePrimary.ID,
		sharedNetwork.ID,
		factories.WithServerNetworksDriver("wireguard"),
		factories.WithServerNetworksConfiguration(json.RawMessage(`{"interface":"wg-shared"}`)),
		factories.WithServerNetworksState("applied"),
		factories.WithServerNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksError(sql.NullString{}),
		factories.WithServerNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach edge primary to shared network: %w", err)
	}

	_, err = factories.CreateServerNetwork(
		ctx,
		exec,
		sql.NullString{String: "wg-production-edge-primary", Valid: true},
		edgePrimary.ID,
		productionNetwork.ID,
		factories.WithServerNetworksDriver("wireguard"),
		factories.WithServerNetworksConfiguration(json.RawMessage(`{"interface":"wg-production"}`)),
		factories.WithServerNetworksState("applied"),
		factories.WithServerNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksError(sql.NullString{}),
		factories.WithServerNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach edge primary to production network: %w", err)
	}

	_, err = factories.CreateServerNetwork(
		ctx,
		exec,
		sql.NullString{String: "wg-shared-edge-secondary", Valid: true},
		edgeSecondary.ID,
		sharedNetwork.ID,
		factories.WithServerNetworksDriver("wireguard"),
		factories.WithServerNetworksConfiguration(json.RawMessage(`{"interface":"wg-shared"}`)),
		factories.WithServerNetworksState("applied"),
		factories.WithServerNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksError(sql.NullString{}),
		factories.WithServerNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach edge secondary to shared network: %w", err)
	}

	_, err = factories.CreateServerNetwork(
		ctx,
		exec,
		sql.NullString{String: "wg-production-edge-secondary", Valid: true},
		edgeSecondary.ID,
		productionNetwork.ID,
		factories.WithServerNetworksDriver("wireguard"),
		factories.WithServerNetworksConfiguration(json.RawMessage(`{"interface":"wg-production"}`)),
		factories.WithServerNetworksState("applied"),
		factories.WithServerNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksError(sql.NullString{}),
		factories.WithServerNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach edge secondary to production network: %w", err)
	}

	_, err = factories.CreateServerNetwork(
		ctx,
		exec,
		sql.NullString{String: "wg-shared-build-worker", Valid: true},
		worker.ID,
		sharedNetwork.ID,
		factories.WithServerNetworksDriver("wireguard"),
		factories.WithServerNetworksConfiguration(json.RawMessage(`{"interface":"wg-shared"}`)),
		factories.WithServerNetworksState("applied"),
		factories.WithServerNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksError(sql.NullString{}),
		factories.WithServerNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach build worker to shared network: %w", err)
	}

	_, err = factories.CreateServerNetwork(
		ctx,
		exec,
		sql.NullString{String: "wg-staging-build-worker", Valid: true},
		worker.ID,
		stagingNetwork.ID,
		factories.WithServerNetworksDriver("wireguard"),
		factories.WithServerNetworksConfiguration(json.RawMessage(`{"interface":"wg-staging"}`)),
		factories.WithServerNetworksState("applied"),
		factories.WithServerNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithServerNetworksError(sql.NullString{}),
		factories.WithServerNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach build worker to staging network: %w", err)
	}

	if err := ensureSystemApplication(ctx, exec, now); err != nil {
		return err
	}

	fmt.Println(
		"Created UI seed data: 4 servers, 4 networks, and the DeployCrate CE system topology",
	)

	return nil
}
