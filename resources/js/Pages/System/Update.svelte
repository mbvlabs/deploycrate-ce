<script lang="ts">
  import AlertTriangleIcon from '@lucide/svelte/icons/triangle-alert'
  import CheckCircleIcon from '@lucide/svelte/icons/circle-check'
  import DownloadIcon from '@lucide/svelte/icons/download'
  import EyeIcon from '@lucide/svelte/icons/eye'
  import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw'
  import { router } from '@inertiajs/svelte'

  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import { Progress } from '@/Components/ui/progress'
  import * as ScrollArea from '@/Components/ui/scroll-area'
  import * as Table from '@/Components/ui/table'
  import DeploymentDialog, { type Deployment } from '@/Components/System/DeploymentDialog.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import { Spinner } from '@/Components/ui/spinner'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type UpdateEvent = {
    id: string
    message: string
    occurredAt: string
  }

  type UpdateStatus = {
    state: 'idle' | 'queued' | 'in_progress' | 'succeeded' | 'failed'
    currentStep: string
    targetVersion: string
    activeInstanceBefore: string
    activeInstance: string
    error: string
    startedAt?: string
    finishedAt?: string
    events: UpdateEvent[] | null
  }

  type UpdateStatusResponse = {
    currentVersion: string
    update: UpdateStatus
  }

  let {
    auth,
    currentVersion: initialCurrentVersion,
    update: initialUpdate,
    deployments,
  }: {
    auth: { email: string }
    currentVersion: string
    update: UpdateStatus
    deployments: Deployment[]
  } = $props()

  let liveStatus = $state<UpdateStatusResponse | null>(null)
  let starting = $state(false)
  let reconnecting = $state(false)
  let deploymentDialogOpen = $state(false)
  let selectedDeployment = $state<Deployment | null>(null)
  const currentVersion = $derived(liveStatus?.currentVersion ?? initialCurrentVersion)
  const update = $derived(liveStatus?.update ?? initialUpdate)
  const running = $derived(update.state === 'queued' || update.state === 'in_progress')
  const canUpdate = $derived(!running)
  const updateEvents = $derived(update.events ?? [])
  const updateProgress = $derived.by(() => {
    if (update.state === 'succeeded' || update.state === 'failed') return 100
    if (update.state === 'queued') return 10
    const steps = ['download', 'verify', 'install', 'start', 'health', 'cutover', 'cleanup']
    const index = steps.findIndex((step) => update.currentStep.toLowerCase().includes(step))
    return index < 0 ? 40 : Math.round(((index + 1) / steps.length) * 90)
  })

  $effect(() => {
    if (!running) return

    const abortController = new AbortController()
    let timer: number | undefined
    let retryDelay = 1000

    async function pollStatus() {
      try {
        const response = await window.fetch(routes.systemUpdateStatus(), {
          cache: 'no-store',
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
          signal: abortController.signal,
        })
        if (!response.ok) throw new Error(`Update status returned ${response.status}`)

        const status = (await response.json()) as UpdateStatusResponse
        if (abortController.signal.aborted) return

        liveStatus = status
        reconnecting = false
        retryDelay = 1000
        if (status.update.state !== 'queued' && status.update.state !== 'in_progress') {
          router.reload({ only: ['deployments'] })
          return
        }
      } catch {
        if (abortController.signal.aborted) return
        reconnecting = true
        retryDelay = Math.min(retryDelay * 2, 5000)
      }

      timer = window.setTimeout(pollStatus, retryDelay)
    }

    timer = window.setTimeout(pollStatus, retryDelay)

    return () => {
      abortController.abort()
      if (timer !== undefined) window.clearTimeout(timer)
    }
  })

  function startUpdate() {
    liveStatus = null
    router.post(
      routes.systemUpdateCreate(),
      {},
      {
        preserveScroll: true,
        onStart: () => (starting = true),
        onFinish: () => (starting = false),
      },
    )
  }

  function openDeployment(deployment: Deployment) {
    selectedDeployment = deployment
    deploymentDialogOpen = true
  }

  function versionLabel(version: string) {
    if (!version) return 'Unavailable'
    return version === 'dev' ? 'Development build' : `v${version.replace(/^v/, '')}`
  }

  function stepLabel(step: string) {
    return step ? step.replaceAll('_', ' ') : 'Waiting'
  }

  function timestamp(value: string) {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(new Date(value))
  }
</script>

<svelte:head>
  <title>Updates</title>
</svelte:head>

