export type JSONValue =
  | string
  | number
  | boolean
  | null
  | JSONValue[]
  | { [key: string]: JSONValue };

export type CredentialField = {
  name: string;
  label: string;
  required: boolean;
  secret: boolean;
};

export type EnvironmentKey = {
  name: string;
  label: string;
  defaultKey: string;
};

export type ResourceEngine = {
  engine: string;
  label: string;
  resourceType: "database" | "cache" | "service";
  protocols: string[];
  endpointRoles: string[];
  tlsModes: string[];
  credentialFields: CredentialField[];
  environmentKeys: EnvironmentKey[];
  healthCheckKinds: string[];
  defaultPort: number;
  defaultProtocol: string;
  defaultTlsMode: string;
};

export type ResourceServer = { id: string; name: string; address: string };

export type PrivateNetwork = {
  id: string;
  name: string;
  serverIds: string[];
  serverAddresses: Record<string, string>;
};

export type ResourceOptions = {
  engines: ResourceEngine[];
  resourceTypes: string[];
  servers: ResourceServer[];
  privateNetworks: PrivateNetwork[];
  registryCredentials: Array<{ id: string; name: string }>;
};

export type PortMapping = {
  hostPort: number;
  containerPort: number;
  protocol: string;
};

export type ResourceInstallation = {
  id: string;
  createdAt: string;
  updatedAt: string;
  imageReference: string;
  imageDigest: string;
  containerName: string;
  restartPolicy: string;
  configuration: Record<string, unknown> & { portMappings?: PortMapping[] };
  serverId: string;
  serverName: string;
  serverAddress: string;
  registryCredentialId: string;
  state: string;
  serviceState: string;
  health: string;
  healthReason: string;
  observedAt: string | null;
  containerDetails?: {
    id?: string;
    exitCode?: number;
    restartCount?: number;
  } | null;
  canControl: boolean;
};

export type ResourceEndpoint = {
  id: string;
  createdAt: string;
  updatedAt: string;
  name: string;
  role: string;
  address: string;
  port: number;
  protocol: string;
  tlsMode: string;
  settings: Record<string, unknown> & {
    caddy?: {
      managed?: boolean;
      origin_address?: string;
      origin_private_network_id?: string;
      origin_port?: number;
      origin_protocol?: string;
      origin_tls_mode?: string;
      health_path?: string;
    };
  };
  privateNetworkId: string | null;
  managed: boolean;
};

export type ResourceCredentialMetadata = Record<string, unknown> & {
  purpose?: string;
  database?: string;
};

export type ResourceCredential = {
  id: string;
  createdAt: string;
  updatedAt: string;
  name: string;
  username: string;
  metadata: ResourceCredentialMetadata;
  hasEncryptedPayload: boolean;
};

export type ResourceDatabase = {
  name: string;
  encoding: string;
  collation: string;
};

export type ResourceVolume = {
  id: string;
  createdAt: string;
  updatedAt: string;
  name: string;
  driver: string;
  configuration: Record<string, unknown>;
  serverId: string;
  serverName: string;
};

export type ResourceMount = {
  id: string;
  createdAt: string;
  updatedAt: string;
  mountPath: string;
  readOnly: boolean;
  resourceVolumeId: string;
  resourceInstallationId: string;
  volumeName: string;
  containerName: string;
};

export type ResourceHealthCheck = {
  id: string;
  createdAt: string;
  updatedAt: string;
  name: string;
  kind: string;
  configuration: Record<string, unknown>;
  intervalSeconds: number;
  timeoutSeconds: number;
  failureThreshold: number;
  successThreshold: number;
  enabled: boolean;
  resourceEndpointId: string;
  resourceCredentialId: string;
  state: string;
  message: string;
  latencyMs: number | null;
  consecutiveSuccesses: number;
  consecutiveFailures: number;
  observedAt: string | null;
  expiresAt: string | null;
};

