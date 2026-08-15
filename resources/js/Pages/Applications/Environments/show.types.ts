export type {
  TelemetryLog as OpenTelemetryLog,
  TelemetryLogSnapshot as OpenTelemetryLogSnapshot,
  TelemetryRange,
  TraceSpan,
} from "@/Components/Telemetry/telemetry.types";

export type EnvironmentSection =
  "overview" | "telemetry" | "releases" | "builds" | "secrets";

export type Secret = {
  id: string;
  key: string;
  digestPrefix: string;
  sourceType: string;
  sourceId: string;
  createdAt: string;
  status: "deployed" | "deploying" | "pending" | "failed" | "pending_removal";
  desired: boolean;
};

export type Variable = {
  key: string;
  value: string;
  source: string;
  sourceId: string;
};

export type Resource = {
  id: string;
  alias: string;
  name: string;
  engine: string;
};

export type Build = {
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

export type BuildLog = {
  id: string;
  sequence: number;
  stream: "system" | "pack";
  message: string;
  occurredAt: string;
};

export type BuildLogSnapshot = {
  build: Build;
  logs: BuildLog[];
  nextSequence: number;
  hasMore: boolean;
};

export type Release = {
  id: string;
  sourceRevision: string;
  artifactReference: string;
  createdAt: string;
};

export type Deployment = {
  id: string;
  status: string;
  currentStep: string;
  error: string;
  releaseId: string;
  targetId: string;
  targetName: string;
  attempt: number;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  active: boolean;
};

export type DeploymentEvent = {
  id: string;
  sequence: number;
  eventType: string;
  status: string;
  step: string;
  message: string;
  error: string;
  occurredAt: string;
};

export type DeploymentEventSnapshot = {
  deployment: Deployment;
  events: DeploymentEvent[];
  nextSequence: number;
  hasMore: boolean;
};

export type EnvironmentLog = {
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

export type EnvironmentLogSnapshot = {
  logs: EnvironmentLog[];
  nextCursor: string;
  hasMore: boolean;
};

export type Process = {
  name: string;
  kind: "web" | "worker" | "release";
  command?: string | null;
  arguments: string[];
  replicas: number;
  container_port?: number;
  health_path?: string;
  timeout_seconds?: number;
};

export type Instance = {
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

export type ReleaseCommand = {
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

export type ReleaseCommandLog = {
  id: string;
  attempt: number;
  sequence: number;
  stream: "system" | "stdout" | "stderr";
  message: string;
  occurredAt: string;
};

export type DNSStatus = {
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

export type TelemetryPoint = {
  observedAt: string;
  cpuCores: number;
  memoryBytes: number;
  cpuAvailable: boolean;
  memoryAvailable: boolean;
};

export type TelemetryRow = {
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

export type Overview = {
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

export type ServingContainer = {
  instanceId: string;
  deploymentId: string;
  targetId: string;
  serverId: string;
  exists: boolean;
  running: boolean;
};

export type HostUsage = {
  cpuCores: number;
  memoryBytes: number;
  available: boolean;
};

export type ApplicationTelemetryPoint = {
  observedAt: string;
  requestsPerSecond: number;
  clientErrorsPerSecond: number;
  serverErrorsPerSecond: number;
  p50DurationMs: number;
  p95DurationMs: number;
  p99DurationMs: number;
};

export type DatabaseTelemetryPoint = {
  observedAt: string;
  operationsPerSecond: number;
  errorsPerSecond: number;
  p50DurationMs: number;
  p95DurationMs: number;
  p99DurationMs: number;
};

export type TraceSummary = {
  traceId: string;
  rootSpanName: string;
  requestMethod: string;
  requestRoute: string;
  responseCode: number;
  startedAt: string;
  durationNs: number;
  spanCount: number;
  errorCount: number;
};

export type RouteTelemetry = {
  route: string;
  method: string;
  requests: number;
  requestsPerSecond: number;
  errorRate: number;
  p95DurationMs: number;
};

export type CountryTelemetry = {
  code: string;
  requests: number;
};

export type QueryTelemetry = {
  query: string;
  databaseSystem: string;
  operation: string;
  executions: number;
  p95DurationMs: number;
};

export type ApplicationTelemetry = {
  available: boolean;
  observedAt: string;
  windowSeconds: number;
  requestsPerSecond: number;
  serverErrorRate: number;
  clientErrorRate: number;
  meanRequestDurationMs: number;
  runtimeMemoryBytes: number;
  heapAllocatedBytes: number;
  heapAllocations: number;
  heapGoalBytes: number;
  goroutines: number;
  history: ApplicationTelemetryPoint[];
  database: {
    available: boolean;
    observedAt: string;
    operationsPerSecond: number;
    errorsPerSecond: number;
    p95DurationMs: number;
    history: DatabaseTelemetryPoint[];
  };
  recentTraces: TraceSummary[];
  routes: RouteTelemetry[];
  countries: CountryTelemetry[];
  queries: QueryTelemetry[];
  moreQueries: boolean;
};
