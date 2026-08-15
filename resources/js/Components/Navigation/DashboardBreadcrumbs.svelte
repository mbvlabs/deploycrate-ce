<script lang="ts">
  import { Link } from "@inertiajs/svelte";

  import * as Breadcrumb from "@/Components/ui/breadcrumb";
  import type { BreadcrumbItem } from "./dashboard-navigation";

  let { items }: { items: BreadcrumbItem[] } = $props();
</script>

{#if items.length > 0}
  <Breadcrumb.Root class="hidden min-w-0 sm:block">
    <Breadcrumb.List class="flex-nowrap">
      {#each items as crumb, index}
        <Breadcrumb.Item class="min-w-0">
          {#if crumb.href && index < items.length - 1}
            <Breadcrumb.Link>
              {#snippet child({ props })}
                <Link {...props} href={crumb.href}>{crumb.label}</Link>
              {/snippet}
            </Breadcrumb.Link>
          {:else}
            <Breadcrumb.Page class="truncate">{crumb.label}</Breadcrumb.Page>
          {/if}
        </Breadcrumb.Item>
        {#if index < items.length - 1}<Breadcrumb.Separator />{/if}
      {/each}
    </Breadcrumb.List>
  </Breadcrumb.Root>
{/if}
