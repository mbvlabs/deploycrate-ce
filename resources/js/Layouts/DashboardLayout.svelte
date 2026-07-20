<script lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down'
  import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard'
  import LogOutIcon from '@lucide/svelte/icons/log-out'
  import { Link, router } from '@inertiajs/svelte'
  import type { Snippet } from 'svelte'

  import { Button } from '@/Components/ui/button'
  import * as DropdownMenu from '@/Components/ui/dropdown-menu'
  import * as Sidebar from '@/Components/ui/sidebar'
  import { SIDEBAR_COOKIE_NAME } from '@/Components/ui/sidebar/constants'
  import { routes } from '@/routes'

  let { children, email }: { children: Snippet; email: string } = $props()

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
        <Sidebar.GroupLabel>Workspace</Sidebar.GroupLabel>
        <Sidebar.GroupContent>
          <Sidebar.Menu>
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive tooltipContent="Dashboard">
                {#snippet child({ props })}
                  <Link {...props} href={routes.homePage()}>
                    <LayoutDashboardIcon />
                    <span>Dashboard</span>
                  </Link>
                {/snippet}
              </Sidebar.MenuButton>
            </Sidebar.MenuItem>
          </Sidebar.Menu>
        </Sidebar.GroupContent>
      </Sidebar.Group>
    </Sidebar.Content>

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
