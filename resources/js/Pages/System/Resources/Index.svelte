<script lang="ts">
  import { Link } from '@inertiajs/svelte'
  import * as Card from '@/Components/ui/card'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Resource = {
    id: string
    name: string
    category: string
    kind: string
    sharingScope: string
    originAddress: string
    originPort: number
    health: string
  }

  let { auth, resources }: { auth: { email: string }; resources: Resource[] } = $props()
  const label = (value: string) => value ? value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()) : 'Unknown'
  const endpoint = (address: string, port: number) => address && port ? `${address}:${port}` : 'Not configured'
</script>

<svelte:head><title>System resources</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header>
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight">Resources</h1>
      <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
        Durable infrastructure services bound to the DeployCrate system environment.
      </p>
    </header>

    <Card.Root>
      <Card.Content class="p-0">
        {#if resources.length === 0}
          <p class="p-6 text-sm text-muted-foreground">No active system resources were found.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="border-b border-border bg-muted/30 text-xs text-muted-foreground">
                <tr><th class="px-5 py-3">Resource</th><th class="px-5 py-3">Kind</th><th class="px-5 py-3">Origin</th><th class="px-5 py-3">Health</th></tr>
              </thead>
              <tbody>
                {#each resources as resource (resource.id)}
                  <tr class="border-b border-border last:border-0">
                    <td class="px-5 py-4"><Link class="font-medium text-primary hover:underline" href={routes.systemResource(resource.id)}>{resource.name}</Link></td>
                    <td class="px-5 py-4">{label(resource.kind)}</td>
                    <td class="px-5 py-4 font-mono text-xs">{endpoint(resource.originAddress, resource.originPort)}</td>
                    <td class="px-5 py-4">{label(resource.health)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>
  </div>
</DashboardLayout>
