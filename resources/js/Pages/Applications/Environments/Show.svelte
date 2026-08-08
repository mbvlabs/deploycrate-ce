<script lang="ts">
  import ActivityIcon from "@lucide/svelte/icons/activity";
  import BoxesIcon from "@lucide/svelte/icons/boxes";
  import ContainerIcon from "@lucide/svelte/icons/container";
  import DatabaseIcon from "@lucide/svelte/icons/database";
  import ServerIcon from "@lucide/svelte/icons/server";
  import { page, router } from "@inertiajs/svelte";
  import { untrack } from "svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Collapsible from "@/Components/ui/collapsible";
  import * as Dialog from "@/Components/ui/dialog";
  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import DataField from "@/Components/DataField.svelte";
  import EnvironmentDeleteDialog from "@/Components/EnvironmentDeleteDialog.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import OpenTelemetry from "@/Components/Applications/Environments/OpenTelemetry.svelte";
  import TelemetryHistory from "@/Components/Applications/Environments/TelemetryHistory.svelte";
  import UsageSummary from "@/Components/Applications/Environments/UsageSummary.svelte";
  import UsageDonut from "@/Components/System/UsageDonut.svelte";
  import BulkEnvironmentSecretsDialog from "@/Components/BulkEnvironmentSecretsDialog.svelte";
  import { Input } from "@/Components/ui/input";
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
  } = $props();
  let key = $state("");
  let value = $state("");
  let bulkSecretDialogOpen = $state(false);
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
  let expandedBuildId = $state("");
  let expandedReleaseId = $state("");
  let selectedDeploymentId = $state("");
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
  const builds = $derived(buildStream.live ?? environment.builds);
  const deployments = $derived(
    deploymentStream.live ?? environment.deployments,
  );
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
    openTelemetryAvailable &&
      new URLSearchParams($page.url.split("?")[1] ?? "").get("source") ===
        "opentelemetry"
      ? "opentelemetry"
      : "standard",
  );
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
  const telemetryHref = (
    range: string,
    source: "standard" | "opentelemetry" = telemetryMode,
  ) => {
    const query = new URLSearchParams({ range, source });
    if (source === "opentelemetry") {
      const view = new URLSearchParams($page.url.split("?")[1] ?? "").get(
        "view",
      );
      const trace = new URLSearchParams($page.url.split("?")[1] ?? "").get(
        "trace",
      );
      query.set(
        "view",
        view === "logs" || view === "traces" || view === "database"
          ? view
          : "insights",
      );
      if (view === "traces" && trace) query.set("trace", trace);
    }
    return `${routes.environmentTelemetry(
      environment.applicationId,
      environment.environment.id,
    )}?${query.toString()}`;
  };
  const stepLabel = (value: string) =>
    value ? value.replaceAll("_", " ") : "waiting for worker";
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
    if (own.some((d) => d.status === "queued" || d.status === "running"))
      return "running";
    if (own.some((d) => d.status === "succeeded")) return "succeeded";
    if (own.some((d) => d.status === "failed")) return "failed";
    return "";
  };
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
  async function toggleBuildLogs(buildId: string) {
    if (expandedBuildId === buildId) {
      expandedBuildId = "";
      return;
    }
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
    if (!buildId) return;
    if (!expandedBuildId) expandedBuildId = buildId;
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
    const runningDeployment = deployments.find(
      (deployment) =>
        deployment.status === "queued" || deployment.status === "running",
    );
    if (!runningDeployment) return;
    return deploymentStream.poll(runningDeployment.id);
  });

  $effect(() => {
    if (section !== "telemetry" || !telemetryLive) return;
    let refreshing = false;
    const refresh = () => {
      if (refreshing || document.visibilityState !== "visible") return;
      refreshing = true;
      router.reload({
        only:
          telemetryMode === "opentelemetry"
            ? ["applicationTelemetry"]
            : ["telemetry"],
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
    const runningDeployment = deployments.find(
      (deployment) =>
        deployment.status === "queued" || deployment.status === "running",
    );
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
      own.find(
        (deployment) =>
          deployment.status === "queued" || deployment.status === "running",
      ) ?? own[0];
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
      <section
        aria-label="Environment dashboard"
        class="grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(20rem,0.9fr)]"
      >
        <Card.Root class="min-w-0 border-primary/25 bg-primary/[0.03]">
          <Card.Header class="border-b border-border/70">
            <Card.Action>
              <StatusBadge
                status={environment.deployability.deployable
                  ? "ready"
                  : "blocked"}
              />
            </Card.Action>
            <div class="flex items-center gap-2">
              <div
                class="grid size-8 place-items-center bg-primary/10 text-primary"
              >
                <ActivityIcon class="size-4" />
              </div>
              <div>
                <Card.Title>Runtime overview</Card.Title>
                <Card.Description>
                  Current deployment posture for this Environment.
                </Card.Description>
              </div>
            </div>
          </Card.Header>
          <Card.Content class="grid gap-3 sm:grid-cols-3">
            <div class="border border-border/70 bg-background/55 p-3">
              <p
                class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
              >
                Deployment
              </p>
              <div class="mt-2 flex items-center gap-2">
                <StatusBadge status={activeDeployment?.status ?? "pending"} />
                <span class="font-mono text-xs text-muted-foreground"
                  >{activeDeployment
                    ? short(activeDeployment.id)
                    : "None"}</span
                >
              </div>
              <p class="mt-2 text-xs text-muted-foreground">
                {activeDeployment
                  ? deploymentStep(activeDeployment)
                  : "Deploy to start serving traffic"}
              </p>
            </div>
            <div class="border border-border/70 bg-background/55 p-3">
              <p
                class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
              >
                Process health
              </p>
              <p class="mt-2 text-2xl font-semibold tracking-tight">
                {servingInstanceCount}<span
                  class="text-base font-normal text-muted-foreground"
                  >/{desiredInstanceCount}</span
                >
              </p>
              <p class="mt-1 text-xs text-muted-foreground">
                serving instances
              </p>
            </div>
            <div class="border border-border/70 bg-background/55 p-3">
              <p
                class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
              >
                Resources
              </p>
              <p class="mt-2 text-2xl font-semibold tracking-tight">
                {environment.resources.length}
              </p>
              <p class="mt-1 text-xs text-muted-foreground">
                connected services
              </p>
            </div>
          </Card.Content>
        </Card.Root>

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
      </section>

      <section
        class="grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(20rem,0.9fr)]"
      >
        <Card.Root class="min-w-0">
          <Card.Header class="border-b border-border">
            <div class="flex items-center gap-2">
              <div
                class="grid size-8 place-items-center bg-muted text-muted-foreground"
              >
                <ServerIcon class="size-4" />
              </div>
              <div>
                <Card.Title>Process formation</Card.Title>
                <Card.Description
                  >Configuration and live capacity for the next rollout.</Card.Description
                >
              </div>
            </div>
          </Card.Header>
          <Card.Content class="space-y-2">
            {#each environment.processes as process (process.name)}
              <article
                class="grid gap-3 border border-border/70 bg-card/40 p-3 sm:grid-cols-[minmax(0,1fr)_7rem_minmax(0,1.5fr)_auto] sm:items-center"
              >
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <p class="truncate font-mono font-medium">{process.name}</p>
                    <StatusBadge status={process.kind} />
                  </div>
                  <p class="mt-1 text-xs text-muted-foreground">
                    {process.kind === "release"
                      ? "One-off release command"
                      : `${process.replicas} desired replica${process.replicas === 1 ? "" : "s"}`}
                  </p>
                </div>
                <p class="text-sm font-medium">
                  {process.kind === "release"
                    ? "Runs once"
                    : `${environment.instances.filter((instance) => instance.processName === process.name && instance.state === "serving").length}/${process.replicas} live`}
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

        <div class="space-y-4">
          <Card.Root>
            <Card.Header class="border-b border-border">
              <div class="flex items-center gap-2">
                <div
                  class="grid size-8 place-items-center bg-muted text-muted-foreground"
                >
                  <BoxesIcon class="size-4" />
                </div>
                <div>
                  <Card.Title>Resources</Card.Title><Card.Description
                    >Services available to this Environment.</Card.Description
                  >
                </div>
              </div>
            </Card.Header>
            <Card.Content class="space-y-2">
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

          <Card.Root>
            <Card.Header>
              <Card.Action>
                {#if container.exists}
                  <Button
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
              </Card.Action>
              <div class="flex items-center gap-2">
                <div
                  class="grid size-8 place-items-center bg-muted text-muted-foreground"
                >
                  <ContainerIcon class="size-4" />
                </div>
                <div>
                  <Card.Title>Web container</Card.Title><Card.Description
                    >Serving container state and controls.</Card.Description
                  >
                </div>
              </div>
            </Card.Header>
            <Card.Content>
              {#if container.exists}
                <div class="flex flex-wrap items-center gap-2">
                  <StatusBadge
                    status={container.running ? "running" : "stopped"}
                    label={container.running ? "Running" : "Stopped"}
                  /><span class="font-mono text-xs text-muted-foreground"
                    >Instance {short(container.instanceId)} · Deployment {short(
                      container.deploymentId,
                    )}</span
                  >
                </div>
              {:else}
                <p class="text-sm text-muted-foreground">
                  No serving container is currently deployed.
                </p>
              {/if}
            </Card.Content>
          </Card.Root>
        </div>
      </section>

      <Card.Root>
        <Card.Header>
          <Card.Action>
            <StatusBadge
              status={servingInstanceCount > 0 ? "serving" : "pending"}
              label={servingInstanceCount > 0
                ? `${servingInstanceCount} serving`
                : "Idle"}
            />
          </Card.Action>
          <div class="flex items-center gap-2">
            <div
              class="grid size-8 place-items-center bg-muted text-muted-foreground"
            >
              <ServerIcon class="size-4" />
            </div>
            <div>
              <Card.Title>Process instances</Card.Title>
              <Card.Description
                >Live workload instances across all process kinds.</Card.Description
              >
            </div>
          </div>
        </Card.Header>
        <Card.Content class="space-y-2">
          {#each environment.instances as instance}
            <div
              class="grid items-center gap-2 border border-border/70 bg-card/40 p-3 text-sm sm:grid-cols-[minmax(0,1fr)_8rem_8rem_minmax(0,1fr)]"
            >
              <div>
                <p class="font-mono font-medium">{instance.processName}</p>
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
            </div>
          {:else}
            <p class="border border-dashed border-border p-4 text-sm">
              No instances running.
            </p>
          {/each}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header
          ><Card.Action
            ><StatusBadge
              status={environment.deployability.deployable
                ? "ready"
                : "blocked"}
            /></Card.Action
          ><Card.Title>Deployment configuration</Card.Title><Card.Description
            >The source and target configuration used by the next Release.</Card.Description
          ></Card.Header
        >
        <Card.Content class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4"
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
        >
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
              {#each environment.dns.records as record (`${record.type}:${record.name}`)}<div
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
        <div class="grid gap-4 xl:grid-cols-2">
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
        </div>
      {/if}
    {/if}

    {#if section === "telemetry"}
      <div
        class="flex flex-col gap-3 border-b border-border pb-4 sm:flex-row sm:items-end sm:justify-between"
      >
        <div>
          <p
            class="mb-2 text-[10px] font-medium uppercase tracking-[0.18em] text-muted-foreground"
          >
            Telemetry source
          </p>
          <div class="flex flex-wrap gap-1" aria-label="Telemetry source">
            <Button
              size="sm"
              variant={telemetryMode === "standard" ? "default" : "outline"}
              aria-pressed={telemetryMode === "standard"}
              href={telemetryHref(telemetryRange, "standard")}>Standard</Button
            >
            {#if openTelemetryAvailable}
              <Button
                size="sm"
                variant={telemetryMode === "opentelemetry"
                  ? "default"
                  : "outline"}
                aria-pressed={telemetryMode === "opentelemetry"}
                href={telemetryHref(telemetryRange, "opentelemetry")}
                >OpenTelemetry</Button
              >
            {/if}
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
                {#if logStream.connectionError}<p
                    class="mb-3 text-xs text-warning"
                  >
                    {logStream.connectionError}
                  </p>{/if}
                <div
                  bind:this={environmentLogViewport}
                  onscroll={updateEnvironmentLogFollow}
                  class="max-h-[32rem] min-h-48 overflow-auto border border-border bg-black/35 p-3 font-mono text-[11px] leading-relaxed"
                >
                  {#each logStream.logs as log (log.id)}
                    <div
                      class={cn(
                        "grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 py-1",
                        { "text-destructive": log.stream === "stderr" },
                      )}
                    >
                      <span
                        class="select-none whitespace-nowrap text-muted-foreground"
                        >{stamp(log.occurredAt)}</span
                      >
                      <div class="min-w-0">
                        <p
                          class="select-none text-[10px] text-muted-foreground"
                        >
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
      <div class="grid gap-8 xl:grid-cols-5">
        <Card.Root class="min-w-0 xl:col-span-2">
          <Card.Header
            ><Card.Title>Builds</Card.Title><Card.Description
              >Select a build to view its output.</Card.Description
            ></Card.Header
          >
          <Card.Content class="space-y-2">
            {#if buildStream.connectionError}<p class="text-xs text-warning">
                {buildStream.connectionError}
              </p>{/if}
            {#each builds as build}
              <div
                class={cn(
                  "border text-sm",
                  expandedBuildId === build.id
                    ? "border-primary/40 bg-primary/[0.04]"
                    : "border-border",
                )}
              >
                <div class="flex items-start gap-2 p-3">
                  <Button
                    type="button"
                    variant="ghost"
                    class="h-auto min-w-0 flex-1 flex-col items-stretch p-0 text-left whitespace-normal hover:bg-transparent"
                    onclick={() => toggleBuildLogs(build.id)}
                    aria-expanded={expandedBuildId === build.id}
                  >
                    <div class="flex justify-between gap-3">
                      <span class="font-mono">{short(build.id)}</span
                      ><StatusBadge status={build.status} />
                    </div>
                    <p
                      class="mt-1 break-all font-mono text-xs text-muted-foreground"
                    >
                      {short(build.sourceRevision)} · {stamp(build.createdAt)}
                    </p>
                    <p class="mt-1 text-xs text-muted-foreground">
                      {stepLabel(build.currentStep)}
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
              </div>
            {:else}<p class="text-sm text-muted-foreground">
                No Builds yet.
              </p>{/each}
          </Card.Content>
        </Card.Root>

        <Card.Root class="min-w-0 xl:col-span-3">
          <Card.Header
            ><Card.Title>Build logs</Card.Title><Card.Description
              >Live output for the selected build.</Card.Description
            ></Card.Header
          >
          <Card.Content class="min-h-0">
            {#if expandedBuildId}
              {@const selectedBuild = builds.find(
                (item) => item.id === expandedBuildId,
              )}
              <div
                class="max-h-[32rem] space-y-2 overflow-auto border border-border bg-black/30 p-3 font-mono text-[11px] leading-relaxed"
              >
                {#each buildStream.logs[expandedBuildId] ?? [] as log (log.id)}
                  <div
                    class={cn({
                      "text-primary": log.stream === "system",
                    })}
                  >
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
              {#if selectedBuild?.error}<pre
                  class="mt-3 whitespace-pre-wrap break-words border-t border-destructive/30 pt-3 text-xs text-destructive">{selectedBuild.error}</pre>{/if}
            {:else}
              <div
                class="flex min-h-64 items-center justify-center border border-dashed border-border p-6 text-sm text-muted-foreground"
              >
                Select a build to view its logs.
              </div>
            {/if}
          </Card.Content>
        </Card.Root>
      </div>
    {/if}

    {#if section === "releases"}
      <div class="grid gap-8 xl:grid-cols-5">
        <Card.Root class="min-w-0 xl:col-span-2">
          <Card.Header
            ><Card.Title>Releases</Card.Title><Card.Description
              >Immutable workload artifacts. Select a release to inspect its
              deployments.</Card.Description
            ></Card.Header
          >
          <Card.Content class="space-y-2">
            {#if deploymentStream.connectionError}<p
                class="text-xs text-warning"
              >
                {deploymentStream.connectionError}
              </p>{/if}
            {#each releases as release}
              <div
                class={cn(
                  "border text-sm",
                  expandedReleaseId === release.id
                    ? "border-primary/40 bg-primary/[0.04]"
                    : "border-border",
                )}
              >
                <Button
                  type="button"
                  variant="ghost"
                  class="h-auto min-w-0 w-full flex-col items-stretch p-3 text-left whitespace-normal hover:bg-transparent"
                  onclick={() => (expandedReleaseId = release.id)}
                  aria-expanded={expandedReleaseId === release.id}
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
                    {short(release.sourceRevision)} · {stamp(release.createdAt)}
                  </p>
                  <p
                    class="mt-1 break-all font-mono text-[11px] text-muted-foreground"
                  >
                    {release.artifactReference}
                  </p>
                </Button>
                <div class="flex items-center justify-between gap-3 px-3 pb-3">
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
            {:else}<p class="text-sm text-muted-foreground">
                No Releases yet.
              </p>{/each}
          </Card.Content>
        </Card.Root>

        <Card.Root class="min-w-0 xl:col-span-3">
          <Card.Header
            ><Card.Title>Deployments</Card.Title><Card.Description
              >Deployments and their release commands for the selected release.
              Select a deployment to examine it in detail.</Card.Description
            ></Card.Header
          >
          <Card.Content class="min-h-0 space-y-4">
            {#if expandedReleaseId}
              {#if deploymentsFor(expandedReleaseId).length > 0 || releaseCommandForRelease}
                <div class="border border-border text-sm">
                  {#if releaseCommandForRelease}
                    {@const execution = releaseCommandForRelease}
                    <div class="border-b border-border bg-muted/20 px-3 py-2">
                      <p
                        class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground"
                      >
                        Release command
                      </p>
                    </div>
                    <div
                      class="grid gap-3 p-3 lg:grid-cols-[minmax(0,1fr)_auto]"
                    >
                      <div class="min-w-0">
                        <div class="flex flex-wrap items-center gap-2">
                          <span class="font-mono">{short(execution.id)}</span
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
                        {#if execution.error}<p
                            class="mt-2 text-xs text-destructive"
                          >
                            {execution.error}
                          </p>{/if}
                      </div>
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
                    <div
                      class="max-h-96 overflow-auto border-t border-border bg-black/30 p-3 font-mono text-[11px] leading-relaxed"
                    >
                      {#each releaseCommandLogs[execution.id] ?? [] as log (log.id)}<div
                          class={cn({
                            "text-destructive": log.stream === "stderr",
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
                  {/if}
                  {#if deploymentsFor(expandedReleaseId).length > 0}
                    <div
                      class="flex flex-wrap items-center gap-2 border-b border-border bg-muted/20 p-3"
                    >
                      <label
                        for="deployment-select"
                        class="text-xs font-medium text-muted-foreground"
                        >Deployment</label
                      >
                      <select
                        id="deployment-select"
                        class="h-9 min-w-0 flex-1 border border-input bg-background px-2 text-xs"
                        bind:value={selectedDeploymentId}
                      >
                        {#each deploymentsFor(expandedReleaseId) as deployment}
                          <option value={deployment.id}>
                            {short(deployment.id)} · {deployment.status}{deployment.attempt >
                            1
                              ? ` · attempt ${deployment.attempt}`
                              : ""} · {stamp(deployment.createdAt)}
                          </option>
                        {/each}
                      </select>
                    </div>
                    {#if selectedDeployment}
                      {@const deployment = selectedDeployment}
                      <div
                        class="grid gap-3 p-3 lg:grid-cols-[minmax(0,1fr)_auto]"
                      >
                        <div class="min-w-0">
                          <div class="flex flex-wrap items-center gap-2">
                            <span class="font-mono">{short(deployment.id)}</span
                            ><StatusBadge
                              status={deployment.status}
                            /><StatusBadge
                              status={deploymentStep(deployment)}
                            />
                          </div>
                          <p class="mt-1 text-xs text-muted-foreground">
                            {stamp(deployment.createdAt)} · {deployment.targetName}
                            {deployment.attempt > 1
                              ? ` · attempt ${deployment.attempt}`
                              : ""}
                          </p>
                          {#if deployment.error}<p
                              class="mt-2 text-xs text-destructive"
                            >
                              {deployment.error}
                            </p>{/if}
                        </div>
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
                      <div class="border-t border-border bg-muted/20 p-3">
                        <div
                          class="max-h-80 space-y-3 overflow-auto font-mono text-[11px] leading-relaxed"
                        >
                          {#each deploymentStream.events[deployment.id] ?? [] as event (event.id)}
                            <div
                              class={cn({
                                "text-destructive": event.status === "failed",
                                "text-warning": event.status === "warning",
                                "text-success": event.status === "succeeded",
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
                              deployment.status === "running"
                                ? "Waiting for Deployment events..."
                                : "No deployment events recorded yet."}
                            </p>
                          {/each}
                        </div>
                      </div>
                    {/if}
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
                class="flex min-h-64 items-center justify-center border border-dashed border-border p-6 text-sm text-muted-foreground"
              >
                Select a release to view its deployments.
              </div>
            {/if}
          </Card.Content>
        </Card.Root>
      </div>
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
