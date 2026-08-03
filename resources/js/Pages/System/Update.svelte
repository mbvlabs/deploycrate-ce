<script lang="ts">
  import AlertTriangleIcon from '@lucide/svelte/icons/triangle-alert'
  import CheckCircleIcon from '@lucide/svelte/icons/circle-check'
  import DownloadIcon from '@lucide/svelte/icons/download'
  import { router } from '@inertiajs/svelte'

  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import { Progress } from '@/Components/ui/progress'
  import * as ScrollArea from '@/Components/ui/scroll-area'
  import { Separator } from '@/Components/ui/separator'
  import { Spinner } from '@/Components/ui/spinner'
  import StatusBadge from '@/Components/StatusBadge.svelte'
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
  }: {
    auth: { email: string }
    currentVersion: string
    update: UpdateStatus
  } = $props()

  let liveStatus = $state<UpdateStatusResponse | null>(null)
  let starting = $state(false)
  let reconnecting = $state(false)
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
        if (status.update.state !== 'queued' && status.update.state !== 'in_progress') return
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

  function versionLabel(version: string) {
    if (!version) return 'Unavailable'
    return version === 'dev' ? 'Development build' : `v${version}`
  }

  function stepLabel(step: string) {
    return step ? step.replaceAll('_', ' ') : 'Waiting'
  }

  function timestamp(value: string) {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'medium',
    }).format(new Date(value))
  }
</script>

<svelte:head>
  <title>Updates</title>
</svelte:head>

<DashboardLayout email={auth.email} version={currentVersion}>
  <div class="space-y-8">
    <section class="max-w-3xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight">DeployCrate CE updates</h1>
      <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
        Keep this development server on the current Cloudflare R2 build. Updates use a blue-green cutover and automatically restore the previous binary if health validation fails.
      </p>
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

    <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(20rem,0.8fr)]">
      <Card.Root>
        <Card.Header>
          <Card.Title>Release channel</Card.Title>
          <Card.Description>Development builds published to get-dev.deploycrate.com.</Card.Description>
        </Card.Header>
        <Card.Content class="space-y-5">
          <div class="border border-border bg-muted/30 p-4">
            <div class="flex items-center justify-between gap-3"><p class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground">Installed</p><StatusBadge status={update.state} /></div>
            <p class="mt-2 font-mono text-lg font-semibold">{versionLabel(currentVersion)}</p>
            {#if running}<div class="mt-4 space-y-2"><div class="flex justify-between gap-3 text-xs"><span class="capitalize">{stepLabel(update.currentStep)}</span><span>{updateProgress}%</span></div><Progress value={updateProgress} aria-label="System update progress" /></div>{/if}
          </div>
        </Card.Content>
        <Card.Footer class="justify-between gap-4 border-t border-border">
          <div class="text-xs text-muted-foreground">
            {#if running}
              Updating: {stepLabel(update.currentStep)}
            {:else}
              Download and install the latest development build.
            {/if}
          </div>
          <Button onclick={startUpdate} disabled={!canUpdate || starting} aria-busy={running || starting}>
            {#if running || starting}
              <Spinner />
              Updating
            {:else}
              <DownloadIcon />
              Update now
            {/if}
          </Button>
        </Card.Footer>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Action>
            {#if running}
              <StatusBadge status={reconnecting ? 'reconnecting' : 'running'} label={reconnecting ? 'Reconnecting' : 'Live'} />
            {/if}
          </Card.Action>
          <Card.Title>Update activity</Card.Title>
          <Card.Description>Deployment and cutover events.</Card.Description>
        </Card.Header>
        <Card.Content>
          {#if updateEvents.length === 0}
            <Empty.Root class="py-8"><Empty.Header><Empty.Title>No update activity</Empty.Title><Empty.Description>Deployment and cutover events will appear after an update starts.</Empty.Description></Empty.Header></Empty.Root>
          {:else}
            <ScrollArea.Root class="max-h-[30rem]"><ol class="space-y-0 pr-3">
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
            </ol></ScrollArea.Root>
          {/if}
        </Card.Content>
        {#if update.activeInstanceBefore || update.activeInstance}
          <Separator />
          <Card.Footer class="grid grid-cols-2 gap-4 text-xs">
            <div>
              <p class="text-muted-foreground">Previous instance</p>
              <p class="mt-1 font-mono">{update.activeInstanceBefore || 'Unknown'}</p>
            </div>
            <div>
              <p class="text-muted-foreground">Serving instance</p>
              <p class="mt-1 font-mono">{update.activeInstance || 'Unknown'}</p>
            </div>
          </Card.Footer>
        {/if}
      </Card.Root>
    </div>
  </div>
</DashboardLayout>
