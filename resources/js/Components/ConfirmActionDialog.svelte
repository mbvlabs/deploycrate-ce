<script lang="ts">
  import { Button } from '@/Components/ui/button'
  import * as Dialog from '@/Components/ui/dialog'

  let {
    open = $bindable(false),
    title,
    description,
    confirmLabel,
    processing = false,
    destructive = false,
    error = '',
    onconfirm,
  }: {
    open?: boolean
    title: string
    description: string
    confirmLabel: string
    processing?: boolean
    destructive?: boolean
    error?: string
    onconfirm: () => void
  } = $props()
</script>

<Dialog.Root bind:open>
  <Dialog.Content showCloseButton={!processing}>
    <Dialog.Header>
      <Dialog.Title>{title}</Dialog.Title>
      <Dialog.Description>{description}</Dialog.Description>
    </Dialog.Header>
    {#if error}<p class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive" role="alert">{error}</p>{/if}
    <Dialog.Footer>
      <Button type="button" variant="outline" disabled={processing} onclick={() => (open = false)}>Cancel</Button>
      <Button type="button" variant={destructive ? 'destructive' : 'default'} disabled={processing} onclick={onconfirm}>
        {processing ? `${confirmLabel}...` : confirmLabel}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
