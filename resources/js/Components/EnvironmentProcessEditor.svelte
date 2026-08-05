<script lang="ts" module>
  export type ProcessInput = {
    name: string
    kind: 'web' | 'worker' | 'release'
    command: string | null
    arguments: string[]
    replicas: number
    containerPort?: number
    healthPath?: string
    timeoutSeconds?: number
  }
</script>

<script lang="ts">
  import FormField from '@/Components/FormField.svelte'
  import { Button } from '@/Components/ui/button'
  import { Input } from '@/Components/ui/input'
  import { Textarea } from '@/Components/ui/textarea'

  let { processes = $bindable(), errors = {} }: { processes: ProcessInput[]; errors?: Record<string, string> } = $props()

  const web = $derived(processes.find((process) => process.kind === 'web'))
  const workers = $derived(processes.filter((process) => process.kind === 'worker'))
  const release = $derived(processes.find((process) => process.kind === 'release'))

  function replace(current: ProcessInput, change: Partial<ProcessInput>) {
    processes = processes.map((process) => process === current ? { ...process, ...change } : process)
  }

  function argumentsFrom(value: string) {
    return value.split('\n').filter((argument) => argument.length > 0)
  }

  function addWorker() {
    let suffix = workers.length + 1
    while (processes.some((process) => process.name === `worker-${suffix}`)) suffix++
    processes = [...processes, { name: `worker-${suffix}`, kind: 'worker', command: '', arguments: [], replicas: 1 }]
  }

  function toggleRelease() {
    processes = release
      ? processes.filter((process) => process.kind !== 'release')
      : [...processes, { name: 'release', kind: 'release', command: '', arguments: [], replicas: 1, timeoutSeconds: 900 }]
  }

  function moveWorker(worker: ProcessInput, direction: number) {
    const indexes = processes.map((process, index) => process.kind === 'worker' ? index : -1).filter((index) => index >= 0)
    const current = processes.findIndex((process) => process === worker)
    const position = indexes.indexOf(current)
    const target = indexes[position + direction]
    if (target === undefined) return
    const next = [...processes]
    ;[next[current], next[target]] = [next[target], next[current]]
    processes = next
  }
</script>

<div class="space-y-5">
  {#if web}
    <div class="grid gap-4 border border-border p-4 sm:grid-cols-2">
      <div class="sm:col-span-2"><p class="font-medium">Web process</p><p class="mt-1 text-xs text-muted-foreground">The only process that publishes a port and receives public traffic.</p></div>
      <FormField label="Command override" error={errors['processes.0.command']}><Input value={web.command ?? ''} oninput={(event) => replace(web, { command: event.currentTarget.value.trim() || null })} placeholder="Use image default" /></FormField>
      <FormField label="Arguments, one per line"><Textarea value={web.arguments.join('\n')} oninput={(event) => replace(web, { arguments: argumentsFrom(event.currentTarget.value) })} placeholder="serve&#10;--http" /></FormField>
      <FormField label="Container port"><Input type="number" min="1" max="65535" value={web.containerPort ?? 8080} oninput={(event) => replace(web, { containerPort: Number(event.currentTarget.value) })} required /></FormField>
      <FormField label="Health path"><Input value={web.healthPath ?? ''} oninput={(event) => replace(web, { healthPath: event.currentTarget.value })} placeholder="/health" /></FormField>
    </div>
  {/if}

  <div class="space-y-3">
    <div class="flex items-center justify-between gap-3"><div><p class="font-medium">Workers</p><p class="mt-1 text-xs text-muted-foreground">Long-running private processes deployed on every target.</p></div><Button type="button" variant="outline" size="sm" onclick={addWorker}>Add worker</Button></div>
    {#each workers as worker, index}
      <div class="grid gap-4 border border-border p-4 sm:grid-cols-2">
        <FormField label="Name"><Input value={worker.name} oninput={(event) => replace(worker, { name: event.currentTarget.value })} required /></FormField>
        <FormField label="Replicas"><Input type="number" min="1" max="32" value={worker.replicas} oninput={(event) => replace(worker, { replicas: Number(event.currentTarget.value) })} required /></FormField>
        <FormField label="Executable"><Input value={worker.command ?? ''} oninput={(event) => replace(worker, { command: event.currentTarget.value })} required /></FormField>
        <FormField label="Arguments, one per line"><Textarea value={worker.arguments.join('\n')} oninput={(event) => replace(worker, { arguments: argumentsFrom(event.currentTarget.value) })} /></FormField>
        <div class="flex flex-wrap gap-2 sm:col-span-2"><Button type="button" size="sm" variant="ghost" disabled={index === 0} onclick={() => moveWorker(worker, -1)}>Move up</Button><Button type="button" size="sm" variant="ghost" disabled={index === workers.length - 1} onclick={() => moveWorker(worker, 1)}>Move down</Button><Button type="button" size="sm" variant="destructive" onclick={() => processes = processes.filter((process) => process !== worker)}>Archive worker</Button></div>
      </div>
    {:else}<p class="border border-dashed border-border p-4 text-sm text-muted-foreground">No worker processes configured.</p>{/each}
  </div>

  <div class="space-y-3 border border-warning/40 bg-warning/5 p-4">
    <div class="flex items-center justify-between gap-3"><div><p class="font-medium">Release command</p><p class="mt-1 text-xs text-muted-foreground">Runs once per Release before any target Deployment is created. Failed or uncertain outcomes require a confirmed manual retry.</p></div><Button type="button" size="sm" variant="outline" onclick={toggleRelease}>{release ? 'Remove' : 'Add release command'}</Button></div>
    {#if release}<div class="grid gap-4 sm:grid-cols-2"><FormField label="Executable"><Input value={release.command ?? ''} oninput={(event) => replace(release, { command: event.currentTarget.value })} required /></FormField><FormField label="Timeout seconds"><Input type="number" min="30" max="3600" value={release.timeoutSeconds ?? 900} oninput={(event) => replace(release, { timeoutSeconds: Number(event.currentTarget.value) })} required /></FormField><div class="sm:col-span-2"><FormField label="Arguments, one per line"><Textarea value={release.arguments.join('\n')} oninput={(event) => replace(release, { arguments: argumentsFrom(event.currentTarget.value) })} /></FormField></div></div>{/if}
  </div>
</div>