export type ResourceConnection = {
  id: string;
  createdAt: string;
  updatedAt: string;
  alias: string;
  configuration: {
    environment_keys?: Record<string, string>;
    [key: string]: unknown;
  };
  database: string;
  environmentId: string;
  environmentName: string;
  environmentKind: string;
  environmentArchived: boolean;
  applicationName: string;
  applicationSlug: string;
  applicationArchived: boolean;
  resourceEndpointId: string;
  endpointName: string;
  resourceCredentialId: string;
  credentialName: string;
  environmentKeys: Record<string, string>;
  environmentKeyOverrides: Record<string, string>;
};

export type ResourceDeviceGrant = {
  deviceId: string;
  deviceName: string;
  ownerEmail: string;
  privateAddress: string;
  grantId: string;
  grantedAt: string;
  applicationState: string;
  applicationError: string;
  latestHandshakeAt: string | null;
  observedAt: string | null;
};

export type ResourcePrivateAccess = {
  id: string;
  address: string;
  port: number;
  protocol: string;
  tlsMode: string;
  privateNetworkId: string;
};

export type ResourceDetail = {
  id: string;
  createdAt: string;
  updatedAt: string;
  name: string;
  slug: string;
  resourceType: string;
  engine: string;
  configuration: Record<string, unknown> & {
    databases?: string[];
    environment_keys?: Record<string, string>;
  };
  databases: ResourceDatabase[];
  connectionCount: number;
  connections: ResourceConnection[];
  endpoints: ResourceEndpoint[];
  credentials: ResourceCredential[];
  installations: ResourceInstallation[];
  volumes: ResourceVolume[];
  mounts: ResourceMount[];
  healthChecks: ResourceHealthCheck[];
  privateAccess: ResourcePrivateAccess | null;
  privateAccessState: string;
  deviceGrants: ResourceDeviceGrant[];
  availableDevices: Array<{
    id: string;
    name: string;
    privateAddress: string;
  }>;
};

export type Publication = {
  id: string;
  resourceEndpointId: string;
  externalId: string;
  hostname: string;
  healthPath: string;
  state: string;
  lastError: string;
  appliedAt: string;
  observedAt: string;
  dns: {
    mode: string;
    zoneId: string;
    zoneName: string;
    connectionName: string;
    state: string;
    lastError: string;
    records?: Array<{ type: string; name: string; content: string }>;
  };
};

export type DNSZone = {
  zoneId: string;
  zoneName: string;
  connectionId: string;
  connectionName: string;
};

export type ResourceEnrollment = {
  deviceId: string;
  grantId: string;
  clientConfiguration: string;
};

export type BackupDestination = {
  id: string;
  name: string;
  provider: string;
  endpoint: string;
  region: string;
  bucket: string;
  prefix: string;
  verifiedAt: string | null;
  lastUsedAt: string | null;
};

export type BackupPolicy = {
  id: string;
  schedule: string;
  active: boolean;
  nextRunAt: string | null;
  backupDestinationId: string;
  keepLast: number;
  keepDaily: number;
  keepWeekly: number;
  keepMonthly: number;
};

export type BackupHistory = {
  id: string;
  status: string;
  triggerType: string;
  scheduledAt: string;
  finishedAt: string | null;
  verifiedAt: string | null;
  sizeBytes: number | null;
  error: string;
  canRestore: boolean;
};

export type RestoreHistory = {
  id: string;
  status: string;
  requestedAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  verifiedAt: string | null;
  cutoverAt: string | null;
  rolledBackAt: string | null;
  error: string;
  backupId: string;
  backupScheduledAt: string;
  safetyBackupId: string | null;
};

export type DatabaseBackups = {
  databaseName: string;
  eligibility: {
    eligible: boolean;
    reason: string;
    installationId: string | null;
  };
  policy: BackupPolicy | null;
  history: BackupHistory[];
  restores: RestoreHistory[];
  activeRestore: boolean;
};

export type ResourceBackups = {
  destinations: BackupDestination[];
  databases: DatabaseBackups[];
};
