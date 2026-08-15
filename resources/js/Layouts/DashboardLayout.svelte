<script lang="ts">
  import ArrowLeftIcon from "@lucide/svelte/icons/arrow-left";
  import ChevronDownIcon from "@lucide/svelte/icons/chevron-down";
  import LogOutIcon from "@lucide/svelte/icons/log-out";
  import { Link, page, router } from "@inertiajs/svelte";
  import type { Snippet } from "svelte";

  import DashboardBreadcrumbs from "@/Components/Navigation/DashboardBreadcrumbs.svelte";
  import DashboardNavigationGroups from "@/Components/Navigation/DashboardNavigationGroups.svelte";
  import {
    breadcrumbsForURL,
    contextualBack,
    contextualNavigationGroups,
    globalNavigationGroups,
    hasNavigationContext,
    type ApplicationNavigation,
    type EnvironmentNavigation,
    type ResourceNavigation,
  } from "@/Components/Navigation/dashboard-navigation";
  import ThemeToggle from "@/Components/ThemeToggle.svelte";
  import { Button } from "@/Components/ui/button";
  import * as DropdownMenu from "@/Components/ui/dropdown-menu";
  import * as Sidebar from "@/Components/ui/sidebar";
  import { SIDEBAR_COOKIE_NAME } from "@/Components/ui/sidebar/constants";
  import { routes } from "@/routes";

  let {
    children,
    email,
    version,
    resourceNavigation = null,
    applicationNavigation = null,
    environmentNavigation = null,
  }: {
    children: Snippet;
    email: string;
    version?: string;
    resourceNavigation?: ResourceNavigation | null;
    applicationNavigation?: ApplicationNavigation | null;
    environmentNavigation?: EnvironmentNavigation | null;
  } = $props();

  const appVersion = $derived(
    version ?? String($page.props.appVersion ?? "dev"),
  );
  const navigationContext = $derived({
    resourceNavigation,
    applicationNavigation,
    environmentNavigation,
  });
  const hasContext = $derived(hasNavigationContext(navigationContext));
  const backLink = $derived(contextualBack(navigationContext));
  const navigationGroups = $derived(
    hasContext
      ? contextualNavigationGroups(navigationContext)
      : globalNavigationGroups(),
  );
  const breadcrumbs = $derived(breadcrumbsForURL($page.url, navigationContext));

  function initialSidebarOpen() {
    if (typeof document === "undefined") return true;

    const stateCookie = document.cookie
      .split("; ")
      .find((cookie) => cookie.startsWith(`${SIDEBAR_COOKIE_NAME}=`));

    return stateCookie ? stateCookie.split("=")[1] !== "false" : true;
  }

  let sidebarOpen = $state(initialSidebarOpen());

  function signOut() {
    router.delete(routes.sessionDestroy());
  }
</script>

<Sidebar.Provider bind:open={sidebarOpen}>
  <Sidebar.Root collapsible="icon">
    <Sidebar.Header
      class="h-14 justify-center border-b border-sidebar-border p-1"
    >
      {#if backLink}
        <Link
          href={backLink.href}
          class="flex h-12 w-full items-center gap-2 px-3 text-sm font-medium group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          aria-label={backLink.ariaLabel}
        >
          <ArrowLeftIcon class="size-4 shrink-0" />
          <span class="truncate group-data-[collapsible=icon]:hidden">
            {backLink.label}
          </span>
        </Link>
      {:else}
        <Link
          href={routes.homePage()}
          class="flex h-12 w-full items-center px-3 text-sm font-semibold tracking-tight group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
        >
          <span class="group-data-[collapsible=icon]:hidden">
            DeployCrate CE
          </span>
          <span class="hidden group-data-[collapsible=icon]:inline">DC</span>
        </Link>
      {/if}
    </Sidebar.Header>

    <Sidebar.Content>
      <DashboardNavigationGroups
        groups={navigationGroups}
        currentURL={$page.url}
      />
    </Sidebar.Content>

    <Sidebar.Footer
      class="border-t border-sidebar-border px-4 py-3 group-data-[collapsible=icon]:hidden"
    >
      <p
        class="truncate font-mono text-[10px] text-sidebar-foreground/60"
        title={`version: ${appVersion}`}
      >
        version: {appVersion}
      </p>
    </Sidebar.Footer>

    <Sidebar.Rail />
  </Sidebar.Root>

  <Sidebar.Inset class="min-w-0">
    <header
      class="sticky top-0 z-10 flex h-14 shrink-0 items-center justify-between gap-3 border-b border-border bg-background px-4"
    >
      <div class="flex min-w-0 items-center gap-3">
        <Sidebar.Trigger class="-ml-1" />
        <DashboardBreadcrumbs items={breadcrumbs} />
      </div>

      <div class="flex items-center gap-2">
        <ThemeToggle />

        <DropdownMenu.Root>
          <DropdownMenu.Trigger>
            {#snippet child({ props })}
              <Button {...props} variant="outline" size="sm" class="max-w-64">
                <span class="hidden truncate sm:block">{email}</span>
                <ChevronDownIcon data-icon="inline-end" />
              </Button>
            {/snippet}
          </DropdownMenu.Trigger>
          <DropdownMenu.Content align="end" class="w-48">
            <DropdownMenu.Item onclick={signOut}>
              <LogOutIcon />
              Sign out
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Root>
      </div>
    </header>

    <div class="flex-1 p-4 sm:p-6 lg:p-8">
      <div class="mx-auto w-full max-w-6xl">
        {@render children()}
      </div>
    </div>
  </Sidebar.Inset>
</Sidebar.Provider>
