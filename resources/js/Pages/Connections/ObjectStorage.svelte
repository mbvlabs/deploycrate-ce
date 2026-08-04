<script lang="ts">
  import CloudIcon from '@lucide/svelte/icons/cloud'
  import { Link, useForm } from '@inertiajs/svelte'

  import FormField from '@/Components/FormField.svelte'
  import PageHeader from '@/Components/PageHeader.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Dialog from '@/Components/ui/dialog'
  import * as Empty from '@/Components/ui/empty'
  import { Input } from '@/Components/ui/input'
  import * as NativeSelect from '@/Components/ui/native-select'
  import { Spinner } from '@/Components/ui/spinner'
  import { Switch } from '@/Components/ui/switch'
  import * as Table from '@/Components/ui/table'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Destination = {
    id: string
    name: string
    provider: string
    endpoint: string
    region: string
    bucket: string
    prefix: string
    verifiedAt: string | null
    lastUsedAt: string | null
  }

  let { auth, destinations }: { auth: { email: string }; destinations: Destination[] } = $props()
  let createDialogOpen = $state(false)
  const form = useForm({
    name: '', provider: 's3', endpoint: '', region: 'us-east-1', bucket: '', prefix: '',
    forcePathStyle: false, accessKeyId: '', secretAccessKey: '',
  })

  function openCreateDialog() {
    $form.reset()
    createDialogOpen = true
  }

  function selectProvider() {
    if ($form.provider === 'r2') {
      $form.region = 'auto'
      $form.forcePathStyle = true
    } else {
      if ($form.region === 'auto') $form.region = 'us-east-1'
      $form.forcePathStyle = false
    }
  }

  function createDestination(event: SubmitEvent) {
    event.preventDefault()
    $form.post(routes.objectStorageCreate(), {
      preserveScroll: true,
      onSuccess: () => { createDialogOpen = false; $form.reset() },
      onError: () => (createDialogOpen = true),
    })
  }

</script>

<svelte:head><title>Object Storage</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader eyebrow="Connections" title="Object Storage" description="Verified S3 and Cloudflare R2 destinations for server and Resource backups.">
      {#snippet actions()}<Button type="button" onclick={openCreateDialog}>Add destination</Button>{/snippet}
    </PageHeader>

    <Card.Root>
      <Card.Header><Card.Title>Backup destinations</Card.Title><Card.Description>{destinations.length} verified destination{destinations.length === 1 ? '' : 's'} available to backup policies.</Card.Description></Card.Header>
      <Card.Content>
        {#if destinations.length === 0}
          <Empty.Root class="border border-dashed border-border py-12">
            <Empty.Header><Empty.Media variant="icon"><CloudIcon /></Empty.Media><Empty.Title>No Object Storage destinations</Empty.Title><Empty.Description>Add an S3 or Cloudflare R2 bucket before enabling backup policies.</Empty.Description></Empty.Header>
          </Empty.Root>
        {:else}
          <div class="overflow-hidden border border-border">
            <Table.Root class="min-w-[640px]">
              <Table.Header class="bg-muted/30"><Table.Row><Table.Head>Destination</Table.Head><Table.Head>Provider</Table.Head><Table.Head>Bucket</Table.Head><Table.Head>Status</Table.Head><Table.Head class="text-right">Actions</Table.Head></Table.Row></Table.Header>
              <Table.Body>
                {#each destinations as destination (destination.id)}
                  <Table.Row>
                    <Table.Cell><Link class="font-medium text-primary hover:underline" href={routes.objectStorageShow(destination.id)}>{destination.name}</Link></Table.Cell>
                    <Table.Cell class="uppercase">{destination.provider}</Table.Cell>
                    <Table.Cell class="font-mono text-xs">{destination.bucket}</Table.Cell>
                    <Table.Cell><StatusBadge status="verified" /></Table.Cell>
                    <Table.Cell class="text-right"><Button size="sm" variant="outline">{#snippet child({ props })}<Link {...props} href={routes.objectStorageShow(destination.id)}>View</Link>{/snippet}</Button></Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>
  </div>

  <Dialog.Root bind:open={createDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl" showCloseButton={!$form.processing}>
      <form class="grid gap-5" onsubmit={createDestination}>
        <Dialog.Header><Dialog.Title>Add Object Storage destination</Dialog.Title><Dialog.Description>DeployCrate verifies read, write, list, and delete access before encrypting the credentials. Export the generated recovery material from the table after creation.</Dialog.Description></Dialog.Header>
        <div class="grid gap-5 sm:grid-cols-2">
          <FormField label="Provider" error={$form.errors.provider}><NativeSelect.Root bind:value={$form.provider} onchange={selectProvider} class="w-full" disabled={$form.processing}><NativeSelect.Option value="s3">Amazon S3 or compatible</NativeSelect.Option><NativeSelect.Option value="r2">Cloudflare R2</NativeSelect.Option></NativeSelect.Root></FormField>
          <FormField label="Display name" error={$form.errors.name}><Input bind:value={$form.name} placeholder="Production backups" required disabled={$form.processing} /></FormField>
          <FormField label="Bucket" error={$form.errors.bucket}><Input bind:value={$form.bucket} placeholder="deploycrate-backups" required disabled={$form.processing} /></FormField>
          <FormField label="Region" error={$form.errors.region}><Input bind:value={$form.region} placeholder="us-east-1" readonly={$form.provider === 'r2'} required disabled={$form.processing} /></FormField>
          <div class="sm:col-span-2"><FormField label={$form.provider === 'r2' ? 'R2 endpoint' : 'Custom endpoint'} error={$form.errors.endpoint}><Input type="url" bind:value={$form.endpoint} placeholder={$form.provider === 'r2' ? 'https://account-id.r2.cloudflarestorage.com' : 'https://s3.example.com'} required={$form.provider === 'r2'} disabled={$form.processing} /><p class="mt-2 text-xs text-muted-foreground">Leave empty for the standard Amazon S3 endpoint.</p></FormField></div>
          <div class="sm:col-span-2"><FormField label="Prefix" error={$form.errors.prefix}><Input bind:value={$form.prefix} placeholder="deploycrate/production" disabled={$form.processing} /><p class="mt-2 text-xs text-muted-foreground">Optional path inside the bucket. Do not include leading or trailing slashes.</p></FormField></div>
          <FormField label="Access key ID" error={$form.errors.accessKeyId}><Input bind:value={$form.accessKeyId} autocomplete="off" required disabled={$form.processing} /></FormField>
          <FormField label="Secret access key" error={$form.errors.secretAccessKey}><Input type="password" bind:value={$form.secretAccessKey} autocomplete="new-password" required disabled={$form.processing} /></FormField>
        </div>
        <div class="flex items-center justify-between gap-4 border border-border p-4"><div><p class="text-sm font-medium">Force path-style requests</p><p class="mt-1 text-xs text-muted-foreground">Required by R2 and some S3-compatible providers.</p></div><Switch bind:checked={$form.forcePathStyle} disabled={$form.processing || $form.provider === 'r2'} aria-label="Force path-style requests" /></div>
        <Dialog.Footer><Button type="button" variant="outline" disabled={$form.processing} onclick={() => (createDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={$form.processing} aria-busy={$form.processing}>{#if $form.processing}<Spinner />{/if}Verify and add</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

</DashboardLayout>
