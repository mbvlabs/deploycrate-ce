<script lang="ts">
  import { Badge } from '@/Components/ui/badge'

  let { status, label }: { status: string; label?: string } = $props()

  const normalized = $derived(status.trim().toLowerCase().replaceAll(' ', '_'))
  const tone = $derived.by(() => {
    if (['healthy', 'ready', 'active', 'applied', 'available', 'completed', 'connected', 'deployed', 'succeeded', 'verified', 'serving'].includes(normalized)) return 'border-success/40 bg-success/10 text-success'
    if (['failed', 'error', 'unhealthy', 'blocked', 'suspended', 'cancelled', 'archived', 'discarded', 'missing', 'verification_failed'].includes(normalized)) return 'border-destructive/40 bg-destructive/10 text-destructive'
    if (['pending', 'running', 'queued', 'in_progress', 'scheduled', 'awaiting_confirmation', 'retryable', 'deploying', 'applying', 'degraded', 'stale', 'warning', 'reconnecting'].includes(normalized)) return 'border-warning/40 bg-warning/10 text-warning'
    return 'border-border bg-muted/30 text-muted-foreground'
  })
  const display = $derived(label ?? normalized.replaceAll('_', ' '))
</script>

<Badge variant="outline" class={`capitalize ${tone}`}>{display}</Badge>
