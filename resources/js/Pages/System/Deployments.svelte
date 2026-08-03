<script lang="ts">
  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import * as ScrollArea from '@/Components/ui/scroll-area'
  import { Separator } from '@/Components/ui/separator'
  import JsonCode from '@/Components/JsonCode.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'

  type DeploymentEvent = {
    id: string
    sequence: number
    eventType: string
    status: string
    step: string
    message: string
    metadata: unknown
    error: string
    occurredAt: string
  }

  type Deployment = {
    id: string
    createdAt: string
    updatedAt: string
    attempt: number
    strategy: unknown
    runtimeConfiguration: unknown
    status: string
    currentStep: string
    startedAt: string | null
    finishedAt: string | null
    error: string
    releaseId: string
    releaseVersion: string
    sourceRevision: string
    artifactReference: string
    artifactDigest: string
    changeId: string
    changeSequence: number
    changeKind: string
    changeSummary: string
    changeStatus: string
    triggerType: string
    requestedAt: string
    instanceId: string
    instanceService: string
    instanceSlot: string
    instanceState: string
    instancePort: number
    instanceObservedAt: string | null
    active: boolean
    events: DeploymentEvent[]
  }

  let { auth, deployments }: { auth: { email: string }; deployments: Deployment[] } = $props()
  let selectedDeploymentId = $state('')
  const selectedDeployment = $derived(
    deployments.find((deployment) => deployment.id === selectedDeploymentId) ?? deployments[0],
  )

  const stateLabel = (value: string) => value ? value.replaceAll('_', ' ') : 'Unknown'
  const versionLabel = (version: string) => version ? `v${version.replace(/^v/, '')}` : 'Development build'
  const timestamp = (value: string | null) => value ? new Date(value).toLocaleString() : 'Not recorded'
  const hasJSONFields = (value: unknown) => Boolean(
    value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length,
  )
</script>

<svelte:head>
  <title>System deployments</title>
</svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <section class="max-w-3xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight">Deployments</h1>
      <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
        Complete deployment history for the system environment, including release, change, runtime, instance, and event records.
      </p>
    </section>

    {#if deployments.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"><Empty.Header><Empty.Title>No system deployments</Empty.Title><Empty.Description>Deployment history will appear after the first system release is installed.</Empty.Description></Empty.Header></Empty.Root>
    {:else}
      <div class="grid gap-4 lg:grid-cols-5 lg:items-start">
        <Card.Root class="lg:sticky lg:top-20 lg:col-span-2">
          <Card.Header>
            <Card.Title>History</Card.Title>
            <Card.Description>{deployments.length} deployment{deployments.length === 1 ? '' : 's'}, newest first.</Card.Description>
          </Card.Header>
          <Card.Content><ScrollArea.Root class="max-h-[70vh]"><div class="grid gap-2 pr-3">
            {#each deployments as option (option.id)}
              <Button
                variant={selectedDeployment?.id === option.id ? 'secondary' : 'ghost'}
                class="h-auto w-full justify-start whitespace-normal p-3 text-left"
                onclick={() => (selectedDeploymentId = option.id)}
              >
                <span class="grid w-full gap-1">
                  <span class="flex items-center justify-between gap-2">
                    <span class="font-semibold">{versionLabel(option.releaseVersion)}</span>
                    {#if option.active}<StatusBadge status="serving" />{/if}
                  </span>
                  <span><StatusBadge status={option.status} /></span>
                  <span class="text-muted-foreground">{timestamp(option.createdAt)}</span>
                </span>
              </Button>
            {/each}
          </div></ScrollArea.Root></Card.Content>
        </Card.Root>

        {#each selectedDeployment ? [selectedDeployment] : [] as deployment (deployment.id)}
          <Card.Root class="min-w-0 lg:col-span-3">
            <Card.Header>
              <Card.Title>{versionLabel(deployment.releaseVersion)}</Card.Title>
              <Card.Description>{deployment.changeSummary}</Card.Description>
              <Card.Action>
                <StatusBadge status={deployment.active ? 'serving' : deployment.status} />
              </Card.Action>
            </Card.Header>
            <Card.Content>
              <div class="space-y-6">
                    <section>
                      <h2 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Deployment record</h2>
                      <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                        <div>
                          <dt class="text-muted-foreground">Deployment ID</dt>
                          <dd class="mt-1 break-all font-mono text-xs">{deployment.id}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Attempt</dt>
                          <dd class="mt-1 text-sm">{deployment.attempt}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Status</dt>
                          <dd class="mt-1"><StatusBadge status={deployment.status} /></dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Current step</dt>
                          <dd class="mt-1 text-sm capitalize">{stateLabel(deployment.currentStep)}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Created</dt>
                          <dd class="mt-1 text-sm">{timestamp(deployment.createdAt)}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Updated</dt>
                          <dd class="mt-1 text-sm">{timestamp(deployment.updatedAt)}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Started</dt>
                          <dd class="mt-1 text-sm">{timestamp(deployment.startedAt)}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Finished</dt>
                          <dd class="mt-1 text-sm">{timestamp(deployment.finishedAt)}</dd>
                        </div>
                        {#if deployment.error}
                          <div class="sm:col-span-2 xl:col-span-4"><Alert.Root variant="destructive"><Alert.Title>Deployment failed</Alert.Title><Alert.Description>{deployment.error}</Alert.Description></Alert.Root></div>
                        {/if}
                      </dl>
                    </section>

                    <Separator />

                    <section>
                      <h2 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Release</h2>
                      <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                        <div>
                          <dt class="text-muted-foreground">Release ID</dt>
                          <dd class="mt-1 break-all font-mono text-xs">{deployment.releaseId}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Version</dt>
                          <dd class="mt-1 text-sm font-medium">{versionLabel(deployment.releaseVersion)}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Source revision</dt>
                          <dd class="mt-1 break-all font-mono text-xs">{deployment.sourceRevision || 'Not recorded'}</dd>
                        </div>
                        <div class="sm:col-span-2 xl:col-span-4">
                          <dt class="text-muted-foreground">Artifact</dt>
                          <dd class="mt-1 break-all font-mono text-xs">{deployment.artifactReference}</dd>
                        </div>
                        <div class="sm:col-span-2 xl:col-span-4">
                          <dt class="text-muted-foreground">Artifact digest</dt>
                          <dd class="mt-1 break-all font-mono text-xs">{deployment.artifactDigest || 'Not recorded'}</dd>
                        </div>
                      </dl>
                    </section>

                    <Separator />

                    <section>
                      <h2 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Change</h2>
                      <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                        <div>
                          <dt class="text-muted-foreground">Change ID</dt>
                          <dd class="mt-1 break-all font-mono text-xs">{deployment.changeId}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Sequence</dt>
                          <dd class="mt-1 text-sm">{deployment.changeSequence}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Kind</dt>
                          <dd class="mt-1 text-sm capitalize">{stateLabel(deployment.changeKind)}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Trigger</dt>
                          <dd class="mt-1 text-sm capitalize">{stateLabel(deployment.triggerType)}</dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Status</dt>
                          <dd class="mt-1"><StatusBadge status={deployment.changeStatus} /></dd>
                        </div>
                        <div>
                          <dt class="text-muted-foreground">Requested</dt>
                          <dd class="mt-1 text-sm">{timestamp(deployment.requestedAt)}</dd>
                        </div>
                        <div class="sm:col-span-2 xl:col-span-4">
                          <dt class="text-muted-foreground">Summary</dt>
                          <dd class="mt-1 text-sm">{deployment.changeSummary}</dd>
                        </div>
                      </dl>
                    </section>

                    <Separator />

                    <section>
                      <h2 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Instance</h2>
                      {#if deployment.instanceId}
                        <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                          <div>
                            <dt class="text-muted-foreground">Instance ID</dt>
                            <dd class="mt-1 break-all font-mono text-xs">{deployment.instanceId}</dd>
                          </div>
                          <div>
                            <dt class="text-muted-foreground">Serving</dt>
                            <dd class="mt-1 text-sm">{deployment.active ? 'Yes' : 'No'}</dd>
                          </div>
                          <div>
                            <dt class="text-muted-foreground">Slot</dt>
                            <dd class="mt-1 text-sm capitalize">{deployment.instanceSlot}</dd>
                          </div>
                          <div>
                            <dt class="text-muted-foreground">State</dt>
                            <dd class="mt-1"><StatusBadge status={deployment.instanceState} /></dd>
                          </div>
                          <div>
                            <dt class="text-muted-foreground">Service</dt>
                            <dd class="mt-1 break-all font-mono text-xs">{deployment.instanceService}</dd>
                          </div>
                          <div>
                            <dt class="text-muted-foreground">HTTP listener</dt>
                            <dd class="mt-1 font-mono text-xs">127.0.0.1:{deployment.instancePort}</dd>
                          </div>
                          <div>
                            <dt class="text-muted-foreground">Observed</dt>
                            <dd class="mt-1 text-sm">{timestamp(deployment.instanceObservedAt)}</dd>
                          </div>
                        </dl>
                      {:else}
                        <p class="mt-4 text-sm text-muted-foreground">No instance was recorded for this deployment.</p>
                      {/if}
                    </section>

                    <Separator />

                    <section class="grid gap-6">
                      <div>
                        <h2 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Strategy</h2>
                        <JsonCode value={deployment.strategy} class="mt-4" />
                      </div>
                      <div>
                        <h2 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Runtime configuration</h2>
                        <JsonCode value={deployment.runtimeConfiguration} class="mt-4" />
                      </div>
                    </section>

                    <Separator />

                    <section>
                      <h2 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Events</h2>
                      {#if deployment.events.length === 0}
                        <p class="mt-4 text-sm text-muted-foreground">No deployment events were recorded.</p>
                      {:else}
                        <ol class="mt-4 space-y-3">
                          {#each deployment.events as event (event.id)}
                            <li class="border border-border bg-muted/10 p-4">
                              <div class="flex flex-wrap items-start justify-between gap-3">
                                <div>
                                  <p class="text-sm font-medium">{event.message}</p>
                                  <p class="mt-1 text-xs capitalize text-muted-foreground">
                                    #{event.sequence} · {stateLabel(event.eventType)}{event.step ? ` · ${stateLabel(event.step)}` : ''}
                                  </p>
                                </div>
                                <div class="flex items-center gap-2"><StatusBadge status={event.status} /><p class="text-xs text-muted-foreground">{timestamp(event.occurredAt)}</p></div>
                              </div>
                              {#if event.error}<p class="mt-3 text-sm text-destructive">{event.error}</p>{/if}
                              {#if hasJSONFields(event.metadata)}
                                <JsonCode value={event.metadata} class="mt-3 bg-background p-3" />
                              {/if}
                            </li>
                          {/each}
                        </ol>
                      {/if}
                    </section>
              </div>
            </Card.Content>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
