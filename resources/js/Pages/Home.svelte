<script lang="ts">
  import ActivityIcon from '@lucide/svelte/icons/activity'
  import AppWindowIcon from '@lucide/svelte/icons/app-window'
  import ArrowRightIcon from '@lucide/svelte/icons/arrow-right'
  import BoxesIcon from '@lucide/svelte/icons/boxes'
  import DatabaseIcon from '@lucide/svelte/icons/database'
  import GitCommitHorizontalIcon from '@lucide/svelte/icons/git-commit-horizontal'
  import PlusIcon from '@lucide/svelte/icons/plus'
  import RocketIcon from '@lucide/svelte/icons/rocket'
  import ServerIcon from '@lucide/svelte/icons/server'
  import SparklesIcon from '@lucide/svelte/icons/sparkles'
  import { Link } from '@inertiajs/svelte'

  import StatusBadge from '@/Components/StatusBadge.svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Metrics = {
    applications: number
    environments: number
    deployments: number
    activeDeployments: number
    successfulDeployments: number
    finishedDeployments: number
    deploymentSuccess: number
    resources: number
    nodes: number
  }

  type DeploymentActivity = {
    day: string
    total: number
    succeeded: number
    failed: number
  }

  type Deployment = {
    id: string
    applicationId: string
    applicationName: string
    environmentId: string
    environmentName: string
    environmentKind: string
    status: string
    currentStep: string
    sourceRevision: string
    createdAt: string
  }

  type Application = {
    id: string
    name: string
    slug: string
    environmentCount: number
    deploymentCount: number
    latestDeploymentStatus: string
    latestDeploymentAt: string
  }

  type Dashboard = {
    metrics: Metrics
    deploymentActivity: DeploymentActivity[]
    recentDeployments: Deployment[]
    applications: Application[]
  }

  let { auth, dashboard }: { auth: { email: string }; dashboard: Dashboard } = $props()

  const hasApplications = $derived(dashboard.metrics.applications > 0)
  const maxActivity = $derived(Math.max(1, ...dashboard.deploymentActivity.map((point) => point.total)))
  const dayFormatter = new Intl.DateTimeFormat(undefined, { weekday: 'short' })
  const dateTimeFormatter = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })

  const summaryCards = $derived([
    {
      label: 'Applications',
      value: dashboard.metrics.applications,
      detail: dashboard.metrics.applications === 1 ? 'service in motion' : 'services in motion',
      icon: AppWindowIcon,
      href: routes.applications(),
    },
    {
      label: 'Environments',
      value: dashboard.metrics.environments,
      detail: dashboard.metrics.environments === 1 ? 'runtime configured' : 'runtimes configured',
      icon: BoxesIcon,
      href: routes.environments(),
    },
    {
      label: 'Deployments',
      value: dashboard.metrics.deployments,
      detail: dashboard.metrics.activeDeployments > 0 ? `${dashboard.metrics.activeDeployments} active now` : 'none running now',
      icon: RocketIcon,
      href: routes.environments(),
    },
    {
      label: 'Success rate',
      value: dashboard.metrics.finishedDeployments > 0 ? `${dashboard.metrics.deploymentSuccess.toFixed(0)}%` : 'Ready',
      detail: dashboard.metrics.finishedDeployments > 0 ? `${dashboard.metrics.successfulDeployments} successful releases` : 'waiting for first launch',
      icon: ActivityIcon,
      href: routes.environments(),
    },
  ])

  function activityHeight(value: number) {
    if (value === 0) return 4
    return Math.max(12, Math.round((value / maxActivity) * 112))
  }

  function dayLabel(day: string) {
    return dayFormatter.format(new Date(`${day}T12:00:00Z`))
  }

  function deploymentTime(value: string) {
    return dateTimeFormatter.format(new Date(value))
  }

  function revisionLabel(value: string) {
    return value ? value.slice(0, 8) : 'manual'
  }
</script>

<svelte:head>
  <title>Dashboard</title>
</svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-6 lg:space-y-8">
    <section class="relative isolate overflow-hidden border border-border bg-card px-5 py-6 sm:px-8 sm:py-8" aria-labelledby="dashboard-title">
      <div class="absolute inset-0 -z-10 bg-[radial-gradient(circle_at_80%_0%,color-mix(in_srgb,var(--primary)_18%,transparent),transparent_38%)]"></div>
      <div class="absolute -right-10 -top-16 -z-10 size-64 border border-primary/20"></div>
      <div class="absolute right-10 top-10 -z-10 size-28 border border-primary/25 bg-primary/5"></div>

      <div class="flex flex-col gap-8 lg:flex-row lg:items-end lg:justify-between">
        <div class="max-w-2xl">
          <div class="flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.24em] text-primary">
            <SparklesIcon class="size-3.5" />
            Deployment command center
          </div>
          <h1 id="dashboard-title" class="mt-4 text-3xl font-semibold tracking-tight sm:text-4xl lg:text-5xl">
            {hasApplications ? 'Everything is in motion.' : 'Your next release starts here.'}
          </h1>
          <p class="mt-4 max-w-xl text-sm leading-6 text-muted-foreground">
            {#if hasApplications}
              See what is running, what just shipped, and where your platform needs attention.
            {:else}
              Create your first application, connect its source, and take it from commit to production.
            {/if}
          </p>
        </div>

        <div class="flex flex-col gap-2 sm:flex-row">
          <Button size="lg">
            {#snippet child({ props })}
              <Link {...props} href={routes.applicationNew()}><PlusIcon />New application</Link>
            {/snippet}
          </Button>
          <Button size="lg" variant="outline">
            {#snippet child({ props })}
              <Link {...props} href={routes.environments()}>View environments<ArrowRightIcon data-icon="inline-end" /></Link>
            {/snippet}
          </Button>
        </div>
      </div>
    </section>

    <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="Platform metrics">
      {#each summaryCards as item (item.label)}
        {@const Icon = item.icon}
        <Link href={item.href} class="group border border-border bg-card/80 p-4 transition-colors hover:border-primary/60 hover:bg-card">
          <div class="flex items-start justify-between gap-4">
            <span class="grid size-9 place-items-center border border-border bg-background text-muted-foreground transition-colors group-hover:border-primary/40 group-hover:text-primary">
              <Icon class="size-4" />
            </span>
            <ArrowRightIcon class="size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-primary" />
          </div>
          <p class="mt-5 font-mono text-3xl font-semibold tabular-nums tracking-tight">{item.value}</p>
          <div class="mt-1 flex items-baseline justify-between gap-3">
            <p class="text-xs font-medium">{item.label}</p>
            <p class="truncate text-[10px] text-muted-foreground">{item.detail}</p>
          </div>
        </Link>
      {/each}
    </section>

    <section class="grid gap-6 xl:grid-cols-[minmax(0,1.45fr)_minmax(18rem,0.75fr)]">
      <Card.Root class="min-w-0">
        <Card.Header class="border-b border-border">
          <Card.Action><span class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">7 day pulse</span></Card.Action>
          <Card.Title>Deployment velocity</Card.Title>
          <Card.Description>Releases created across all application environments.</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="flex h-44 items-end gap-2 sm:gap-3" role="img" aria-label="Deployment activity over the last seven days">
            {#each dashboard.deploymentActivity as point (point.day)}
              <div class="group flex min-w-0 flex-1 flex-col items-center gap-2">
                <div class="invisible text-[10px] font-medium tabular-nums group-hover:visible">{point.total}</div>
                <div class="flex h-28 w-full max-w-14 items-end bg-muted/45">
                  <div
                    class="w-full bg-primary transition-[height,opacity] duration-300 group-hover:opacity-80"
                    style:height={`${activityHeight(point.total)}px`}
                    title={`${point.total} deployments, ${point.succeeded} succeeded, ${point.failed} failed`}
                  ></div>
                </div>
                <span class="text-[10px] text-muted-foreground">{dayLabel(point.day)}</span>
              </div>
            {/each}
          </div>
          <div class="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4 text-xs">
            <p class="text-muted-foreground">{dashboard.metrics.deployments === 0 ? 'Your deployment history will appear here.' : `${dashboard.metrics.deployments} deployments recorded`}</p>
            <span class="flex items-center gap-2"><span class="size-2 bg-primary"></span>Total deployments</span>
          </div>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header class="border-b border-border">
          <Card.Action><StatusBadge status={dashboard.metrics.nodes > 0 ? 'ready' : 'pending'} label={dashboard.metrics.nodes > 0 ? 'Available' : 'Needs a node'} /></Card.Action>
          <Card.Title>Platform capacity</Card.Title>
          <Card.Description>Infrastructure available to your workloads.</Card.Description>
        </Card.Header>
        <Card.Content class="grid gap-3">
          <Link href={routes.nodes()} class="group flex items-center gap-4 border border-border bg-background/50 p-4 hover:border-primary/50">
            <span class="grid size-10 place-items-center border border-border text-muted-foreground group-hover:text-primary"><ServerIcon class="size-4" /></span>
            <div class="min-w-0 flex-1"><p class="font-mono text-xl font-semibold tabular-nums">{dashboard.metrics.nodes}</p><p class="text-xs text-muted-foreground">Compute nodes</p></div>
            <ArrowRightIcon class="size-4 text-muted-foreground" />
          </Link>
          <Link href={routes.resources()} class="group flex items-center gap-4 border border-border bg-background/50 p-4 hover:border-primary/50">
            <span class="grid size-10 place-items-center border border-border text-muted-foreground group-hover:text-primary"><DatabaseIcon class="size-4" /></span>
            <div class="min-w-0 flex-1"><p class="font-mono text-xl font-semibold tabular-nums">{dashboard.metrics.resources}</p><p class="text-xs text-muted-foreground">Managed resources</p></div>
            <ArrowRightIcon class="size-4 text-muted-foreground" />
          </Link>
        </Card.Content>
        <Card.Footer class="border-t border-border bg-muted/25 py-3">
          <p class="text-[10px] uppercase tracking-[0.16em] text-muted-foreground">Ready for the next workload</p>
        </Card.Footer>
      </Card.Root>
    </section>

    <section class="grid gap-6 xl:grid-cols-2">
      <Card.Root>
        <Card.Header class="border-b border-border">
          <Card.Action>
            <Button size="sm" variant="ghost">
              {#snippet child({ props })}<Link {...props} href={routes.environments()}>All environments<ArrowRightIcon data-icon="inline-end" /></Link>{/snippet}
            </Button>
          </Card.Action>
          <Card.Title>Latest deployments</Card.Title>
          <Card.Description>The freshest activity across your environments.</Card.Description>
        </Card.Header>
        <Card.Content class="p-0">
          {#if dashboard.recentDeployments.length === 0}
            <div class="grid min-h-64 place-items-center p-8 text-center">
              <div>
                <span class="mx-auto grid size-10 place-items-center border border-dashed border-border text-muted-foreground"><RocketIcon class="size-4" /></span>
                <p class="mt-4 text-sm font-medium">No deployments yet</p>
                <p class="mt-1 text-xs text-muted-foreground">Your first release will light up this feed.</p>
              </div>
            </div>
          {:else}
            <div class="divide-y divide-border">
              {#each dashboard.recentDeployments as deployment (deployment.id)}
                <Link href={routes.environmentShow(deployment.applicationId, deployment.environmentId)} class="group flex items-center gap-3 px-4 py-3 hover:bg-muted/35">
                  <span class="grid size-8 shrink-0 place-items-center border border-border bg-background text-muted-foreground group-hover:text-primary"><GitCommitHorizontalIcon class="size-3.5" /></span>
                  <div class="min-w-0 flex-1">
                    <p class="truncate text-xs font-medium">{deployment.applicationName} <span class="text-muted-foreground">/ {deployment.environmentName}</span></p>
                    <p class="mt-1 truncate font-mono text-[10px] text-muted-foreground">{revisionLabel(deployment.sourceRevision)} · {deploymentTime(deployment.createdAt)}</p>
                  </div>
                  <StatusBadge status={deployment.status} />
                </Link>
              {/each}
            </div>
          {/if}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header class="border-b border-border">
          <Card.Action>
            <Button size="sm" variant="ghost">
              {#snippet child({ props })}<Link {...props} href={routes.applications()}>All applications<ArrowRightIcon data-icon="inline-end" /></Link>{/snippet}
            </Button>
          </Card.Action>
          <Card.Title>Applications</Card.Title>
          <Card.Description>Your most recently active services.</Card.Description>
        </Card.Header>
        <Card.Content class="p-0">
          {#if dashboard.applications.length === 0}
            <div class="grid min-h-64 place-items-center p-8 text-center">
              <div>
                <span class="mx-auto grid size-10 place-items-center border border-dashed border-border text-muted-foreground"><AppWindowIcon class="size-4" /></span>
                <p class="mt-4 text-sm font-medium">The runway is clear</p>
                <p class="mt-1 text-xs text-muted-foreground">Create an application and start shipping.</p>
                <Button class="mt-4" size="sm">
                  {#snippet child({ props })}<Link {...props} href={routes.applicationNew()}><PlusIcon />New application</Link>{/snippet}
                </Button>
              </div>
            </div>
          {:else}
            <div class="divide-y divide-border">
              {#each dashboard.applications as application (application.id)}
                <Link href={routes.applicationShow(application.id)} class="group flex items-center gap-3 px-4 py-3 hover:bg-muted/35">
                  <span class="grid size-8 shrink-0 place-items-center border border-border bg-primary/5 text-primary"><AppWindowIcon class="size-3.5" /></span>
                  <div class="min-w-0 flex-1">
                    <p class="truncate text-xs font-medium">{application.name}</p>
                    <p class="mt-1 truncate text-[10px] text-muted-foreground">{application.environmentCount} {application.environmentCount === 1 ? 'environment' : 'environments'} · {application.deploymentCount} deployments</p>
                  </div>
                  {#if application.latestDeploymentStatus}
                    <StatusBadge status={application.latestDeploymentStatus} />
                  {:else}
                    <span class="text-[10px] text-muted-foreground">Ready to deploy</span>
                  {/if}
                </Link>
              {/each}
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    </section>
  </div>
</DashboardLayout>
