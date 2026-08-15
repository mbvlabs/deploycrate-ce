<script lang="ts">
  import { Link } from "@inertiajs/svelte";

  import * as Sidebar from "@/Components/ui/sidebar";
  import {
    isNavigationItemActive,
    type NavigationGroup,
  } from "./dashboard-navigation";

  let {
    groups,
    currentURL,
  }: {
    groups: NavigationGroup[];
    currentURL: string;
  } = $props();
</script>

{#each groups as group (group.id)}
  <Sidebar.Group>
    <Sidebar.GroupLabel
      class={group.title ? "truncate" : undefined}
      title={group.title}
    >
      {group.label}
    </Sidebar.GroupLabel>
    <Sidebar.GroupContent>
      <Sidebar.Menu>
        {#each group.items as item (item.id)}
          {@const active = isNavigationItemActive(item, currentURL)}
          {@const Icon = item.icon}
          <Sidebar.MenuItem>
            <Sidebar.MenuButton isActive={active} tooltipContent={item.label}>
              {#snippet child({ props })}
                <Link
                  {...props}
                  href={item.href}
                  aria-current={item.showAriaCurrent && active
                    ? "page"
                    : undefined}
                >
                  <Icon />
                  <span>{item.label}</span>
                </Link>
              {/snippet}
            </Sidebar.MenuButton>
          </Sidebar.MenuItem>
        {/each}
      </Sidebar.Menu>
    </Sidebar.GroupContent>
  </Sidebar.Group>
{/each}
