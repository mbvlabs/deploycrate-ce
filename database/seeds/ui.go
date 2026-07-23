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

	githubCredential, err := factories.CreateCredential(ctx, exec,
		factories.WithCredentialsName("DeployCrate GitHub"),
		factories.WithCredentialsProvider("github"),
		factories.WithCredentialsMetadata(json.RawMessage(`{"account":"deploycrate-demo","scopes":["repo","read:org"]}`)),
		factories.WithCredentialsEncPayload([]byte("encrypted-github-token")),
		factories.WithCredentialsVerifiedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithCredentialsLastUsedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithCredentialsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create GitHub credential: %w", err)
	}

	gitlabCredential, err := factories.CreateCredential(ctx, exec,
		factories.WithCredentialsName("DeployCrate GitLab"),
		factories.WithCredentialsProvider("gitlab"),
		factories.WithCredentialsMetadata(json.RawMessage(`{"account":"deploycrate-labs","scopes":["read_repository"]}`)),
		factories.WithCredentialsEncPayload([]byte("encrypted-gitlab-token")),
		factories.WithCredentialsVerifiedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithCredentialsLastUsedAt(sql.NullTime{}),
		factories.WithCredentialsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create GitLab credential: %w", err)
	}

	edgePrimary, err := factories.CreateServer(ctx, exec,
		factories.WithServersName("Edge Primary"),
		factories.WithServersSlug("edge-primary"),
		factories.WithServersAddress("203.0.113.10"),
		factories.WithServersArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create edge primary server: %w", err)
	}

	_, err = factories.CreateServerSSHCredential(ctx, exec, edgePrimary.ID,
		factories.WithServerSshCredentialsUsername("deploy"),
		factories.WithServerSshCredentialsPort(22),
		factories.WithServerSshCredentialsEncPrivateKey([]byte("encrypted-edge-primary-private-key")),
		factories.WithServerSshCredentialsKnownHostKey("ssh-ed25519 AAAA-edge-primary"),
	)
	if err != nil {
		return fmt.Errorf("create edge primary SSH credential: %w", err)
	}

	_, err = factories.CreateServerStatus(ctx, exec, edgePrimary.ID,
		factories.WithServerStatusesState("ready"),
		factories.WithServerStatusesOperatingSystem(sql.NullString{String: "linux", Valid: true}),
		factories.WithServerStatusesDistribution(sql.NullString{String: "ubuntu", Valid: true}),
		factories.WithServerStatusesDistributionVersion(sql.NullString{String: "24.04", Valid: true}),
		factories.WithServerStatusesArchitecture(sql.NullString{String: "amd64", Valid: true}),
		factories.WithServerStatusesPackageManager(sql.NullString{String: "apt", Valid: true}),
		factories.WithServerStatusesInitSystem(sql.NullString{String: "systemd", Valid: true}),
		factories.WithServerStatusesCapabilities(json.RawMessage(`{"caddy":true,"docker":true,"wireguard":true}`)),
		factories.WithServerStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create edge primary status: %w", err)
	}

	edgePrimaryPeer, err := factories.CreateWireGuardPeer(ctx, exec, edgePrimary.ID,
		factories.WithWireguardPeersPublicKey("edge-primary-public-key"),
		factories.WithWireguardPeersEncPrivateKey([]byte("encrypted-edge-primary-wireguard-private-key")),
		factories.WithWireguardPeersPrivateAddress("10.99.0.10"),
		factories.WithWireguardPeersEndpoint(sql.NullString{String: "203.0.113.10:51820", Valid: true}),
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

	edgeSecondary, err := factories.CreateServer(ctx, exec,
		factories.WithServersName("Edge Secondary"),
		factories.WithServersSlug("edge-secondary"),
		factories.WithServersAddress("203.0.113.11"),
		factories.WithServersArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create edge secondary server: %w", err)
	}

	_, err = factories.CreateServerSSHCredential(ctx, exec, edgeSecondary.ID,
		factories.WithServerSshCredentialsUsername("deploy"),
		factories.WithServerSshCredentialsPort(22),
		factories.WithServerSshCredentialsEncPrivateKey([]byte("encrypted-edge-secondary-private-key")),
		factories.WithServerSshCredentialsKnownHostKey("ssh-ed25519 AAAA-edge-secondary"),
	)
	if err != nil {
		return fmt.Errorf("create edge secondary SSH credential: %w", err)
	}

	_, err = factories.CreateServerStatus(ctx, exec, edgeSecondary.ID,
		factories.WithServerStatusesState("ready"),
		factories.WithServerStatusesOperatingSystem(sql.NullString{String: "linux", Valid: true}),
		factories.WithServerStatusesDistribution(sql.NullString{String: "debian", Valid: true}),
		factories.WithServerStatusesDistributionVersion(sql.NullString{String: "13", Valid: true}),
		factories.WithServerStatusesArchitecture(sql.NullString{String: "arm64", Valid: true}),
		factories.WithServerStatusesPackageManager(sql.NullString{String: "apt", Valid: true}),
		factories.WithServerStatusesInitSystem(sql.NullString{String: "systemd", Valid: true}),
		factories.WithServerStatusesCapabilities(json.RawMessage(`{"caddy":true,"docker":true,"wireguard":true}`)),
		factories.WithServerStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create edge secondary status: %w", err)
	}

	edgeSecondaryPeer, err := factories.CreateWireGuardPeer(ctx, exec, edgeSecondary.ID,
		factories.WithWireguardPeersPublicKey("edge-secondary-public-key"),
		factories.WithWireguardPeersEncPrivateKey([]byte("encrypted-edge-secondary-wireguard-private-key")),
		factories.WithWireguardPeersPrivateAddress("10.99.0.11"),
		factories.WithWireguardPeersEndpoint(sql.NullString{String: "203.0.113.11:51820", Valid: true}),
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

	worker, err := factories.CreateServer(ctx, exec,
		factories.WithServersName("Build Worker"),
		factories.WithServersSlug("build-worker"),
		factories.WithServersAddress("198.51.100.20"),
		factories.WithServersArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create build worker server: %w", err)
	}

	_, err = factories.CreateServerSSHCredential(ctx, exec, worker.ID,
		factories.WithServerSshCredentialsUsername("deploy"),
		factories.WithServerSshCredentialsPort(2222),
		factories.WithServerSshCredentialsEncPrivateKey([]byte("encrypted-build-worker-private-key")),
		factories.WithServerSshCredentialsKnownHostKey("ssh-ed25519 AAAA-build-worker"),
	)
	if err != nil {
		return fmt.Errorf("create build worker SSH credential: %w", err)
	}

	_, err = factories.CreateServerStatus(ctx, exec, worker.ID,
		factories.WithServerStatusesState("ready"),
		factories.WithServerStatusesOperatingSystem(sql.NullString{String: "linux", Valid: true}),
		factories.WithServerStatusesDistribution(sql.NullString{String: "alpine", Valid: true}),
		factories.WithServerStatusesDistributionVersion(sql.NullString{String: "3.22", Valid: true}),
		factories.WithServerStatusesArchitecture(sql.NullString{String: "amd64", Valid: true}),
		factories.WithServerStatusesPackageManager(sql.NullString{String: "apk", Valid: true}),
		factories.WithServerStatusesInitSystem(sql.NullString{String: "openrc", Valid: true}),
		factories.WithServerStatusesCapabilities(json.RawMessage(`{"buildkit":true,"docker":true,"wireguard":true}`)),
		factories.WithServerStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create build worker status: %w", err)
	}

	workerPeer, err := factories.CreateWireGuardPeer(ctx, exec, worker.ID,
		factories.WithWireguardPeersPublicKey("build-worker-public-key"),
		factories.WithWireguardPeersEncPrivateKey([]byte("encrypted-build-worker-wireguard-private-key")),
		factories.WithWireguardPeersPrivateAddress("10.99.0.20"),
		factories.WithWireguardPeersEndpoint(sql.NullString{String: "198.51.100.20:51820", Valid: true}),
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
		factories.WithPrivateNetworksCidr("10.20.0.0/24"),
		factories.WithPrivateNetworksScope("global"),
		factories.WithPrivateNetworksArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create shared services network: %w", err)
	}

	productionNetwork, err := factories.CreatePrivateNetwork(ctx, exec, nil,
		factories.WithPrivateNetworksName("Production Mesh"),
		factories.WithPrivateNetworksCidr("10.21.0.0/24"),
		factories.WithPrivateNetworksScope("global"),
		factories.WithPrivateNetworksArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production network: %w", err)
	}

	stagingNetwork, err := factories.CreatePrivateNetwork(ctx, exec, nil,
		factories.WithPrivateNetworksName("Staging Mesh"),
		factories.WithPrivateNetworksCidr("10.22.0.0/24"),
		factories.WithPrivateNetworksScope("global"),
		factories.WithPrivateNetworksArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create staging network: %w", err)
	}

	_, err = factories.CreateServerNetwork(ctx, exec, edgePrimary.ID, sharedNetwork.ID, sql.NullString{String: "wg-shared-edge-primary", Valid: true},
		factories.WithServerNetworksAddress("10.20.0.10"),
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

	_, err = factories.CreateServerNetwork(ctx, exec, edgePrimary.ID, productionNetwork.ID, sql.NullString{String: "wg-production-edge-primary", Valid: true},
		factories.WithServerNetworksAddress("10.21.0.10"),
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

	_, err = factories.CreateServerNetwork(ctx, exec, edgeSecondary.ID, sharedNetwork.ID, sql.NullString{String: "wg-shared-edge-secondary", Valid: true},
		factories.WithServerNetworksAddress("10.20.0.11"),
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

	_, err = factories.CreateServerNetwork(ctx, exec, edgeSecondary.ID, productionNetwork.ID, sql.NullString{String: "wg-production-edge-secondary", Valid: true},
		factories.WithServerNetworksAddress("10.21.0.11"),
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

	_, err = factories.CreateServerNetwork(ctx, exec, worker.ID, sharedNetwork.ID, sql.NullString{String: "wg-shared-build-worker", Valid: true},
		factories.WithServerNetworksAddress("10.20.0.20"),
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

	_, err = factories.CreateServerNetwork(ctx, exec, worker.ID, stagingNetwork.ID, sql.NullString{String: "wg-staging-build-worker", Valid: true},
		factories.WithServerNetworksAddress("10.22.0.20"),
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

	storefront, err := factories.CreateApplication(ctx, exec,
		factories.WithApplicationsName("Acme Storefront"),
		factories.WithApplicationsSlug("acme-storefront"),
		factories.WithApplicationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront application: %w", err)
	}

	analytics, err := factories.CreateApplication(ctx, exec,
		factories.WithApplicationsName("Acme Analytics"),
		factories.WithApplicationsSlug("acme-analytics"),
		factories.WithApplicationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create analytics application: %w", err)
	}

	_, err = factories.CreateApplication(ctx, exec,
		factories.WithApplicationsName("Empty Canvas"),
		factories.WithApplicationsSlug("empty-canvas"),
		factories.WithApplicationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create empty canvas application: %w", err)
	}

	storefrontProduction, err := factories.CreateEnvironment(ctx, exec, storefront.ID,
		factories.WithEnvironmentsName("Production"),
		factories.WithEnvironmentsSlug("production"),
		factories.WithEnvironmentsKind("production"),
		factories.WithEnvironmentsWebhookTokenPrefix(sql.NullString{String: "wh_prod", Valid: true}),
		factories.WithEnvironmentsWebhookTokenDigest([]byte("storefront-production-webhook-digest")),
		factories.WithEnvironmentsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront production environment: %w", err)
	}

	storefrontProductionSource, err := factories.CreateEnvironmentSource(ctx, exec, storefrontProduction.ID, &githubCredential.ID,
		factories.WithEnvironmentSourcesKind("git"),
		factories.WithEnvironmentSourcesProvider("github"),
		factories.WithEnvironmentSourcesRepository("deploycrate-demo/storefront"),
		factories.WithEnvironmentSourcesReference("main"),
		factories.WithEnvironmentSourcesSettings(json.RawMessage(`{"auto_deploy":true,"pull_request_previews":false}`)),
		factories.WithEnvironmentSourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront production source: %w", err)
	}

	_, err = factories.CreateEnvironmentBuildConfiguration(ctx, exec, storefrontProduction.ID,
		factories.WithEnvironmentBuildConfigurationsMethod("dockerfile"),
		factories.WithEnvironmentBuildConfigurationsContextPath("."),
		factories.WithEnvironmentBuildConfigurationsDockerfilePath(sql.NullString{String: "Dockerfile", Valid: true}),
		factories.WithEnvironmentBuildConfigurationsBuilderReference(sql.NullString{}),
		factories.WithEnvironmentBuildConfigurationsSettings(json.RawMessage(`{"build_args":{"NODE_ENV":"production"}}`)),
	)
	if err != nil {
		return fmt.Errorf("create storefront production build configuration: %w", err)
	}

	_, err = factories.CreateEnvironmentRuntimeConfiguration(ctx, exec, storefrontProduction.ID,
		factories.WithEnvironmentRuntimeConfigurationsRuntime("container"),
		factories.WithEnvironmentRuntimeConfigurationsCommand(sql.NullString{String: "node", Valid: true}),
		factories.WithEnvironmentRuntimeConfigurationsArguments(json.RawMessage(`["build/index.js"]`)),
		factories.WithEnvironmentRuntimeConfigurationsReplicas(3),
		factories.WithEnvironmentRuntimeConfigurationsPorts(json.RawMessage(`[{"name":"http","port":3000,"protocol":"tcp"}]`)),
		factories.WithEnvironmentRuntimeConfigurationsResourceLimits(json.RawMessage(`{"cpu":"1000m","memory":"1024Mi"}`)),
		factories.WithEnvironmentRuntimeConfigurationsRestartPolicy("always"),
		factories.WithEnvironmentRuntimeConfigurationsSettings(json.RawMessage(`{"grace_period_seconds":30}`)),
	)
	if err != nil {
		return fmt.Errorf("create storefront production runtime configuration: %w", err)
	}

	_, err = factories.CreateEnvironmentSecret(ctx, exec, storefrontProduction.ID, storefrontProductionSource.ID,
		factories.WithEnvironmentSecretsKey("SESSION_SECRET"),
		factories.WithEnvironmentSecretsEncValue([]byte("encrypted-storefront-production-session-secret")),
		factories.WithEnvironmentSecretsDigest([]byte("storefront-production-session-secret-digest")),
		factories.WithEnvironmentSecretsSourceType("environment_source"),
		factories.WithEnvironmentSecretsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront production secret: %w", err)
	}

	storefrontProductionPrimaryTarget, err := factories.CreateEnvironmentTarget(ctx, exec, storefrontProduction.ID, edgePrimary.ID,
		factories.WithEnvironmentTargetsAttachedAt(now),
		factories.WithEnvironmentTargetsDetachedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront production primary target: %w", err)
	}

	storefrontProductionSecondaryTarget, err := factories.CreateEnvironmentTarget(ctx, exec, storefrontProduction.ID, edgeSecondary.ID,
		factories.WithEnvironmentTargetsAttachedAt(now),
		factories.WithEnvironmentTargetsDetachedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront production secondary target: %w", err)
	}

	_, err = factories.CreateEnvironmentNetwork(ctx, exec, storefrontProduction.ID, productionNetwork.ID,
		factories.WithEnvironmentNetworksRole("runtime"),
		factories.WithEnvironmentNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach storefront production to production network: %w", err)
	}

	_, err = factories.CreateEnvironmentNetwork(ctx, exec, storefrontProduction.ID, sharedNetwork.ID,
		factories.WithEnvironmentNetworksRole("services"),
		factories.WithEnvironmentNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach storefront production to shared network: %w", err)
	}

	_, err = factories.CreateEnvironmentTargetNetwork(ctx, exec, storefrontProductionPrimaryTarget.ID, productionNetwork.ID, sql.NullString{String: "storefront-production-primary", Valid: true},
		factories.WithEnvironmentTargetNetworksAddress("10.21.0.110"),
		factories.WithEnvironmentTargetNetworksDriver("wireguard"),
		factories.WithEnvironmentTargetNetworksConfiguration(json.RawMessage(`{"dns_alias":"storefront-primary"}`)),
		factories.WithEnvironmentTargetNetworksState("applied"),
		factories.WithEnvironmentTargetNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksError(sql.NullString{}),
		factories.WithEnvironmentTargetNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach storefront production primary target to production network: %w", err)
	}

	_, err = factories.CreateEnvironmentTargetNetwork(ctx, exec, storefrontProductionPrimaryTarget.ID, sharedNetwork.ID, sql.NullString{String: "storefront-production-primary-shared", Valid: true},
		factories.WithEnvironmentTargetNetworksAddress("10.20.0.110"),
		factories.WithEnvironmentTargetNetworksDriver("wireguard"),
		factories.WithEnvironmentTargetNetworksConfiguration(json.RawMessage(`{"dns_alias":"storefront-primary-shared"}`)),
		factories.WithEnvironmentTargetNetworksState("applied"),
		factories.WithEnvironmentTargetNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksError(sql.NullString{}),
		factories.WithEnvironmentTargetNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach storefront production primary target to shared network: %w", err)
	}

	_, err = factories.CreateEnvironmentTargetNetwork(ctx, exec, storefrontProductionSecondaryTarget.ID, productionNetwork.ID, sql.NullString{String: "storefront-production-secondary", Valid: true},
		factories.WithEnvironmentTargetNetworksAddress("10.21.0.111"),
		factories.WithEnvironmentTargetNetworksDriver("wireguard"),
		factories.WithEnvironmentTargetNetworksConfiguration(json.RawMessage(`{"dns_alias":"storefront-secondary"}`)),
		factories.WithEnvironmentTargetNetworksState("applied"),
		factories.WithEnvironmentTargetNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksError(sql.NullString{}),
		factories.WithEnvironmentTargetNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach storefront production secondary target to production network: %w", err)
	}

	_, err = factories.CreateEnvironmentTargetNetwork(ctx, exec, storefrontProductionSecondaryTarget.ID, sharedNetwork.ID, sql.NullString{String: "storefront-production-secondary-shared", Valid: true},
		factories.WithEnvironmentTargetNetworksAddress("10.20.0.111"),
		factories.WithEnvironmentTargetNetworksDriver("wireguard"),
		factories.WithEnvironmentTargetNetworksConfiguration(json.RawMessage(`{"dns_alias":"storefront-secondary-shared"}`)),
		factories.WithEnvironmentTargetNetworksState("applied"),
		factories.WithEnvironmentTargetNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksError(sql.NullString{}),
		factories.WithEnvironmentTargetNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach storefront production secondary target to shared network: %w", err)
	}

	storefrontStaging, err := factories.CreateEnvironment(ctx, exec, storefront.ID,
		factories.WithEnvironmentsName("Staging"),
		factories.WithEnvironmentsSlug("staging"),
		factories.WithEnvironmentsKind("staging"),
		factories.WithEnvironmentsWebhookTokenPrefix(sql.NullString{String: "wh_stg", Valid: true}),
		factories.WithEnvironmentsWebhookTokenDigest([]byte("storefront-staging-webhook-digest")),
		factories.WithEnvironmentsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront staging environment: %w", err)
	}

	_, err = factories.CreateEnvironmentSource(ctx, exec, storefrontStaging.ID, &githubCredential.ID,
		factories.WithEnvironmentSourcesKind("git"),
		factories.WithEnvironmentSourcesProvider("github"),
		factories.WithEnvironmentSourcesRepository("deploycrate-demo/storefront"),
		factories.WithEnvironmentSourcesReference("develop"),
		factories.WithEnvironmentSourcesSettings(json.RawMessage(`{"auto_deploy":true,"pull_request_previews":true}`)),
		factories.WithEnvironmentSourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront staging source: %w", err)
	}

	_, err = factories.CreateEnvironmentBuildConfiguration(ctx, exec, storefrontStaging.ID,
		factories.WithEnvironmentBuildConfigurationsMethod("buildpack"),
		factories.WithEnvironmentBuildConfigurationsContextPath("."),
		factories.WithEnvironmentBuildConfigurationsDockerfilePath(sql.NullString{}),
		factories.WithEnvironmentBuildConfigurationsBuilderReference(sql.NullString{String: "paketobuildpacks/builder-jammy-base", Valid: true}),
		factories.WithEnvironmentBuildConfigurationsSettings(json.RawMessage(`{"environment":{"NODE_ENV":"staging"}}`)),
	)
	if err != nil {
		return fmt.Errorf("create storefront staging build configuration: %w", err)
	}

	_, err = factories.CreateEnvironmentRuntimeConfiguration(ctx, exec, storefrontStaging.ID,
		factories.WithEnvironmentRuntimeConfigurationsRuntime("container"),
		factories.WithEnvironmentRuntimeConfigurationsCommand(sql.NullString{String: "node", Valid: true}),
		factories.WithEnvironmentRuntimeConfigurationsArguments(json.RawMessage(`["build/index.js"]`)),
		factories.WithEnvironmentRuntimeConfigurationsReplicas(1),
		factories.WithEnvironmentRuntimeConfigurationsPorts(json.RawMessage(`[{"name":"http","port":3000,"protocol":"tcp"}]`)),
		factories.WithEnvironmentRuntimeConfigurationsResourceLimits(json.RawMessage(`{"cpu":"500m","memory":"512Mi"}`)),
		factories.WithEnvironmentRuntimeConfigurationsRestartPolicy("on-failure"),
		factories.WithEnvironmentRuntimeConfigurationsSettings(json.RawMessage(`{"grace_period_seconds":10}`)),
	)
	if err != nil {
		return fmt.Errorf("create storefront staging runtime configuration: %w", err)
	}

	storefrontStagingTarget, err := factories.CreateEnvironmentTarget(ctx, exec, storefrontStaging.ID, worker.ID,
		factories.WithEnvironmentTargetsAttachedAt(now),
		factories.WithEnvironmentTargetsDetachedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront staging target: %w", err)
	}

	_, err = factories.CreateEnvironmentNetwork(ctx, exec, storefrontStaging.ID, stagingNetwork.ID,
		factories.WithEnvironmentNetworksRole("runtime"),
		factories.WithEnvironmentNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach storefront staging to staging network: %w", err)
	}

	_, err = factories.CreateEnvironmentNetwork(ctx, exec, storefrontStaging.ID, sharedNetwork.ID,
		factories.WithEnvironmentNetworksRole("services"),
		factories.WithEnvironmentNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach storefront staging to shared network: %w", err)
	}

	_, err = factories.CreateEnvironmentTargetNetwork(ctx, exec, storefrontStagingTarget.ID, stagingNetwork.ID, sql.NullString{String: "storefront-staging", Valid: true},
		factories.WithEnvironmentTargetNetworksAddress("10.22.0.120"),
		factories.WithEnvironmentTargetNetworksDriver("wireguard"),
		factories.WithEnvironmentTargetNetworksConfiguration(json.RawMessage(`{"dns_alias":"storefront-staging"}`)),
		factories.WithEnvironmentTargetNetworksState("applied"),
		factories.WithEnvironmentTargetNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksError(sql.NullString{}),
		factories.WithEnvironmentTargetNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach storefront staging target to staging network: %w", err)
	}

	analyticsProduction, err := factories.CreateEnvironment(ctx, exec, analytics.ID,
		factories.WithEnvironmentsName("Production"),
		factories.WithEnvironmentsSlug("production"),
		factories.WithEnvironmentsKind("production"),
		factories.WithEnvironmentsWebhookTokenPrefix(sql.NullString{}),
		factories.WithEnvironmentsWebhookTokenDigest(nil),
		factories.WithEnvironmentsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create analytics production environment: %w", err)
	}

	_, err = factories.CreateEnvironmentSource(ctx, exec, analyticsProduction.ID, &gitlabCredential.ID,
		factories.WithEnvironmentSourcesKind("git"),
		factories.WithEnvironmentSourcesProvider("gitlab"),
		factories.WithEnvironmentSourcesRepository("deploycrate-labs/analytics"),
		factories.WithEnvironmentSourcesReference("main"),
		factories.WithEnvironmentSourcesSettings(json.RawMessage(`{"auto_deploy":false,"submodules":true}`)),
		factories.WithEnvironmentSourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create analytics production source: %w", err)
	}

	_, err = factories.CreateEnvironmentBuildConfiguration(ctx, exec, analyticsProduction.ID,
		factories.WithEnvironmentBuildConfigurationsMethod("dockerfile"),
		factories.WithEnvironmentBuildConfigurationsContextPath("services/analytics"),
		factories.WithEnvironmentBuildConfigurationsDockerfilePath(sql.NullString{String: "services/analytics/Dockerfile", Valid: true}),
		factories.WithEnvironmentBuildConfigurationsBuilderReference(sql.NullString{}),
		factories.WithEnvironmentBuildConfigurationsSettings(json.RawMessage(`{"target":"production"}`)),
	)
	if err != nil {
		return fmt.Errorf("create analytics production build configuration: %w", err)
	}

	_, err = factories.CreateEnvironmentRuntimeConfiguration(ctx, exec, analyticsProduction.ID,
		factories.WithEnvironmentRuntimeConfigurationsRuntime("container"),
		factories.WithEnvironmentRuntimeConfigurationsCommand(sql.NullString{String: "./analytics", Valid: true}),
		factories.WithEnvironmentRuntimeConfigurationsArguments(json.RawMessage(`[]`)),
		factories.WithEnvironmentRuntimeConfigurationsReplicas(2),
		factories.WithEnvironmentRuntimeConfigurationsPorts(json.RawMessage(`[{"name":"http","port":8080,"protocol":"tcp"}]`)),
		factories.WithEnvironmentRuntimeConfigurationsResourceLimits(json.RawMessage(`{"cpu":"2000m","memory":"2048Mi"}`)),
		factories.WithEnvironmentRuntimeConfigurationsRestartPolicy("always"),
		factories.WithEnvironmentRuntimeConfigurationsSettings(json.RawMessage(`{"grace_period_seconds":45}`)),
	)
	if err != nil {
		return fmt.Errorf("create analytics production runtime configuration: %w", err)
	}

	analyticsProductionTarget, err := factories.CreateEnvironmentTarget(ctx, exec, analyticsProduction.ID, edgeSecondary.ID,
		factories.WithEnvironmentTargetsAttachedAt(now),
		factories.WithEnvironmentTargetsDetachedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create analytics production target: %w", err)
	}

	_, err = factories.CreateEnvironmentNetwork(ctx, exec, analyticsProduction.ID, sharedNetwork.ID,
		factories.WithEnvironmentNetworksRole("services"),
		factories.WithEnvironmentNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach analytics production to shared network: %w", err)
	}

	_, err = factories.CreateEnvironmentTargetNetwork(ctx, exec, analyticsProductionTarget.ID, sharedNetwork.ID, sql.NullString{String: "analytics-production", Valid: true},
		factories.WithEnvironmentTargetNetworksAddress("10.20.0.130"),
		factories.WithEnvironmentTargetNetworksDriver("wireguard"),
		factories.WithEnvironmentTargetNetworksConfiguration(json.RawMessage(`{"dns_alias":"analytics-production"}`)),
		factories.WithEnvironmentTargetNetworksState("applied"),
		factories.WithEnvironmentTargetNetworksAppliedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksObservedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithEnvironmentTargetNetworksError(sql.NullString{}),
		factories.WithEnvironmentTargetNetworksRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("attach analytics production target to shared network: %w", err)
	}

	productionPostgres, err := factories.CreateResource(ctx, exec, storefrontProduction.ID,
		factories.WithResourcesName("Primary PostgreSQL"),
		factories.WithResourcesCategory("database"),
		factories.WithResourcesKind("postgresql"),
		factories.WithResourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production PostgreSQL resource: %w", err)
	}

	productionPostgresInstallation, err := factories.CreateResourceInstallation(ctx, exec, productionPostgres.ID, &edgePrimary.ID,
		factories.WithResourceInstallationsMode("managed"),
		factories.WithResourceInstallationsDriver("docker"),
		factories.WithResourceInstallationsDesiredVersion(sql.NullString{String: "17.5", Valid: true}),
		factories.WithResourceInstallationsConfiguration(json.RawMessage(`{"volume":"postgres-production","backup_enabled":true}`)),
		factories.WithResourceInstallationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production PostgreSQL installation: %w", err)
	}

	_, err = factories.CreateResourceInstallationStatus(ctx, exec, productionPostgresInstallation.ID, sql.NullString{String: "postgres-production", Valid: true},
		factories.WithResourceInstallationStatusesState("ready"),
		factories.WithResourceInstallationStatusesInstalledVersion(sql.NullString{String: "17.5", Valid: true}),
		factories.WithResourceInstallationStatusesServiceState(sql.NullString{String: "running", Valid: true}),
		factories.WithResourceInstallationStatusesHealth(sql.NullString{String: "healthy", Valid: true}),
		factories.WithResourceInstallationStatusesDetails(json.RawMessage(`{"connections":12,"storage_used_percent":37}`)),
		factories.WithResourceInstallationStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create production PostgreSQL installation status: %w", err)
	}

	productionPostgresEndpoint, err := factories.CreateResourceEndpoint(ctx, exec, productionPostgres.ID, &productionPostgresInstallation.ID, &productionNetwork.ID,
		factories.WithResourceEndpointsName("Primary"),
		factories.WithResourceEndpointsRole("read-write"),
		factories.WithResourceEndpointsAddress("postgres.production.internal"),
		factories.WithResourceEndpointsPort(5432),
		factories.WithResourceEndpointsProtocol("postgresql"),
		factories.WithResourceEndpointsTlsMode("required"),
		factories.WithResourceEndpointsSettings(json.RawMessage(`{"database":"storefront"}`)),
		factories.WithResourceEndpointsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production PostgreSQL endpoint: %w", err)
	}

	productionPostgresCredential, err := factories.CreateResourceCredential(ctx, exec, productionPostgres.ID, &productionPostgresInstallation.ID,
		factories.WithResourceCredentialsName("Storefront application"),
		factories.WithResourceCredentialsRole("read-write"),
		factories.WithResourceCredentialsUsername(sql.NullString{String: "storefront_app", Valid: true}),
		factories.WithResourceCredentialsMetadata(json.RawMessage(`{"database":"storefront"}`)),
		factories.WithResourceCredentialsEncPayload([]byte("encrypted-production-postgres-credential")),
		factories.WithResourceCredentialsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production PostgreSQL credential: %w", err)
	}

	productionRedis, err := factories.CreateResource(ctx, exec, storefrontProduction.ID,
		factories.WithResourcesName("Session Cache"),
		factories.WithResourcesCategory("cache"),
		factories.WithResourcesKind("redis"),
		factories.WithResourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production Redis resource: %w", err)
	}

	productionRedisInstallation, err := factories.CreateResourceInstallation(ctx, exec, productionRedis.ID, &edgeSecondary.ID,
		factories.WithResourceInstallationsMode("managed"),
		factories.WithResourceInstallationsDriver("docker"),
		factories.WithResourceInstallationsDesiredVersion(sql.NullString{String: "8.0", Valid: true}),
		factories.WithResourceInstallationsConfiguration(json.RawMessage(`{"max_memory":"512mb","persistence":"aof"}`)),
		factories.WithResourceInstallationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production Redis installation: %w", err)
	}

	_, err = factories.CreateResourceInstallationStatus(ctx, exec, productionRedisInstallation.ID, sql.NullString{String: "redis-production", Valid: true},
		factories.WithResourceInstallationStatusesState("ready"),
		factories.WithResourceInstallationStatusesInstalledVersion(sql.NullString{String: "8.0", Valid: true}),
		factories.WithResourceInstallationStatusesServiceState(sql.NullString{String: "running", Valid: true}),
		factories.WithResourceInstallationStatusesHealth(sql.NullString{String: "healthy", Valid: true}),
		factories.WithResourceInstallationStatusesDetails(json.RawMessage(`{"memory_used_percent":24,"connected_clients":8}`)),
		factories.WithResourceInstallationStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create production Redis installation status: %w", err)
	}

	productionRedisEndpoint, err := factories.CreateResourceEndpoint(ctx, exec, productionRedis.ID, &productionRedisInstallation.ID, &productionNetwork.ID,
		factories.WithResourceEndpointsName("Primary"),
		factories.WithResourceEndpointsRole("read-write"),
		factories.WithResourceEndpointsAddress("redis.production.internal"),
		factories.WithResourceEndpointsPort(6379),
		factories.WithResourceEndpointsProtocol("redis"),
		factories.WithResourceEndpointsTlsMode("required"),
		factories.WithResourceEndpointsSettings(json.RawMessage(`{"database":0}`)),
		factories.WithResourceEndpointsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production Redis endpoint: %w", err)
	}

	productionRedisCredential, err := factories.CreateResourceCredential(ctx, exec, productionRedis.ID, &productionRedisInstallation.ID,
		factories.WithResourceCredentialsName("Storefront application"),
		factories.WithResourceCredentialsRole("read-write"),
		factories.WithResourceCredentialsUsername(sql.NullString{String: "default", Valid: true}),
		factories.WithResourceCredentialsMetadata(json.RawMessage(`{"database":0}`)),
		factories.WithResourceCredentialsEncPayload([]byte("encrypted-production-redis-credential")),
		factories.WithResourceCredentialsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production Redis credential: %w", err)
	}

	stagingPostgres, err := factories.CreateResource(ctx, exec, storefrontStaging.ID,
		factories.WithResourcesName("Staging PostgreSQL"),
		factories.WithResourcesCategory("database"),
		factories.WithResourcesKind("postgresql"),
		factories.WithResourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create staging PostgreSQL resource: %w", err)
	}

	stagingPostgresInstallation, err := factories.CreateResourceInstallation(ctx, exec, stagingPostgres.ID, &worker.ID,
		factories.WithResourceInstallationsMode("managed"),
		factories.WithResourceInstallationsDriver("docker"),
		factories.WithResourceInstallationsDesiredVersion(sql.NullString{String: "17.5", Valid: true}),
		factories.WithResourceInstallationsConfiguration(json.RawMessage(`{"volume":"postgres-staging","backup_enabled":false}`)),
		factories.WithResourceInstallationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create staging PostgreSQL installation: %w", err)
	}

	_, err = factories.CreateResourceInstallationStatus(ctx, exec, stagingPostgresInstallation.ID, sql.NullString{String: "postgres-staging", Valid: true},
		factories.WithResourceInstallationStatusesState("ready"),
		factories.WithResourceInstallationStatusesInstalledVersion(sql.NullString{String: "17.5", Valid: true}),
		factories.WithResourceInstallationStatusesServiceState(sql.NullString{String: "running", Valid: true}),
		factories.WithResourceInstallationStatusesHealth(sql.NullString{String: "healthy", Valid: true}),
		factories.WithResourceInstallationStatusesDetails(json.RawMessage(`{"connections":3,"storage_used_percent":12}`)),
		factories.WithResourceInstallationStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create staging PostgreSQL installation status: %w", err)
	}

	stagingPostgresEndpoint, err := factories.CreateResourceEndpoint(ctx, exec, stagingPostgres.ID, &stagingPostgresInstallation.ID, &stagingNetwork.ID,
		factories.WithResourceEndpointsName("Primary"),
		factories.WithResourceEndpointsRole("read-write"),
		factories.WithResourceEndpointsAddress("postgres.staging.internal"),
		factories.WithResourceEndpointsPort(5432),
		factories.WithResourceEndpointsProtocol("postgresql"),
		factories.WithResourceEndpointsTlsMode("preferred"),
		factories.WithResourceEndpointsSettings(json.RawMessage(`{"database":"storefront_staging"}`)),
		factories.WithResourceEndpointsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create staging PostgreSQL endpoint: %w", err)
	}

	stagingPostgresCredential, err := factories.CreateResourceCredential(ctx, exec, stagingPostgres.ID, &stagingPostgresInstallation.ID,
		factories.WithResourceCredentialsName("Storefront staging"),
		factories.WithResourceCredentialsRole("read-write"),
		factories.WithResourceCredentialsUsername(sql.NullString{String: "storefront_staging", Valid: true}),
		factories.WithResourceCredentialsMetadata(json.RawMessage(`{"database":"storefront_staging"}`)),
		factories.WithResourceCredentialsEncPayload([]byte("encrypted-staging-postgres-credential")),
		factories.WithResourceCredentialsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create staging PostgreSQL credential: %w", err)
	}

	eventBus, err := factories.CreateResource(ctx, exec, analyticsProduction.ID,
		factories.WithResourcesName("Shared Event Bus"),
		factories.WithResourcesCategory("messaging"),
		factories.WithResourcesKind("nats"),
		factories.WithResourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create shared event bus resource: %w", err)
	}

	eventBusInstallation, err := factories.CreateResourceInstallation(ctx, exec, eventBus.ID, &edgeSecondary.ID,
		factories.WithResourceInstallationsMode("managed"),
		factories.WithResourceInstallationsDriver("docker"),
		factories.WithResourceInstallationsDesiredVersion(sql.NullString{String: "2.11.6", Valid: true}),
		factories.WithResourceInstallationsConfiguration(json.RawMessage(`{"jetstream":true,"storage":"file"}`)),
		factories.WithResourceInstallationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create shared event bus installation: %w", err)
	}

	_, err = factories.CreateResourceInstallationStatus(ctx, exec, eventBusInstallation.ID, sql.NullString{String: "nats-shared", Valid: true},
		factories.WithResourceInstallationStatusesState("degraded"),
		factories.WithResourceInstallationStatusesInstalledVersion(sql.NullString{String: "2.11.6", Valid: true}),
		factories.WithResourceInstallationStatusesServiceState(sql.NullString{String: "running", Valid: true}),
		factories.WithResourceInstallationStatusesHealth(sql.NullString{String: "degraded", Valid: true}),
		factories.WithResourceInstallationStatusesDetails(json.RawMessage(`{"warning":"replica count below desired","connections":15}`)),
		factories.WithResourceInstallationStatusesObservedAt(now),
	)
	if err != nil {
		return fmt.Errorf("create shared event bus installation status: %w", err)
	}

	eventBusEndpoint, err := factories.CreateResourceEndpoint(ctx, exec, eventBus.ID, &eventBusInstallation.ID, &sharedNetwork.ID,
		factories.WithResourceEndpointsName("Client"),
		factories.WithResourceEndpointsRole("publish-subscribe"),
		factories.WithResourceEndpointsAddress("nats.shared.internal"),
		factories.WithResourceEndpointsPort(4222),
		factories.WithResourceEndpointsProtocol("nats"),
		factories.WithResourceEndpointsTlsMode("required"),
		factories.WithResourceEndpointsSettings(json.RawMessage(`{"jetstream":true}`)),
		factories.WithResourceEndpointsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create shared event bus endpoint: %w", err)
	}

	eventBusCredential, err := factories.CreateResourceCredential(ctx, exec, eventBus.ID, &eventBusInstallation.ID,
		factories.WithResourceCredentialsName("Shared clients"),
		factories.WithResourceCredentialsRole("publish-subscribe"),
		factories.WithResourceCredentialsUsername(sql.NullString{String: "shared_clients", Valid: true}),
		factories.WithResourceCredentialsMetadata(json.RawMessage(`{"account":"applications"}`)),
		factories.WithResourceCredentialsEncPayload([]byte("encrypted-event-bus-credential")),
		factories.WithResourceCredentialsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create shared event bus credential: %w", err)
	}

	productionPostgresDependency, err := factories.CreateEnvironmentDependency(ctx, exec, storefrontProduction.ID, productionPostgres.ID, productionPostgresEndpoint.ID, &productionNetwork.ID,
		factories.WithEnvironmentDependenciesAlias("DATABASE"),
		factories.WithEnvironmentDependenciesRequired(true),
		factories.WithEnvironmentDependenciesSecretMapping(json.RawMessage(`{"username":"DATABASE_USER","password":"DATABASE_PASSWORD"}`)),
		factories.WithEnvironmentDependenciesSettings(json.RawMessage(`{"database":"storefront","sslmode":"require"}`)),
		factories.WithEnvironmentDependenciesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production PostgreSQL dependency: %w", err)
	}

	productionPostgresBinding, err := factories.CreateResourceBinding(ctx, exec, productionPostgres.ID, productionPostgresEndpoint.ID, productionPostgresDependency.ID,
		factories.WithResourceBindingsProvisioningMode("existing"),
		factories.WithResourceBindingsSecretManagementMode("managed"),
		factories.WithResourceBindingsKind("database"),
		factories.WithResourceBindingsExternalDatabase(sql.NullString{String: "storefront", Valid: true}),
		factories.WithResourceBindingsExternalPrincipal(sql.NullString{String: "storefront_app", Valid: true}),
		factories.WithResourceBindingsConfiguration(json.RawMessage(`{"schema":"public"}`)),
		factories.WithResourceBindingsStatus("active"),
		factories.WithResourceBindingsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production PostgreSQL binding: %w", err)
	}

	_, err = factories.CreateResourceBindingCredential(ctx, exec, productionPostgresBinding.ID, productionPostgresCredential.ID,
		factories.WithResourceBindingCredentialsGeneration(1),
		factories.WithResourceBindingCredentialsState("active"),
		factories.WithResourceBindingCredentialsActivatedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithResourceBindingCredentialsRetiredAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production PostgreSQL binding credential: %w", err)
	}

	productionRedisDependency, err := factories.CreateEnvironmentDependency(ctx, exec, storefrontProduction.ID, productionRedis.ID, productionRedisEndpoint.ID, &productionNetwork.ID,
		factories.WithEnvironmentDependenciesAlias("CACHE"),
		factories.WithEnvironmentDependenciesRequired(true),
		factories.WithEnvironmentDependenciesSecretMapping(json.RawMessage(`{"password":"REDIS_PASSWORD"}`)),
		factories.WithEnvironmentDependenciesSettings(json.RawMessage(`{"database":0}`)),
		factories.WithEnvironmentDependenciesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production Redis dependency: %w", err)
	}

	productionRedisBinding, err := factories.CreateResourceBinding(ctx, exec, productionRedis.ID, productionRedisEndpoint.ID, productionRedisDependency.ID,
		factories.WithResourceBindingsProvisioningMode("existing"),
		factories.WithResourceBindingsSecretManagementMode("managed"),
		factories.WithResourceBindingsKind("cache"),
		factories.WithResourceBindingsExternalDatabase(sql.NullString{String: "0", Valid: true}),
		factories.WithResourceBindingsExternalPrincipal(sql.NullString{String: "default", Valid: true}),
		factories.WithResourceBindingsConfiguration(json.RawMessage(`{"key_prefix":"storefront:"}`)),
		factories.WithResourceBindingsStatus("active"),
		factories.WithResourceBindingsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production Redis binding: %w", err)
	}

	_, err = factories.CreateResourceBindingCredential(ctx, exec, productionRedisBinding.ID, productionRedisCredential.ID,
		factories.WithResourceBindingCredentialsGeneration(1),
		factories.WithResourceBindingCredentialsState("active"),
		factories.WithResourceBindingCredentialsActivatedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithResourceBindingCredentialsRetiredAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create production Redis binding credential: %w", err)
	}

	stagingPostgresDependency, err := factories.CreateEnvironmentDependency(ctx, exec, storefrontStaging.ID, stagingPostgres.ID, stagingPostgresEndpoint.ID, &stagingNetwork.ID,
		factories.WithEnvironmentDependenciesAlias("DATABASE"),
		factories.WithEnvironmentDependenciesRequired(true),
		factories.WithEnvironmentDependenciesSecretMapping(json.RawMessage(`{"username":"DATABASE_USER","password":"DATABASE_PASSWORD"}`)),
		factories.WithEnvironmentDependenciesSettings(json.RawMessage(`{"database":"storefront_staging","sslmode":"prefer"}`)),
		factories.WithEnvironmentDependenciesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create staging PostgreSQL dependency: %w", err)
	}

	stagingPostgresBinding, err := factories.CreateResourceBinding(ctx, exec, stagingPostgres.ID, stagingPostgresEndpoint.ID, stagingPostgresDependency.ID,
		factories.WithResourceBindingsProvisioningMode("existing"),
		factories.WithResourceBindingsSecretManagementMode("managed"),
		factories.WithResourceBindingsKind("database"),
		factories.WithResourceBindingsExternalDatabase(sql.NullString{String: "storefront_staging", Valid: true}),
		factories.WithResourceBindingsExternalPrincipal(sql.NullString{String: "storefront_staging", Valid: true}),
		factories.WithResourceBindingsConfiguration(json.RawMessage(`{"schema":"public"}`)),
		factories.WithResourceBindingsStatus("active"),
		factories.WithResourceBindingsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create staging PostgreSQL binding: %w", err)
	}

	_, err = factories.CreateResourceBindingCredential(ctx, exec, stagingPostgresBinding.ID, stagingPostgresCredential.ID,
		factories.WithResourceBindingCredentialsGeneration(1),
		factories.WithResourceBindingCredentialsState("active"),
		factories.WithResourceBindingCredentialsActivatedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithResourceBindingCredentialsRetiredAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create staging PostgreSQL binding credential: %w", err)
	}

	storefrontEventBusDependency, err := factories.CreateEnvironmentDependency(ctx, exec, storefrontProduction.ID, eventBus.ID, eventBusEndpoint.ID, &sharedNetwork.ID,
		factories.WithEnvironmentDependenciesAlias("EVENT_BUS"),
		factories.WithEnvironmentDependenciesRequired(false),
		factories.WithEnvironmentDependenciesSecretMapping(json.RawMessage(`{"username":"NATS_USER","password":"NATS_PASSWORD"}`)),
		factories.WithEnvironmentDependenciesSettings(json.RawMessage(`{"stream":"storefront-events"}`)),
		factories.WithEnvironmentDependenciesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront event bus dependency: %w", err)
	}

	storefrontEventBusBinding, err := factories.CreateResourceBinding(ctx, exec, eventBus.ID, eventBusEndpoint.ID, storefrontEventBusDependency.ID,
		factories.WithResourceBindingsProvisioningMode("existing"),
		factories.WithResourceBindingsSecretManagementMode("shared"),
		factories.WithResourceBindingsKind("messaging"),
		factories.WithResourceBindingsExternalDatabase(sql.NullString{}),
		factories.WithResourceBindingsExternalPrincipal(sql.NullString{String: "shared_clients", Valid: true}),
		factories.WithResourceBindingsConfiguration(json.RawMessage(`{"subject_prefix":"storefront"}`)),
		factories.WithResourceBindingsStatus("active"),
		factories.WithResourceBindingsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront event bus binding: %w", err)
	}

	_, err = factories.CreateResourceBindingCredential(ctx, exec, storefrontEventBusBinding.ID, eventBusCredential.ID,
		factories.WithResourceBindingCredentialsGeneration(1),
		factories.WithResourceBindingCredentialsState("active"),
		factories.WithResourceBindingCredentialsActivatedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithResourceBindingCredentialsRetiredAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create storefront event bus binding credential: %w", err)
	}

	analyticsEventBusDependency, err := factories.CreateEnvironmentDependency(ctx, exec, analyticsProduction.ID, eventBus.ID, eventBusEndpoint.ID, &sharedNetwork.ID,
		factories.WithEnvironmentDependenciesAlias("EVENT_BUS"),
		factories.WithEnvironmentDependenciesRequired(true),
		factories.WithEnvironmentDependenciesSecretMapping(json.RawMessage(`{"username":"NATS_USER","password":"NATS_PASSWORD"}`)),
		factories.WithEnvironmentDependenciesSettings(json.RawMessage(`{"consumer":"analytics","stream":"storefront-events"}`)),
		factories.WithEnvironmentDependenciesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create analytics event bus dependency: %w", err)
	}

	analyticsEventBusBinding, err := factories.CreateResourceBinding(ctx, exec, eventBus.ID, eventBusEndpoint.ID, analyticsEventBusDependency.ID,
		factories.WithResourceBindingsProvisioningMode("existing"),
		factories.WithResourceBindingsSecretManagementMode("shared"),
		factories.WithResourceBindingsKind("messaging"),
		factories.WithResourceBindingsExternalDatabase(sql.NullString{}),
		factories.WithResourceBindingsExternalPrincipal(sql.NullString{String: "shared_clients", Valid: true}),
		factories.WithResourceBindingsConfiguration(json.RawMessage(`{"subject_prefix":"analytics"}`)),
		factories.WithResourceBindingsStatus("active"),
		factories.WithResourceBindingsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create analytics event bus binding: %w", err)
	}

	_, err = factories.CreateResourceBindingCredential(ctx, exec, analyticsEventBusBinding.ID, eventBusCredential.ID,
		factories.WithResourceBindingCredentialsGeneration(1),
		factories.WithResourceBindingCredentialsState("active"),
		factories.WithResourceBindingCredentialsActivatedAt(sql.NullTime{Time: now, Valid: true}),
		factories.WithResourceBindingCredentialsRetiredAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create analytics event bus binding credential: %w", err)
	}

	if err := ensureSystemApplication(ctx, exec, now); err != nil {
		return err
	}

	fmt.Println("Created UI seed data: 4 servers, 4 networks, 4 applications, 4 environments, 5 resources, 5 dependencies, including the DeployCrate CE system topology")

	return nil
}
