<script lang="ts">
  import { router } from '@inertiajs/svelte'

  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import JsonCode from '@/Components/JsonCode.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Job = {
    id: number
    state: string
    attempt: number
    maxAttempts: number
    attemptedAt: string | null
    attemptedBy: string[]
    createdAt: string
    finalizedAt: string | null
    scheduledAt: string
    priority: number
    args: unknown
    errors: Array<{ at: string; attempt: number; error: string; trace: string }>
    kind: string
    metadata: unknown
    queue: string
    tags: string[]
    uniqueKey: string
    uniqueStates: string[]
    canRun: boolean
    canRestart: boolean
    canCancel: boolean
    canDelete: boolean
  }

  let { auth, item }: { auth: { email: string }; item: Job } = $props()
  let activeAction = $state('')
  let actionDialogOpen = $state(false)
  let pendingAction = $state<{ action: string; url: string; title: string; description: string; confirmLabel: string; destructive: boolean; method: 'post' | 'delete' } | null>(null)
  const isActive = $derived(['available', 'running', 'scheduled', 'retryable', 'pending'].includes(item.state))

  function timestamp(value: string | null) {
    if (!value) return 'Not yet'
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
  }

  function stateClass(state: string) {
    if (state === 'completed') return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400'
    if (state === 'running') return 'border-sky-500/30 bg-sky-500/10 text-sky-400'
    if (state === 'available') return 'border-cyan-500/30 bg-cyan-500/10 text-cyan-400'
    if (state === 'scheduled' || state === 'pending') return 'border-amber-500/30 bg-amber-500/10 text-amber-400'
    if (state === 'retryable') return 'border-orange-500/30 bg-orange-500/10 text-orange-400'
    return 'border-red-500/30 bg-red-500/10 text-red-400'
  }

  function askForAction(action: NonNullable<typeof pendingAction>) {
    pendingAction = action
    actionDialogOpen = true
  }

  function confirmAction() {
    if (!pendingAction) return
    activeAction = pendingAction.action
    const options = {
      preserveScroll: true,
      onSuccess: () => (actionDialogOpen = false),
      onFinish: () => (activeAction = ''),
    }
    if (pendingAction.method === 'delete') router.delete(pendingAction.url, options)
    else router.post(pendingAction.url, {}, options)
  }

  $effect(() => {
    if (!isActive || activeAction) return
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') router.reload({ only: ['item'], preserveScroll: true })
    }, 3000)
    return () => window.clearInterval(timer)
  })
</script>

