<script lang="ts">
  import AppWindowIcon from '@lucide/svelte/icons/app-window'
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down'
  import DatabaseIcon from '@lucide/svelte/icons/database'
  import GitBranchIcon from '@lucide/svelte/icons/git-branch'
  import KeyRoundIcon from '@lucide/svelte/icons/key-round'
  import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard'
  import LogOutIcon from '@lucide/svelte/icons/log-out'
  import NetworkIcon from '@lucide/svelte/icons/network'
  import ServerIcon from '@lucide/svelte/icons/server'
  import SettingsIcon from '@lucide/svelte/icons/settings'
  import ShieldCheckIcon from '@lucide/svelte/icons/shield-check'
  import { Link, page, router } from '@inertiajs/svelte'
  import type { Snippet } from 'svelte'

  import { Button } from '@/Components/ui/button'
  import * as DropdownMenu from '@/Components/ui/dropdown-menu'
  import * as Sidebar from '@/Components/ui/sidebar'
  import { SIDEBAR_COOKIE_NAME } from '@/Components/ui/sidebar/constants'
  import { routes } from '@/routes'

  let { children, email }: { children: Snippet; email: string } = $props()
  const appVersion = $derived(String($page.props.appVersion ?? 'dev'))

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
      <Link
        href={routes.homePage()}
        class="flex h-12 w-full items-center px-3 text-sm font-semibold tracking-tight group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
      >
        <span class="group-data-[collapsible=icon]:hidden">DeployCrate CE</span>
        <span class="hidden group-data-[collapsible=icon]:inline">DC</span>
      </Link>
    </Sidebar.Header>

    <Sidebar.Content>
      <Sidebar.Group>
        <Sidebar.GroupLabel>Dashboard</Sidebar.GroupLabel>
        <Sidebar.GroupContent>
          <Sidebar.Menu>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url === routes.homePage()} tooltipContent="Home">
                {#snippet child({ props })}
                  <Link {...props} href={routes.homePage()}>
                    <LayoutDashboardIcon />
                    <span>Home</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton disabled tooltipContent="Applications">
                <AppWindowIcon />
                <span>Applications</span>
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton disabled tooltipContent="Resources">
                <DatabaseIcon />
                <span>Resources</span>
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
              <Sidebar.MenuButton disabled tooltipContent="Servers">
                <ServerIcon />
                <span>Servers</span>
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton disabled tooltipContent="Networks">
                <NetworkIcon />
                <span>Networks</span>
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
              <Sidebar.MenuButton disabled tooltipContent="Provider Credentials">
                <KeyRoundIcon />
                <span>Provider Credentials</span>
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton disabled tooltipContent="Repositories">
                <GitBranchIcon />
                <span>Repositories</span>
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
                  <Link {...props} href={routes.systemOverview()}>
                    <ShieldCheckIcon />
                    <span>Overview</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={$page.url.startsWith(routes.systemUpdate())} tooltipContent="Updates">
                {#snippet child({ props })}
                  <Link {...props} href={routes.systemUpdate()}>
                    <SettingsIcon />
                    <span>Updates</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
          </Sidebar.Menu>
        </Sidebar.GroupContent>
      </Sidebar.Group>
    </Sidebar.Content>

    <Sidebar.Footer class="border-t border-sidebar-border px-4 py-3 group-data-[collapsible=icon]:hidden">
      <p class="truncate font-mono text-[10px] text-sidebar-foreground/60" title={`version: ${appVersion}`}>
        version: {appVersion}
      </p>
    </Sidebar.Footer>

    <Sidebar.Rail />
  </Sidebar.Root>

  <Sidebar.Inset class="min-w-0">
    <header class="sticky top-0 z-10 flex h-14 shrink-0 items-center justify-between border-b border-border bg-background px-4">
      <Sidebar.Trigger class="-ml-1" />

      <DropdownMenu.Root>
        <DropdownMenu.Trigger>
          {#snippet child({ props })}
            <Button {...props} variant="outline" size="sm" class="max-w-64">
              <span class="truncate">{email}</span>
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
    </header>

    <div class="flex-1 p-4 sm:p-6 lg:p-8">
      <div class="mx-auto w-full max-w-6xl">
        {@render children()}
      </div>
    </div>
  </Sidebar.Inset>
</Sidebar.Provider>
