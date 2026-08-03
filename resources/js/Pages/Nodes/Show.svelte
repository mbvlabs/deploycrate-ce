<script lang="ts">
  import { router, useForm } from '@inertiajs/svelte'
  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import { Checkbox } from '@/Components/ui/checkbox'
  import { Progress } from '@/Components/ui/progress'
  import { Spinner } from '@/Components/ui/spinner'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Node = { id: string; serverId: string; name: string; address: string; sshPort: number; state: string; currentStep: string; error: string; fingerprint: string; hostKeyConfirmedAt: string | null; wireGuardAddress: string; installerVersion: string; jobId: number | null; createdAt: string; startedAt: string | null; completedAt: string | null; configured: boolean; capabilities: Record<string, boolean> }
  let { auth, node }: { auth: { email: string }; node: Node } = $props()
  let confirmed = $state(false)
  let retrying = $state(false)
  const confirmForm = useForm(() => ({ fingerprint: node.fingerprint }))
  function confirm() { $confirmForm.post(routes.nodeConfirm(node.id)) }
  function retry() { retrying = true; router.post(routes.nodeRetry(node.id), {}, { onFinish: () => (retrying = false) }) }
  function timestamp(value: string | null) { return value ? new Date(value).toLocaleString() : 'Not yet' }
  const progressValue = $derived(node.state === 'ready' ? 100 : node.state === 'awaiting_confirmation' ? 15 : node.state === 'failed' ? 100 : 55)

  $effect(() => {
    if (['awaiting_confirmation', 'ready', 'failed'].includes(node.state)) return
    const timer = window.setInterval(() => router.reload({ only: ['node'], preserveScroll: true }), 3000)
    return () => window.clearInterval(timer)
  })
</script>

<svelte:head><title>{node.name}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-6">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"><div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Node</p><h1 class="mt-3 text-3xl font-semibold">{node.name}</h1><p class="mt-2 font-mono text-xs text-muted-foreground">{node.address}:{node.sshPort} · {node.wireGuardAddress}</p></div><Button href={routes.nodes()} variant="outline">Back to nodes</Button></header>

    {#if node.state === 'awaiting_confirmation'}
      <Card.Root class="border-amber-500/30">
        <Card.Header><Card.Title>Confirm SSH host identity</Card.Title><Card.Description>Compare this fingerprint with the value shown in your VPS provider console before continuing.</Card.Description></Card.Header>
        <Card.Content class="space-y-4"><div class="break-all border border-border bg-muted/20 p-4 font-mono text-sm">{node.fingerprint}</div><label class="flex cursor-pointer items-start gap-3 text-sm"><Checkbox bind:checked={confirmed} class="mt-1" /><span>I verified that this fingerprint belongs to the new VPS.</span></label></Card.Content>
        <Card.Footer class="justify-end"><Button onclick={confirm} disabled={!confirmed || $confirmForm.processing} aria-busy={$confirmForm.processing}>{#if $confirmForm.processing}<Spinner />{/if}Confirm and install</Button></Card.Footer>
      </Card.Root>
    {/if}

    {#if node.error}
      <Alert.Root variant="destructive"><Alert.Title>Enrollment failed</Alert.Title><Alert.Description>{node.error}</Alert.Description><Alert.Action><Button onclick={retry} disabled={node.state !== 'failed' || retrying} aria-busy={retrying}>{#if retrying}<Spinner />{/if}Retry enrollment</Button></Alert.Action></Alert.Root>
    {/if}

    <div class="grid gap-6 lg:grid-cols-2">
      <Card.Root><Card.Header><Card.Title>Status</Card.Title><Card.Description>Durable enrollment progress.</Card.Description></Card.Header><Card.Content class="grid gap-4 text-sm"><div class="space-y-2"><div class="flex items-center justify-between gap-3"><StatusBadge status={node.state} /><span class="capitalize text-muted-foreground">{node.currentStep.replaceAll('_', ' ')}</span></div><Progress value={progressValue} aria-label="Node enrollment progress" /></div><div><p class="text-xs text-muted-foreground">Configured</p><p class="mt-1">{node.configured ? 'Yes' : 'No'}</p></div></Card.Content></Card.Root>
      <Card.Root><Card.Header><Card.Title>Capabilities</Card.Title><Card.Description>Workloads this node may accept.</Card.Description></Card.Header><Card.Content class="flex flex-wrap gap-2">{#each Object.entries(node.capabilities).filter(([key, enabled]) => key !== 'telemetry' && enabled) as [capability]}<span class="border border-border px-2 py-1 text-xs capitalize">{capability}</span>{/each}</Card.Content></Card.Root>
      <Card.Root><Card.Header><Card.Title>OpenTelemetry endpoint</Card.Title><Card.Description>Node-local OTLP/HTTP receiver available only through WireGuard and to workloads on this node.</Card.Description></Card.Header><Card.Content><p class="break-all font-mono text-sm">http://{node.wireGuardAddress}:4318</p><p class="mt-2 text-xs text-muted-foreground">Protocol: http/protobuf</p></Card.Content></Card.Root>
      <Card.Root><Card.Header><Card.Title>Timeline</Card.Title><Card.Description>Enrollment lifecycle timestamps.</Card.Description></Card.Header><Card.Content class="grid gap-4 text-sm"><div><p class="text-xs text-muted-foreground">Created</p><p class="mt-1">{timestamp(node.createdAt)}</p></div><div><p class="text-xs text-muted-foreground">Started</p><p class="mt-1">{timestamp(node.startedAt)}</p></div><div><p class="text-xs text-muted-foreground">Completed</p><p class="mt-1">{timestamp(node.completedAt)}</p></div></Card.Content></Card.Root>
    </div>
  </div>
</DashboardLayout>
