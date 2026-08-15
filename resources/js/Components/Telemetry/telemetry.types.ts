export type TelemetryRange = "1h" | "6h" | "24h" | "7d";

export type TelemetryLog = {
  id: string;
  message: string;
  severity: string;
  severityNumber: number;
  attributes: Record<string, string>;
  traceId: string;
  spanId: string;
  scope: string;
  source: string;
  line: string;
  instance: string;
  slot: string;
  service: string;
  processName: string;
  processKind: string;
  processReplica: string;
  requestPath: string;
  responseCode: number;
  occurredAt: string;
};

export type TelemetryLogSnapshot<TLog extends TelemetryLog = TelemetryLog> = {
  logs: TLog[];
  nextCursor: string;
  hasMore: boolean;
};

export type TraceSpan = {
  traceId: string;
  spanId: string;
  parentSpanId: string;
  name: string;
  kind: string;
  serviceName: string;
  scope: string;
  statusCode: string;
  statusMessage: string;
  resourceAttributes: Record<string, string>;
  spanAttributes: Record<string, string>;
  startedAt: string;
  durationNs: number;
};
