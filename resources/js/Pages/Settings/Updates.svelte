<script lang="ts">
  import AlertTriangleIcon from '@lucide/svelte/icons/triangle-alert'
  import CheckCircleIcon from '@lucide/svelte/icons/circle-check'
  import DownloadIcon from '@lucide/svelte/icons/download'
  import ExternalLinkIcon from '@lucide/svelte/icons/external-link'
  import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle'
  import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw'
  import { router } from '@inertiajs/svelte'

  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import { Separator } from '@/Components/ui/separator'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type ReleaseStatus = {
    currentVersion: string
    latestVersion: string
    updateAvailable: boolean
    releaseUrl: string
  }

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

  let {
    auth,
    release,
    releaseError = '',
    update,
  }: {
    auth: { email: string }
    release: ReleaseStatus
    releaseError?: string
    update: UpdateStatus
  } = $props()

  let starting = $state(false)
  const running = $derived(update.state === 'queued' || update.state === 'in_progress')
  const canUpdate = $derived(!releaseError && release.updateAvailable && !running)
  const updateEvents = $derived(update.events ?? [])

  $effect(() => {
    if (!running) return

    const timer = window.setInterval(() => {
      router.reload({ only: ['release', 'releaseError', 'update'], preserveScroll: true })
    }, 1000)

    return () => window.clearInterval(timer)
  })

  function startUpdate() {
    router.post(
      routes.selfUpdateSettingsCreate(),
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

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <section class="max-w-3xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Settings</p>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight">DeployCrate CE updates</h1>
      <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
        Keep this server on the latest verified community edition release. Updates use a blue-green cutover and automatically restore the previous binary if health validation fails.
      </p>
    </section>

    {#if releaseError}
      <Alert.Root variant="destructive">
        <AlertTriangleIcon />
        <Alert.Title>Release information is unavailable</Alert.Title>
        <Alert.Description>{releaseError}</Alert.Description>
      </Alert.Root>
    {/if}

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
          <Card.Description>Latest stable releases from mbvlabs/deploycrate-ce on GitHub.</Card.Description>
        </Card.Header>
        <Card.Content class="space-y-5">
          <div class="grid gap-4 sm:grid-cols-2">
            <div class="border border-border bg-muted/30 p-4">
              <p class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground">Installed</p>
              <p class="mt-2 font-mono text-lg font-semibold">{versionLabel(release.currentVersion)}</p>
            </div>
            <div class="border border-border bg-muted/30 p-4">
              <p class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground">Latest</p>
              <p class="mt-2 font-mono text-lg font-semibold">{versionLabel(release.latestVersion)}</p>
            </div>
          </div>

          {#if release.releaseUrl}
            <a
              class="inline-flex items-center gap-1.5 text-xs font-medium text-primary underline-offset-4 hover:underline"
              href={release.releaseUrl}
              target="_blank"
              rel="noreferrer"
            >
              View release on GitHub
              <ExternalLinkIcon class="size-3.5" />
            </a>
          {/if}
        </Card.Content>
        <Card.Footer class="justify-between gap-4 border-t border-border">
          <div class="text-xs text-muted-foreground">
            {#if running}
              Updating: {stepLabel(update.currentStep)}
            {:else if release.updateAvailable}
              A newer release is ready to install.
            {:else if !releaseError}
              This installation is current.
            {/if}
          </div>
          <Button onclick={startUpdate} disabled={!canUpdate || starting}>
            {#if running || starting}
              <LoaderCircleIcon class="animate-spin" />
              Updating
            {:else if release.updateAvailable}
              <DownloadIcon />
              Update now
            {:else}
              <CheckCircleIcon />
              Up to date
            {/if}
          </Button>
        </Card.Footer>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Action>
            {#if running}
              <span class="inline-flex items-center gap-1.5 text-[10px] uppercase tracking-[0.14em] text-primary">
                <RefreshCwIcon class="size-3 animate-spin" />
                Live
              </span>
            {/if}
          </Card.Action>
          <Card.Title>Update activity</Card.Title>
          <Card.Description>Verified deployment and cutover events.</Card.Description>
        </Card.Header>
        <Card.Content>
          {#if updateEvents.length === 0}
            <p class="text-sm text-muted-foreground">No update has been started on this installation.</p>
          {:else}
            <ol class="space-y-0">
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
