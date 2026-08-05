<script lang="ts">
  import AppWindowIcon from '@lucide/svelte/icons/app-window'
  import ActivityIcon from '@lucide/svelte/icons/activity'
  import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left'
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down'
  import CloudIcon from '@lucide/svelte/icons/cloud'
  import DatabaseIcon from '@lucide/svelte/icons/database'
  import GithubIcon from '@lucide/svelte/icons/git-fork'
  import KeyRoundIcon from '@lucide/svelte/icons/key-round'
  import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard'
  import ListTodoIcon from '@lucide/svelte/icons/list-todo'
  import LogOutIcon from '@lucide/svelte/icons/log-out'
  import NetworkIcon from '@lucide/svelte/icons/network'
  import RouteIcon from '@lucide/svelte/icons/route'
  import ServerIcon from '@lucide/svelte/icons/server'
  import SettingsIcon from '@lucide/svelte/icons/settings'
  import ShieldCheckIcon from '@lucide/svelte/icons/shield-check'
  import { Link, page, router } from '@inertiajs/svelte'
  import type { Snippet } from 'svelte'

  import * as Breadcrumb from '@/Components/ui/breadcrumb'
  import { Button } from '@/Components/ui/button'
  import * as DropdownMenu from '@/Components/ui/dropdown-menu'
  import * as Sidebar from '@/Components/ui/sidebar'
  import ThemeToggle from '@/Components/ThemeToggle.svelte'
  import { SIDEBAR_COOKIE_NAME } from '@/Components/ui/sidebar/constants'
  import { routes } from '@/routes'

  type ResourceNavigation = {
    id: string
    name: string
    engine: string
    resourceType: string
	systemManaged?: boolean
  }

  let { children, email, version, resourceNavigation = null }: { children: Snippet; email: string; version?: string; resourceNavigation?: ResourceNavigation | null } = $props()
  const appVersion = $derived(version ?? String($page.props.appVersion ?? 'dev'))
  const environmentPage = $derived($page.url.startsWith(routes.environments()) || /^\/applications\/[^/]+\/environments(?:\/|$)/.test($page.url))
	const contextualResourceRoutes = $derived.by((): Record<string, string> | null => {
		if (!resourceNavigation) return null
		if (resourceNavigation.systemManaged) {
			return {
				overview: routes.systemResource(resourceNavigation.id),
				backups: routes.systemResourceBackups(resourceNavigation.id),
				endpoints: routes.systemResourceEndpoints(resourceNavigation.id),
				credentials: routes.systemResourceCredentials(resourceNavigation.id),
				health: routes.systemResourceHealth(resourceNavigation.id),
				access: routes.systemResourceAccess(resourceNavigation.id),
			}
		}
		return {
			overview: routes.resourceShow(resourceNavigation.id),
			databases: routes.resourceDatabases(resourceNavigation.id),
			backups: routes.resourceBackups(resourceNavigation.id),
			endpoints: routes.resourceEndpoints(resourceNavigation.id),
			credentials: routes.resourceCredentials(resourceNavigation.id),
			health: routes.resourceHealth(resourceNavigation.id),
			settings: routes.resourceSettings(resourceNavigation.id),
		}
	})
	function contextualResourceURL(section: string) {
		return contextualResourceRoutes?.[section] ?? routes.resources()
	}
  const breadcrumbs = $derived.by(() => {
    const path = $page.url.split('?')[0]
    const leaf = path.endsWith('/new') ? 'New' : path.endsWith('/edit') ? 'Edit' : path.includes('/source') ? 'Source' : 'Details'

    if (path === routes.homePage()) return []
    if (environmentPage) return [{ label: 'Applications', href: routes.applications() }, { label: path === routes.environments() ? 'Environments' : leaf }]
    if (path.startsWith(routes.applications())) return [{ label: 'Applications', href: routes.applications() }, ...(path === routes.applications() ? [] : [{ label: leaf }])]
    if (resourceNavigation && contextualResourceRoutes && Object.values(contextualResourceRoutes).includes(path)) {
		const resourceSections: Record<string, string> = {
			[contextualResourceRoutes.overview]: 'Overview',
			[contextualResourceURL('backups')]: 'Backups',
			[contextualResourceURL('endpoints')]: 'Endpoints',
			[contextualResourceURL('credentials')]: 'Credentials',
			[contextualResourceURL('health')]: 'Health checks',
			[contextualResourceURL('databases')]: 'Databases',
			[contextualResourceURL('settings')]: 'Settings',
			[contextualResourceURL('access')]: 'Access',
		}
		return [
			{ label: 'Resources', href: routes.resources() },
			{ label: resourceNavigation.name, href: contextualResourceRoutes.overview },
			{ label: resourceSections[path] ?? 'Overview' },
		]
	}
    if (path.startsWith(routes.resources())) {
      if (!resourceNavigation) return [{ label: 'Resources', href: routes.resources() }, ...(path === routes.resources() ? [] : [{ label: leaf }])]
		return [{ label: 'Resources', href: routes.resources() }, { label: resourceNavigation.name }]
    }
    if (path.startsWith(routes.nodes())) return [{ label: 'Nodes', href: routes.nodes() }, ...(path === routes.nodes() ? [] : [{ label: leaf }])]
    if (path.startsWith(routes.networks())) return [{ label: 'Networks' }]
    if (path.startsWith(routes.caddyRoutes())) return path === routes.caddyRoutes()
      ? [{ label: 'Infrastructure' }, { label: 'Caddy Routes' }]
      : [{ label: 'Infrastructure' }, { label: 'Caddy Routes', href: routes.caddyRoutes() }, { label: 'Details' }]
    if (path.startsWith(routes.registryResources())) return path === routes.registryResources()
      ? [{ label: 'Connections' }, { label: 'Image Registry' }]
      : [{ label: 'Connections' }, { label: 'Image Registry', href: routes.registryResources() }, { label: 'Details' }]
    if (path.startsWith(routes.dnsConnections())) return [{ label: 'Connections' }, { label: 'DNS' }]
    if (path.startsWith(routes.objectStorage())) return path === routes.objectStorage()
      ? [{ label: 'Connections' }, { label: 'Object Storage' }]
      : [{ label: 'Connections' }, { label: 'Object Storage', href: routes.objectStorage() }, { label: 'Details' }]
    if (path.startsWith(routes.gitHubConnection())) return [{ label: 'Connections' }, { label: 'GitHub' }]
    if (path.startsWith(routes.systemTelemetry())) return [{ label: 'System', href: routes.systemOverview() }, { label: 'Telemetry' }]
    if (path.startsWith(routes.systemTasks())) return [{ label: 'System', href: routes.systemOverview() }, { label: 'Tasks' }]
    if (path.startsWith(routes.systemUpdate())) return [{ label: 'System', href: routes.systemOverview() }, { label: 'Updates' }]
    if (path.startsWith(routes.systemResources())) return [{ label: 'Resources', href: routes.resources() }, { label: 'System Resource' }]
    return [{ label: 'Overview' }]
  })

  function initialSidebarOpen() {
    if (typeof document === 'undefined') return true

    const stateCookie = document.cookie
      .split('; ')
      .find((cookie) => cookie.startsWith(`${SIDEBAR_COOKIE_NAME}=`))

    return stateCookie ? stateCookie.split('=')[1] !== 'false' : true
  }

  let sidebarOpen = $state(initialSidebarOpen())

  function signOut() {
    router.delete(routes.sessionDestroy())
  }