<svelte:head><title>System Task #{item.id}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div class="min-w-0">
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System Tasks</p>
        <div class="mt-3 flex flex-wrap items-center gap-3"><h1 class="break-all text-3xl font-semibold tracking-tight">{item.kind}</h1><span class={`inline-flex border px-2 py-0.5 text-xs capitalize ${stateClass(item.state)}`}>{item.state}</span></div>
        <p class="mt-3 font-mono text-xs text-muted-foreground">Task #{item.id}{isActive ? ' · live updates every three seconds' : ''}</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <Button href={routes.systemTasks()} variant="outline">Back to tasks</Button>
        {#if item.canRun}<Button disabled={Boolean(activeAction)} onclick={() => askForAction({ action: 'run', url: routes.systemTaskRun(item.id), title: 'Run task now?', description: 'Make this task immediately available to a background worker.', confirmLabel: 'Run task', destructive: false, method: 'post' })}>{activeAction === 'run' ? 'Starting...' : 'Run now'}</Button>{/if}
        {#if item.canRestart}<Button variant="outline" disabled={Boolean(activeAction)} onclick={() => askForAction({ action: 'restart', url: routes.systemTaskRetry(item.id), title: 'Restart task?', description: 'Restart this task and retain its existing attempt history.', confirmLabel: 'Restart task', destructive: false, method: 'post' })}>{activeAction === 'restart' ? 'Restarting...' : 'Restart'}</Button>{/if}
        {#if item.canCancel}<Button variant="outline" disabled={Boolean(activeAction)} onclick={() => askForAction({ action: 'cancel', url: routes.systemTaskCancel(item.id), title: 'Cancel task?', description: 'A running worker will receive a cancellation signal.', confirmLabel: 'Cancel task', destructive: true, method: 'post' })}>{activeAction === 'cancel' ? 'Cancelling...' : 'Cancel'}</Button>{/if}
        {#if item.canDelete}<Button variant="destructive" disabled={Boolean(activeAction)} onclick={() => askForAction({ action: 'delete', url: routes.systemTaskDestroy(item.id), title: 'Permanently delete task?', description: 'This removes the task and all of its attempt history. This cannot be undone.', confirmLabel: 'Delete task', destructive: true, method: 'delete' })}>{activeAction === 'delete' ? 'Deleting...' : 'Delete'}</Button>{/if}
        <Button variant="outline" disabled={Boolean(activeAction)} onclick={() => router.reload({ only: ['item'] })}>Refresh</Button>
      </div>
    </header>

    <section class="grid gap-4 lg:grid-cols-2">
      <Card.Root>
        <Card.Header><Card.Title>Execution</Card.Title><Card.Description>How River processes this System Task.</Card.Description></Card.Header>
        <Card.Content>
          <dl class="grid gap-5 sm:grid-cols-2">
            <div><dt class="text-xs text-muted-foreground">Queue</dt><dd class="mt-1 font-mono text-sm">{item.queue}</dd></div>
            <div><dt class="text-xs text-muted-foreground">Priority</dt><dd class="mt-1 text-sm">{item.priority} <span class="text-muted-foreground">(1 is highest)</span></dd></div>
            <div><dt class="text-xs text-muted-foreground">Attempts</dt><dd class="mt-1 text-sm tabular-nums">{item.attempt} of {item.maxAttempts}</dd></div>
            <div><dt class="text-xs text-muted-foreground">Tags</dt><dd class="mt-1 text-sm">{item.tags.length ? item.tags.join(', ') : 'None'}</dd></div>
          </dl>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>Timeline</Card.Title><Card.Description>Creation, scheduling, and execution timestamps.</Card.Description></Card.Header>
        <Card.Content>
          <dl class="grid gap-5 sm:grid-cols-2">
            <div><dt class="text-xs text-muted-foreground">Created</dt><dd class="mt-1 text-sm">{timestamp(item.createdAt)}</dd></div>
            <div><dt class="text-xs text-muted-foreground">Scheduled</dt><dd class="mt-1 text-sm">{timestamp(item.scheduledAt)}</dd></div>
            <div><dt class="text-xs text-muted-foreground">Last attempted</dt><dd class="mt-1 text-sm">{timestamp(item.attemptedAt)}</dd></div>
            <div><dt class="text-xs text-muted-foreground">Finalized</dt><dd class="mt-1 text-sm">{timestamp(item.finalizedAt)}</dd></div>
          </dl>
        </Card.Content>
      </Card.Root>
    </section>

    <section class="grid gap-4 xl:grid-cols-2">
      <Card.Root><Card.Header><Card.Title>Arguments</Card.Title><Card.Description>The JSON payload passed to the worker.</Card.Description></Card.Header><Card.Content><JsonCode value={item.args} /></Card.Content></Card.Root>
      <Card.Root><Card.Header><Card.Title>Metadata</Card.Title><Card.Description>River metadata recorded for this System Task.</Card.Description></Card.Header><Card.Content><JsonCode value={item.metadata} /></Card.Content></Card.Root>
      <Card.Root class="xl:col-span-2"><Card.Header><Card.Title>Attempt errors</Card.Title><Card.Description>{item.errors.length ? `${item.errors.length} failed attempt${item.errors.length === 1 ? '' : 's'} recorded.` : 'No failures have been recorded.'}</Card.Description></Card.Header><Card.Content><JsonCode value={item.errors} /></Card.Content></Card.Root>
    </section>

    <Card.Root>
      <Card.Header><Card.Title>River identifiers</Card.Title><Card.Description>Low-level execution and uniqueness values.</Card.Description></Card.Header>
      <Card.Content>
        <dl class="grid gap-5 sm:grid-cols-3">
          <div><dt class="text-xs text-muted-foreground">Attempted by</dt><dd class="mt-1 break-all font-mono text-xs">{item.attemptedBy.length ? item.attemptedBy.join(', ') : 'No client yet'}</dd></div>
          <div><dt class="text-xs text-muted-foreground">Unique key</dt><dd class="mt-1 break-all font-mono text-xs">{item.uniqueKey || 'Not unique'}</dd></div>
          <div><dt class="text-xs text-muted-foreground">Unique states</dt><dd class="mt-1 text-sm">{item.uniqueStates.length ? item.uniqueStates.join(', ') : 'Not configured'}</dd></div>
        </dl>
      </Card.Content>
    </Card.Root>
  </div>

  <ConfirmActionDialog
    bind:open={actionDialogOpen}
    title={pendingAction?.title ?? 'Confirm action'}
    description={pendingAction?.description ?? ''}
    confirmLabel={pendingAction?.confirmLabel ?? 'Confirm'}
    destructive={pendingAction?.destructive ?? false}
    processing={Boolean(activeAction)}
    onconfirm={confirmAction}
  />
</DashboardLayout>
