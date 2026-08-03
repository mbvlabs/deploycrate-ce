<script lang="ts">
  import { Link, router, useForm } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Dialog from '@/Components/ui/dialog'
  import EnvironmentDeleteDialog from '@/Components/EnvironmentDeleteDialog.svelte'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type ResourceOption = { id: string; name: string; engine: string; database: string; endpointId: string; endpoint: string; credentialId?: string; credential: string; serverId?: string }
  type ServerOption = { id: string; name: string; kind: string; address: string }
  type Overview = { applicationId: string; applicationName: string; environment: { id: string; name: string; kind: string }; repository: string; reference: string; contextPath: string }
  let { auth, environment, options, setupUrl, setupError = '' }: { auth: { email: string }; environment: Overview; options: { resources: ResourceOption[]; servers: ServerOption[] }; setupUrl: string; setupError?: string } = $props()
  let step = $state(1)
  let selectedResource = $state('')
  let bulkSecretDialogOpen = $state(false)
  let bulkSecretText = $state('')
  let bulkSecretErrors = $state<string[]>([])
  let submissionError = $state('')
  const form = useForm(() => ({ serverId: options.servers[0]?.id ?? '', hostname: '', containerPort: 8080, healthPath: '/health', bpGoTargets: '', resources: [] as Array<{ resourceId: string; endpointId: string; credentialId?: string; alias: string; database: string; credentialProjection: 'connection_url' | 'individual_parts' }>, secrets: [] as Array<{ key: string; value: string }> }))
  const formErrorMessages = $derived([...new Set(Object.values($form.errors).map((error) => String(error)))])
  const availableResources = $derived(options.resources.filter((resource) => !resource.serverId || resource.serverId === $form.serverId))

  function addResource() {
    const option = availableResources.find((candidate) => `${candidate.id}:${candidate.endpointId}:${candidate.credentialId ?? ''}` === selectedResource)
    if (!option || $form.resources.some((resource) => resource.resourceId === option.id)) return
    $form.resources = [...$form.resources, { resourceId: option.id, endpointId: option.endpointId, credentialId: option.credentialId, alias: 'DATABASE', database: option.database, credentialProjection: 'connection_url' }]
  }
  function addSecret() { $form.secrets = [...$form.secrets, { key: '', value: '' }] }
  function selectServer(event: Event) {
    const serverId = (event.currentTarget as HTMLSelectElement).value
    $form.serverId = serverId
    const available = new Set(options.resources.filter((resource) => !resource.serverId || resource.serverId === serverId).map((resource) => resource.id))
    $form.resources = $form.resources.filter((resource) => available.has(resource.resourceId))
    selectedResource = ''
  }
  function resourceOwnedSecretKeys() {
    const keys = new Map<string, string>([['PORT', 'the DeployCrate runtime']])
    for (const resource of $form.resources) {
      const alias = resource.alias.trim().toUpperCase() || 'DATABASE'
      const suffixes = resource.credentialProjection === 'connection_url' ? ['URL'] : ['HOST', 'PORT', 'USER', 'TLS_MODE', 'PASSWORD']
      for (const suffix of suffixes) keys.set(`${alias}_${suffix}`, `the attached ${alias} Resource`)
    }
    return keys
  }
  function openBulkSecretDialog() {
    bulkSecretText = ''
    bulkSecretErrors = []
    bulkSecretDialogOpen = true
  }
  function closeBulkSecretDialog() {
    bulkSecretText = ''
    bulkSecretErrors = []
    bulkSecretDialogOpen = false
  }
  function importBulkSecrets(event: SubmitEvent) {
    event.preventDefault()
    const parsed: Array<{ key: string; value: string }> = []
    const errors: string[] = []
    const keys = new Set($form.secrets.map((secret) => secret.key.trim().toUpperCase()).filter((key) => key !== ''))
    const resourceKeys = resourceOwnedSecretKeys()
    for (const [index, line] of bulkSecretText.split(/\r?\n/).entries()) {
      if (line.trim() === '') continue
      const separator = line.indexOf('=')
      if (separator < 1) {
        errors.push(`Line ${index + 1} must use KEY=VALUE.`)
        continue
      }
      const key = line.slice(0, separator).trim().toUpperCase()
      const value = line.slice(separator + 1)
      if (!/^[A-Z_][A-Z0-9_]*$/.test(key)) errors.push(`Line ${index + 1} has an invalid key: ${key || '(empty)'}.`)
      else if (resourceKeys.has(key)) errors.push(`Line ${index + 1} uses ${key}, which is managed by ${resourceKeys.get(key)}.`)
      else if (keys.has(key)) errors.push(`Line ${index + 1} duplicates ${key}.`)
      else if (value === '') errors.push(`Line ${index + 1} must have a value.`)
      else {
        keys.add(key)
        parsed.push({ key, value })
      }
    }
    if (parsed.length === 0 && errors.length === 0) errors.push('Paste at least one KEY=VALUE line.')
    bulkSecretErrors = errors
    if (errors.length > 0) return
    $form.secrets = [...$form.secrets, ...parsed]
    closeBulkSecretDialog()
  }
  function setupErrors() {
    const errors: Record<string, string> = {}
    if ($form.serverId === '') errors.serverId = 'Select a runtime Server.'
    if ($form.hostname.trim() === '') errors.hostname = 'Primary hostname is required.'
    if ($form.containerPort < 1 || $form.containerPort > 65535) errors.containerPort = 'Container port must be between 1 and 65535.'
    const resourceAliases = new Set<string>()
    const resourceIDs = new Set<string>()
    for (const [index, resource] of $form.resources.entries()) {
      const alias = resource.alias.trim().toUpperCase() || 'DATABASE'
      if (resourceAliases.has(alias)) errors[`resources.${index}.alias`] = `${alias} is already used by another Resource.`
      if (resourceIDs.has(resource.resourceId)) errors[`resources.${index}.resourceId`] = 'This Resource is already attached.'
      if (!resource.credentialId) errors[`resources.${index}.credentialId`] = 'Select a Resource with an application credential.'
      if (resource.database.trim() === '') errors[`resources.${index}.database`] = 'Database name is required.'
      resourceAliases.add(alias)
      resourceIDs.add(resource.resourceId)
    }
    const resourceKeys = resourceOwnedSecretKeys()
    const secretKeys = new Set<string>()
    for (const [index, secret] of $form.secrets.entries()) {
      const key = secret.key.trim().toUpperCase()
      if (!/^[A-Z_][A-Z0-9_]*$/.test(key)) errors[`secrets.${index}.key`] = 'Secret key must match [A-Z_][A-Z0-9_]*.'
      else if (resourceKeys.has(key)) errors[`secrets.${index}.key`] = `${key} is managed by ${resourceKeys.get(key)}. Remove it or change the Resource alias.`
      else if (secretKeys.has(key)) errors[`secrets.${index}.key`] = `${key} is duplicated.`
      if (secret.value === '') errors[`secrets.${index}.value`] = 'Secret value is required.'
      secretKeys.add(key)
    }
    return errors
  }
  function showErrorStep(errors: Record<string, unknown>) {
    const fields = Object.keys(errors)
    if (fields.some((field) => field === 'hostname')) step = 1
    else if (fields.some((field) => ['serverId', 'containerPort', 'healthPath', 'bpGoTargets'].includes(field))) step = 2
    else if (fields.some((field) => field.startsWith('resources.'))) step = 3
    else if (fields.some((field) => field === 'key' || field === 'value' || field.startsWith('secrets.'))) step = 4
    else step = 5
  }
  function completeSetup() {
    submissionError = ''
    $form.clearErrors()
    const errors = setupErrors()
    if (Object.keys(errors).length > 0) {
      $form.setError(errors)
      showErrorStep(errors)
      return
    }
    if (!setupUrl || setupUrl.includes(':applicationID') || setupUrl.includes(':environmentID')) {
      submissionError = 'The Environment setup URL is invalid. Reload this page before trying again.'
      step = 5
      return
    }
    try {
      $form.patch(setupUrl, { onError: (responseErrors) => showErrorStep(responseErrors) })
    } catch (error) {
      submissionError = error instanceof Error ? error.message : 'The setup request could not be started.'
      step = 5
    }
  }
  function submit(event: SubmitEvent) {
    event.preventDefault()
    completeSetup()
  }
