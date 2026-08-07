<script lang="ts">
  import { router } from "@inertiajs/svelte";
  import { untrack } from "svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Collapsible from "@/Components/ui/collapsible";
  import * as Dialog from "@/Components/ui/dialog";
  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import DataField from "@/Components/DataField.svelte";
  import EnvironmentDeleteDialog from "@/Components/EnvironmentDeleteDialog.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import TelemetryHistory from "@/Components/Applications/Environments/TelemetryHistory.svelte";
  import UsageSummary from "@/Components/Applications/Environments/UsageSummary.svelte";
  import UsageDonut from "@/Components/System/UsageDonut.svelte";
  import { Input } from "@/Components/ui/input";
  import { Spinner } from "@/Components/ui/spinner";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Secret = {
    id: string;
    key: string;
    digestPrefix: string;
    sourceType: string;
    sourceId: string;
    createdAt: string;
    status: "deployed" | "deploying" | "pending" | "failed" | "pending_removal";
    desired: boolean;
  };
  type Variable = {
    key: string;
    value: string;
    source: string;
    sourceId: string;
  };
  type Resource = { id: string; alias: string; name: string; engine: string };
  type Build = {
    id: string;
    sourceRevision: string;
    status: string;
    currentStep: string;
    error: string;
    createdAt: string;
    startedAt?: string;
    finishedAt?: string;
    registryEndpoint: string;
    jobId: number | null;
    jobState: string;
  };
  type BuildLog = {
    id: string;
    sequence: number;
    stream: "system" | "pack";
    message: string;
    occurredAt: string;
  };
  type BuildLogSnapshot = {
    build: Build;
    logs: BuildLog[];
    nextSequence: number;
    hasMore: boolean;
  };
  type Release = {
    id: string;
    sourceRevision: string;
    artifactReference: string;
    createdAt: string;
  };
  type Deployment = {
    id: string;
    status: string;
    currentStep: string;
    error: string;
    releaseId: string;
    createdAt: string;
    active: boolean;
  };
  type DeploymentEvent = {
    id: string;
    sequence: number;
    eventType: string;
    status: string;
    step: string;
    message: string;
    error: string;
    occurredAt: string;
  };
  type DeploymentEventSnapshot = {
    deployment: Deployment;
    events: DeploymentEvent[];
    nextSequence: number;
    hasMore: boolean;
  };
  type EnvironmentLog = {
    id: string;
    message: string;
    stream: string;
    container: string;
    deployment: string;
    instance: string;
    release: string;
    processName: string;
    processKind: string;
    processReplica: string;
    occurredAt: string;
  };
  type EnvironmentLogSnapshot = {
    logs: EnvironmentLog[];
    nextCursor: string;
    hasMore: boolean;
  };
  type Process = {
    name: string;
    kind: "web" | "worker" | "release";
    command?: string | null;
    arguments: string[];
    replicas: number;
    container_port?: number;
    health_path?: string;
    timeout_seconds?: number;
  };
  type Instance = {
    id: string;
    state: string;
    slot: string;
    processName: string;
    processKind: "web" | "worker";
    replicaKey: string;
    ports: { host?: string; http?: number };
    releaseId: string;
    deploymentId: string;
    targetId: string;
    targetName: string;
    observedAt: string;
  };
  type ReleaseCommand = {
    id: string;
    status: "queued" | "running" | "succeeded" | "failed" | "ambiguous";
    attempt: number;
    externalId: string;
    exitCode?: number;
    startedAt?: string;
    finishedAt?: string;
    error: string;
    releaseId: string;
    targetId: string;
    targetName: string;
    command: string;
    arguments: string[];
    timeoutSeconds: number;
    createdAt: string;
  };
  type ReleaseCommandLog = {
    id: string;
    attempt: number;
    sequence: number;
    stream: "system" | "stdout" | "stderr";
    message: string;
    occurredAt: string;
  };
  type DNSStatus = {
    mode: "manual" | "cloudflare";
    bindingId?: string;
    zoneId?: string;
    zoneName: string;
    connectionName: string;
    state: string;
    generation: number;
    appliedGeneration: number;
    lastError: string;
    reconciliationQueued: boolean;
    records: { type: string; name: string; content: string }[];
  };
  type TelemetryPoint = {
    observedAt: string;
    cpuCores: number;
    memoryBytes: number;
    cpuAvailable: boolean;
    memoryAvailable: boolean;
  };
  type TelemetryRow = {
    application: string;
    environment: string;
    release: string;
    deployment: string;
    target: string;
    instance: string;
    available: boolean;
    observedAt: string;
    cpuCores: number;
    memoryBytes: number;
    oomEvents: number;
    cpuThrottlingRatio: number;
    tasks: number;
    history: TelemetryPoint[];
    cpuAvailable: boolean;
    memoryAvailable: boolean;
    oomAvailable: boolean;
    cpuThrottlingAvailable: boolean;
    tasksAvailable: boolean;
  };
  type Overview = {
    applicationId: string;
    applicationName: string;
    sourceType: "buildpacks" | "image";
    environment: { id: string; name: string; kind: string };
    repository: string;
    reference: string;
    contextPath: string;
    registryName: string;
    registryEndpoint: string;
    runtimeServerIds: string[];
    runtimeTargetIds: string[];
    runtimeServers: string[];
    domain: string;
    deployability: { deployable: boolean; missing: string[] };
    secrets: Secret[];
    variables: Variable[];
    resources: Resource[];
    builds: Build[];
    releases: Release[];
    deployments: Deployment[];
    instances: Instance[];
    processes: Process[];
    releaseCommands: ReleaseCommand[];
    apiTokenPrefix: string;
    dns: DNSStatus;
    canPromoteToProduction: boolean;
    promotionTargetName: string;
    latestSuccessfulDeploymentId?: string;
    latestSuccessfulReleaseId?: string;
  };
  type ServingContainer = {
    instanceId: string;
    deploymentId: string;
    targetId: string;
    serverId: string;
    exists: boolean;
    running: boolean;
  };
  type HostUsage = {
    cpuCores: number;
    memoryBytes: number;
    available: boolean;
  };
  let {
    auth,
    environment,
    telemetry,
    container,
    host = { cpuCores: 0, memoryBytes: 0, available: false },
    telemetryRange = "24h",
    section = "overview",
  }: {
    auth: { email: string };
    environment: Overview;
    telemetry: TelemetryRow[];
    container: ServingContainer;
    host: HostUsage;
    telemetryRange: "1h" | "6h" | "24h" | "7d";
    section: "overview" | "telemetry" | "deployments" | "builds" | "secrets";
  } = $props();
  let key = $state("");
  let value = $state("");
  let imageReference = $state(untrack(() => environment.reference));
  let apiToken = $state("");
  let apiTokenError = $state("");
  let apiTokenDialogOpen = $state(false);
  let apiTokenConfirmOpen = $state(false);
  let apiTokenProcessing = $state(false);
  let deploymentCreationProcessing = $state(false);
  let containerActionProcessing = $state(false);
  let dnsActionProcessing = $state(false);
  let secretCreationProcessing = $state(false);
  let deploymentRetrying = $state("");
  let liveBuilds = $state<Build[] | null>(null);
  let liveDeployments = $state<Deployment[] | null>(null);
  let buildLogs = $state<Record<string, BuildLog[]>>({});
  let buildLogCursors = $state<Record<string, number>>({});
  let expandedBuildId = $state("");
  let buildLogConnectionError = $state("");
  let deploymentEvents = $state<Record<string, DeploymentEvent[]>>({});
  let deploymentEventCursors = $state<Record<string, number>>({});
  let expandedDeploymentId = $state("");
  let deploymentEventConnectionError = $state("");
  let environmentLogs = $state<EnvironmentLog[]>([]);
  let environmentLogCursor = $state("");
  let environmentLogsLoaded = $state(false);
  let environmentLogConnectionError = $state("");
  let environmentLogsPaused = $state(false);
  let followingEnvironmentLogs = $state(true);
  let workloadLogsOpen = $state(true);
  let telemetryLive = $state(false);
  let environmentLogViewport = $state<HTMLDivElement | undefined>(undefined);
  let activeReleaseDeployment = $state("");
  let promotionDialogOpen = $state(false);
  let promotionProcessing = $state(false);
  let promotionError = $state("");
  let activeBuildAction = $state("");
  let buildActionDialogOpen = $state(false);
  let pendingBuildAction = $state<{
    action: "start" | "stop" | "retry";
    build: Build;
  } | null>(null);
  let rotateDialogOpen = $state(false);
  let rotatingSecret = $state<Secret | null>(null);
  let rotatedSecretValue = $state("");
  let secretActionProcessing = $state(false);
  let secretActionError = $state("");
  let archiveSecretDialogOpen = $state(false);
  let archivingSecret = $state<Secret | null>(null);
  let expandedReleaseCommandId = $state("");
  let releaseCommandLogs = $state<Record<string, ReleaseCommandLog[]>>({});
  let releaseCommandLoading = $state("");
  let releaseCommandRetrying = $state("");
  let releaseCommandRetryTargets = $state<Record<string, string>>({});
  let releaseCommandRetryDialogOpen = $state(false);
  let pendingReleaseCommandRetry = $state<ReleaseCommand | null>(null);
  let releaseCommandRetryError = $state("");
  const loadingBuilds = new Set<string>();
  const loadingDeployments = new Set<string>();
  const builds = $derived(liveBuilds ?? environment.builds);
  const deployments = $derived(liveDeployments ?? environment.deployments);
  const activeDeployment = $derived(
    deployments.find((deployment) => deployment.active) ?? null,
  );
  const deploymentRequestReady = $derived(
    environment.deployability.missing.every(
      (missing) => missing === "managed_dns",
    ) &&
      !environment.dns.reconciliationQueued &&
      (environment.dns.mode === "manual" ||
        ["applied", "pending", "reconciling", "removing"].includes(
          environment.dns.state,
        )),
  );
  const activeInstance = $derived(
    environment.instances.find(
      (instance) =>
        instance.state === "serving" && instance.processKind === "web",
    ) ?? null,
  );
  const activeTelemetry = $derived(
    activeDeployment && activeInstance
      ? (telemetry.find(
          (row) =>
            row.deployment === activeDeployment.id &&
            row.instance === activeInstance.id,
        ) ?? null)
      : null,
  );
  const memorySeries = $derived(
    telemetry
      .filter((row) =>
        row.history.some(
          (point) =>
            point.memoryAvailable &&
            Number.isFinite(new Date(point.observedAt).getTime()),
        ),
      )
      .map((row) => ({
        id: row.instance,
        label: short(row.instance),
        active: row === activeTelemetry,
        points: row.history,
      }))
      .sort((a, b) => Number(b.active) - Number(a.active)),
  );
  const usageChange = $derived.by(() => {
    if (!activeTelemetry) return null;
    const hourAgo = Date.now() - 60 * 60 * 1000;
    const cpuSamples: number[] = [];
    const memorySamples: number[] = [];
    for (const point of activeTelemetry.history) {
      const timestamp = new Date(point.observedAt).getTime();
      if (!Number.isFinite(timestamp) || timestamp < hourAgo) continue;
      if (point.cpuAvailable) cpuSamples.push(point.cpuCores);
      if (point.memoryAvailable) memorySamples.push(point.memoryBytes);
    }
    const diff = (current: number, samples: number[]) => {
      if (samples.length === 0) return null;
      const average =
        samples.reduce((total, value) => total + value, 0) / samples.length;
      if (average <= 0) return null;
      return ((current - average) / average) * 100;
    };
    return {
      cpu: diff(activeTelemetry.cpuCores, cpuSamples),
      memory: diff(activeTelemetry.memoryBytes, memorySamples),
    };
  });
  const activeBuildId = $derived(
    builds.find((build) => build.status === "running")?.id ??
      builds.find((build) => build.status === "pending")?.id ??
      "",
  );
  const expandedDeploymentStatus = $derived(
    deployments.find((deployment) => deployment.id === expandedDeploymentId)
      ?.status ?? "",
  );
  const short = (value: string) => (value ? value.slice(0, 12) : "Unavailable");
  const formatBytes = (value: number) => {
    if (!Number.isFinite(value) || value < 0) return "Unavailable";
    if (value === 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    const index = Math.min(
      Math.floor(Math.log(value) / Math.log(1024)),
      units.length - 1,
    );
    return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
  };
  const formatCPU = (value: number) => `${value.toFixed(2)} cores`;
  const stamp = (value: string) =>
    value ? new Date(value).toLocaleString() : "Pending";
  const telemetryHref = (range: string) =>
    `${routes.environmentTelemetry(
      environment.applicationId,
      environment.environment.id,
    )}?range=${range}`;
  const stepLabel = (value: string) =>
    value ? value.replaceAll("_", " ") : "waiting for worker";
  const deploymentStep = (deployment: Deployment) =>
    deployment.active
      ? "serving"
      : deployment.status === "succeeded"
        ? "superseded"
        : deployment.currentStep || "queued";
  const secretStatusLabel = (secret: Secret) =>
    secret.status === "pending_removal"
      ? "Pending removal"
      : secret.status[0].toUpperCase() + secret.status.slice(1);
  const secretSourceLabel = (secret: Secret) => {
    if (secret.sourceType !== "environment_resource") return secret.sourceType;
    const resource = environment.resources.find(
      (candidate) => candidate.id === secret.sourceId,
    );
    return resource
      ? `Resource managed by ${resource.name}`
      : "Resource managed";
  };
  function createSecret() {
    if (!key.trim() || !value || secretCreationProcessing) return;
    secretCreationProcessing = true;
    router.post(
      routes.environmentSecretsCreate(
        environment.applicationId,
        environment.environment.id,
      ),
      { key, value },
      {
        preserveScroll: true,
        onSuccess: () => {
          key = "";
          value = "";
        },
        onFinish: () => (secretCreationProcessing = false),
      },
    );
  }

  async function loadReleaseCommandLogs(executionId: string) {
    releaseCommandLoading = executionId;
    try {
      const response = await window.fetch(
        routes.environmentReleaseCommandLogs(
          environment.applicationId,
          environment.environment.id,
          executionId,
        ),
        { headers: { Accept: "application/json" } },
      );
      if (!response.ok) throw new Error("Release command logs are unavailable");
      const payload = (await response.json()) as { logs: ReleaseCommandLog[] };
      releaseCommandLogs = {
        ...releaseCommandLogs,
        [executionId]: payload.logs,
      };
    } finally {
      releaseCommandLoading = "";
    }
  }

  async function toggleReleaseCommandLogs(executionId: string) {
    if (expandedReleaseCommandId === executionId) {
      expandedReleaseCommandId = "";
      return;
    }
    expandedReleaseCommandId = executionId;
    if (!releaseCommandLogs[executionId])
      await loadReleaseCommandLogs(executionId);
  }

  function askToRetryReleaseCommand(execution: ReleaseCommand) {
    pendingReleaseCommandRetry = execution;
    releaseCommandRetryError = "";
    releaseCommandRetryDialogOpen = true;
  }

  function releaseCommandRetryTarget(execution: ReleaseCommand) {
    const selected =
      releaseCommandRetryTargets[execution.id] ?? execution.targetId;
    return environment.runtimeTargetIds.includes(selected)
      ? selected
      : (environment.runtimeTargetIds[0] ?? "");
  }

  function retryReleaseCommand() {
    const execution = pendingReleaseCommandRetry;
    if (!execution) return;
    releaseCommandRetrying = execution.id;
    releaseCommandRetryError = "";
    router.post(
      routes.environmentReleaseCommandRetry(
        environment.applicationId,
        environment.environment.id,
        execution.id,
      ),
      { targetId: releaseCommandRetryTarget(execution) },
      {
        preserveScroll: true,
        onSuccess: () => (releaseCommandRetryDialogOpen = false),
        onError: (errors) =>
          (releaseCommandRetryError =
            Object.values(errors).map(String).join("\n") ||
            "The release command could not be retried."),
        onFinish: () => (releaseCommandRetrying = ""),
      },
    );
  }
  function askToRotate(secret: Secret) {
    rotatingSecret = secret;
    rotatedSecretValue = "";
    secretActionError = "";
    rotateDialogOpen = true;
  }
  function rotateSecret(event: SubmitEvent) {
    event.preventDefault();
    if (!rotatingSecret || rotatedSecretValue === "" || secretActionProcessing)
      return;
    secretActionProcessing = true;
    secretActionError = "";
    router.post(
      routes.environmentSecretRotate(
        environment.applicationId,
        environment.environment.id,
        rotatingSecret.id,
      ),
      { value: rotatedSecretValue },
      {
        preserveScroll: true,
        onSuccess: () => {
          rotatedSecretValue = "";
          rotateDialogOpen = false;
        },
        onError: (errors) =>
          (secretActionError =
            Object.values(errors).map(String).join("\n") ||
            "The secret could not be rotated."),
        onFinish: () => (secretActionProcessing = false),
      },
    );
  }
  function buildAndDeploy() {
    if (deploymentCreationProcessing) return;
    deploymentCreationProcessing = true;
    router.post(
      routes.environmentDeploymentsCreate(
        environment.applicationId,
        environment.environment.id,
      ),
      environment.sourceType === "image" ? { reference: imageReference } : {},
      {
        onSuccess: () => {
          liveBuilds = null;
          buildLogs = {};
          buildLogCursors = {};
          expandedBuildId = "";
        },
        onFinish: () => (deploymentCreationProcessing = false),
      },
    );
  }
  function startContainer() {
    if (containerActionProcessing) return;
    containerActionProcessing = true;
    router.post(
      routes.environmentStart(
        environment.applicationId,
        environment.environment.id,
      ),
      {},
      {
        preserveScroll: true,
        onFinish: () => (containerActionProcessing = false),
      },
    );
  }
  function restartContainer() {
    if (containerActionProcessing) return;
    containerActionProcessing = true;
    router.post(
      routes.environmentRestart(
        environment.applicationId,
        environment.environment.id,
      ),
      {},
      {
        preserveScroll: true,
        onFinish: () => (containerActionProcessing = false),
      },
    );
  }
  function adoptDNS() {
    if (dnsActionProcessing) return;
    dnsActionProcessing = true;
    router.post(
      routes.environmentDNSAdopt(
        environment.applicationId,
        environment.environment.id,
      ),
      {},
      {
        preserveScroll: true,
        onFinish: () => (dnsActionProcessing = false),
      },
    );
  }
  function retryDNS() {
    if (dnsActionProcessing) return;
    dnsActionProcessing = true;
    router.post(
      routes.environmentDNSRetry(
        environment.applicationId,
        environment.environment.id,
      ),
      {},
      {
        preserveScroll: true,
        onFinish: () => (dnsActionProcessing = false),
      },
    );
  }
  function refreshDNS() {
    if (dnsActionProcessing) return;
    dnsActionProcessing = true;
    router.post(
      routes.environmentDNSRefresh(
        environment.applicationId,
        environment.environment.id,
      ),
      {},
      {
        preserveScroll: true,
        onFinish: () => (dnsActionProcessing = false),
      },
    );
  }
  async function rotateAPIToken() {
    apiTokenProcessing = true;
    apiTokenError = "";
    try {
      const response = await window.fetch(
        routes.environmentAPITokenRotate(
          environment.applicationId,
          environment.environment.id,
        ),
        {
          method: "POST",
          credentials: "same-origin",
          headers: { Accept: "application/json" },
        },
      );
      const payload = (await response.json()) as {
        token?: string;
        error?: string;
      };
      if (!response.ok || !payload.token)
        throw new Error(payload.error || "API token could not be created");
      apiToken = payload.token;
      apiTokenConfirmOpen = false;
      apiTokenDialogOpen = true;
      router.reload({ only: ["environment"], preserveScroll: true });
    } catch (error) {
      apiTokenError =
        error instanceof Error
          ? error.message
          : "API token could not be created";
    } finally {
      apiTokenProcessing = false;
    }
  }
  function redeployRelease(releaseId: string) {
    activeReleaseDeployment = releaseId;
    router.post(
      routes.environmentReleaseDeploymentsCreate(
        environment.applicationId,
        environment.environment.id,
        releaseId,
      ),
      {},
      {
        preserveScroll: true,
        onSuccess: () => {
          liveDeployments = null;
          deploymentEvents = {};
          deploymentEventCursors = {};
          expandedDeploymentId = "";
        },
        onFinish: () => (activeReleaseDeployment = ""),
      },
    );
  }
  function askToPromote() {
    promotionError = "";
    promotionDialogOpen = true;
  }
  function promoteToProduction() {
    if (promotionProcessing) return;
    promotionProcessing = true;
    promotionError = "";
    router.post(
      routes.environmentPromoteToProduction(
        environment.applicationId,
        environment.environment.id,
      ),
      {},
      {
        preserveScroll: true,
        onSuccess: () => {
          promotionDialogOpen = false;
          liveDeployments = null;
          deploymentEvents = {};
          deploymentEventCursors = {};
          expandedDeploymentId = "";
        },
        onError: (errors) =>
          (promotionError =
            Object.values(errors).map(String).join("\n") ||
            "The release could not be promoted to production."),
        onFinish: () => (promotionProcessing = false),
      },
    );
  }
  function askToArchiveSecret(secret: Secret) {
    archivingSecret = secret;
    secretActionError = "";
    archiveSecretDialogOpen = true;
  }
  function archiveSecret() {
    if (!archivingSecret || secretActionProcessing) return;
    secretActionProcessing = true;
    secretActionError = "";
    router.delete(
      routes.environmentSecretDestroy(
        environment.applicationId,
        environment.environment.id,
        archivingSecret.id,
      ),
      {
        preserveScroll: true,
        onSuccess: () => (archiveSecretDialogOpen = false),
        onError: (errors) =>
          (secretActionError =
            Object.values(errors).map(String).join("\n") ||
            "The secret could not be archived."),
        onFinish: () => (secretActionProcessing = false),
      },
    );
  }
  function retryDeployment(deploymentId: string) {
    if (deploymentRetrying) return;
    deploymentRetrying = deploymentId;
    router.post(
      routes.environmentDeploymentRetry(
        environment.applicationId,
        environment.environment.id,
        deploymentId,
      ),
      {},
      {
        preserveScroll: true,
        onFinish: () => (deploymentRetrying = ""),
      },
    );
  }
  function askForBuildAction(action: "start" | "stop" | "retry", build: Build) {
    pendingBuildAction = { action, build };
    buildActionDialogOpen = true;
  }
  function confirmBuildAction() {
    if (!pendingBuildAction) return;
    const { action, build } = pendingBuildAction;
    activeBuildAction = `${action}:${build.id}`;
    const url =
      action === "start"
        ? routes.environmentBuildStart(
            environment.applicationId,
            environment.environment.id,
            build.id,
          )
        : action === "stop"
          ? routes.environmentBuildStop(
              environment.applicationId,
              environment.environment.id,
              build.id,
            )
          : routes.environmentBuildRetry(
              environment.applicationId,
              environment.environment.id,
              build.id,
            );
    router.post(
      url,
      {},
      {
        preserveScroll: true,
        onSuccess: () => (buildActionDialogOpen = false),
        onFinish: () => (activeBuildAction = ""),
      },
    );
  }
  async function loadBuildLogs(buildId: string, signal?: AbortSignal) {
    if (loadingBuilds.has(buildId)) return null;
    loadingBuilds.add(buildId);
    try {
      const after = buildLogCursors[buildId] ?? 0;
      const response = await window.fetch(
        `${routes.environmentBuildLogs(environment.environment.id, buildId)}?after=${after}`,
        {
          cache: "no-store",
          credentials: "same-origin",
          headers: { Accept: "application/json" },
          signal,
        },
      );
      if (!response.ok)
        throw new Error(`Build logs returned ${response.status}`);
      const snapshot = (await response.json()) as BuildLogSnapshot;
      liveBuilds = builds.map((build) =>
        build.id === snapshot.build.id ? snapshot.build : build,
      );
      if (snapshot.logs.length > 0) {
        buildLogs = {
          ...buildLogs,
          [buildId]: [...(buildLogs[buildId] ?? []), ...snapshot.logs],
        };
      } else if (!(buildId in buildLogs)) {
        buildLogs = { ...buildLogs, [buildId]: [] };
      }
      buildLogCursors = {
        ...buildLogCursors,
        [buildId]: snapshot.nextSequence,
      };
      buildLogConnectionError = "";
      return snapshot;
    } finally {
      loadingBuilds.delete(buildId);
    }
  }
  async function toggleBuildLogs(buildId: string) {
    if (expandedBuildId === buildId) {
      expandedBuildId = "";
      return;
    }
    expandedBuildId = buildId;
    if (!(buildId in buildLogs)) {
      try {
        let snapshot = await loadBuildLogs(buildId);
        while (snapshot?.hasMore) snapshot = await loadBuildLogs(buildId);
      } catch {
        buildLogConnectionError = "Build logs are temporarily unavailable.";
      }
    }
  }

  async function loadDeploymentEvents(
    deploymentId: string,
    signal?: AbortSignal,
  ) {
    if (loadingDeployments.has(deploymentId)) return null;
    loadingDeployments.add(deploymentId);
    try {
      const after = deploymentEventCursors[deploymentId] ?? 0;
      const response = await window.fetch(
        `${routes.environmentDeploymentEvents(environment.environment.id, deploymentId)}?after=${after}`,
        {
          cache: "no-store",
          credentials: "same-origin",
          headers: { Accept: "application/json" },
          signal,
        },
      );
      if (!response.ok)
        throw new Error(`Deployment events returned ${response.status}`);
      const snapshot = (await response.json()) as DeploymentEventSnapshot;
      liveDeployments = deployments.map((deployment) => {
        if (deployment.id === snapshot.deployment.id)
          return snapshot.deployment;
        if (snapshot.deployment.active) return { ...deployment, active: false };
        return deployment;
      });
      if (snapshot.events.length > 0) {
        deploymentEvents = {
          ...deploymentEvents,
          [deploymentId]: [
            ...(deploymentEvents[deploymentId] ?? []),
            ...snapshot.events,
          ],
        };
      } else if (!(deploymentId in deploymentEvents)) {
        deploymentEvents = { ...deploymentEvents, [deploymentId]: [] };
      }
      deploymentEventCursors = {
        ...deploymentEventCursors,
        [deploymentId]: snapshot.nextSequence,
      };
      deploymentEventConnectionError = "";
      return snapshot;
    } finally {
      loadingDeployments.delete(deploymentId);
    }
  }

  async function toggleDeploymentEvents(deploymentId: string) {
    if (expandedDeploymentId === deploymentId) {
      expandedDeploymentId = "";
      return;
    }
    expandedDeploymentId = deploymentId;
    const deployment = deployments.find((item) => item.id === deploymentId);
    if (deployment?.status === "queued" || deployment?.status === "running")
      return;
    if (!(deploymentId in deploymentEvents)) {
      try {
        let snapshot = await loadDeploymentEvents(deploymentId);
        while (snapshot?.hasMore)
          snapshot = await loadDeploymentEvents(deploymentId);
      } catch {
        deploymentEventConnectionError =
          "Deployment events are temporarily unavailable.";
      }
    }
  }

  async function loadEnvironmentLogs(signal?: AbortSignal) {
    const endpoint = new URL(
      routes.environmentLogs(
        environment.applicationId,
        environment.environment.id,
      ),
      window.location.origin,
    );
    if (environmentLogCursor)
      endpoint.searchParams.set("after", environmentLogCursor);
    const response = await window.fetch(endpoint, {
      cache: "no-store",
      credentials: "same-origin",
      headers: { Accept: "application/json" },
      signal,
    });
    if (!response.ok)
      throw new Error(`Environment logs returned ${response.status}`);
    const snapshot = (await response.json()) as EnvironmentLogSnapshot;
    if (snapshot.logs.length > 0) {
      environmentLogs = [...environmentLogs, ...snapshot.logs].slice(-2000);
    }
    environmentLogCursor = snapshot.nextCursor;
    environmentLogsLoaded = true;
    environmentLogConnectionError = "";
    return snapshot;
  }

  function updateEnvironmentLogFollow() {
    if (!environmentLogViewport) return;
    followingEnvironmentLogs =
      environmentLogViewport.scrollHeight -
        environmentLogViewport.scrollTop -
        environmentLogViewport.clientHeight <
      48;
  }

  $effect(() => {
    if (section !== "telemetry") return;
    environmentLogs.length;
    if (!followingEnvironmentLogs) return;
    const frame = window.requestAnimationFrame(() => {
      environmentLogViewport?.scrollTo({
        top: environmentLogViewport.scrollHeight,
      });
    });
    return () => window.cancelAnimationFrame(frame);
  });

  $effect(() => {
    if (section !== "telemetry" || environmentLogsPaused) return;
    const abortController = new AbortController();
    let timer: number | undefined;
    let retryDelay = 2000;

    async function poll() {
      try {
        const snapshot = await loadEnvironmentLogs(abortController.signal);
        if (abortController.signal.aborted) return;
        retryDelay = 2000;
        timer = window.setTimeout(poll, snapshot.hasMore ? 0 : retryDelay);
      } catch {
        if (abortController.signal.aborted) return;
        environmentLogConnectionError =
          "Reconnecting to the workload log stream...";
        retryDelay = Math.min(retryDelay * 2, 10000);
        timer = window.setTimeout(poll, retryDelay);
      }
    }

    timer = window.setTimeout(poll, 0);
    return () => {
      abortController.abort();
      if (timer !== undefined) window.clearTimeout(timer);
    };
  });

  $effect(() => {
    if (section !== "builds") return;
    const buildId = activeBuildId;
    if (!buildId) return;
    if (!expandedBuildId) expandedBuildId = buildId;

    const abortController = new AbortController();
    let timer: number | undefined;
    let retryDelay = 1000;

    async function poll() {
      try {
        const snapshot = await loadBuildLogs(buildId, abortController.signal);
        if (!snapshot || abortController.signal.aborted) return;
        retryDelay = 1000;
        if (snapshot.hasMore) {
          timer = window.setTimeout(poll, 0);
          return;
        }
        if (
          snapshot.build.status !== "pending" &&
          snapshot.build.status !== "running"
        ) {
          router.reload({
            only: ["environment", "telemetry"],
            preserveScroll: true,
          });
          return;
        }
      } catch {
        if (abortController.signal.aborted) return;
        buildLogConnectionError = "Reconnecting to the Build log...";
        retryDelay = Math.min(retryDelay * 2, 5000);
      }
      timer = window.setTimeout(poll, retryDelay);
    }

    timer = window.setTimeout(poll, 0);
    return () => {
      abortController.abort();
      if (timer !== undefined) window.clearTimeout(timer);
    };
  });

  $effect(() => {
    if (section !== "overview") return;
    const dnsState = environment.dns.state;
    if (!environment.dns.reconciliationQueued && dnsState !== "reconciling")
      return;

    const timer = window.setInterval(() => {
      router.reload({ only: ["environment"], preserveScroll: true });
    }, 2000);
    return () => window.clearInterval(timer);
  });

  $effect(() => {
    if (section !== "overview") return;
    const timer = window.setInterval(() => {
      router.reload({ only: ["telemetry"], preserveScroll: true });
    }, 30000);
    return () => window.clearInterval(timer);
  });

  $effect(() => {
    if (section !== "telemetry" || !telemetryLive) return;
    let refreshing = false;
    const refresh = () => {
      if (refreshing || document.visibilityState !== "visible") return;
      refreshing = true;
      router.reload({
        only: ["telemetry"],
        preserveScroll: true,
        preserveState: true,
        onFinish: () => (refreshing = false),
      });
    };
    refresh();
    const timer = window.setInterval(refresh, 3000);
    return () => window.clearInterval(timer);
  });

  $effect(() => {
    if (section !== "deployments") return;
    const activeReleaseCommand = environment.releaseCommands.find(
      (execution) =>
        execution.status === "queued" || execution.status === "running",
    );
    if (!activeReleaseCommand) return;
    const timer = window.setInterval(() => {
      if (expandedReleaseCommandId === activeReleaseCommand.id)
        void loadReleaseCommandLogs(activeReleaseCommand.id);
      router.reload({ only: ["environment"], preserveScroll: true });
    }, 2000);
    return () => window.clearInterval(timer);
  });

  $effect(() => {
    if (section !== "deployments") return;
    const deploymentId = expandedDeploymentId;
    const deploymentStatus = expandedDeploymentStatus;
    if (
      !deploymentId ||
      (deploymentStatus !== "queued" && deploymentStatus !== "running")
    )
      return;

    const abortController = new AbortController();
    let timer: number | undefined;
    let retryDelay = 1000;

    async function poll() {
      try {
        const snapshot = await loadDeploymentEvents(
          deploymentId,
          abortController.signal,
        );
        if (!snapshot || abortController.signal.aborted) return;
        retryDelay = 1000;
        if (snapshot.hasMore) {
          timer = window.setTimeout(poll, 0);
          return;
        }
        if (
          snapshot.deployment.status !== "queued" &&
          snapshot.deployment.status !== "running"
        ) {
          router.reload({
            only: ["environment", "telemetry"],
            preserveScroll: true,
          });
          return;
        }
      } catch {
        if (abortController.signal.aborted) return;
        deploymentEventConnectionError =
          "Reconnecting to the Deployment timeline...";
        retryDelay = Math.min(retryDelay * 2, 5000);
      }
      timer = window.setTimeout(poll, retryDelay);
    }

    timer = window.setTimeout(poll, 0);
    return () => {
      abortController.abort();
      if (timer !== undefined) window.clearTimeout(timer);
    };
  });
</script>

<svelte:head><title>{environment.environment.name}</title></svelte:head>
<DashboardLayout
  email={auth.email}
  environmentNavigation={{
    applicationId: environment.applicationId,
    applicationName: environment.applicationName,
    id: environment.environment.id,
    name: environment.environment.name,
  }}
>
  <div class="space-y-8">
    <header
      class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"
    >
      <div>
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          {environment.applicationName} · {environment.environment.kind}
        </p>
        <h1 class="mt-3 text-3xl font-semibold">
          {environment.environment.name}
        </h1>
      </div>
      {#if section === "overview"}
        <div class="flex flex-wrap gap-2">
          {#if environment.sourceType === "buildpacks"}<Button
              disabled={deploymentCreationProcessing || !deploymentRequestReady}
              aria-busy={deploymentCreationProcessing}
              onclick={buildAndDeploy}
              >{#if deploymentCreationProcessing}<Spinner />{/if}Build & deploy</Button
            >{/if}<EnvironmentDeleteDialog
            applicationId={environment.applicationId}
            environmentId={environment.environment.id}
            environmentName={environment.environment.name}
          />
        </div>
      {/if}
    </header>

    {#if section === "overview"}
      <UsageSummary
        cpuCores={activeTelemetry?.cpuCores ?? 0}
        memoryBytes={activeTelemetry?.memoryBytes ?? 0}
        cpuChange={usageChange?.cpu ?? null}
        memoryChange={usageChange?.memory ?? null}
        cpuAvailable={Boolean(
          activeTelemetry?.available && activeTelemetry.cpuAvailable,
        )}
        memoryAvailable={Boolean(
          activeTelemetry?.available && activeTelemetry.memoryAvailable,
        )}
        observedAt={activeTelemetry?.observedAt ?? ""}
        telemetryUrl={routes.environmentTelemetry(
          environment.applicationId,
          environment.environment.id,
        )}
      />

      <Card.Root
        ><Card.Header
          ><Card.Action
            ><StatusBadge
              status={environment.deployability.deployable
                ? "ready"
                : "blocked"}
            /></Card.Action
          ><Card.Title>Desired state</Card.Title></Card.Header
        ><Card.Content class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4"
          ><DataField
            label="Repository"
            value={environment.repository}
          /><DataField
            label="Reference"
            value={environment.reference}
          /><DataField
            label="Build context"
            value={environment.contextPath}
          /><DataField label="Domain" value={environment.domain} /><DataField
            label="Runtime Server targets"
            value={environment.runtimeServers.join(", ")}
          /><DataField
            label="Registry"
            value={environment.registryName}
          /><DataField
            label="Registry endpoint"
            value={environment.registryEndpoint}
          />{#if !environment.deployability.deployable}<DataField
              label="Missing"
              value={environment.deployability.missing.join(", ")}
            />{/if}</Card.Content
        ></Card.Root
      >

      <Card.Root>
        <Card.Header
          ><Card.Title>Process formation</Card.Title><Card.Description
            >The immutable process configuration used by the next Release
            rollout.</Card.Description
          ></Card.Header
        >
        <Card.Content class="space-y-2">
          {#each environment.processes as process}
            <div
              class="grid gap-2 border border-border p-3 text-sm sm:grid-cols-[8rem_8rem_minmax(0,1fr)_auto]"
            >
              <div>
                <p class="font-mono font-medium">{process.name}</p>
                <StatusBadge status={process.kind} />
              </div>
              <p>
                {process.kind === "release"
                  ? "one-off"
                  : `${process.replicas} replica${process.replicas === 1 ? "" : "s"}`}
              </p>
              <p class="break-all font-mono text-xs text-muted-foreground">
                {process.command
                  ? [process.command, ...process.arguments].join(" ")
                  : "OCI image default command"}
              </p>
              <p class="text-xs text-muted-foreground">
                {process.kind === "web"
                  ? `Port ${process.container_port}${process.health_path ? ` · ${process.health_path}` : ""}`
                  : process.kind === "release"
                    ? `${process.timeout_seconds}s timeout`
                    : "Private"}
              </p>
            </div>
          {/each}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header
          ><Card.Action
            ><StatusBadge
              status={environment.dns.state}
              label={environment.dns.mode === "manual"
                ? "Manual"
                : environment.dns.state.replaceAll("_", " ")}
            /></Card.Action
          ><Card.Title>DNS</Card.Title><Card.Description
            >{environment.dns.mode === "manual"
              ? "DeployCrate does not change DNS records for this Environment."
              : `${environment.dns.connectionName} · ${environment.dns.zoneName}`}</Card.Description
          ></Card.Header
        >
        <Card.Content class="space-y-4">
          {#if environment.dns.lastError}<p
              class="border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive"
            >
              {environment.dns.lastError}
            </p>{/if}
          {#if environment.dns.mode === "cloudflare" && !environment.dns.reconciliationQueued && ["pending", "removing"].includes(environment.dns.state)}<p
              class="border border-border bg-muted/20 p-3 text-sm text-muted-foreground"
            >
              DNS changes are staged. Use the deployment action when you are
              ready to apply DNS and deploy this desired state.
            </p>{/if}
          {#if environment.dns.records.length > 0}<div class="space-y-2">
              {#each environment.dns.records as record}<div
                  class="grid gap-1 border border-border p-3 font-mono text-sm sm:grid-cols-[auto_1fr_1fr]"
                >
                  <span>{record.type}</span><span>{record.name}</span><span
                    >{record.content}</span
                  >
                </div>{/each}
            </div>{/if}
          {#if environment.dns.state === "conflict"}<div
              class="flex flex-wrap items-center justify-between gap-3"
            >
              <p class="text-sm text-muted-foreground">
                Existing records are unmanaged. Confirm adoption to replace them
                with this Environment's server addresses.
              </p>
              <Button disabled={dnsActionProcessing} onclick={adoptDNS}
                >{#if dnsActionProcessing}<Spinner />{/if}Adopt and replace</Button
              >
            </div>{/if}
          {#if environment.dns.state === "failed" || environment.dns.state === "removal_failed"}<div
              class="flex flex-wrap items-center justify-between gap-3"
            >
              <p class="text-sm text-muted-foreground">
                The Environment stays saved, but deployment remains blocked
                until the DNS operation succeeds.
              </p>
              <Button
                variant="outline"
                disabled={dnsActionProcessing}
                onclick={retryDNS}
                >{#if dnsActionProcessing}<Spinner />{/if}Retry DNS</Button
              >
            </div>{/if}
          {#if environment.dns.state === "applied" && !environment.dns.reconciliationQueued}<div
              class="flex flex-wrap items-center justify-between gap-3"
            >
              <p class="text-sm text-muted-foreground">
                The DNS records are up to date. Re-sync to re-verify the current
                server addresses against Cloudflare.
              </p>
              <Button
                variant="outline"
                disabled={dnsActionProcessing}
                onclick={refreshDNS}
                >{#if dnsActionProcessing}<Spinner />{/if}Re-sync DNS</Button
              >
            </div>{/if}
        </Card.Content>
      </Card.Root>

      {#if environment.sourceType === "image"}
        <Card.Root
          ><Card.Header
            ><Card.Title>Deploy image version</Card.Title><Card.Description
              >Use the configured reference or override it with another tag or
              sha256 digest.</Card.Description
            ></Card.Header
          ><Card.Content class="flex flex-col gap-3 sm:flex-row"
            ><Input bind:value={imageReference} placeholder="latest" /><Button
              disabled={deploymentCreationProcessing ||
                !imageReference.trim() ||
                !deploymentRequestReady}
              aria-busy={deploymentCreationProcessing}
              onclick={buildAndDeploy}
              >{#if deploymentCreationProcessing}<Spinner />{/if}Resolve &
              deploy</Button
            ></Card.Content
          ></Card.Root
        >
      {/if}

      {#if environment.sourceType === "image"}
        <Card.Root
          ><Card.Header
            ><Card.Action
              ><Button
                size="sm"
                variant="outline"
                disabled={apiTokenProcessing}
                onclick={() => (apiTokenConfirmOpen = true)}
                >{#if apiTokenProcessing}<Spinner
                  />{/if}{environment.apiTokenPrefix
                  ? "Rotate token"
                  : "Create token"}</Button
              ></Card.Action
            ><Card.Title>Deployment API</Card.Title><Card.Description
              >Environment-scoped bearer token for image deployments. A
              replacement token invalidates the previous token immediately.</Card.Description
            ></Card.Header
          ><Card.Content class="space-y-2 text-sm"
            ><p class="font-mono">
              POST /api/environments/{environment.environment.id}/deployments
            </p>
            <p class="text-xs text-muted-foreground">
              JSON: {`{"reference":"1.2.3"}`} · Token: {environment.apiTokenPrefix
                ? `${environment.apiTokenPrefix}...`
                : "Not configured"}
            </p>
            {#if apiTokenError}<p class="text-xs text-destructive">
                {apiTokenError}
              </p>{/if}</Card.Content
          ></Card.Root
        >
      {/if}

      <Card.Root
        ><Card.Header
          ><Card.Action
            >{#if container.exists}<div class="flex gap-2">
                {#if container.running}<Button
                    size="sm"
                    variant="outline"
                    disabled={containerActionProcessing}
                    aria-busy={containerActionProcessing}
                    onclick={restartContainer}
                    >{#if containerActionProcessing}<Spinner
                      />{/if}Restart</Button
                  >{:else}<Button
                    size="sm"
                    variant="outline"
                    disabled={containerActionProcessing}
                    aria-busy={containerActionProcessing}
                    onclick={startContainer}
                    >{#if containerActionProcessing}<Spinner
                      />{/if}Start</Button
                  >{/if}
              </div>{/if}</Card.Action
          ><Card.Title>Container</Card.Title><Card.Description
            >Runtime state and controls for the serving web container.</Card.Description
          ></Card.Header
        ><Card.Content
          >{#if container.exists}<div class="flex flex-wrap items-center gap-2">
              <StatusBadge
                status={container.running ? "running" : "stopped"}
                label={container.running ? "Running" : "Stopped"}
              /><span class="font-mono text-xs text-muted-foreground"
                >Instance {short(container.instanceId)} · Deployment {short(
                  container.deploymentId,
                )}</span
              >
            </div>{:else}<p class="text-sm text-muted-foreground">
              No serving container is currently deployed.
            </p>{/if}</Card.Content
        ></Card.Root
      >
    {/if}

    {#if section === "telemetry"}
      <section aria-labelledby="workload-telemetry-heading" class="space-y-4">
        <div class="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2
              id="workload-telemetry-heading"
              class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
            >
              Workload telemetry
            </h2>
            <p class="mt-1 text-xs text-muted-foreground">
              Current rates and usage for the active container
            </p>
          </div>
          {#if activeTelemetry}
            <p
              class="flex flex-wrap items-center gap-x-4 gap-y-1 font-mono text-[11px] text-muted-foreground"
            >
              <span class="flex items-baseline gap-1.5">
                <span
                  class="font-sans text-[10px] font-medium uppercase tracking-[0.14em]"
                  >Deployment</span
                >
                {short(activeTelemetry.deployment)}
              </span>
              <span class="flex items-baseline gap-1.5">
                <span
                  class="font-sans text-[10px] font-medium uppercase tracking-[0.14em]"
                  >Release</span
                >
                {short(activeTelemetry.release)}
              </span>
            </p>
          {/if}
          <div
            class="flex flex-wrap items-center gap-1"
            aria-label="Telemetry time range"
          >
            {#each [{ value: "1h", label: "1h" }, { value: "6h", label: "6h" }, { value: "24h", label: "24h" }, { value: "7d", label: "7d" }] as option}
              <Button
                size="sm"
                variant={telemetryRange === option.value
                  ? "default"
                  : "outline"}
                aria-pressed={telemetryRange === option.value}
                href={telemetryHref(option.value)}>{option.label}</Button
              >
            {/each}
            <Button
              size="sm"
              variant={telemetryLive ? "default" : "outline"}
              aria-pressed={telemetryLive}
              onclick={() => (telemetryLive = !telemetryLive)}
            >
              <span
                class={`size-1.5 rounded-full ${telemetryLive ? "bg-primary-foreground animate-pulse" : "bg-muted-foreground"}`}
              ></span>
              Live
            </Button>
          </div>
        </div>

        {#if activeTelemetry}
          <div class="grid gap-3 lg:grid-cols-2">
            <UsageDonut
              label="CPU"
              used={activeTelemetry.cpuCores}
              total={host.cpuCores}
              formatValue={formatCPU}
              available={activeTelemetry.available &&
                activeTelemetry.cpuAvailable &&
                host.available}
            />
            <UsageDonut
              label="Memory"
              used={activeTelemetry.memoryBytes}
              total={host.memoryBytes}
              formatValue={formatBytes}
              available={activeTelemetry.available &&
                activeTelemetry.memoryAvailable &&
                host.available}
            />
          </div>
        {/if}
        {#if activeTelemetry || memorySeries.length > 0}
          <TelemetryHistory series={memorySeries} {telemetryRange} />
        {:else}
          <div class="border border-border bg-card/35 px-5 py-16 text-center">
            <p class="text-sm text-muted-foreground">
              No telemetry is available for the active container yet.
            </p>
          </div>
        {/if}
      </section>

      <Collapsible.Root bind:open={workloadLogsOpen}>
        <Card.Root>
          <Card.Header>
            <Card.Action
              ><div class="flex gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  onclick={() => (workloadLogsOpen = !workloadLogsOpen)}
                  aria-expanded={workloadLogsOpen}
                  >{workloadLogsOpen ? "Hide logs" : "Show logs"}</Button
                ><Button
                  size="sm"
                  variant="outline"
                  disabled={!workloadLogsOpen}
                  onclick={() =>
                    (environmentLogsPaused = !environmentLogsPaused)}
                  >{environmentLogsPaused ? "Resume" : "Pause"}</Button
                >
              </div></Card.Action
            >
            <Card.Title>Workload logs</Card.Title>
            <Card.Description
              >Live stdout and stderr from this Environment's containers.
              ClickHouse retains logs for seven days.</Card.Description
            >
          </Card.Header>
          <Collapsible.Content>
            <Card.Content>
              {#if environmentLogConnectionError}<p
                  class="mb-3 text-xs text-warning"
                >
                  {environmentLogConnectionError}
                </p>{/if}
              <div
                bind:this={environmentLogViewport}
                onscroll={updateEnvironmentLogFollow}
                class="max-h-[32rem] min-h-48 overflow-auto border border-border bg-black/35 p-3 font-mono text-[11px] leading-relaxed"
              >
                {#each environmentLogs as log (log.id)}
                  <div
                    class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 py-1"
                    class:text-destructive={log.stream === "stderr"}
                  >
                    <span
                      class="select-none whitespace-nowrap text-muted-foreground"
                      >{stamp(log.occurredAt)}</span
                    >
                    <div class="min-w-0">
                      <p class="select-none text-[10px] text-muted-foreground">
                        {log.stream} · {log.processKind || "process"}
                        {log.processName || "unknown"}{log.processReplica
                          ? ` · ${log.processReplica}`
                          : ""} · {log.container || "container"} · deployment {short(
                          log.deployment,
                        )} · instance {short(log.instance)}
                      </p>
                      <pre
                        class="whitespace-pre-wrap break-words font-mono">{log.message}</pre>
                    </div>
                  </div>
                {:else}
                  <p class="text-muted-foreground">
                    {environmentLogsLoaded
                      ? "No workload logs have been collected yet."
                      : "Loading workload logs..."}
                  </p>
                {/each}
              </div>
            </Card.Content>
          </Collapsible.Content>
        </Card.Root>
      </Collapsible.Root>
    {/if}

    {#if section === "overview"}
      <Card.Root
        ><Card.Header
          ><Card.Title>Resources</Card.Title><Card.Description
            >Explicit connections available to this Environment.</Card.Description
          ></Card.Header
        ><Card.Content class="space-y-2"
          >{#each environment.resources as resource}<div
              class="grid gap-1 border border-border p-3 sm:grid-cols-3"
            >
              <span class="font-mono text-sm">{resource.alias}</span><span
                >{resource.name}</span
              ><span class="text-muted-foreground">{resource.engine}</span>
            </div>{:else}<p class="text-sm text-muted-foreground">
              No Resources selected.
            </p>{/each}</Card.Content
        ></Card.Root
      >
    {/if}

    {#if section === "secrets"}
      <Card.Root
        ><Card.Header
          ><Card.Title>Environment secrets</Card.Title><Card.Description
            >Status compares each desired value fingerprint with the revision
            running in the serving container. Resource-managed values are
            changed only from their Resource.</Card.Description
          ></Card.Header
        ><Card.Content class="space-y-3"
          >{#each environment.variables as variable}<div
              class="grid gap-1 border border-border p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto]"
            >
              <p class="font-mono text-sm">{variable.key}</p>
              <p class="break-all font-mono text-sm">{variable.value}</p>
              <p class="text-xs text-muted-foreground">{variable.source}</p>
            </div>{/each}{#each environment.secrets as secret}<div
              class="flex flex-col gap-4 border border-border p-3 sm:flex-row sm:items-center sm:justify-between"
            >
              <div>
                <div class="flex flex-wrap items-center gap-2">
                  <p class="font-mono text-sm">{secret.key}</p>
                  <StatusBadge
                    status={secret.status}
                    label={secretStatusLabel(secret)}
                  />
                </div>
                <p class="font-mono text-sm">••••••••</p>
                <p class="text-xs text-muted-foreground">
                  {secret.digestPrefix} · {secretSourceLabel(secret)}
                </p>
              </div>
              {#if secret.desired && secret.sourceType === "user"}<div
                  class="flex gap-2"
                >
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={secretActionProcessing}
                    onclick={() => askToRotate(secret)}>Rotate</Button
                  ><Button
                    size="sm"
                    variant="destructive"
                    disabled={secretActionProcessing}
                    onclick={() => askToArchiveSecret(secret)}>Archive</Button
                  >
                </div>{/if}
            </div>{/each}{#if environment.variables.length === 0 && environment.secrets.length === 0}<p
              class="border border-dashed border-border p-4 text-sm text-muted-foreground"
            >
              No Environment secrets configured.
            </p>{/if}
          <div
            class="grid gap-3 border-t border-border pt-4 sm:grid-cols-[1fr_2fr_auto]"
          >
            <Input
              bind:value={key}
              placeholder="SECRET_KEY"
              autocomplete="off"
            /><Input
              type="password"
              bind:value
              placeholder="Write-only value"
              autocomplete="new-password"
            /><Button
              disabled={!key.trim() || !value || secretCreationProcessing}
              aria-busy={secretCreationProcessing}
              onclick={createSecret}
              >{#if secretCreationProcessing}<Spinner />{/if}Add secret</Button
            >
          </div></Card.Content
        ></Card.Root
      >
    {/if}

    {#if section === "builds"}
      <div class="grid gap-8 xl:grid-cols-2">
        <Card.Root>
          <Card.Header
            ><Card.Title>Builds</Card.Title><Card.Description
              >Builds run in the background. Active output is loaded
              automatically.</Card.Description
            ></Card.Header
          >
          <Card.Content class="space-y-2">
            {#if buildLogConnectionError}<p class="text-xs text-warning">
                {buildLogConnectionError}
              </p>{/if}
            {#each builds as build}
              <div class="border border-border text-sm">
                <div class="flex items-start gap-2 p-3">
                  <Button
                    type="button"
                    variant="ghost"
                    class="h-auto min-w-0 flex-1 flex-col items-stretch p-0 text-left whitespace-normal hover:bg-transparent"
                    onclick={() => toggleBuildLogs(build.id)}
                    aria-expanded={expandedBuildId === build.id}
                  >
                    <div class="flex justify-between gap-3">
                      <span class="font-mono"
                        >{short(build.sourceRevision)}</span
                      ><StatusBadge status={build.status} />
                    </div>
                    <p class="mt-1 text-xs text-muted-foreground">
                      {stamp(build.createdAt)} · {stepLabel(build.currentStep)}
                    </p>
                    <p
                      class="mt-1 break-all font-mono text-[11px] text-muted-foreground"
                    >
                      {build.registryEndpoint ||
                        "Registry unavailable"}{build.jobId
                        ? ` · Job #${build.jobId} · ${build.jobState}`
                        : ""}
                    </p>
                  </Button>
                  <div class="flex shrink-0 flex-wrap justify-end gap-1">
                    {#if build.status === "pending" || (build.status === "running" && ["scheduled", "retryable", "pending"].includes(build.jobState))}<Button
                        size="xs"
                        disabled={Boolean(activeBuildAction)}
                        onclick={() => askForBuildAction("start", build)}
                        >{#if activeBuildAction === `start:${build.id}`}<Spinner
                          />{/if}Run now</Button
                      >{/if}
                    {#if build.status === "pending" || build.status === "running"}<Button
                        size="xs"
                        variant="outline"
                        disabled={Boolean(activeBuildAction)}
                        onclick={() => askForBuildAction("stop", build)}
                        >{#if activeBuildAction === `stop:${build.id}`}<Spinner
                          />{/if}Stop</Button
                      >{/if}
                    {#if build.status === "failed" || build.status === "cancelled"}<Button
                        size="xs"
                        variant="outline"
                        disabled={Boolean(activeBuildAction)}
                        onclick={() => askForBuildAction("retry", build)}
                        >{#if activeBuildAction === `retry:${build.id}`}<Spinner
                          />{/if}Retry</Button
                      >{/if}
                  </div>
                </div>
                {#if expandedBuildId === build.id}
                  <div class="border-t border-border bg-black/30 p-3">
                    <div
                      class="max-h-96 space-y-2 overflow-auto font-mono text-[11px] leading-relaxed"
                    >
                      {#each buildLogs[build.id] ?? [] as log (log.id)}
                        <div class:text-primary={log.stream === "system"}>
                          <span class="select-none text-muted-foreground"
                            >{stamp(log.occurredAt)} · {log.stream}</span
                          >
                          <pre
                            class="whitespace-pre-wrap break-words font-mono">{log.message}</pre>
                        </div>
                      {:else}
                        <p class="text-muted-foreground">
                          Waiting for Build output...
                        </p>
                      {/each}
                    </div>
                    {#if build.error}<pre
                        class="mt-3 whitespace-pre-wrap break-words border-t border-destructive/30 pt-3 text-xs text-destructive">{build.error}</pre>{/if}
                  </div>
                {/if}
              </div>
            {:else}<p class="text-sm text-muted-foreground">
                No Builds yet.
              </p>{/each}
          </Card.Content>
        </Card.Root>
        <Card.Root>
          <Card.Header
            ><Card.Title>Releases</Card.Title><Card.Description
              >Redeploy an existing image with the current Environment secrets
              and configuration.</Card.Description
            ></Card.Header
          >
          <Card.Content class="space-y-2">
            {#if environment.canPromoteToProduction}
              <div
                class="flex items-start justify-between gap-3 border border-primary/40 bg-primary/10 p-3 text-sm"
              >
                <div>
                  <p class="font-medium">Promote to production</p>
                  <p class="mt-1 text-xs text-muted-foreground">
                    Promote the latest successful staging deployment (Release{" "}
                    {short(environment.latestSuccessfulReleaseId ?? "")}) to{" "}
                    {environment.promotionTargetName}, creating a new immutable
                    production Release and queuing its deployment.
                  </p>
                </div>
                <Button
                  size="sm"
                  disabled={promotionProcessing}
                  aria-busy={promotionProcessing}
                  onclick={askToPromote}
                  >{#if promotionProcessing}<Spinner />{/if}Promote</Button
                >
              </div>
            {/if}
            {#each environment.releases as release}
              <div class="border border-border p-3 text-sm">
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <span class="flex justify-between">
                      <p class="font-mono">ID {short(release.id)}</p>
                      <p class="font-mono mx-2">·</p>
                      <p class="font-mono">
                        Revision {short(release.sourceRevision)}
                      </p>
                    </span>
                    <p class="mt-1 text-xs text-muted-foreground">
                      {stamp(release.createdAt)}
                    </p>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={Boolean(activeReleaseDeployment)}
                    aria-busy={activeReleaseDeployment === release.id}
                    onclick={() => redeployRelease(release.id)}
                    >{#if activeReleaseDeployment === release.id}<Spinner
                      />{/if}Redeploy</Button
                  >
                </div>
                <p
                  class="mt-2 break-all font-mono text-xs text-muted-foreground"
                >
                  {release.artifactReference}
                </p>
              </div>
            {:else}<p class="text-sm text-muted-foreground">
                No Releases yet.
              </p>{/each}
          </Card.Content>
        </Card.Root>
      </div>
    {/if}

    {#if section === "deployments"}
      <Card.Root>
        <Card.Header
          ><Card.Title>Release commands</Card.Title><Card.Description
            >One-off commands gate target creation. Ambiguous outcomes are never
            rerun automatically.</Card.Description
          ></Card.Header
        >
        <Card.Content class="space-y-2">
          {#each environment.releaseCommands as execution}
            <div class="border border-border text-sm">
              <div class="grid gap-3 p-3 lg:grid-cols-[minmax(0,1fr)_auto]">
                <button
                  type="button"
                  class="min-w-0 text-left"
                  onclick={() => toggleReleaseCommandLogs(execution.id)}
                  aria-expanded={expandedReleaseCommandId === execution.id}
                >
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-mono">{short(execution.id)}</span
                    ><StatusBadge status={execution.status} /><span
                      class="text-xs text-muted-foreground"
                      >Attempt {execution.attempt} · Release {short(
                        execution.releaseId,
                      )} · {execution.targetName}</span
                    >
                  </div>
                  <p
                    class="mt-2 break-all font-mono text-xs text-muted-foreground"
                  >
                    {[execution.command, ...execution.arguments].join(" ")}
                  </p>
                  {#if execution.error}<p class="mt-2 text-xs text-destructive">
                      {execution.error}
                    </p>{/if}
                </button>
                {#if execution.status === "failed" || execution.status === "ambiguous"}
                  <div class="flex flex-wrap items-center gap-2">
                    <select
                      class="h-8 border border-input bg-background px-2 text-xs"
                      value={releaseCommandRetryTarget(execution)}
                      onchange={(event) =>
                        (releaseCommandRetryTargets = {
                          ...releaseCommandRetryTargets,
                          [execution.id]: event.currentTarget.value,
                        })}
                      aria-label="Release command retry target"
                    >
                      {#each environment.runtimeTargetIds as targetId, index}<option
                          value={targetId}
                          >{environment.runtimeServers[index]}</option
                        >{/each}
                    </select>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={Boolean(releaseCommandRetrying) ||
                        environment.runtimeTargetIds.length === 0}
                      onclick={() => askToRetryReleaseCommand(execution)}
                      >Review retry</Button
                    >
                  </div>
                {/if}
              </div>
              {#if expandedReleaseCommandId === execution.id}
                <div
                  class="max-h-96 overflow-auto border-t border-border bg-black/30 p-3 font-mono text-[11px] leading-relaxed"
                >
                  {#each releaseCommandLogs[execution.id] ?? [] as log (log.id)}<div
                      class:text-destructive={log.stream === "stderr"}
                      class:text-primary={log.stream === "system"}
                    >
                      <span class="text-muted-foreground"
                        >{stamp(log.occurredAt)} · attempt {log.attempt} · {log.stream}</span
                      >
                      <pre
                        class="whitespace-pre-wrap break-words font-mono">{log.message}</pre>
                    </div>{:else}<p class="text-muted-foreground">
                      {releaseCommandLoading === execution.id
                        ? "Loading release command output..."
                        : "No release command output was persisted."}
                    </p>{/each}
                </div>
              {/if}
            </div>
          {:else}<p
              class="border border-dashed border-border p-4 text-sm text-muted-foreground"
            >
              No Release has required a release command yet.
            </p>{/each}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header
          ><Card.Title>Deployments</Card.Title><Card.Description
            >Durable blue-green rollout attempts and recovery actions.</Card.Description
          ></Card.Header
        >
        <Card.Content class="space-y-2">
          {#if deploymentEventConnectionError}<p class="text-xs text-warning">
              {deploymentEventConnectionError}
            </p>{/if}
          {#each deployments as deployment}
            <div class="border border-border text-sm">
              <div class="flex items-start justify-between gap-4 p-3">
                <Button
                  type="button"
                  variant="ghost"
                  class="h-auto min-w-0 flex-1 flex-col items-stretch p-0 text-left whitespace-normal hover:bg-transparent"
                  onclick={() => toggleDeploymentEvents(deployment.id)}
                  aria-expanded={expandedDeploymentId === deployment.id}
                >
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-mono">{short(deployment.id)}</span
                    ><StatusBadge status={deployment.status} /><StatusBadge
                      status={deploymentStep(deployment)}
                    />
                  </div>
                  <p class="mt-1 text-xs text-muted-foreground">
                    {stamp(deployment.createdAt)} · Release {short(
                      deployment.releaseId,
                    )} · {expandedDeploymentId === deployment.id
                      ? "Hide timeline"
                      : "Show timeline"}
                  </p>
                  {#if deployment.error}<p
                      class="mt-2 text-xs text-destructive"
                    >
                      {deployment.error}
                    </p>{/if}
                </Button>
                {#if deployment.status === "failed"}<Button
                    size="sm"
                    variant="outline"
                    disabled={Boolean(deploymentRetrying)}
                    aria-busy={deploymentRetrying === deployment.id}
                    onclick={() => retryDeployment(deployment.id)}
                    >{#if deploymentRetrying === deployment.id}<Spinner
                      />{/if}Retry</Button
                  >{/if}
              </div>
              {#if expandedDeploymentId === deployment.id}
                <div class="border-t border-border bg-muted/20 p-3">
                  <div
                    class="max-h-80 space-y-3 overflow-auto font-mono text-[11px] leading-relaxed"
                  >
                    {#each deploymentEvents[deployment.id] ?? [] as event (event.id)}
                      <div
                        class:text-destructive={event.status === "failed"}
                        class:text-warning={event.status === "warning"}
                        class:text-success={event.status === "succeeded"}
                      >
                        <p class="select-none text-muted-foreground">
                          {stamp(event.occurredAt)} · {event.step ||
                            event.eventType} · {event.status}
                        </p>
                        <pre
                          class="whitespace-pre-wrap break-words font-mono">{event.message}</pre>
                        {#if event.error && event.error !== event.message}<pre
                            class="whitespace-pre-wrap break-words font-mono text-destructive">{event.error}</pre>{/if}
                      </div>
                    {:else}
                      <p class="text-muted-foreground">
                        Waiting for Deployment events...
                      </p>
                    {/each}
                  </div>
                </div>
              {/if}
            </div>
          {:else}
            <p class="text-sm text-muted-foreground">No Deployments yet.</p>
          {/each}
        </Card.Content>
      </Card.Root>

      <Card.Root
        ><Card.Header
          ><Card.Title>Process instances</Card.Title><Card.Description
            >Current and candidate formation grouped by target, process, and
            stable replica identity.</Card.Description
          ></Card.Header
        ><Card.Content class="space-y-4"
          >{#each environment.runtimeTargetIds as targetId, targetIndex}<div
              class="space-y-2"
            >
              <p
                class="text-xs font-semibold uppercase tracking-wide text-muted-foreground"
              >
                {environment.runtimeServers[targetIndex]}
              </p>
              {#each environment.instances.filter((instance) => instance.targetId === targetId) as instance}<div
                  class="grid items-center gap-2 border border-border p-3 text-sm sm:grid-cols-[minmax(0,1fr)_8rem_8rem_minmax(0,1fr)]"
                >
                  <div>
                    <p class="font-mono">{instance.processName}</p>
                    <p class="text-xs text-muted-foreground">
                      {instance.replicaKey}
                    </p>
                  </div>
                  <StatusBadge status={instance.processKind} /><StatusBadge
                    status={instance.state}
                  /><span class="text-muted-foreground"
                    >{instance.processKind === "web" && instance.ports?.http
                      ? `${instance.ports.host || "127.0.0.1"}:${instance.ports.http}`
                      : "No public port"}</span
                  >
                </div>{:else}<p
                  class="border border-dashed border-border p-3 text-sm text-muted-foreground"
                >
                  No Instances on this target.
                </p>{/each}
            </div>{/each}</Card.Content
        ></Card.Root
      >
    {/if}
  </div>

  <ConfirmActionDialog
    bind:open={releaseCommandRetryDialogOpen}
    title="Retry release command?"
    description={`This starts attempt ${(pendingReleaseCommandRetry?.attempt ?? 0) + 1} for the same immutable Release and Environment revision. Review prior output first. An ambiguous attempt may already have changed external state, so retry only after confirming it is safe.`}
    confirmLabel="Retry release command"
    destructive
    processing={Boolean(releaseCommandRetrying)}
    error={releaseCommandRetryError}
    onconfirm={retryReleaseCommand}
  />

  <ConfirmActionDialog
    bind:open={buildActionDialogOpen}
    title={pendingBuildAction?.action === "stop"
      ? "Stop Build?"
      : pendingBuildAction?.action === "retry"
        ? "Retry Build?"
        : "Start Build?"}
    description={pendingBuildAction?.action === "stop"
      ? "Running Pack and Docker work will receive a cancellation signal."
      : pendingBuildAction?.action === "retry"
        ? `Retry this Build using its original source revision and ${pendingBuildAction.build.registryEndpoint} registry snapshot.`
        : "Start this pending Build now."}
    confirmLabel={pendingBuildAction?.action === "stop"
      ? "Stop Build"
      : pendingBuildAction?.action === "retry"
        ? "Retry Build"
        : "Start Build"}
    destructive={pendingBuildAction?.action === "stop"}
    processing={Boolean(activeBuildAction)}
    onconfirm={confirmBuildAction}
  />

  <ConfirmActionDialog
    bind:open={archiveSecretDialogOpen}
    title="Archive secret?"
    description={`Archive ${archivingSecret?.key ?? "this secret"} from the desired Environment configuration. The change takes effect on the next deployment.`}
    confirmLabel="Archive secret"
    destructive
    processing={secretActionProcessing}
    error={secretActionError}
    onconfirm={archiveSecret}
  />

  <ConfirmActionDialog
    bind:open={apiTokenConfirmOpen}
    title={environment.apiTokenPrefix
      ? "Rotate deployment API token?"
      : "Create deployment API token?"}
    description={environment.apiTokenPrefix
      ? "The current token will stop working immediately. You will only be able to copy the replacement once."
      : "You will only be able to copy the new token once."}
    confirmLabel={environment.apiTokenPrefix ? "Rotate token" : "Create token"}
    destructive={Boolean(environment.apiTokenPrefix)}
    processing={apiTokenProcessing}
    error={apiTokenError}
    onconfirm={rotateAPIToken}
  />

  <ConfirmActionDialog
    bind:open={promotionDialogOpen}
    title="Promote to production?"
    description={`Create a new immutable production Release from the latest successful staging deployment (Release ${short(
      environment.latestSuccessfulReleaseId ?? "",
    )}) and queue its deployment to ${environment.promotionTargetName}. The staging deployment is left unchanged.`}
    confirmLabel="Promote to production"
    processing={promotionProcessing}
    error={promotionError}
    onconfirm={promoteToProduction}
  />

  <Dialog.Root bind:open={rotateDialogOpen}>
    <Dialog.Content showCloseButton={!secretActionProcessing}>
      <form class="grid gap-4" onsubmit={rotateSecret}>
        <Dialog.Header
          ><Dialog.Title>Rotate {rotatingSecret?.key ?? "secret"}</Dialog.Title
          ><Dialog.Description
            >Enter the replacement value. It remains visible while typing and
            takes effect on the next deployment.</Dialog.Description
          ></Dialog.Header
        >
        <Input
          type="text"
          bind:value={rotatedSecretValue}
          autocomplete="off"
          autofocus
          required
          disabled={secretActionProcessing}
        />
        {#if secretActionError}<p
            class="whitespace-pre-wrap border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive"
            role="alert"
          >
            {secretActionError}
          </p>{/if}
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={secretActionProcessing}
            onclick={() => (rotateDialogOpen = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={!rotatedSecretValue || secretActionProcessing}
            >{#if secretActionProcessing}<Spinner />{/if}Rotate secret</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={apiTokenDialogOpen}>
    <Dialog.Content
      ><Dialog.Header
        ><Dialog.Title>Deployment API token</Dialog.Title><Dialog.Description
          >Copy this token now. DeployCrate stores only its digest and cannot
          show it again.</Dialog.Description
        ></Dialog.Header
      >
      <pre
        class="whitespace-pre-wrap break-all border border-border bg-muted/30 p-3 font-mono text-xs">{apiToken}</pre>
      <Dialog.Footer
        ><Button
          variant="outline"
          onclick={() => navigator.clipboard.writeText(apiToken)}
          >Copy token</Button
        ><Button onclick={() => (apiTokenDialogOpen = false)}>Done</Button
        ></Dialog.Footer
      ></Dialog.Content
    >
  </Dialog.Root>
</DashboardLayout>
