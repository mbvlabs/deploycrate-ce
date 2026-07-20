<script lang="ts">
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert'

  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import { routes } from '@/routes'

  let { status, title, description }: { status: string; title: string; description: string } = $props()
</script>

<Card.Root class="w-full max-w-md border-border/80 bg-card text-center">
  <Card.Header class="items-center border-b border-border/70">
    <span class="grid size-10 place-items-center border border-destructive/40 bg-destructive/10 text-destructive">
      <TriangleAlertIcon class="size-5" />
    </span>
    <p class="mt-2 text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Error {status}</p>
    <Card.Title class="text-xl font-semibold tracking-tight text-foreground">{title}</Card.Title>
    <Card.Description>{description}</Card.Description>
  </Card.Header>
  <Card.Content>
    <Button href={routes.homePage()} variant="outline" class="w-full">Return home</Button>
  </Card.Content>
</Card.Root>
