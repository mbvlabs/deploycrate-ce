import { routes } from "@/routes";

export type BuildpackRuntime = "go" | "rails" | "laravel" | "django";

export type BuildpackFrontendSSRSettings = {
  enabled: boolean;
  script: string;
};

export type FrontendBuildSettings = {
  runtime: "node";
  directory: string;
  script: string;
  ssr?: BuildpackFrontendSSRSettings | null;
};

export type BuildpackAdvancedSettings = {
  go_version?: string;
  go_build_flags?: string;
  node_version?: string;
};

export type BuildpackSettings = {
  schema_version: 4;
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
    schema_version: 4,
    runtime,
    frontend: null,
    advanced: null,
  };
}

export function normalizeBuildpackSettings(raw: unknown): BuildpackSettings {
  const value =
    raw && typeof raw === "object"
      ? (raw as Partial<BuildpackSettings> & {
          frontend?: Partial<FrontendBuildSettings> & {
            ssr?: Partial<BuildpackFrontendSSRSettings>;
          };
          advanced?: Partial<BuildpackAdvancedSettings> | null;
        })
      : {};
  const runtime = (value.runtime ?? "go") as BuildpackRuntime;
  let frontend: FrontendBuildSettings | null = null;
  if (value.frontend) {
    frontend = {
      runtime: "node",
      directory: value.frontend.directory?.trim() || ".",
      script: value.frontend.script?.trim() || "build",
      ssr:
        value.frontend.ssr?.enabled === true
          ? {
              enabled: true,
              script: value.frontend.ssr.script?.trim() || "build:ssr",
            }
          : null,
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
    schema_version: 4,
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
