<script lang="ts">
  import * as AlertDialog from '@/Components/ui/alert-dialog'
  import { Button } from '@/Components/ui/button'
  import { Input } from '@/Components/ui/input'
  import { Spinner } from '@/Components/ui/spinner'

  let {
    open = $bindable(false),
    title,
    description,
    confirmLabel,
    processing = false,
    destructive = false,
    error = '',
    requiredPhrase = '',
    onconfirm,
  }: {
    open?: boolean
    title: string
    description: string
    confirmLabel: string
    processing?: boolean
    destructive?: boolean
    error?: string
    requiredPhrase?: string
    onconfirm: () => void
  } = $props()

  let confirmation = $state('')
  const confirmationMatches = $derived(!requiredPhrase || confirmation === requiredPhrase)

  $effect(() => {
    if (!open) confirmation = ''
  })
</script>

<AlertDialog.Root {open} onOpenChange={(next) => { if (!processing) open = next }}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>{title}</AlertDialog.Title>
      <AlertDialog.Description>{description}</AlertDialog.Description>
    </AlertDialog.Header>
    {#if requiredPhrase}
      <label class="grid gap-2 text-sm">
        <span>Type <span class="font-mono font-semibold">{requiredPhrase}</span> to confirm</span>
        <Input bind:value={confirmation} autocomplete="off" disabled={processing} />
      </label>
    {/if}
    {#if error}<p class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive" role="alert">{error}</p>{/if}
    <AlertDialog.Footer>
      <AlertDialog.Cancel disabled={processing}>Cancel</AlertDialog.Cancel>
      <Button type="button" variant={destructive ? 'destructive' : 'default'} disabled={processing || !confirmationMatches} aria-busy={processing} onclick={onconfirm}>
        {#if processing}<Spinner />{/if}
        {confirmLabel}
      </Button>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
