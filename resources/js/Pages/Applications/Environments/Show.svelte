<script lang="ts">
  import BoxesIcon from "@lucide/svelte/icons/boxes";
  import ContainerIcon from "@lucide/svelte/icons/container";
  import DatabaseIcon from "@lucide/svelte/icons/database";
  import EyeIcon from "@lucide/svelte/icons/eye";
  import EyeOffIcon from "@lucide/svelte/icons/eye-off";
  import ServerIcon from "@lucide/svelte/icons/server";
  import { page, router } from "@inertiajs/svelte";
  import { untrack } from "svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Collapsible from "@/Components/ui/collapsible";
  import * as Dialog from "@/Components/ui/dialog";
  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import DataField from "@/Components/DataField.svelte";
  import LogEntry from "@/Components/TelemetryLogEntry.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import OpenTelemetry from "@/Components/Applications/Environments/OpenTelemetry.svelte";
  import RequestInsights from "@/Components/Applications/Environments/RequestInsights.svelte";
  import TelemetryHistory from "@/Components/Applications/Environments/TelemetryHistory.svelte";
  import UsageDonut from "@/Components/System/UsageDonut.svelte";
  import BulkEnvironmentSecretsDialog from "@/Components/BulkEnvironmentSecretsDialog.svelte";
  import { Input } from "@/Components/ui/input";
  import * as NativeSelect from "@/Components/ui/native-select";
  import { Spinner } from "@/Components/ui/spinner";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { cn } from "@/lib/utils";
  import { routes } from "@/routes";
  import {
    BuildLogStream,
    DeploymentEventStream,
    EnvironmentLogStream,
  } from "./show-streams.svelte";
  import type {
    ApplicationTelemetry,
    Build,
    Deployment,
    EnvironmentSection,
    HostUsage,
    Overview,
    ReleaseCommand,
    ReleaseCommandLog,
    RequestTelemetry,
    Secret,
    ServingContainer,
    TelemetryRange,
    TelemetryRow,
  } from "./show.types";

  let {
    auth,
    environment,
    telemetry,
    container,
    host = { cpuCores: 0, memoryBytes: 0, available: false },
    telemetryRange = "24h",
    section = "overview",
    openTelemetryAvailable = false,
    applicationTelemetry,
    requestTelemetry,
  }: {
    auth: { email: string };
    environment: Overview;
    telemetry: TelemetryRow[];
    container: ServingContainer;
    host: HostUsage;
    telemetryRange: TelemetryRange;
    section: EnvironmentSection;
    openTelemetryAvailable: boolean;
    applicationTelemetry: ApplicationTelemetry;
    requestTelemetry: RequestTelemetry;
  } = $props();
  const requestedDeploymentId =
    new URLSearchParams(untrack(() => $page.url.split("?")[1] ?? "")).get(
      "deployment",
    ) ?? "";
  const requestedReleaseId = untrack(
    () =>
      environment.deployments.find(
        (deployment) => deployment.id === requestedDeploymentId,
      )?.releaseId ?? "",
  );
  let key = $state("");
  let value = $state("");
  let bulkSecretDialogOpen = $state(false);
  let sensitiveInformationVisible = $state(false);
  let bulkSecretImporting = $state(false);
  let secretAddError = $state("");
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
  let deploymentStopDialogOpen = $state(false);
  let pendingDeploymentStop = $state<Deployment | null>(null);
  let deploymentStopping = $state("");
  let deploymentStopError = $state("");
  let expandedBuildId = $state("");
  let buildSearch = $state("");
  let buildStatus = $state("all");
  let expandedReleaseId = $state(requestedReleaseId);
  let releaseSearch = $state("");
  let releaseStatusFilter = $state("all");
  let selectedDeploymentId = $state(requestedDeploymentId);
  let autoSelectedForRelease = $state("");
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
  let releaseCommandLogs = $state<Record<string, ReleaseCommandLog[]>>({});
  let releaseCommandLoading = $state("");
  let releaseCommandRetrying = $state("");
  let releaseCommandRetryTargets = $state<Record<string, string>>({});
  let releaseCommandRetryDialogOpen = $state(false);
  let pendingReleaseCommandRetry = $state<ReleaseCommand | null>(null);
  let releaseCommandRetryError = $state("");
  // Route ids are fixed for the lifetime of this page.
  const applicationId = untrack(() => environment.applicationId);
  const environmentId = untrack(() => environment.environment.id);
  const buildStream = new BuildLogStream(
    environmentId,
    () => environment.builds,
  );
  const deploymentStream = new DeploymentEventStream(
    environmentId,
    () => environment.deployments,
  );
  const logStream = new EnvironmentLogStream(applicationId, environmentId);
  const builds = $derived(buildStream.current);
  const deployments = $derived(deploymentStream.current);
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
  const containerProcessName = $derived(
    environment.instances.find(
      (instance) => instance.id === container.instanceId,
    )?.processName ?? "",
  );
  const servingInstanceCount = $derived(
    environment.instances.filter((instance) => instance.state === "serving")
      .length,
  );
  const desiredInstanceCount = $derived(
    environment.processes
      .filter((process) => process.kind !== "release")
      .reduce((total, process) => total + process.replicas, 0),
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
  const telemetryMode = $derived(
    openTelemetryAvailable ? "opentelemetry" : "standard",
  );
  const openTelemetryView = $derived.by(() => {
    const view = new URLSearchParams($page.url.split("?")[1] ?? "").get("view");
    return view === "logs" || view === "traces" || view === "database"
      ? view
      : "insights";
  });
  const environmentMemorySeries = $derived.by(() => {
    const totals: Record<
      string,
      { observedAt: string; memoryBytes: number; memoryAvailable: true }
    > = {};

    for (const row of telemetry) {
      for (const point of row.history) {
        if (!point.memoryAvailable) continue;
        const timestamp = new Date(point.observedAt).getTime();
        if (!Number.isFinite(timestamp)) continue;

        const key = String(timestamp);
        const total = totals[key];
        if (total) {
          total.memoryBytes += point.memoryBytes;
        } else {
          totals[key] = {
            observedAt: point.observedAt,
            memoryBytes: point.memoryBytes,
            memoryAvailable: true,
          };
        }
      }
    }

    const points = Object.entries(totals)
      .sort(([left], [right]) => Number(left) - Number(right))
      .map(([, point]) => point);

    return points.length > 0
      ? [{ id: "environment", label: "Environment", points }]
      : [];
  });
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
  const activeBuildCount = $derived(
    builds.filter((build) => ["pending", "running"].includes(build.status))
      .length,
  );
  const filteredBuilds = $derived.by(() => {
    const query = buildSearch.trim().toLowerCase();
    return builds.filter((build) => {
      const matchesStatus =
        buildStatus === "all" ||
        (buildStatus === "active"
          ? ["pending", "running"].includes(build.status)
          : build.status === buildStatus);
      const matchesQuery =
        !query ||
        [
          build.id,
          build.sourceRevision,
          build.registryEndpoint,
          build.currentStep,
          build.status,
        ].some((value) => value.toLowerCase().includes(query));
      return matchesStatus && matchesQuery;
    });
  });
  const selectedBuild = $derived(
    builds.find((build) => build.id === expandedBuildId) ?? null,
  );
  const selectedBuildLogs = $derived(
    expandedBuildId ? (buildStream.logs[expandedBuildId] ?? []) : [],
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
  const formatChange = (value: number | null) =>
    value === null
      ? "Unavailable"
      : `${value > 0 ? "+" : ""}${value.toFixed(1)}%`;
  const stamp = (value: string) =>
    value ? new Date(value).toLocaleString() : "Pending";
  const telemetryHref = (
    range: string,
    source: "standard" | "opentelemetry" = telemetryMode,
  ) => {
    const query = new URLSearchParams({ range, source });
    if (source === "opentelemetry") {
      const trace = new URLSearchParams($page.url.split("?")[1] ?? "").get(
        "trace",
      );
      query.set("view", openTelemetryView);
      if (openTelemetryView === "traces" && trace) query.set("trace", trace);
    }
    return `${routes.environmentTelemetry(
      environment.applicationId,
      environment.environment.id,
    )}?${query.toString()}`;
  };
  const stepLabel = (value: string) =>
    value ? value.replaceAll("_", " ") : "waiting for worker";
  const deploymentIsActive = (deployment: Deployment) =>
    deployment.status === "queued" ||
    deployment.status === "running" ||
    deployment.status === "cancelling";
  const deploymentStep = (deployment: Deployment) =>
    deployment.active
      ? "serving"
      : deployment.status === "succeeded"
        ? "superseded"
        : deployment.currentStep || "queued";
  const releases = $derived(environment.releases);
  const deploymentsFor = (releaseId: string) =>
    deployments.filter((deployment) => deployment.releaseId === releaseId);
  const releaseStatus = (releaseId: string) => {
    const own = deploymentsFor(releaseId);
    if (own.some(deploymentIsActive)) return "running";
    if (own.some((d) => d.status === "succeeded")) return "succeeded";
    if (own.some((d) => d.status === "failed")) return "failed";
    if (own.some((d) => d.status === "cancelled")) return "cancelled";
    return "";
  };
  const activeReleaseCount = $derived(
    releases.filter((release) => releaseStatus(release.id) === "running")
      .length,
  );
  const filteredReleases = $derived.by(() => {
    const query = releaseSearch.trim().toLowerCase();
    return releases.filter((release) => {
      const status = releaseStatus(release.id);
      const matchesStatus =
        releaseStatusFilter === "all" ||
        (releaseStatusFilter === "active"
          ? status === "running"
          : status === releaseStatusFilter);
      const matchesQuery =
        !query ||
        [release.id, release.sourceRevision, release.artifactReference].some(
          (value) => value.toLowerCase().includes(query),
        );
      return matchesStatus && matchesQuery;
    });
  });
  const selectedRelease = $derived(
    releases.find((release) => release.id === expandedReleaseId) ?? null,
  );
  const releaseCommandForRelease = $derived(
    environment.releaseCommands.find(
      (execution) => execution.releaseId === expandedReleaseId,
    ) ?? null,
  );
  const selectedDeployment = $derived(
    deploymentsFor(expandedReleaseId).find(
      (deployment) => deployment.id === selectedDeploymentId,
    ) ?? null,
  );
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
    secretAddError = "";
    router.post(
      routes.environmentSecretsCreate(
        environment.applicationId,
        environment.environment.id,
      ),
      { key: key.trim().toUpperCase(), value },
      {
        headers: { "X-Deploycrate-Section": section },
        preserveScroll: true,
        onSuccess: () => {
          key = "";
          value = "";
        },
        onError: (errors) =>
          (secretAddError =
            Object.values(errors).map(String).join("\n") ||
            "The secret could not be added."),
        onFinish: () => (secretCreationProcessing = false),
      },
    );
  }

  function importBulkSecrets(secrets: { key: string; value: string }[]) {
    if (bulkSecretImporting || secrets.length === 0) return;
    bulkSecretImporting = true;
    router.post(
      routes.environmentSecretsBulkCreate(
        environment.applicationId,
        environment.environment.id,
      ),
      { secrets },
      {
        headers: { "X-Deploycrate-Section": section },
        preserveScroll: true,
        onFinish: () => (bulkSecretImporting = false),
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
        headers: { "X-Deploycrate-Section": section },
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
          buildStream.reset();
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
    expandedReleaseId = releaseId;
    router.post(
      routes.environmentReleaseDeploymentsCreate(
        environment.applicationId,
        environment.environment.id,
        releaseId,
      ),
      {},
      {
        headers: { "X-Deploycrate-Section": section },
        preserveScroll: true,
        onSuccess: () => {
          deploymentStream.reset();
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
          deploymentStream.reset();
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
        headers: { "X-Deploycrate-Section": section },
        preserveScroll: true,
        onFinish: () => (deploymentRetrying = ""),
      },
    );
  }
  function askToStopDeployment(deployment: Deployment) {
    pendingDeploymentStop = deployment;
    deploymentStopError = "";
    deploymentStopDialogOpen = true;
  }
  function stopDeployment() {
    if (!pendingDeploymentStop || deploymentStopping) return;
    const deployment = pendingDeploymentStop;
    deploymentStopping = deployment.id;
    deploymentStopError = "";
    router.post(
      routes.environmentDeploymentStop(
        environment.applicationId,
        environment.environment.id,
        deployment.id,
      ),
      {},
      {
        headers: { "X-Deploycrate-Section": section },
        preserveScroll: true,
        onSuccess: () => {
          deploymentStopDialogOpen = false;
          deploymentStream.reset();
        },
        onError: (errors) =>
          (deploymentStopError =
            Object.values(errors).map(String).join("\n") ||
            "The Deployment could not be stopped."),
        onFinish: () => (deploymentStopping = ""),
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
        headers: { "X-Deploycrate-Section": section },
        preserveScroll: true,
        onSuccess: () => (buildActionDialogOpen = false),
        onFinish: () => (activeBuildAction = ""),
      },
    );
  }
  async function selectBuild(buildId: string) {
    if (!buildId) return;
    expandedBuildId = buildId;
    if (!(buildId in buildStream.logs)) {
      try {
        let snapshot = await buildStream.load(buildId);
        while (snapshot?.hasMore) snapshot = await buildStream.load(buildId);
      } catch {
        buildStream.connectionError = "Build logs are temporarily unavailable.";
      }
    }
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
    if (section !== "telemetry" || telemetryMode !== "standard") return;
    logStream.logs.length;
    if (!followingEnvironmentLogs) return;
    const frame = window.requestAnimationFrame(() => {
      environmentLogViewport?.scrollTo({
        top: environmentLogViewport.scrollHeight,
      });
    });
    return () => window.cancelAnimationFrame(frame);
  });

  $effect(() => {
    if (
      section !== "telemetry" ||
      telemetryMode !== "standard" ||
      environmentLogsPaused
    )
      return;
    return logStream.poll();
  });

  $effect(() => {
    if (section !== "builds") return;
    const buildId = activeBuildId;
    const preferredBuildId = buildId || builds[0]?.id || "";
    if (!expandedBuildId && preferredBuildId)
      void selectBuild(preferredBuildId);
    if (!buildId) return;
    return buildStream.poll(buildId);
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
    if (section !== "overview") return;
    const runningDeployment = deployments.find(deploymentIsActive);
    if (!runningDeployment) return;
    return deploymentStream.poll(runningDeployment.id);
  });

  $effect(() => {
    if (section !== "telemetry" || !telemetryLive) return;
    if (telemetryMode === "opentelemetry" && openTelemetryView === "logs")
      return;
    let refreshing = false;
    const refresh = () => {
      if (refreshing || document.visibilityState !== "visible") return;
      refreshing = true;
      router.reload({
        only:
          telemetryMode === "opentelemetry"
            ? ["applicationTelemetry", "requestTelemetry"]
            : ["telemetry", "requestTelemetry"],
        preserveScroll: true,
        preserveState: true,
        onFinish: () => (refreshing = false),
      });
    };
    refresh();
    const timer = window.setInterval(refresh, 10000);
    return () => window.clearInterval(timer);
  });

  $effect(() => {
    if (section !== "releases") return;
    const activeReleaseCommand = environment.releaseCommands.find(
      (execution) =>
        execution.status === "queued" || execution.status === "running",
    );
    if (!activeReleaseCommand) return;
    const timer = window.setInterval(() => {
      void loadReleaseCommandLogs(activeReleaseCommand.id);
      router.reload({ only: ["environment"], preserveScroll: true });
    }, 2000);
    return () => window.clearInterval(timer);
  });

  $effect(() => {
    if (section !== "releases") return;
    const runningDeployment = deployments.find(deploymentIsActive);
    const candidateReleaseId =
      runningDeployment?.releaseId ?? releases[0]?.id ?? "";
    if (!candidateReleaseId) return;
    if (!expandedReleaseId) expandedReleaseId = candidateReleaseId;
    const deploymentId =
      runningDeployment && expandedReleaseId === runningDeployment.releaseId
        ? runningDeployment.id
        : "";
    if (!deploymentId) return;
    return deploymentStream.poll(deploymentId);
  });

  $effect(() => {
    if (section !== "releases") return;
    const releaseId = expandedReleaseId;
    const own = deploymentsFor(releaseId);
    if (!releaseId || own.length === 0) return;
    if (autoSelectedForRelease === releaseId) return;
    autoSelectedForRelease = releaseId;
    const preferred =
      own.find((deployment) => deployment.id === selectedDeploymentId) ??
      own.find(deploymentIsActive) ??
      own[0];
    selectedDeploymentId = preferred.id;
  });

  $effect(() => {
    if (section !== "releases" || !expandedReleaseId) return;
    const execution = releaseCommandForRelease;
    if (execution && !releaseCommandLogs[execution.id])
      void loadReleaseCommandLogs(execution.id);
    const deployment = selectedDeployment;
    if (!deployment) return;
    if (
      deployment.status !== "queued" &&
      deployment.status !== "running" &&
      deployment.status !== "cancelling" &&
      !(deployment.id in deploymentStream.events)
    ) {
      void (async () => {
        try {
          let snapshot = await deploymentStream.load(deployment.id);
          while (snapshot?.hasMore)
            snapshot = await deploymentStream.load(deployment.id);
        } catch {
          deploymentStream.connectionError =
            "Deployment events are temporarily unavailable.";
        }
      })();
    }
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
  fullWidth={section === "builds" || section === "releases"}
>
  <div class="space-y-8">
    {#if section === "overview"}
      <header
        class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"
      >
        <div>
          <p
            class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
          >
            {environment.applicationName} · {environment.environment.kind}
          </p>
          <div class="mt-3 flex flex-wrap items-center gap-3">
            <h1 class="text-3xl font-semibold">
              {environment.environment.name}
            </h1>
            <StatusBadge
              status={environment.deployability.deployable
                ? "ready"
                : "blocked"}
            />
          </div>
        </div>
        <div class="flex flex-wrap gap-2">
          <Button
            variant="outline"
            aria-pressed={sensitiveInformationVisible}
            onclick={() =>
              (sensitiveInformationVisible = !sensitiveInformationVisible)}
          >
            {#if sensitiveInformationVisible}<EyeOffIcon />Hide sensitive
              information{:else}<EyeIcon />Reveal sensitive information{/if}
          </Button>
          {#if environment.sourceType === "buildpacks"}<Button
              disabled={deploymentCreationProcessing || !deploymentRequestReady}
              aria-busy={deploymentCreationProcessing}
              onclick={buildAndDeploy}
              >{#if deploymentCreationProcessing}<Spinner />{/if}Build & deploy</Button
            >{/if}
        </div>
      </header>
      <Card.Root
        class="gap-0 border-primary/25 bg-primary/[0.03] py-0"
        aria-label="Environment status"
      >
        <Card.Content
          class="grid px-0 sm:grid-cols-2 xl:grid-cols-[minmax(0,1.35fr)_repeat(3,minmax(0,1fr))]"
        >
          <div class="min-w-0 p-4">
            <p
              class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
            >
              Active deployment
            </p>
            <div class="mt-2 flex flex-wrap items-center gap-2">
              <StatusBadge status={activeDeployment?.status ?? "pending"} />
              <span class="font-mono text-xs text-muted-foreground">
                {activeDeployment ? short(activeDeployment.id) : "None"}
              </span>
            </div>
            <p class="mt-2 text-xs text-muted-foreground">
              {activeDeployment
                ? deploymentStep(activeDeployment)
                : "Deploy to start serving traffic"}
            </p>
          </div>
          <div
            class="min-w-0 border-t border-border/70 p-4 sm:border-l sm:border-t-0"
          >
            <p
              class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
            >
              Workloads
            </p>
            <p class="mt-2 text-2xl font-semibold tracking-tight">
              {servingInstanceCount}<span
                class="text-base font-normal text-muted-foreground"
                >/{desiredInstanceCount} serving</span
              >
            </p>
            <p class="mt-1 text-xs text-muted-foreground">
              Desired process capacity
            </p>
          </div>
          <div
            class="min-w-0 border-t border-border/70 p-4 xl:border-l xl:border-t-0"
          >
            <p
              class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
            >
              CPU
            </p>
            <p class="mt-2 text-2xl font-semibold tracking-tight">
              {activeTelemetry?.available && activeTelemetry.cpuAvailable
                ? formatCPU(activeTelemetry.cpuCores)
                : "Unavailable"}
            </p>
            <p
              class={cn(
                "mt-1 text-xs",
                usageChange?.cpu == null
                  ? "text-muted-foreground"
                  : usageChange.cpu > 0
                    ? "text-warning"
                    : "text-success",
              )}
            >
              {formatChange(usageChange?.cpu ?? null)} vs last hour
            </p>
          </div>
          <div
            class="min-w-0 border-t border-border/70 p-4 sm:border-l xl:border-t-0"
          >
            <p
              class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
            >
              Memory
            </p>
            <p class="mt-2 text-2xl font-semibold tracking-tight">
              {activeTelemetry?.available && activeTelemetry.memoryAvailable
                ? formatBytes(activeTelemetry.memoryBytes)
                : "Unavailable"}
            </p>
            <p
              class={cn(
                "mt-1 text-xs",
                usageChange?.memory == null
                  ? "text-muted-foreground"
                  : usageChange.memory > 0
                    ? "text-warning"
                    : "text-success",
              )}
            >
              {formatChange(usageChange?.memory ?? null)} vs last hour
            </p>
          </div>
        </Card.Content>
        <div
          class="flex flex-col gap-2 border-t border-border/70 px-4 py-2.5 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
        >
          <span>
            {activeTelemetry?.observedAt
              ? `Telemetry observed ${stamp(activeTelemetry.observedAt)}`
              : "No telemetry is available for the active container yet."}
          </span>
          <Button
            size="xs"
            variant="ghost"
            href={routes.environmentTelemetry(
              environment.applicationId,
              environment.environment.id,
            )}>View telemetry</Button
          >
        </div>
      </Card.Root>

      <section aria-label="Environment workloads">
        <Card.Root class="h-full min-w-0 gap-0 py-0">
          <Card.Header class="border-b border-border py-4">
            <div class="flex items-center gap-2">
              <div
                class="grid size-8 place-items-center bg-muted text-muted-foreground"
              >
                <ServerIcon class="size-4" />
              </div>
              <div>
                <Card.Title>Workloads</Card.Title>
                <Card.Description>
                  Process configuration and live instances in one place.
                </Card.Description>
              </div>
            </div>
          </Card.Header>
          <Card.Content class="px-0">
            <div
              class="hidden grid-cols-[minmax(0,1.05fr)_6rem_minmax(0,1.15fr)_minmax(0,1.2fr)_5rem] gap-4 border-b border-border/70 px-4 py-2 text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground lg:grid"
              aria-hidden="true"
            >
              <span>Process</span><span>Capacity</span><span>Runtime</span><span
                >Instances</span
              ><span class="text-right">Controls</span>
            </div>
            {#each environment.processes as process (process.name)}
              {@const processInstances = environment.instances.filter(
                (instance) => instance.processName === process.name,
              )}
              {@const servingProcessInstances = processInstances.filter(
                (instance) => instance.state === "serving",
              )}
              <article
                class="grid gap-4 border-b border-border/70 p-4 last:border-b-0 lg:grid-cols-[minmax(0,1.05fr)_6rem_minmax(0,1.15fr)_minmax(0,1.2fr)_5rem] lg:items-center"
              >
                <div class="flex min-w-0 items-center gap-3">
                  <div
                    class="grid size-8 shrink-0 place-items-center border border-border bg-background/50 text-muted-foreground"
                  >
                    {#if process.kind === "web"}
                      <ContainerIcon class="size-4" />
                    {:else}
                      <ServerIcon class="size-4" />
                    {/if}
                  </div>
                  <div class="min-w-0">
                    <p class="truncate font-mono text-sm font-medium">
                      {process.name}
                    </p>
                    <div class="mt-1">
                      <StatusBadge status={process.kind} />
                    </div>
                  </div>
                </div>
                <div>
                  <p class="text-sm font-medium">
                    {process.kind === "release"
                      ? "Runs once"
                      : `${servingProcessInstances.length}/${process.replicas} live`}
                  </p>
                  <p class="mt-1 text-xs text-muted-foreground">
                    {process.kind === "release"
                      ? "One-off release command"
                      : `${process.replicas} desired`}
                  </p>
                </div>
                <div class="min-w-0">
                  <p class="break-all font-mono text-xs">
                    {process.command
                      ? [process.command, ...process.arguments].join(" ")
                      : "OCI image default command"}
                  </p>
                  <p class="mt-1 text-xs text-muted-foreground">
                    {process.kind === "web"
                      ? `${process.container_port ? `Port ${process.container_port}` : "Port unavailable"}${process.health_path ? ` · ${process.health_path}` : ""}`
                      : process.kind === "release"
                        ? process.timeout_seconds
                          ? `${process.timeout_seconds}s timeout`
                          : "Timeout unavailable"
                        : "Private"}
                  </p>
                </div>
                <div class="min-w-0 space-y-2">
                  {#if process.kind === "release"}
                    <span
                      class="text-xs text-muted-foreground"
                      aria-label="Not applicable">—</span
                    >
                  {:else if processInstances.length > 0}
                    {#each processInstances as instance (instance.id)}
                      <div class="min-w-0">
                        <div class="flex flex-wrap items-center gap-2">
                          <StatusBadge status={instance.state} />
                          <span class="font-mono text-xs">
                            {instance.processKind === "web" &&
                            instance.ports?.http
                              ? sensitiveInformationVisible
                                ? `${instance.ports.host || "127.0.0.1"}:${instance.ports.http}`
                                : "••••••••"
                              : "Private"}
                          </span>
                        </div>
                        <p
                          class="mt-1 truncate font-mono text-[11px] text-muted-foreground"
                        >
                          {instance.replicaKey} · {short(instance.id)}
                        </p>
                      </div>
                    {/each}
                  {:else}
                    <p class="text-xs text-muted-foreground">
                      No instances running
                    </p>
                  {/if}
                </div>
                <div class="flex lg:justify-end">
                  {#if process.kind === "web" && container.exists && process.name === containerProcessName}
                    <Button
                      class="w-fit lg:w-full"
                      size="sm"
                      variant="outline"
                      disabled={containerActionProcessing}
                      aria-busy={containerActionProcessing}
                      onclick={container.running
                        ? restartContainer
                        : startContainer}
                      >{#if containerActionProcessing}<Spinner
                        />{/if}{container.running ? "Restart" : "Start"}</Button
                    >
                  {/if}
                </div>
              </article>
            {:else}
              <p
                class="border border-dashed border-border p-4 text-sm text-muted-foreground"
              >
                No processes configured.
              </p>
            {/each}
          </Card.Content>
        </Card.Root>
      </section>

      <section
        aria-label="Environment resources and DNS"
        class="grid items-stretch gap-4 xl:grid-cols-2"
      >
        <Card.Root class="h-full gap-0 py-0">
          <Card.Header class="border-b border-border py-4">
            <Card.Action>
              <StatusBadge
                status={environment.resources.length > 0
                  ? "available"
                  : "pending"}
                label={`${environment.resources.length} connected`}
              />
            </Card.Action>
            <div class="flex items-center gap-2">
              <div
                class="grid size-8 place-items-center bg-muted text-muted-foreground"
              >
                <BoxesIcon class="size-4" />
              </div>
              <div>
                <Card.Title>Resources</Card.Title>
                <Card.Description>
                  Services available to this Environment.
                </Card.Description>
              </div>
            </div>
          </Card.Header>
          <Card.Content class="space-y-2 py-4">
            {#each environment.resources as resource (resource.id)}
              <div
                class="flex items-center gap-3 border border-border/70 bg-card/40 p-3"
              >
                <DatabaseIcon class="size-4 shrink-0 text-muted-foreground" />
                <div class="min-w-0">
                  <p class="truncate font-mono text-sm font-medium">
                    {resource.alias}
                  </p>
                  <p class="truncate text-xs text-muted-foreground">
                    {resource.name} · {resource.engine}
                  </p>
                </div>
              </div>
            {:else}
              <p
                class="border border-dashed border-border p-4 text-sm text-muted-foreground"
              >
                No resources connected.
              </p>
            {/each}
          </Card.Content>
        </Card.Root>

        <Card.Root class="h-full">
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
                {#each environment.dns.records as record (`${record.type}:${record.name}`)}<div
                    class="grid gap-1 border border-border p-3 font-mono text-sm sm:grid-cols-[auto_1fr_1fr]"
                  >
                    <span>{record.type}</span><span
                      >{sensitiveInformationVisible
                        ? record.name
                        : "••••••••"}</span
                    ><span
                      >{sensitiveInformationVisible
                        ? record.content
                        : "••••••••"}</span
                    >
                  </div>{/each}
              </div>{/if}
            {#if environment.dns.state === "conflict"}<div
                class="flex flex-wrap items-center justify-between gap-3"
              >
                <p class="text-sm text-muted-foreground">
                  Existing records are unmanaged. Confirm adoption to replace
                  them with this Environment's server addresses.
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
                  The DNS records are up to date. Re-sync to re-verify the
                  current server addresses against Cloudflare.
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
      </section>

      <Card.Root>
        <Card.Header
          ><Card.Title>Deployment configuration</Card.Title><Card.Description
            >The source and target configuration used by the next Release.</Card.Description
          ></Card.Header
        >
        <Card.Content class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4"
          ><DataField
            label="Repository"
            value={sensitiveInformationVisible
              ? environment.repository
              : "••••••••"}
          /><DataField
            label="Reference"
            value={sensitiveInformationVisible
              ? environment.reference
              : "••••••••"}
          /><DataField
            label="Build context"
            value={sensitiveInformationVisible
              ? environment.contextPath
              : "••••••••"}
          /><DataField
            label="Domain"
            value={sensitiveInformationVisible
              ? environment.domain
              : "••••••••"}
          /><DataField
            label="Runtime Server targets"
            value={sensitiveInformationVisible
              ? environment.runtimeServers.join(", ")
              : "••••••••"}
          /><DataField
            label="Registry"
            value={sensitiveInformationVisible
              ? environment.registryName
              : "••••••••"}
          /><DataField
            label="Registry endpoint"
            value={sensitiveInformationVisible
              ? environment.registryEndpoint
              : "••••••••"}
          />{#if !environment.deployability.deployable}<DataField
              label="Missing"
              value={environment.deployability.missing.join(", ")}
            />{/if}</Card.Content
        >
      </Card.Root>

      {#if environment.sourceType === "image"}
        <div class="grid gap-4 xl:grid-cols-2">
          <Card.Root
            ><Card.Header
              ><Card.Title>Deploy image version</Card.Title><Card.Description
                >Use the configured reference or override it with another tag or
                sha256 digest.</Card.Description
              ></Card.Header
            ><Card.Content class="flex flex-col gap-3 sm:flex-row"
              ><Input
                bind:value={imageReference}
                type={sensitiveInformationVisible ? "text" : "password"}
                autocomplete="off"
                placeholder="latest"
                aria-label="Image reference"
              /><Button
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
                {sensitiveInformationVisible
                  ? `POST /api/environments/${environment.environment.id}/deployments`
                  : "POST /api/environments/••••••••/deployments"}
              </p>
              <p class="text-xs text-muted-foreground">
                JSON: {`{"reference":"1.2.3"}`} · Token: {environment.apiTokenPrefix
                  ? sensitiveInformationVisible
                    ? `${environment.apiTokenPrefix}...`
                    : "••••••••"
                  : "Not configured"}
              </p>
              {#if apiTokenError}<p class="text-xs text-destructive">
                  {apiTokenError}
                </p>{/if}</Card.Content
            ></Card.Root
          >
        </div>
      {/if}
    {/if}

    {#if section === "telemetry"}
      <div
        class="flex flex-col gap-3 border-b border-border pb-4 sm:flex-row sm:items-end sm:justify-between"
      >
        <div>
          <p
            class="text-[10px] font-medium uppercase tracking-[0.18em] text-muted-foreground"
          >
            Collection mode
          </p>
          <div class="mt-2 flex flex-wrap items-center gap-2">
            <StatusBadge
              status="active"
              label={openTelemetryAvailable ? "OpenTelemetry" : "Standard"}
            />
          </div>
        </div>
        <div
          class="flex flex-wrap items-center gap-1"
          aria-label="Telemetry time range"
        >
          {#each [{ value: "1h", label: "1h" }, { value: "6h", label: "6h" }, { value: "24h", label: "24h" }, { value: "7d", label: "7d" }] as option (option.value)}
            <Button
              size="sm"
              variant={telemetryRange === option.value ? "default" : "outline"}
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

      {#if telemetryMode === "standard"}
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
          {#if activeTelemetry || environmentMemorySeries.length > 0}
            <TelemetryHistory
              series={environmentMemorySeries}
              {telemetryRange}
            />
          {:else}
            <div class="border border-border bg-card/35 px-5 py-16 text-center">
              <p class="text-sm text-muted-foreground">
                No telemetry is available for the active container yet.
              </p>
            </div>
          {/if}
          <RequestInsights
            routes={requestTelemetry.routes}
            countries={requestTelemetry.countries}
            {telemetryRange}
          />
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
                >Live process output from this Environment's containers.
                stdout/stderr identifies the process stream, not severity.
                ClickHouse retains logs for seven days.</Card.Description
              >
            </Card.Header>
            <Collapsible.Content>
              <Card.Content>
                {#if logStream.connectionError}<p
                    class="mb-3 text-xs text-warning"
                  >
                    {logStream.connectionError}
                  </p>{/if}
                <div
                  bind:this={environmentLogViewport}
                  onscroll={updateEnvironmentLogFollow}
                  class="max-h-[42rem] min-h-48 overflow-auto border border-border bg-muted/10"
                >
                  {#each logStream.logs as log (log.id)}
                    <LogEntry
                      occurredAt={log.occurredAt}
                      message={log.message}
                      status="info"
                      statusLabel={log.stream || "stdout"}
                      source={`${log.processKind || "process"} · ${log.processName || "unknown"}${log.processReplica ? ` · ${log.processReplica}` : ""}`}
                      metadata={[
                        {
                          label: "Container",
                          value: log.container || "unknown",
                        },
                        {
                          label: "Deployment",
                          value: short(log.deployment),
                          mono: true,
                        },
                        {
                          label: "Instance",
                          value: short(log.instance),
                          mono: true,
                        },
                      ]}
                    />
                  {:else}
                    <p class="text-muted-foreground">
                      {logStream.loaded
                        ? "No workload logs have been collected yet."
                        : "Loading workload logs..."}
                    </p>
                  {/each}
                </div>
              </Card.Content>
            </Collapsible.Content>
          </Card.Root>
        </Collapsible.Root>
      {:else}
        <OpenTelemetry
          applicationId={environment.applicationId}
          environmentId={environment.environment.id}
          telemetry={applicationTelemetry}
          {requestTelemetry}
          {telemetryRange}
          live={telemetryLive}
        />
      {/if}
    {/if}

    {#if section === "secrets"}
      <Card.Root
        ><Card.Header
          ><Card.Action
            ><Button
              type="button"
              variant="outline"
              onclick={() => (bulkSecretDialogOpen = true)}
              >Import secrets</Button
            ></Card.Action
          ><Card.Title>Environment secrets</Card.Title><Card.Description
            >Status compares each desired value fingerprint with the revision
            running in the serving container. Resource-managed values are
            changed only from their Resource.</Card.Description
          ></Card.Header
        ><Card.Content class="space-y-3"
          >{#each environment.variables as variable (variable.key)}<div
              class="grid gap-1 border border-border p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto]"
            >
              <p class="font-mono text-sm">{variable.key}</p>
              <p class="break-all font-mono text-sm">{variable.value}</p>
              <p class="text-xs text-muted-foreground">{variable.source}</p>
            </div>{/each}{#each environment.secrets as secret (secret.id)}<div
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
              type="text"
              bind:value
              placeholder="Secret value"
              autocomplete="off"
            /><Button
              disabled={!key.trim() || !value || secretCreationProcessing}
              aria-busy={secretCreationProcessing}
              onclick={createSecret}
              >{#if secretCreationProcessing}<Spinner />{/if}Add secret</Button
            >
          </div>
          {#if secretAddError}<p
              class="whitespace-pre-wrap border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive"
              role="alert"
            >
              {secretAddError}
            </p>{/if}</Card.Content
        ></Card.Root
      >
    {/if}

    {#if section === "builds"}
      <Card.Root class="min-w-0">
        <Card.Header class="gap-4 border-b border-border">
          <div
            class="flex flex-col justify-between gap-4 lg:flex-row lg:items-end"
          >
            <div>
              <Card.Title>Build output</Card.Title>
              <Card.Description>
                Inspect build history and follow active output in real time.
              </Card.Description>
            </div>
            <NativeSelect.Root
              class="w-full lg:w-[24rem]"
              value={expandedBuildId}
              aria-label="Select a build"
              onchange={(event) => void selectBuild(event.currentTarget.value)}
            >
              <NativeSelect.Option value="" disabled
                >Select a build</NativeSelect.Option
              >
              {#each builds as build (build.id)}
                <NativeSelect.Option value={build.id}>
                  {short(build.sourceRevision)} · {build.status} · {stamp(
                    build.createdAt,
                  )}
                </NativeSelect.Option>
              {/each}
            </NativeSelect.Root>
          </div>
          <div class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_12rem]">
            <Input
              bind:value={buildSearch}
              placeholder="Search ID, commit, step, or registry"
              aria-label="Search builds"
            />
            <NativeSelect.Root
              class="w-full"
              bind:value={buildStatus}
              aria-label="Filter builds by status"
            >
              <NativeSelect.Option value="all"
                >All builds ({builds.length})</NativeSelect.Option
              >
              <NativeSelect.Option value="active"
                >Active ({activeBuildCount})</NativeSelect.Option
              >
              <NativeSelect.Option value="succeeded"
                >Succeeded</NativeSelect.Option
              >
              <NativeSelect.Option value="failed">Failed</NativeSelect.Option>
              <NativeSelect.Option value="cancelled"
                >Cancelled</NativeSelect.Option
              >
            </NativeSelect.Root>
          </div>
        </Card.Header>

        <Card.Content class="min-h-0 p-0">
          {#if buildStream.connectionError}<p
              class="border-b border-warning/30 bg-warning/5 px-4 py-2 text-xs text-warning"
            >
              {buildStream.connectionError}
            </p>{/if}
          <div class="grid min-h-0 xl:grid-cols-[19rem_minmax(0,1fr)]">
            <aside
              class="max-h-80 overflow-auto border-b border-border xl:h-[calc(100vh-18rem)] xl:max-h-none xl:border-r xl:border-b-0"
              aria-label="Build history"
            >
              {#each filteredBuilds as build (build.id)}
                <div
                  class={cn(
                    "border-b border-border border-l-2 p-3 text-sm",
                    expandedBuildId === build.id
                      ? "border-l-primary"
                      : "border-l-transparent",
                  )}
                >
                  <div class="flex items-start gap-2">
                    <Button
                      type="button"
                      variant="ghost"
                      class="h-auto min-w-0 flex-1 flex-col items-stretch p-0 text-left whitespace-normal hover:bg-transparent"
                      onclick={() => void selectBuild(build.id)}
                      aria-current={expandedBuildId === build.id
                        ? "true"
                        : undefined}
                    >
                      <div class="flex justify-between gap-3">
                        <span class="font-mono"
                          >{short(build.sourceRevision)}</span
                        >
                        <StatusBadge status={build.status} />
                      </div>
                      <p
                        class="mt-1 font-mono text-[11px] text-muted-foreground"
                      >
                        {short(build.id)} · {stamp(build.createdAt)}
                      </p>
                      <p class="mt-1 truncate text-xs text-muted-foreground">
                        {stepLabel(build.currentStep)}
                      </p>
                    </Button>
                    <div class="flex shrink-0 flex-col gap-1">
                      {#if build.status === "pending" || (build.status === "running" && ["scheduled", "retryable", "pending"].includes(build.jobState))}<Button
                          size="xs"
                          disabled={Boolean(activeBuildAction)}
                          onclick={() => askForBuildAction("start", build)}
                          >{#if activeBuildAction === `start:${build.id}`}<Spinner
                            />{/if}Run</Button
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
                </div>
              {:else}
                <p class="p-6 text-center text-sm text-muted-foreground">
                  No builds match these filters.
                </p>
              {/each}
            </aside>

            <section
              class="min-w-0 p-3 lg:p-4"
              aria-label="Selected build logs"
            >
              {#if selectedBuild}
                <div
                  class="mb-3 flex flex-wrap items-center justify-between gap-2"
                >
                  <div class="min-w-0">
                    <p class="truncate font-mono text-xs">
                      Commit {short(selectedBuild.sourceRevision)}
                    </p>
                    <p class="truncate text-[11px] text-muted-foreground">
                      {selectedBuild.registryEndpoint || "Registry unavailable"}
                      {selectedBuild.jobId
                        ? ` · Job #${selectedBuild.jobId}`
                        : ""}
                    </p>
                  </div>
                  <div class="flex items-center gap-2">
                    {#if selectedBuild.id === activeBuildId}<span
                        class="text-xs text-primary"
                        aria-label="Receiving live output">● Live</span
                      >{/if}
                    <StatusBadge status={selectedBuild.status} />
                  </div>
                </div>
                <div
                  class="h-[calc(100vh-22rem)] min-h-[32rem] space-y-2 overflow-auto border border-border bg-black/30 p-3 font-mono text-[11px] leading-relaxed"
                >
                  {#each selectedBuildLogs as log (log.id)}
                    <div
                      class={cn({ "text-primary": log.stream === "system" })}
                    >
                      <span class="select-none text-muted-foreground">
                        {stamp(log.occurredAt)} · {log.stream}
                      </span>
                      <pre
                        class="whitespace-pre-wrap break-words font-mono">{log.message}</pre>
                    </div>
                  {:else}
                    {#if selectedBuild.error}<pre
                        class="whitespace-pre-wrap break-words text-destructive">{selectedBuild.error}</pre>
                    {:else}<p class="text-muted-foreground">
                        Waiting for build output...
                      </p>{/if}
                  {/each}
                </div>
              {:else}
                <div
                  class="flex h-[calc(100vh-22rem)] min-h-[32rem] items-center justify-center border border-dashed border-border p-6 text-sm text-muted-foreground"
                >
                  Select a build to view its logs.
                </div>
              {/if}
            </section>
          </div>
        </Card.Content>
      </Card.Root>
    {/if}

    {#if section === "releases"}
      <Card.Root class="min-w-0">
        <Card.Header class="gap-4 border-b border-border">
          <div
            class="flex flex-col justify-between gap-4 lg:flex-row lg:items-end"
          >
            <div>
              <Card.Title>Release activity</Card.Title>
              <Card.Description>
                Inspect immutable artifacts, deployments, and release commands.
              </Card.Description>
            </div>
            <NativeSelect.Root
              class="w-full lg:w-[24rem]"
              value={expandedReleaseId}
              aria-label="Select a release"
              onchange={(event) =>
                (expandedReleaseId = event.currentTarget.value)}
            >
              <NativeSelect.Option value="" disabled
                >Select a release</NativeSelect.Option
              >
              {#each releases as release (release.id)}
                <NativeSelect.Option value={release.id}>
                  {short(release.sourceRevision)} · {releaseStatus(
                    release.id,
                  ) || "not deployed"} · {stamp(release.createdAt)}
                </NativeSelect.Option>
              {/each}
            </NativeSelect.Root>
          </div>
          <div class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_12rem]">
            <Input
              bind:value={releaseSearch}
              placeholder="Search ID, commit, or artifact"
              aria-label="Search releases"
            />
            <NativeSelect.Root
              class="w-full"
              bind:value={releaseStatusFilter}
              aria-label="Filter releases by status"
            >
              <NativeSelect.Option value="all"
                >All releases ({releases.length})</NativeSelect.Option
              >
              <NativeSelect.Option value="active"
                >Active ({activeReleaseCount})</NativeSelect.Option
              >
              <NativeSelect.Option value="succeeded"
                >Succeeded</NativeSelect.Option
              >
              <NativeSelect.Option value="failed">Failed</NativeSelect.Option>
              <NativeSelect.Option value="cancelled"
                >Cancelled</NativeSelect.Option
              >
            </NativeSelect.Root>
          </div>
        </Card.Header>

        <Card.Content class="min-h-0 p-0">
          {#if deploymentStream.connectionError}<p
              class="border-b border-warning/30 bg-warning/5 px-4 py-2 text-xs text-warning"
            >
              {deploymentStream.connectionError}
            </p>{/if}
          <div class="grid min-h-0 xl:grid-cols-[19rem_minmax(0,1fr)]">
            <aside
              class="max-h-80 overflow-auto border-b border-border xl:h-[calc(100vh-18rem)] xl:max-h-none xl:border-r xl:border-b-0"
              aria-label="Release history"
            >
              {#each filteredReleases as release (release.id)}
                <div
                  class={cn(
                    "border-b border-border border-l-2 text-sm",
                    expandedReleaseId === release.id
                      ? "border-l-primary"
                      : "border-l-transparent",
                  )}
                >
                  <Button
                    type="button"
                    variant="ghost"
                    class="h-auto min-w-0 w-full flex-col items-stretch p-3 text-left whitespace-normal hover:bg-transparent"
                    onclick={() => (expandedReleaseId = release.id)}
                    aria-current={expandedReleaseId === release.id
                      ? "true"
                      : undefined}
                  >
                    <div class="flex justify-between gap-3">
                      <span class="font-mono">{short(release.id)}</span>
                      {#if releaseStatus(release.id)}<StatusBadge
                          status={releaseStatus(release.id)}
                        />{/if}
                    </div>
                    <p
                      class="mt-1 break-all font-mono text-xs text-muted-foreground"
                    >
                      {short(release.sourceRevision)} · {stamp(
                        release.createdAt,
                      )}
                    </p>
                    <p
                      class="mt-1 break-all font-mono text-[11px] text-muted-foreground"
                    >
                      {release.artifactReference}
                    </p>
                  </Button>
                  <div
                    class="flex items-center justify-between gap-3 px-3 pb-3"
                  >
                    <span class="text-xs text-muted-foreground"
                      >{deploymentsFor(release.id).length} deployment
                      {deploymentsFor(release.id).length === 1 ? "" : "s"}</span
                    >
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={Boolean(activeReleaseDeployment)}
                      onclick={() => redeployRelease(release.id)}
                      >{#if activeReleaseDeployment === release.id}<Spinner
                        />{/if}Deploy now</Button
                    >
                  </div>
                </div>
              {:else}<p class="p-6 text-center text-sm text-muted-foreground">
                  No releases match these filters.
                </p>{/each}
            </aside>

            <section
              class="min-w-0 overflow-auto p-3 xl:h-[calc(100vh-18rem)] lg:p-4"
              aria-label="Selected release activity"
            >
              {#if selectedRelease}
                <div
                  class="mb-3 flex flex-wrap items-center justify-between gap-2"
                >
                  <div class="min-w-0">
                    <p class="truncate font-mono text-xs">
                      Commit {short(selectedRelease.sourceRevision)}
                    </p>
                    <p class="truncate text-[11px] text-muted-foreground">
                      {selectedRelease.artifactReference}
                    </p>
                  </div>
                  <div class="flex items-center gap-2">
                    {#if releaseStatus(selectedRelease.id) === "running"}<span
                        class="text-xs text-primary"
                        aria-label="Deployment activity is live">● Live</span
                      >{/if}
                    {#if releaseStatus(selectedRelease.id)}<StatusBadge
                        status={releaseStatus(selectedRelease.id)}
                      />{/if}
                  </div>
                </div>
                {#if deploymentsFor(expandedReleaseId).length > 0 || releaseCommandForRelease}
                  <div class="space-y-4 text-sm">
                    {#if releaseCommandForRelease}
                      {@const execution = releaseCommandForRelease}
                      <section class="overflow-hidden border border-border">
                        <div class="border-b border-border px-4 py-3">
                          <p
                            class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground"
                          >
                            Release command
                          </p>
                        </div>
                        <div
                          class="grid gap-3 px-4 py-3 lg:grid-cols-[minmax(0,1fr)_auto]"
                        >
                          <div class="min-w-0">
                            <div class="flex flex-wrap items-center gap-2">
                              <span class="font-mono"
                                >{short(execution.id)}</span
                              ><StatusBadge status={execution.status} /><span
                                class="text-xs text-muted-foreground"
                                >Attempt {execution.attempt} · {execution.targetName}</span
                              >
                            </div>
                            <p
                              class="mt-2 break-all font-mono text-xs text-muted-foreground"
                            >
                              {[execution.command, ...execution.arguments].join(
                                " ",
                              )}
                            </p>
                            {#if execution.error && (releaseCommandLogs[execution.id] ?? []).length === 0}<p
                                class="mt-2 text-xs text-destructive"
                              >
                                {execution.error}
                              </p>{/if}
                          </div>
                          {#if execution.status === "failed" || execution.status === "ambiguous"}
                            <div class="flex flex-wrap items-center gap-2">
                              <NativeSelect.Root
                                value={releaseCommandRetryTarget(execution)}
                                onchange={(event) =>
                                  (releaseCommandRetryTargets = {
                                    ...releaseCommandRetryTargets,
                                    [execution.id]: event.currentTarget.value,
                                  })}
                                aria-label="Release command retry target"
                              >
                                {#each environment.runtimeTargetIds as targetId, index (targetId)}<NativeSelect.Option
                                    value={targetId}
                                    >{environment.runtimeServers[
                                      index
                                    ]}</NativeSelect.Option
                                  >{/each}
                              </NativeSelect.Root>
                              <Button
                                size="sm"
                                variant="outline"
                                disabled={Boolean(releaseCommandRetrying) ||
                                  environment.runtimeTargetIds.length === 0}
                                onclick={() =>
                                  askToRetryReleaseCommand(execution)}
                                >Review retry</Button
                              >
                            </div>
                          {/if}
                        </div>
                        <div
                          class="max-h-96 min-h-32 space-y-2 overflow-auto border-t border-border bg-black/30 p-4 font-mono text-[11px] leading-relaxed"
                        >
                          {#each releaseCommandLogs[execution.id] ?? [] as log (log.id)}<div
                              class={cn({
                                "text-primary": log.stream === "system",
                              })}
                            >
                              <span class="text-muted-foreground"
                                >{stamp(log.occurredAt)} · release command · attempt
                                {log.attempt} ·
                                {log.stream}</span
                              >
                              <pre
                                class="whitespace-pre-wrap break-words font-mono">{log.message}</pre>
                            </div>{:else}<p class="text-muted-foreground">
                              {releaseCommandLoading === execution.id
                                ? "Loading release command output..."
                                : "No release command output was persisted."}
                            </p>{/each}
                        </div>
                      </section>
                    {/if}
                    {#if deploymentsFor(expandedReleaseId).length > 0}
                      <section class="overflow-hidden border border-border">
                        <div
                          class="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3"
                        >
                          <label
                            for="deployment-select"
                            class="text-xs font-medium text-muted-foreground"
                            >Deployment</label
                          >
                          <NativeSelect.Root
                            id="deployment-select"
                            class="min-w-0 flex-1"
                            bind:value={selectedDeploymentId}
                          >
                            {#each deploymentsFor(expandedReleaseId) as deployment (deployment.id)}
                              <NativeSelect.Option value={deployment.id}>
                                {short(deployment.id)} · {deployment.status}{deployment.attempt >
                                1
                                  ? ` · attempt ${deployment.attempt}`
                                  : ""} · {stamp(deployment.createdAt)}
                              </NativeSelect.Option>
                            {/each}
                          </NativeSelect.Root>
                        </div>
                        {#if selectedDeployment}
                          {@const deployment = selectedDeployment}
                          <div
                            class="grid gap-3 px-4 py-3 lg:grid-cols-[minmax(0,1fr)_auto]"
                          >
                            <div class="min-w-0">
                              <div class="flex flex-wrap items-center gap-2">
                                <span class="font-mono"
                                  >{short(deployment.id)}</span
                                ><StatusBadge
                                  status={deployment.status}
                                />{#if deploymentStep(deployment) !== deployment.status}<StatusBadge
                                    status={deploymentStep(deployment)}
                                  />{/if}
                              </div>
                              <p class="mt-1 text-xs text-muted-foreground">
                                {stamp(deployment.createdAt)} · {deployment.targetName}
                                {deployment.attempt > 1
                                  ? ` · attempt ${deployment.attempt}`
                                  : ""}
                              </p>
                              {#if deployment.error && (deploymentStream.events[deployment.id] ?? []).length === 0}<p
                                  class="mt-2 text-xs text-destructive"
                                >
                                  {deployment.error}
                                </p>{/if}
                            </div>
                            <div class="flex flex-wrap gap-2">
                              {#if deploymentIsActive(deployment)}<Button
                                  size="sm"
                                  variant="destructive"
                                  disabled={Boolean(deploymentStopping)}
                                  aria-busy={deploymentStopping ===
                                    deployment.id}
                                  onclick={() =>
                                    askToStopDeployment(deployment)}
                                  >{deployment.status === "cancelling"
                                    ? "Finish rollback"
                                    : "Stop & roll back"}</Button
                                >{/if}
                              {#if deployment.status === "failed" || deployment.status === "cancelled"}<Button
                                  size="sm"
                                  variant="outline"
                                  disabled={Boolean(deploymentRetrying)}
                                  aria-busy={deploymentRetrying ===
                                    deployment.id}
                                  onclick={() => retryDeployment(deployment.id)}
                                  >{#if deploymentRetrying === deployment.id}<Spinner
                                    />{/if}Retry</Button
                                >{/if}
                            </div>
                          </div>
                          <div class="border-t border-border bg-black/30 p-4">
                            <div
                              class="max-h-96 min-h-32 space-y-3 overflow-auto font-mono text-[11px] leading-relaxed"
                            >
                              {#each deploymentStream.events[deployment.id] ?? [] as event (event.id)}
                                <div
                                  class={cn({
                                    "text-destructive":
                                      event.status === "failed",
                                    "text-warning": event.status === "warning",
                                    "text-success":
                                      event.status === "succeeded",
                                  })}
                                >
                                  <p class="select-none text-muted-foreground">
                                    {stamp(event.occurredAt)} · deployment · {event.step ||
                                      event.eventType} · {event.status}
                                  </p>
                                  <pre
                                    class="whitespace-pre-wrap break-words font-mono">{event.message}</pre>
                                  {#if event.error && event.error !== event.message}<pre
                                      class="whitespace-pre-wrap break-words font-mono text-destructive">{event.error}</pre>{/if}
                                </div>
                              {:else}
                                <p class="text-muted-foreground">
                                  {deployment.status === "queued" ||
                                  deployment.status === "running" ||
                                  deployment.status === "cancelling"
                                    ? "Waiting for Deployment events..."
                                    : "No deployment events recorded yet."}
                                </p>
                              {/each}
                            </div>
                          </div>
                        {/if}
                      </section>
                    {/if}
                  </div>
                {:else}
                  <p
                    class="border border-dashed border-border p-4 text-sm text-muted-foreground"
                  >
                    No deployments or release commands for this release yet.
                  </p>
                {/if}
              {:else}
                <div
                  class="flex h-full min-h-[32rem] items-center justify-center border border-dashed border-border p-6 text-sm text-muted-foreground"
                >
                  Select a release to view its deployments.
                </div>
              {/if}
            </section>
          </div>
        </Card.Content>
      </Card.Root>
    {/if}
  </div>

  <ConfirmActionDialog
    bind:open={deploymentStopDialogOpen}
    title={pendingDeploymentStop?.status === "cancelling"
      ? "Finish Deployment rollback?"
      : "Stop and roll back Deployment?"}
    description="The candidate route and containers will be removed. If traffic already switched, DeployCrate will restore the previous serving release before removing the candidate."
    confirmLabel={pendingDeploymentStop?.status === "cancelling"
      ? "Finish rollback"
      : "Stop & roll back"}
    destructive
    processing={Boolean(deploymentStopping)}
    error={deploymentStopError}
    onconfirm={stopDeployment}
  />

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

  <BulkEnvironmentSecretsDialog
    bind:open={bulkSecretDialogOpen}
    existingSecrets={environment.secrets.map((secret) => ({
      key: secret.key,
      value: "",
    }))}
    reservedKeys={["PORT"]}
    onImport={importBulkSecrets}
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
            >Enter the replacement value. It takes effect on the next
            deployment.</Dialog.Description
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