<DashboardLayout email={auth.email} version={currentVersion}>
  <div class="space-y-8">
    <section class="flex flex-col justify-between gap-5 lg:flex-row lg:items-end">
      <div class="max-w-3xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">Updates</h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          Keep DeployCrate CE current and review every system deployment from one place.
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="outline" disabled title="Update checks are not available yet">
          <RefreshCwIcon />
          Check for updates
        </Button>
        <Button onclick={startUpdate} disabled={!canUpdate || starting} aria-busy={running || starting}>
          {#if running || starting}
            <Spinner />
            Updating
          {:else}
            <DownloadIcon />
            Update
          {/if}
        </Button>
      </div>
    </section>

    {#if update.state === 'failed'}
      <Alert.Root variant="destructive">
        <AlertTriangleIcon />
        <Alert.Title>Update failed during {stepLabel(update.currentStep)}</Alert.Title>
        <Alert.Description>{update.error}</Alert.Description>
      </Alert.Root>
    {:else if update.state === 'succeeded'}
      <Alert.Root>
        <CheckCircleIcon />
        <Alert.Title>Update completed</Alert.Title>
        <Alert.Description>
          DeployCrate CE is serving from the {update.activeInstance} instance on {versionLabel(update.targetVersion)}.
        </Alert.Description>
      </Alert.Root>
    {/if}

    <Card.Root>
      <Card.Header>
        <Card.Action>
          {#if running}
            <StatusBadge status={reconnecting ? 'reconnecting' : 'running'} label={reconnecting ? 'Reconnecting' : 'Live'} />
          {:else}
            <StatusBadge status={update.state} />
          {/if}
        </Card.Action>
        <Card.Title>Current installation</Card.Title>
        <Card.Description>Development builds published to get-dev.deploycrate.com.</Card.Description>
      </Card.Header>
      <Card.Content class="grid gap-6 lg:grid-cols-[minmax(16rem,0.65fr)_minmax(0,1fr)]">
        <div class="space-y-4 border border-border bg-muted/30 p-4">
          <div>
            <p class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground">Installed version</p>
            <p class="mt-2 font-mono text-lg font-semibold">{versionLabel(currentVersion)}</p>
          </div>
          {#if running}
            <div class="space-y-2">
              <div class="flex justify-between gap-3 text-xs">
                <span class="capitalize">{stepLabel(update.currentStep)}</span>
                <span>{updateProgress}%</span>
              </div>
              <Progress value={updateProgress} aria-label="System update progress" />
            </div>
          {:else}
            <p class="text-xs leading-5 text-muted-foreground">Updates use a blue-green cutover and restore the previous binary if health validation fails.</p>
          {/if}
          {#if update.activeInstanceBefore || update.activeInstance}
            <dl class="grid grid-cols-2 gap-4 border-t border-border pt-4 text-xs">
              <div>
                <dt class="text-muted-foreground">Previous instance</dt>
                <dd class="mt-1 font-mono">{update.activeInstanceBefore || 'Unknown'}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Serving instance</dt>
                <dd class="mt-1 font-mono">{update.activeInstance || 'Unknown'}</dd>
              </div>
            </dl>
          {/if}
        </div>

        <div class="min-w-0">
          <h2 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Update activity</h2>
          {#if updateEvents.length === 0}
            <Empty.Root class="py-8">
              <Empty.Header>
                <Empty.Title>No update activity</Empty.Title>
                <Empty.Description>Deployment and cutover events will appear after an update starts.</Empty.Description>
              </Empty.Header>
            </Empty.Root>
          {:else}
            <ScrollArea.Root class="mt-4 max-h-72">
              <ol class="space-y-0 pr-3">
                {#each [...updateEvents].reverse() as event, index (event.id)}
                  <li class="grid grid-cols-[0.75rem_1fr] gap-3">
                    <div class="flex flex-col items-center">
                      <span class="mt-1.5 size-2 border border-primary bg-primary/20"></span>
                      {#if index < updateEvents.length - 1}<span class="w-px flex-1 bg-border"></span>{/if}
                    </div>
                    <div class="pb-4">
                      <p class="text-sm leading-5">{event.message}</p>
                      <p class="mt-1 text-[10px] uppercase tracking-[0.12em] text-muted-foreground">{timestamp(event.occurredAt)}</p>
                    </div>
                  </li>
                {/each}
              </ol>
            </ScrollArea.Root>
          {/if}
        </div>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Deployment history</Card.Title>
        <Card.Description>{deployments.length} deployment{deployments.length === 1 ? '' : 's'}, newest first.</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if deployments.length === 0}
          <Empty.Root class="border border-dashed border-border py-12">
            <Empty.Header>
              <Empty.Title>No system deployments</Empty.Title>
              <Empty.Description>Deployment history will appear after the first system release is installed.</Empty.Description>
            </Empty.Header>
          </Empty.Root>
        {:else}
          <div class="overflow-x-auto border border-border">
            <Table.Root class="min-w-[720px]">
              <Table.Header>
                <Table.Row>
                  <Table.Head>Version</Table.Head>
                  <Table.Head>Status</Table.Head>
                  <Table.Head>Change</Table.Head>
                  <Table.Head>Created</Table.Head>
                  <Table.Head class="w-24"><span class="sr-only">Actions</span></Table.Head>
                </Table.Row>
              </Table.Header>
              <Table.Body>
                {#each deployments as deployment (deployment.id)}
                  <Table.Row>
                    <Table.Cell>
                      <div class="flex items-center gap-2">
                        <span class="font-mono font-medium">{versionLabel(deployment.releaseVersion)}</span>
                        {#if deployment.active}<StatusBadge status="serving" />{/if}
                      </div>
                    </Table.Cell>
                    <Table.Cell><StatusBadge status={deployment.status} /></Table.Cell>
                    <Table.Cell class="max-w-md whitespace-normal">
                      <p class="line-clamp-2">{deployment.changeSummary || 'No change summary recorded.'}</p>
                    </Table.Cell>
                    <Table.Cell>{timestamp(deployment.createdAt)}</Table.Cell>
                    <Table.Cell class="text-right">
                      <Button variant="outline" size="sm" onclick={() => openDeployment(deployment)}>
                        <EyeIcon />
                        View
                      </Button>
                    </Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>

    <DeploymentDialog bind:open={deploymentDialogOpen} deployment={selectedDeployment} />
  </div>
</DashboardLayout>