</script>

<Sidebar.Provider bind:open={sidebarOpen}>
  <Sidebar.Root collapsible="icon">
    <Sidebar.Header class="h-14 justify-center border-b border-sidebar-border p-1">
      {#if resourceNavigation}
        <Link
          href={routes.resources()}
          class="flex h-12 w-full items-center gap-2 px-3 text-sm font-medium group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          aria-label="Back to all resources"
        >
          <ArrowLeftIcon class="size-4 shrink-0" />
          <span class="truncate group-data-[collapsible=icon]:hidden">All resources</span>
        </Link>
      {:else}
        <Link
          href={routes.homePage()}
          class="flex h-12 w-full items-center px-3 text-sm font-semibold tracking-tight group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
        >
          <span class="group-data-[collapsible=icon]:hidden">DeployCrate CE</span>
          <span class="hidden group-data-[collapsible=icon]:inline">DC</span>
        </Link>
      {/if}
    </Sidebar.Header>

    <Sidebar.Content>
      {#if resourceNavigation}
        <Sidebar.Group>
          <Sidebar.GroupLabel class="truncate" title={resourceNavigation.name}>{resourceNavigation.name}</Sidebar.GroupLabel>
          <Sidebar.GroupContent>
            <Sidebar.Menu>
              <Sidebar.MenuItem>
                <Sidebar.MenuButton isActive={$page.url.split('?')[0] === contextualResourceURL('overview')} tooltipContent="Overview">
                  {#snippet child({ props })}<Link {...props} href={contextualResourceURL('overview')}><LayoutDashboardIcon /><span>Overview</span></Link>{/snippet}
                </Sidebar.MenuButton>
              </Sidebar.MenuItem>
              {#if resourceNavigation.resourceType === 'database' && !resourceNavigation.systemManaged}
                <Sidebar.MenuItem>
                  <Sidebar.MenuButton isActive={$page.url.split('?')[0] === contextualResourceURL('databases')} tooltipContent="Databases">
                    {#snippet child({ props })}<Link {...props} href={contextualResourceURL('databases')}><DatabaseIcon /><span>Databases</span></Link>{/snippet}
                  </Sidebar.MenuButton>
                </Sidebar.MenuItem>
              {/if}
              {#if resourceNavigation.resourceType === 'database'}
                <Sidebar.MenuItem>
                  <Sidebar.MenuButton isActive={$page.url.split('?')[0] === contextualResourceURL('backups')} tooltipContent="Backups">
                    {#snippet child({ props })}<Link {...props} href={contextualResourceURL('backups')}><CloudIcon /><span>Backups</span></Link>{/snippet}
                  </Sidebar.MenuButton>
                </Sidebar.MenuItem>
              {/if}
              <Sidebar.MenuItem>
                <Sidebar.MenuButton isActive={$page.url.split('?')[0] === contextualResourceURL('endpoints')} tooltipContent="Endpoints">
                  {#snippet child({ props })}<Link {...props} href={contextualResourceURL('endpoints')}><NetworkIcon /><span>Endpoints</span></Link>{/snippet}
                </Sidebar.MenuButton>
              </Sidebar.MenuItem>
              <Sidebar.MenuItem>
                <Sidebar.MenuButton isActive={$page.url.split('?')[0] === contextualResourceURL('credentials')} tooltipContent="Credentials">
                  {#snippet child({ props })}<Link {...props} href={contextualResourceURL('credentials')}><KeyRoundIcon /><span>Credentials</span></Link>{/snippet}
                </Sidebar.MenuButton>
              </Sidebar.MenuItem>
              <Sidebar.MenuItem>
                <Sidebar.MenuButton isActive={$page.url.split('?')[0] === contextualResourceURL('health')} tooltipContent="Health checks">
                  {#snippet child({ props })}<Link {...props} href={contextualResourceURL('health')}><ActivityIcon /><span>Health checks</span></Link>{/snippet}
                </Sidebar.MenuButton>
              </Sidebar.MenuItem>
              {#if resourceNavigation.systemManaged}
				<Sidebar.MenuItem>
					<Sidebar.MenuButton isActive={$page.url.split('?')[0] === contextualResourceURL('access')} tooltipContent="Access">
						{#snippet child({ props })}<Link {...props} href={contextualResourceURL('access')}><ShieldCheckIcon /><span>Access</span></Link>{/snippet}
					</Sidebar.MenuButton>
				</Sidebar.MenuItem>
			  {:else}
				<Sidebar.MenuItem>
					<Sidebar.MenuButton isActive={$page.url.split('?')[0] === contextualResourceURL('settings')} tooltipContent="Settings">
						{#snippet child({ props })}<Link {...props} href={contextualResourceURL('settings')}><SettingsIcon /><span>Settings</span></Link>{/snippet}
					</Sidebar.MenuButton>
				</Sidebar.MenuItem>
			  {/if}
            </Sidebar.Menu>
          </Sidebar.GroupContent>
        </Sidebar.Group>
      {:else}
      <Sidebar.Group>
        <Sidebar.GroupLabel>Dashboard</Sidebar.GroupLabel>
        <Sidebar.GroupContent>
          <Sidebar.Menu>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url === routes.homePage()} tooltipContent="Home">
                {#snippet child({ props })}
                  <Link {...props} href={routes.homePage()} aria-current={$page.url === routes.homePage() ? 'page' : undefined}>
                    <LayoutDashboardIcon />
                    <span>Home</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.applications())} tooltipContent="Applications">
                {#snippet child({ props })}<Link {...props} href={routes.applications()} aria-current={$page.url.startsWith(routes.applications()) ? 'page' : undefined}><AppWindowIcon /><span>Applications</span></Link>{/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.resources())} tooltipContent="Resources">
                {#snippet child({ props })}
                  <Link {...props} href={routes.resources()} aria-current={$page.url.startsWith(routes.resources()) ? 'page' : undefined}>
                    <DatabaseIcon />
                    <span>Resources</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
          </Sidebar.Menu>
        </Sidebar.GroupContent>
      </Sidebar.Group>

      <Sidebar.Group>
        <Sidebar.GroupLabel>Infrastructure</Sidebar.GroupLabel>
        <Sidebar.GroupContent>
          <Sidebar.Menu>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.nodes())} tooltipContent="Nodes">
                {#snippet child({ props })}<Link {...props} href={routes.nodes()} aria-current={$page.url.startsWith(routes.nodes()) ? 'page' : undefined}><ServerIcon /><span>Nodes</span></Link>{/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.networks())} tooltipContent="Networks">
                {#snippet child({ props })}
                  <Link {...props} href={routes.networks()} aria-current={$page.url.startsWith(routes.networks()) ? 'page' : undefined}>
                    <NetworkIcon />
                    <span>Networks</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.caddyRoutes())} tooltipContent="Caddy Routes">
                {#snippet child({ props })}<Link {...props} href={routes.caddyRoutes()} aria-current={$page.url.startsWith(routes.caddyRoutes()) ? 'page' : undefined}><RouteIcon /><span>Caddy Routes</span></Link>{/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
          </Sidebar.Menu>
        </Sidebar.GroupContent>
      </Sidebar.Group>

      <Sidebar.Group>
        <Sidebar.GroupLabel>Connections</Sidebar.GroupLabel>
        <Sidebar.GroupContent>
          <Sidebar.Menu>
            <Sidebar.MenuItem>
				<Sidebar.MenuButton isActive={$page.url.startsWith(routes.registryResources())} tooltipContent="Image Registry">
					{#snippet child({ props })}<Link {...props} href={routes.registryResources()} aria-current={$page.url.startsWith(routes.registryResources()) ? 'page' : undefined}><KeyRoundIcon /><span>Image Registry</span></Link>{/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.objectStorage())} tooltipContent="Object Storage">
                {#snippet child({ props })}<Link {...props} href={routes.objectStorage()} aria-current={$page.url.startsWith(routes.objectStorage()) ? 'page' : undefined}><CloudIcon /><span>Object Storage</span></Link>{/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.dnsConnections())} tooltipContent="DNS">
                {#snippet child({ props })}<Link {...props} href={routes.dnsConnections()} aria-current={$page.url.startsWith(routes.dnsConnections()) ? 'page' : undefined}><NetworkIcon /><span>DNS</span></Link>{/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.gitHubConnection())} tooltipContent="GitHub">
                {#snippet child({ props })}<Link {...props} href={routes.gitHubConnection()} aria-current={$page.url.startsWith(routes.gitHubConnection()) ? 'page' : undefined}><GithubIcon /><span>GitHub</span></Link>{/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
          </Sidebar.Menu>
        </Sidebar.GroupContent>
      </Sidebar.Group>

      <Sidebar.Group>
        <Sidebar.GroupLabel>System</Sidebar.GroupLabel>
        <Sidebar.GroupContent>
          <Sidebar.Menu>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url === routes.systemOverview()} tooltipContent="Overview">
                {#snippet child({ props })}
                  <Link {...props} href={routes.systemOverview()} aria-current={$page.url === routes.systemOverview() ? 'page' : undefined}>
                    <ShieldCheckIcon />
                    <span>Overview</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.systemTelemetry())} tooltipContent="Telemetry">
                {#snippet child({ props })}
                  <Link {...props} href={routes.systemTelemetry()} aria-current={$page.url.startsWith(routes.systemTelemetry()) ? 'page' : undefined}>
                    <ActivityIcon />
                    <span>Telemetry</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.systemTasks())} tooltipContent="System Tasks">
                {#snippet child({ props })}
                  <Link {...props} href={routes.systemTasks()} aria-current={$page.url.startsWith(routes.systemTasks()) ? 'page' : undefined}>
                    <ListTodoIcon />
                    <span>System Tasks</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.systemUpdate())} tooltipContent="Updates">
                {#snippet child({ props })}
                  <Link {...props} href={routes.systemUpdate()} aria-current={$page.url.startsWith(routes.systemUpdate()) ? 'page' : undefined}>
                    <SettingsIcon />
                    <span>Updates</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
          </Sidebar.Menu>
        </Sidebar.GroupContent>
      </Sidebar.Group>
      {/if}
    </Sidebar.Content>

    <Sidebar.Footer class="border-t border-sidebar-border px-4 py-3 group-data-[collapsible=icon]:hidden">
      <p class="truncate font-mono text-[10px] text-sidebar-foreground/60" title={`version: ${appVersion}`}>
        version: {appVersion}
      </p>
    </Sidebar.Footer>

    <Sidebar.Rail />
  </Sidebar.Root>

  <Sidebar.Inset class="min-w-0">
    <header class="sticky top-0 z-10 flex h-14 shrink-0 items-center justify-between gap-3 border-b border-border bg-background px-4">
      <div class="flex min-w-0 items-center gap-3">
        <Sidebar.Trigger class="-ml-1" />
        {#if breadcrumbs.length > 0}
          <Breadcrumb.Root class="hidden min-w-0 sm:block">
            <Breadcrumb.List class="flex-nowrap">
              {#each breadcrumbs as crumb, index (crumb.label)}
                <Breadcrumb.Item class="min-w-0">
                  {#if crumb.href && index < breadcrumbs.length - 1}
                    <Breadcrumb.Link>
                      {#snippet child({ props })}<Link {...props} href={crumb.href}>{crumb.label}</Link>{/snippet}
                    </Breadcrumb.Link>
                  {:else}
                    <Breadcrumb.Page class="truncate">{crumb.label}</Breadcrumb.Page>
                  {/if}
                </Breadcrumb.Item>
                {#if index < breadcrumbs.length - 1}<Breadcrumb.Separator />{/if}
              {/each}
            </Breadcrumb.List>
          </Breadcrumb.Root>
        {/if}
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