</script>

<svelte:head><title>Set up {environment.environment.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex items-start justify-between gap-4"><div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{environment.applicationName} · Environment setup · Step {step} of 5</p><h1 class="mt-3 text-3xl font-semibold">Prepare {environment.environment.name}</h1><p class="mt-2 text-sm text-muted-foreground">{environment.repository} at {environment.reference}, context {environment.contextPath}</p></div><div class="flex gap-2"><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentSourceEdit(environment.applicationId, environment.environment.id)}>Edit source</Link>{/snippet}</Button><EnvironmentDeleteDialog applicationId={environment.applicationId} environmentId={environment.environment.id} environmentName={environment.environment.name} /></div></header>
    <form onsubmit={submit}>
      <Card.Root>
        <Card.Header><Card.Title>{['Domain', 'Go runtime', 'Resources', 'Secrets', 'Review and deploy'][step - 1]}</Card.Title></Card.Header>
        <Card.Content class="space-y-5">
          {#if setupError || submissionError}<div class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive"><p class="font-medium">Environment setup failed</p><p class="mt-1 whitespace-pre-wrap">{setupError || submissionError}</p></div>{/if}
          {#if formErrorMessages.length > 0}<div class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive"><p class="font-medium">Setup needs attention</p>{#each formErrorMessages as error}<p class="mt-1">{error}</p>{/each}</div>{/if}
          {#if step === 1}
            <FormField label="Primary hostname" error={$form.errors.hostname}><Input bind:value={$form.hostname} placeholder="app.example.com" required /></FormField>
          {:else if step === 2}
            <div class="grid gap-5 sm:grid-cols-2"><div class="sm:col-span-2"><FormField label="Runtime Server" error={$form.errors.serverId}><select value={$form.serverId} onchange={selectServer} class="h-9 w-full border border-input bg-background px-3 text-sm" required>{#each options.servers as server}<option value={server.id}>{server.name} · {server.kind === 'worker' ? server.address : 'Control plane'}</option>{/each}</select></FormField></div><FormField label="Container port" error={$form.errors.containerPort}><Input type="number" min="1" max="65535" bind:value={$form.containerPort} required /></FormField><FormField label="HTTP health path" error={$form.errors.healthPath}><Input bind:value={$form.healthPath} placeholder="/health" /></FormField><FormField label="BP_GO_TARGETS" error={$form.errors.bpGoTargets}><Input bind:value={$form.bpGoTargets} placeholder="./cmd/server" /></FormField></div>
          {:else if step === 3}
            <div class="flex gap-2"><select bind:value={selectedResource} class="h-9 flex-1 border border-input bg-background px-3 text-sm"><option value="">Select an existing PostgreSQL Resource</option>{#each availableResources as option}<option value={`${option.id}:${option.endpointId}:${option.credentialId ?? ''}`}>{option.name} · {option.database} · {option.endpoint} · {option.credential || 'No credential'}</option>{/each}</select><Button type="button" variant="outline" onclick={addResource}>Attach</Button></div>
            {#each $form.resources as resource, index}<div class="grid gap-3 border border-border p-4 sm:grid-cols-2"><FormField label="Alias" error={$form.errors[`resources.${index}.alias`]}><Input bind:value={resource.alias} /></FormField><FormField label="Database" error={$form.errors[`resources.${index}.database`]}><Input bind:value={resource.database} readonly /></FormField><FormField label="Connection format" error={$form.errors[`resources.${index}.credentialProjection`]}><select bind:value={resource.credentialProjection} class="h-9 w-full border border-input bg-background px-3 text-sm"><option value="connection_url">Connection URL</option><option value="individual_parts">Individual parts</option></select></FormField><div class="border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">{resource.credentialProjection === 'connection_url' ? `${resource.alias.trim().toUpperCase() || 'DATABASE'}_URL` : ['HOST', 'PORT', 'USER', 'PASSWORD', 'TLS_MODE'].map((suffix) => `${resource.alias.trim().toUpperCase() || 'DATABASE'}_${suffix}`).join(', ')}</div>{#if $form.errors[`resources.${index}.credentialId`]}<p class="text-xs text-destructive sm:col-span-2">{$form.errors[`resources.${index}.credentialId`]}</p>{/if}<Button type="button" variant="ghost" onclick={() => $form.resources = $form.resources.filter((_, itemIndex) => itemIndex !== index)}>Remove</Button></div>{/each}
          {:else if step === 4}
            <div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={openBulkSecretDialog}>Import secrets</Button><Button type="button" variant="outline" onclick={addSecret}>Add secret</Button></div>
            {#each $form.secrets as secret, index}<div class="grid gap-3 border border-border p-4 sm:grid-cols-2"><FormField label="Key" error={$form.errors[`secrets.${index}.key`]}><Input bind:value={secret.key} autocomplete="off" /></FormField><FormField label="Value" error={$form.errors[`secrets.${index}.value`]}><Input type="password" bind:value={secret.value} autocomplete="new-password" /></FormField><Button type="button" variant="ghost" onclick={() => $form.secrets = $form.secrets.filter((_, itemIndex) => itemIndex !== index)}>Remove</Button></div>{/each}
          {:else}
            <div class="grid gap-4 text-sm sm:grid-cols-2"><p><span class="text-muted-foreground">Domain</span><br />{$form.hostname}</p><p><span class="text-muted-foreground">Runtime</span><br />Go · port {$form.containerPort}</p><p><span class="text-muted-foreground">Runtime Server</span><br />{options.servers.find((server) => server.id === $form.serverId)?.name ?? 'Unavailable'}</p><p><span class="text-muted-foreground">Resources</span><br />{$form.resources.length}</p><p><span class="text-muted-foreground">Secret keys</span><br />{$form.secrets.map((secret) => secret.key).join(', ') || 'None'}</p></div><p class="text-xs text-muted-foreground">Submitting resolves the configured GitHub ref to an exact commit and atomically queues the first build. Secret values are write-only.</p>
          {/if}
        </Card.Content>
        <Card.Footer class="justify-between border-t border-border"><Button type="button" variant="outline" disabled={step === 1 || $form.processing} onclick={() => step--}>Back</Button>{#if step < 5}<Button type="button" disabled={$form.processing} onclick={() => step++}>Continue</Button>{:else}<Button type="button" disabled={$form.processing} onclick={completeSetup}>{$form.processing ? 'Completing setup...' : 'Complete setup and deploy'}</Button>{/if}</Card.Footer>
      </Card.Root>
    </form>
  </div>

  <Dialog.Root bind:open={bulkSecretDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
    <form class="space-y-5" onsubmit={importBulkSecrets}>
      <div><h2 class="text-lg font-semibold">Import Environment secrets</h2><p class="mt-1 text-sm text-muted-foreground">Paste one secret per line using KEY=VALUE. Values may contain additional equals signs.</p></div>
      <label class="grid gap-2 text-xs">Secrets<textarea class="min-h-64 w-full border border-input bg-background px-3 py-2 font-mono text-xs" bind:value={bulkSecretText} placeholder={'DATABASE_URL=postgres://...\nAPI_TOKEN=...'} autocomplete="off" spellcheck="false"></textarea></label>
      {#if bulkSecretErrors.length > 0}<div class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">{#each bulkSecretErrors as error}<p>{error}</p>{/each}</div>{/if}
      <div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={closeBulkSecretDialog}>Cancel</Button><Button type="submit">Import secrets</Button></div>
    </form>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
