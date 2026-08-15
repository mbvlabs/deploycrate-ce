import ActivityIcon from "@lucide/svelte/icons/activity";
import AppWindowIcon from "@lucide/svelte/icons/app-window";
import CloudIcon from "@lucide/svelte/icons/cloud";
import DatabaseIcon from "@lucide/svelte/icons/database";
import GithubIcon from "@lucide/svelte/icons/git-fork";
import KeyRoundIcon from "@lucide/svelte/icons/key-round";
import LayoutDashboardIcon from "@lucide/svelte/icons/layout-dashboard";
import ListTodoIcon from "@lucide/svelte/icons/list-todo";
import NetworkIcon from "@lucide/svelte/icons/network";
import RouteIcon from "@lucide/svelte/icons/route";
import ServerIcon from "@lucide/svelte/icons/server";
import SettingsIcon from "@lucide/svelte/icons/settings";
import ShieldCheckIcon from "@lucide/svelte/icons/shield-check";
import type { LucideIcon } from "@lucide/svelte";

import { routes } from "@/routes";

export type ResourceNavigation = {
  id: string;
  name: string;
  engine: string;
  resourceType: string;
  systemManaged?: boolean;
};

export type ApplicationNavigation = {
  id: string;
  name: string;
};

export type EnvironmentNavigation = {
  applicationId: string;
  applicationName: string;
  id: string;
  name: string;
};

export type NavigationContext = {
  resourceNavigation?: ResourceNavigation | null;
  applicationNavigation?: ApplicationNavigation | null;
  environmentNavigation?: EnvironmentNavigation | null;
};

export type NavigationItem = {
  id: string;
  label: string;
  href: string;
  icon: LucideIcon;
  match: "path-exact" | "url-exact" | "url-prefix";
  showAriaCurrent?: boolean;
};

export type NavigationGroup = {
  id: string;
  label: string;
  title?: string;
  items: NavigationItem[];
};

export type BreadcrumbItem = {
  label: string;
  href?: string;
};

export type ContextualBack = {
  label: string;
  href: string;
  ariaLabel: string;
};

type ApplicationRoutes = {
  overview: string;
  source: string;
  settings: string;
};

type EnvironmentRoutes = {
  overview: string;
  telemetry: string;
  releases: string;
  builds: string;
  secrets: string;
  source: string;
  settings: string;
};

type ResourceRoutes = {
  overview: string;
  backups: string;
  endpoints: string;
  credentials: string;
  health: string;
  databases?: string;
  settings?: string;
  access?: string;
};

export function pathFromURL(url: string): string {
  return url.split("?")[0];
}

export function applicationRoutes(
  application: ApplicationNavigation,
): ApplicationRoutes {
  return {
    overview: routes.applicationShow(application.id),
    source: routes.applicationSourceEdit(application.id),
    settings: routes.applicationEdit(application.id),
  };
}

export function environmentRoutes(
  environment: EnvironmentNavigation,
): EnvironmentRoutes {
  return {
    overview: routes.environmentShow(environment.applicationId, environment.id),
    telemetry: routes.environmentTelemetry(
      environment.applicationId,
      environment.id,
    ),
    releases: routes.environmentReleases(
      environment.applicationId,
      environment.id,
    ),
    builds: routes.environmentBuilds(environment.applicationId, environment.id),
    secrets: routes.environmentSecrets(
      environment.applicationId,
      environment.id,
    ),
    source: routes.environmentSourceEdit(
      environment.applicationId,
      environment.id,
    ),
    settings: routes.environmentEdit(environment.applicationId, environment.id),
  };
}

export function resourceRoutes(resource: ResourceNavigation): ResourceRoutes {
  if (resource.systemManaged) {
    return {
      overview: routes.systemResource(resource.id),
      backups: routes.systemResourceBackups(resource.id),
      endpoints: routes.systemResourceEndpoints(resource.id),
      credentials: routes.systemResourceCredentials(resource.id),
      health: routes.systemResourceHealth(resource.id),
      access: routes.systemResourceAccess(resource.id),
    };
  }

  return {
    overview: routes.resourceShow(resource.id),
    databases: routes.resourceDatabases(resource.id),
    backups: routes.resourceBackups(resource.id),
    endpoints: routes.resourceEndpoints(resource.id),
    credentials: routes.resourceCredentials(resource.id),
    health: routes.resourceHealth(resource.id),
    settings: routes.resourceSettings(resource.id),
  };
}

