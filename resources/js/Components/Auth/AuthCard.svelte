<script lang="ts">
  import ShieldCheckIcon from '@lucide/svelte/icons/shield-check'
  import type { Snippet } from 'svelte'

  import * as Card from '@/Components/ui/card'

  let {
    title,
    description,
    children,
    footer,
  }: {
    title: string
    description: string
    children: Snippet
    footer?: Snippet
  } = $props()
</script>

<Card.Root class="w-full max-w-md border-border/80 bg-card">
  <Card.Header class="border-b border-border/70">
    <div class="mb-2 flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.2em] text-primary">
      <ShieldCheckIcon class="size-3.5" />
      <span>DeployCrate CE</span>
    </div>
    <Card.Title class="text-lg font-semibold tracking-tight text-foreground">{title}</Card.Title>
    <Card.Description>{description}</Card.Description>
  </Card.Header>
  <Card.Content>
    {@render children()}
  </Card.Content>
  {#if footer}
    <Card.Footer class="justify-center text-muted-foreground">
      {@render footer()}
    </Card.Footer>
  {/if}
</Card.Root>
