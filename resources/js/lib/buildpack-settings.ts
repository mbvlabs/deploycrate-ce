import { routes } from "@/routes";

export type BuildpackRuntime = "go" | "rails" | "laravel" | "django";

export type FrontendBuildSettings = {
  runtime: "node";
  directory: string;
  scripts: string[];
  keep_node_runtime?: boolean;
};

export type BuildpackAdvancedSettings = {
  go_version?: string;
  go_build_flags?: string;
  node_version?: string;
};

export type BuildpackSettings = {
  schema_version: 5;
  runtime: BuildpackRuntime;
  frontend: FrontendBuildSettings | null;
  advanced?: BuildpackAdvancedSettings | null;
};

export type BuildServer = {
  id: string;
  name: string;
  kind: string;
  address: string;
  architecture: string;
  buildpacks: BuildpackRuntime[];
};

export type BuildpackRepositoryHints = {
  hasGoMod: boolean;
  hasPackageJson: boolean;
  packageManager?: string;
  hasLockfile: boolean;
  scripts?: string[];
  hasBuildScript: boolean;
  hasSSRScript: boolean;
  suggestedGoTargets?: string[];
  suggestedFrontendDirectory?: string;
  warnings?: string[];
};

export function defaultBuildpackSettings(
  runtime: BuildpackRuntime = "go",
): BuildpackSettings {
  return {
    schema_version: 5,
    runtime,
    frontend: null,
    advanced: null,
  };
}

function normalizeScripts(raw: unknown): string[] {
  if (Array.isArray(raw)) {
    const scripts = raw
      .map((script) => (typeof script === "string" ? script.trim() : ""))
      .filter((script) => script.length > 0);
    if (scripts.length > 0) {
      return scripts;
    }
  }
  return ["build"];
}

export function normalizeBuildpackSettings(raw: unknown): BuildpackSettings {
  const value =
    raw && typeof raw === "object"
      ? (raw as Partial<BuildpackSettings> & {
          frontend?: Partial<FrontendBuildSettings> & {
            script?: string;
            ssr?: { enabled?: boolean; script?: string };
          };
          advanced?: Partial<BuildpackAdvancedSettings> | null;
        })
      : {};
  const runtime = (value.runtime ?? "go") as BuildpackRuntime;
  let frontend: FrontendBuildSettings | null = null;
  if (value.frontend) {
    let scripts = normalizeScripts(value.frontend.scripts);
    const legacyScript = value.frontend.script?.trim();
    if (
      legacyScript &&
      (!value.frontend.scripts || value.frontend.scripts.length === 0)
    ) {
      scripts = [legacyScript];
    }
    if (value.frontend.ssr?.enabled === true) {
      const ssrScript = value.frontend.ssr.script?.trim() || "build:ssr";
      scripts = [...scripts, ssrScript];
    }
    frontend = {
      runtime: "node",
      directory: value.frontend.directory?.trim() || ".",
      scripts,
      keep_node_runtime:
        value.frontend.keep_node_runtime === true ||
        value.frontend.ssr?.enabled === true,
    };
  }
  const advanced = value.advanced
    ? {
        go_version: value.advanced.go_version?.trim() || undefined,
        go_build_flags: value.advanced.go_build_flags?.trim() || undefined,
        node_version: value.advanced.node_version?.trim() || undefined,
      }
    : null;
  const hasAdvanced =
    advanced &&
    (advanced.go_version || advanced.go_build_flags || advanced.node_version);
  return {
    schema_version: 5,
    runtime,
    frontend,
    advanced: hasAdvanced ? advanced : null,
  };
}

export function buildHintsURL(
  repositoryId: string,
  reference: string,
  contextPath: string,
) {
  const params = new URLSearchParams({
    reference: reference.trim(),
    contextPath: contextPath.trim() || ".",
  });
  return `${routes.gitHubRepositoryBuildHints(repositoryId)}?${params}`;
}