function contextualItem(
  id: string,
  label: string,
  href: string,
  icon: LucideIcon,
): NavigationItem {
  return { id, label, href, icon, match: "path-exact" };
}

function globalItem(
  id: string,
  label: string,
  href: string,
  icon: LucideIcon,
  match: "url-exact" | "url-prefix" = "url-prefix",
): NavigationItem {
  return { id, label, href, icon, match, showAriaCurrent: true };
}

function applicationGroup(application: ApplicationNavigation): NavigationGroup {
  const appRoutes = applicationRoutes(application);

  return {
    id: "application",
    label: application.name,
    title: application.name,
    items: [
      contextualItem(
        "application-overview",
        "Overview",
        appRoutes.overview,
        LayoutDashboardIcon,
      ),
      contextualItem(
        "application-source",
        "Source",
        appRoutes.source,
        GithubIcon,
      ),
      contextualItem(
        "application-settings",
        "Settings",
        appRoutes.settings,
        SettingsIcon,
      ),
    ],
  };
}

function environmentGroup(environment: EnvironmentNavigation): NavigationGroup {
  const envRoutes = environmentRoutes(environment);

  return {
    id: "environment",
    label: environment.name,
    title: environment.name,
    items: [
      contextualItem(
        "environment-overview",
        "Overview",
        envRoutes.overview,
        LayoutDashboardIcon,
      ),
      contextualItem(
        "environment-telemetry",
        "Telemetry",
        envRoutes.telemetry,
        ActivityIcon,
      ),
      contextualItem(
        "environment-releases",
        "Releases",
        envRoutes.releases,
        RouteIcon,
      ),
      contextualItem(
        "environment-builds",
        "Builds",
        envRoutes.builds,
        ListTodoIcon,
      ),
      contextualItem(
        "environment-secrets",
        "Secrets",
        envRoutes.secrets,
        KeyRoundIcon,
      ),
      contextualItem(
        "environment-source",
        "Source",
        envRoutes.source,
        GithubIcon,
      ),
      contextualItem(
        "environment-settings",
        "Settings",
        envRoutes.settings,
        SettingsIcon,
      ),
    ],
  };
}

function resourceGroup(resource: ResourceNavigation): NavigationGroup {
  const resourceURLs = resourceRoutes(resource);
  const items = [
    contextualItem(
      "resource-overview",
      "Overview",
      resourceURLs.overview,
      LayoutDashboardIcon,
    ),
  ];

  if (resource.resourceType === "database" && !resource.systemManaged) {
    items.push(
      contextualItem(
        "resource-databases",
        "Databases",
        resourceURLs.databases!,
        DatabaseIcon,
      ),
    );
  }
  if (resource.resourceType === "database") {
    items.push(
      contextualItem(
        "resource-backups",
        "Backups",
        resourceURLs.backups,
        CloudIcon,
      ),
    );
  }

  items.push(
    contextualItem(
      "resource-endpoints",
      "Endpoints",
      resourceURLs.endpoints,
      NetworkIcon,
    ),
    contextualItem(
      "resource-credentials",
      "Credentials",
      resourceURLs.credentials,
      KeyRoundIcon,
    ),
    contextualItem(
      "resource-health",
      "Health checks",
      resourceURLs.health,
      ActivityIcon,
    ),
  );

  if (resource.systemManaged) {
    items.push(
      contextualItem(
        "resource-access",
        "Access",
        resourceURLs.access!,
        ShieldCheckIcon,
      ),
    );
  } else {
    items.push(
      contextualItem(
        "resource-settings",
        "Settings",
        resourceURLs.settings!,
        SettingsIcon,
      ),
    );
  }

  return {
    id: "resource",
    label: resource.name,
    title: resource.name,
    items,
  };
}

export function hasNavigationContext(context: NavigationContext): boolean {
  return Boolean(
    context.resourceNavigation ||
    context.applicationNavigation ||
    context.environmentNavigation,
  );
}

export function contextualNavigationGroups(
  context: NavigationContext,
): NavigationGroup[] {
  if (context.resourceNavigation) {
    return [resourceGroup(context.resourceNavigation)];
  }

  const groups: NavigationGroup[] = [];
  const sidebarApplication =
    context.applicationNavigation ??
    (context.environmentNavigation
      ? {
          id: context.environmentNavigation.applicationId,
          name: context.environmentNavigation.applicationName,
        }
      : null);

  if (sidebarApplication) groups.push(applicationGroup(sidebarApplication));
  if (context.environmentNavigation) {
    groups.push(environmentGroup(context.environmentNavigation));
  }

  return groups;
}

