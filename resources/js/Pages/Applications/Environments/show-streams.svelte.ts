import { router } from "@inertiajs/svelte";

import { routes } from "@/routes";

import type {
  Build,
  BuildLog,
  BuildLogSnapshot,
  Deployment,
  DeploymentEvent,
  DeploymentEventSnapshot,
  EnvironmentLog,
  EnvironmentLogSnapshot,
} from "./show.types";

export class BuildLogStream {
  live = $state<Build[] | null>(null);
  logs = $state<Record<string, BuildLog[]>>({});
  connectionError = $state("");
  private cursors = $state<Record<string, number>>({});
  private loading = new Set<string>();

  constructor(
    private environmentId: string,
    private baseBuilds: () => Build[],
  ) {}

  async load(
    buildId: string,
    signal?: AbortSignal,
  ): Promise<BuildLogSnapshot | null> {
    if (this.loading.has(buildId)) return null;
    this.loading.add(buildId);
    try {
      const after = this.cursors[buildId] ?? 0;
      const response = await window.fetch(
        `${routes.environmentBuildLogs(this.environmentId, buildId)}?after=${after}`,
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
      this.live = (this.live ?? this.baseBuilds()).map((build) =>
        build.id === snapshot.build.id ? snapshot.build : build,
      );
      if (snapshot.logs.length > 0) {
        this.logs = {
          ...this.logs,
          [buildId]: [...(this.logs[buildId] ?? []), ...snapshot.logs],
        };
      } else if (!(buildId in this.logs)) {
        this.logs = { ...this.logs, [buildId]: [] };
      }
      this.cursors = { ...this.cursors, [buildId]: snapshot.nextSequence };
      this.connectionError = "";
      return snapshot;
    } finally {
      this.loading.delete(buildId);
    }
  }

  poll(buildId: string): () => void {
    const abortController = new AbortController();
    let timer: number | undefined;
    let retryDelay = 1000;

    const next = async () => {
      try {
        const snapshot = await this.load(buildId, abortController.signal);
        if (!snapshot || abortController.signal.aborted) return;
        retryDelay = 1000;
        if (snapshot.hasMore) {
          timer = window.setTimeout(next, 0);
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
        this.connectionError = "Reconnecting to the Build log...";
        retryDelay = Math.min(retryDelay * 2, 5000);
      }
      timer = window.setTimeout(next, retryDelay);
    };

    timer = window.setTimeout(next, 0);
    return () => {
      abortController.abort();
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }

  reset() {
    this.live = null;
    this.logs = {};
    this.cursors = {};
  }
}

export class DeploymentEventStream {
  live = $state<Deployment[] | null>(null);
  events = $state<Record<string, DeploymentEvent[]>>({});
  connectionError = $state("");
  private cursors = $state<Record<string, number>>({});
  private loading = new Set<string>();

  constructor(
    private environmentId: string,
    private baseDeployments: () => Deployment[],
  ) {}

  async load(
    deploymentId: string,
    signal?: AbortSignal,
  ): Promise<DeploymentEventSnapshot | null> {
    if (this.loading.has(deploymentId)) return null;
    this.loading.add(deploymentId);
    try {
      const after = this.cursors[deploymentId] ?? 0;
      const response = await window.fetch(
        `${routes.environmentDeploymentEvents(this.environmentId, deploymentId)}?after=${after}`,
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
      this.live = (this.live ?? this.baseDeployments()).map((deployment) => {
        if (deployment.id === snapshot.deployment.id)
          return snapshot.deployment;
        if (snapshot.deployment.active) return { ...deployment, active: false };
        return deployment;
      });
      if (snapshot.events.length > 0) {
        this.events = {
          ...this.events,
          [deploymentId]: [
            ...(this.events[deploymentId] ?? []),
            ...snapshot.events,
          ],
        };
      } else if (!(deploymentId in this.events)) {
        this.events = { ...this.events, [deploymentId]: [] };
      }
      this.cursors = { ...this.cursors, [deploymentId]: snapshot.nextSequence };
      this.connectionError = "";
      return snapshot;
    } finally {
      this.loading.delete(deploymentId);
    }
  }

  poll(deploymentId: string): () => void {
    const abortController = new AbortController();
    let timer: number | undefined;
    let retryDelay = 1000;

    const next = async () => {
      try {
        const snapshot = await this.load(deploymentId, abortController.signal);
        if (!snapshot || abortController.signal.aborted) return;
        retryDelay = 1000;
        if (snapshot.hasMore) {
          timer = window.setTimeout(next, 0);
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
        this.connectionError = "Reconnecting to the Deployment timeline...";
        retryDelay = Math.min(retryDelay * 2, 5000);
      }
      timer = window.setTimeout(next, retryDelay);
    };

    timer = window.setTimeout(next, 0);
    return () => {
      abortController.abort();
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }

  reset() {
    this.live = null;
    this.events = {};
    this.cursors = {};
  }
}

export class EnvironmentLogStream {
  logs = $state<EnvironmentLog[]>([]);
  loaded = $state(false);
  connectionError = $state("");
  private cursor = $state("");

  constructor(
    private applicationId: string,
    private environmentId: string,
  ) {}

  async load(signal?: AbortSignal): Promise<EnvironmentLogSnapshot> {
    const endpoint = new URL(
      routes.environmentLogs(this.applicationId, this.environmentId),
      window.location.origin,
    );
    if (this.cursor) endpoint.searchParams.set("after", this.cursor);
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
      this.logs = [...this.logs, ...snapshot.logs].slice(-2000);
    }
    this.cursor = snapshot.nextCursor;
    this.loaded = true;
    this.connectionError = "";
    return snapshot;
  }

  poll(): () => void {
    const abortController = new AbortController();
    let timer: number | undefined;
    let retryDelay = 2000;

    const next = async () => {
      try {
        const snapshot = await this.load(abortController.signal);
        if (abortController.signal.aborted) return;
        retryDelay = 2000;
        timer = window.setTimeout(next, snapshot.hasMore ? 0 : retryDelay);
      } catch {
        if (abortController.signal.aborted) return;
        this.connectionError = "Reconnecting to the workload log stream...";
        retryDelay = Math.min(retryDelay * 2, 10000);
        timer = window.setTimeout(next, retryDelay);
      }
    };

    timer = window.setTimeout(next, 0);
    return () => {
      abortController.abort();
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }
}
