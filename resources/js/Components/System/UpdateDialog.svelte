<script lang="ts" module>
  export type UpdateEvent = {
    id: string
    message: string
    occurredAt: string
  }

  export type UpdateStatus = {
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
</script>

<script lang="ts">
  import AlertTriangleIcon from '@lucide/svelte/icons/triangle-alert'
  import CheckCircleIcon from '@lucide/svelte/icons/circle-check'
  import DownloadIcon from '@lucide/svelte/icons/download'

  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Dialog from '@/Components/ui/dialog'
  import * as Empty from '@/Components/ui/empty'
  import { Progress } from '@/Components/ui/progress'
  import * as ScrollArea from '@/Components/ui/scroll-area'
  import { Separator } from '@/Components/ui/separator'
  import { Spinner } from '@/Components/ui/spinner'
  import StatusBadge from '@/Components/StatusBadge.svelte'

  let {
    open = $bindable(false),
    tracking,
    currentVersion,
    update,
    running,
    starting,
    reconnecting,
    canUpdate,
    onStart,
  }: {
    open?: boolean
    tracking: boolean
    currentVersion: string
    update: UpdateStatus
    running: boolean
    starting: boolean
    reconnecting: boolean
    canUpdate: boolean
    onStart: () => void
  } = $props()

  const updateEvents = $derived(update.events ?? [])
  const visibleEvents = $derived(starting && !running ? [] : updateEvents)
  const updateProgress = $derived.by(() => {
    if (update.state === 'succeeded' || update.state === 'failed') return 100
    if (update.state === 'queued') return 10
    const steps = ['download', 'verify', 'install', 'start', 'health', 'cutover', 'cleanup']
    const index = steps.findIndex((step) => update.currentStep.toLowerCase().includes(step))
    return index < 0 ? 40 : Math.round(((index + 1) / steps.length) * 90)
  })

  function setOpen(next: boolean) {
    if (!next && (running || starting)) return
    open = next
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

<Dialog.Root {open} onOpenChange={setOpen}>
  <Dialog.Content class="sm:max-w-3xl" showCloseButton={!running && !starting}>
    <Dialog.Header>
      <div class="flex flex-wrap items-center gap-3 pr-8">
        <Dialog.Title>{tracking ? 'DeployCrate CE update' : 'Update DeployCrate CE'}</Dialog.Title>
        {#if tracking}
          <StatusBadge
            status={starting && !running ? 'queued' : reconnecting && running ? 'reconnecting' : update.state}
            label={starting && !running ? 'Starting' : reconnecting && running ? 'Reconnecting' : undefined}
          />
        {/if}
      </div>
      <Dialog.Description>
        {#if tracking}
          Follow the blue-green deployment and traffic cutover without leaving this dialog.
        {:else}
          Install the latest development build using the inactive service slot.
        {/if}
      </Dialog.Description>
    </Dialog.Header>

    {#if !tracking}
      <div class="space-y-5">
        <div class="border border-border bg-muted/30 p-4">
          <p class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground">Installed version</p>
          <p class="mt-2 font-mono text-lg font-semibold">{versionLabel(currentVersion)}</p>
        </div>
        <p class="text-sm leading-6 text-muted-foreground">
          DeployCrate downloads and verifies the new binary, starts it in the inactive slot, switches traffic only after health validation, and restores the current binary if validation fails.
        </p>
      </div>
      <Dialog.Footer>
        <Button type="button" variant="outline" onclick={() => setOpen(false)}>Cancel</Button>
        <Button type="button" onclick={onStart} disabled={!canUpdate || starting}>
          <DownloadIcon />
          Start update
        </Button>
      </Dialog.Footer>
    {:else}
      <div class="space-y-5">
        {#if !starting && update.state === 'failed'}
          <Alert.Root variant="destructive">
            <AlertTriangleIcon />
            <Alert.Title>Update failed during {stepLabel(update.currentStep)}</Alert.Title>
            <Alert.Description>{update.error}</Alert.Description>
          </Alert.Root>
        {:else if !starting && update.state === 'succeeded'}
          <Alert.Root>
            <CheckCircleIcon />
            <Alert.Title>Update completed</Alert.Title>
            <Alert.Description>
              DeployCrate CE is serving from the {update.activeInstance} instance on {versionLabel(update.targetVersion)}.
            </Alert.Description>
          </Alert.Root>
        {/if}

        <dl class="grid gap-4 border border-border bg-muted/20 p-4 text-xs sm:grid-cols-3">
          <div>
            <dt class="text-muted-foreground">Installed version</dt>
            <dd class="mt-1 font-mono">{versionLabel(currentVersion)}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Previous instance</dt>
            <dd class="mt-1 font-mono">{update.activeInstanceBefore || 'Unknown'}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Serving instance</dt>
            <dd class="mt-1 font-mono">{update.activeInstance || 'Unknown'}</dd>
          </div>
        </dl>

        {#if running || starting}
          <div class="space-y-2">
            <div class="flex justify-between gap-3 text-xs">
              <span class="flex items-center gap-2 capitalize"><Spinner />{starting && !running ? 'Starting update' : stepLabel(update.currentStep)}</span>
              <span>{starting && !running ? 0 : updateProgress}%</span>
            </div>
            <Progress value={starting && !running ? 0 : updateProgress} aria-label="System update progress" />
          </div>
        {/if}

        <Separator />

        <section>
          <h2 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Update activity</h2>
          {#if visibleEvents.length === 0}
            <Empty.Root class="py-8">
              <Empty.Header>
                <Empty.Title>{starting ? 'Starting update' : 'No update activity'}</Empty.Title>
                <Empty.Description>Deployment and cutover events will appear here as they are recorded.</Empty.Description>
              </Empty.Header>
            </Empty.Root>
          {:else}
            <ScrollArea.Root class="mt-4 max-h-72">
              <ol class="space-y-0 pr-3">
                {#each [...visibleEvents].reverse() as event, index (event.id)}
                  <li class="grid grid-cols-[0.75rem_1fr] gap-3">
                    <div class="flex flex-col items-center">
                      <span class="mt-1.5 size-2 border border-primary bg-primary/20"></span>
                      {#if index < visibleEvents.length - 1}<span class="w-px flex-1 bg-border"></span>{/if}
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
        </section>
      </div>

      {#if !running && !starting}
        <Dialog.Footer><Button type="button" onclick={() => setOpen(false)}>Close</Button></Dialog.Footer>
      {/if}
    {/if}
  </Dialog.Content>
</Dialog.Root>