export function globalNavigationGroups(): NavigationGroup[] {
  return [
    {
      id: "dashboard",
      label: "Dashboard",
      items: [
        globalItem(
          "home",
          "Home",
          routes.homePage(),
          LayoutDashboardIcon,
          "url-exact",
        ),
        globalItem(
          "applications",
          "Applications",
          routes.applications(),
          AppWindowIcon,
        ),
        globalItem("resources", "Resources", routes.resources(), DatabaseIcon),
      ],
    },
    {
      id: "infrastructure",
      label: "Infrastructure",
      items: [
        globalItem("nodes", "Nodes", routes.nodes(), ServerIcon),
        globalItem("networks", "Networks", routes.networks(), NetworkIcon),
        globalItem(
          "caddy-routes",
          "Caddy Routes",
          routes.caddyRoutes(),
          RouteIcon,
        ),
      ],
    },
    {
      id: "connections",
      label: "Connections",
      items: [
        globalItem(
          "image-registry",
          "Image Registry",
          routes.registryResources(),
          KeyRoundIcon,
        ),
        globalItem(
          "object-storage",
          "Object Storage",
          routes.objectStorage(),
          CloudIcon,
        ),
        globalItem("dns", "DNS", routes.dnsConnections(), NetworkIcon),
        globalItem("github", "GitHub", routes.gitHubConnection(), GithubIcon),
      ],
    },
    {
      id: "system",
      label: "System",
      items: [
        globalItem(
          "system-overview",
          "Overview",
          routes.systemOverview(),
          ShieldCheckIcon,
          "url-exact",
        ),
        globalItem(
          "system-telemetry",
          "Telemetry",
          routes.systemTelemetry(),
          ActivityIcon,
        ),
        globalItem(
          "system-tasks",
          "System Tasks",
          routes.systemTasks(),
          ListTodoIcon,
        ),
        globalItem(
          "system-updates",
          "Updates",
          routes.systemUpdate(),
          SettingsIcon,
        ),
      ],
    },
  ];
}

export function isNavigationItemActive(
  item: NavigationItem,
  currentURL: string,
): boolean {
  switch (item.match) {
    case "path-exact":
      return pathFromURL(currentURL) === item.href;
    case "url-exact":
      return currentURL === item.href;
    case "url-prefix":
      return currentURL.startsWith(item.href);
  }
}

export function contextualBack(
  context: NavigationContext,
): ContextualBack | null {
  if (context.resourceNavigation) {
    return {
      label: "All resources",
      href: routes.resources(),
      ariaLabel: "Back to all resources",
    };
  }
  if (context.applicationNavigation || context.environmentNavigation) {
    return {
      label: "All applications",
      href: routes.applications(),
      ariaLabel: "Back to all applications",
    };
  }
  return null;
}

function detailLeaf(path: string): string {
  if (path.endsWith("/new")) return "New";
  if (path.endsWith("/edit")) return "Edit";
  if (path.includes("/source")) return "Source";
  return "Details";
}

