<script lang="ts">
  import { useForm } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import { Input } from '@/Components/ui/input'
  import { Label } from '@/Components/ui/label'
  import { Spinner } from '@/Components/ui/spinner'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'
  let { auth, application }: { auth: { email: string }; application: { id: string; name: string; slug: string } } = $props()
  const form = useForm(() => ({ name: application.name, slug: application.slug }))
  function submit(event: SubmitEvent) { event.preventDefault(); $form.patch(routes.applicationUpdate(application.id)) }
</script>
<svelte:head><title>Edit {application.name}</title></svelte:head>
<DashboardLayout email={auth.email}><form class="mx-auto max-w-2xl" onsubmit={submit}><Card.Root><Card.Header><Card.Title>Edit application</Card.Title><Card.Description>Presentation details only. Source access is managed separately.</Card.Description></Card.Header><Card.Content class="grid gap-5"><div class="grid gap-2"><Label for="application-name">Name</Label><Input id="application-name" bind:value={$form.name} required /></div><div class="grid gap-2"><Label for="application-slug">Slug</Label><Input id="application-slug" bind:value={$form.slug} required /></div></Card.Content><Card.Footer class="border-t border-border"><Button type="submit" disabled={$form.processing} aria-busy={$form.processing}>{#if $form.processing}<Spinner />{/if}Save changes</Button></Card.Footer></Card.Root></form></DashboardLayout>