export function breadcrumbsForURL(
  url: string,
  context: NavigationContext,
): BreadcrumbItem[] {
  const path = pathFromURL(url);
  const leaf = detailLeaf(path);
  const { applicationNavigation, environmentNavigation, resourceNavigation } =
    context;

  if (path === routes.homePage()) return [];

  if (applicationNavigation) {
    const appRoutes = applicationRoutes(applicationNavigation);
    const sectionLabels: Record<string, string> = {
      [appRoutes.overview]: "Overview",
      [appRoutes.source]: "Source",
      [appRoutes.settings]: "Settings",
    };
    if (sectionLabels[path]) {
      return [
        { label: "Applications", href: routes.applications() },
        { label: applicationNavigation.name, href: appRoutes.overview },
        { label: sectionLabels[path] },
      ];
    }
  }

  if (environmentNavigation) {
    const envRoutes = environmentRoutes(environmentNavigation);
    const sectionLabels: Record<string, string> = {
      [envRoutes.overview]: "Overview",
      [envRoutes.telemetry]: "Telemetry",
      [envRoutes.releases]: "Releases",
      [envRoutes.builds]: "Builds",
      [envRoutes.secrets]: "Secrets",
      [envRoutes.source]: "Source",
      [envRoutes.settings]: "Settings",
    };
    if (sectionLabels[path]) {
      return [
        { label: "Applications", href: routes.applications() },
        {
          label: environmentNavigation.applicationName,
          href: routes.applicationShow(environmentNavigation.applicationId),
        },
        { label: environmentNavigation.name, href: envRoutes.overview },
        { label: sectionLabels[path] },
      ];
    }
  }

  const environmentPage =
    url.startsWith(routes.environments()) ||
    /^\/applications\/[^/]+\/environments(?:\/|$)/.test(url);
  if (environmentPage) {
    return [
      { label: "Applications", href: routes.applications() },
      { label: path === routes.environments() ? "Environments" : leaf },
    ];
  }

  if (path.startsWith(routes.applications())) {
    return [
      { label: "Applications", href: routes.applications() },
      ...(path === routes.applications() ? [] : [{ label: leaf }]),
    ];
  }

  if (resourceNavigation) {
    const resourceURLs = resourceRoutes(resourceNavigation);
    const sectionLabels: Record<string, string> = {
      [resourceURLs.overview]: "Overview",
      [resourceURLs.backups]: "Backups",
      [resourceURLs.endpoints]: "Endpoints",
      [resourceURLs.credentials]: "Credentials",
      [resourceURLs.health]: "Health checks",
      ...(resourceURLs.databases
        ? { [resourceURLs.databases]: "Databases" }
        : {}),
      ...(resourceURLs.settings ? { [resourceURLs.settings]: "Settings" } : {}),
      ...(resourceURLs.access ? { [resourceURLs.access]: "Access" } : {}),
    };
    if (sectionLabels[path]) {
      return [
        { label: "Resources", href: routes.resources() },
        { label: resourceNavigation.name, href: resourceURLs.overview },
        { label: sectionLabels[path] ?? "Overview" },
      ];
    }
  }

  if (path.startsWith(routes.resources())) {
    if (!resourceNavigation) {
      return [
        { label: "Resources", href: routes.resources() },
        ...(path === routes.resources() ? [] : [{ label: leaf }]),
      ];
    }
    return [
      { label: "Resources", href: routes.resources() },
      { label: resourceNavigation.name },
    ];
  }

  if (path.startsWith(routes.nodes())) {
    return [
      { label: "Nodes", href: routes.nodes() },
      ...(path === routes.nodes() ? [] : [{ label: leaf }]),
    ];
  }
  if (path.startsWith(routes.networks())) return [{ label: "Networks" }];
  if (path.startsWith(routes.caddyRoutes())) {
    return path === routes.caddyRoutes()
      ? [{ label: "Infrastructure" }, { label: "Caddy Routes" }]
      : [
          { label: "Infrastructure" },
          { label: "Caddy Routes", href: routes.caddyRoutes() },
          { label: "Details" },
        ];
  }
  if (path.startsWith(routes.registryResources())) {
    return path === routes.registryResources()
      ? [{ label: "Connections" }, { label: "Image Registry" }]
      : [
          { label: "Connections" },
          { label: "Image Registry", href: routes.registryResources() },
          { label: "Details" },
        ];
  }
  if (path.startsWith(routes.dnsConnections())) {
    return [{ label: "Connections" }, { label: "DNS" }];
  }
  if (path.startsWith(routes.objectStorage())) {
    return path === routes.objectStorage()
      ? [{ label: "Connections" }, { label: "Object Storage" }]
      : [
          { label: "Connections" },
          { label: "Object Storage", href: routes.objectStorage() },
          { label: "Details" },
        ];
  }
  if (path.startsWith(routes.gitHubConnection())) {
    return path === routes.gitHubConnection()
      ? [{ label: "Connections" }, { label: "GitHub" }]
      : [
          { label: "Connections" },
          { label: "GitHub", href: routes.gitHubConnection() },
          { label: "Details" },
        ];
  }
  if (path.startsWith(routes.systemTelemetry())) {
    return [
      { label: "System", href: routes.systemOverview() },
      { label: "Telemetry" },
    ];
  }
  if (path.startsWith(routes.systemTasks())) {
    return [
      { label: "System", href: routes.systemOverview() },
      { label: "Tasks" },
    ];
  }
  if (path.startsWith(routes.systemUpdate())) {
    return [
      { label: "System", href: routes.systemOverview() },
      { label: "Updates" },
    ];
  }
  if (path.startsWith(routes.systemResources())) {
    return [
      { label: "Resources", href: routes.resources() },
      { label: "System Resource" },
    ];
  }
  return [{ label: "Overview" }];
}
