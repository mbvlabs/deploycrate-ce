<script lang="ts">
  import { router } from '@inertiajs/svelte'

  import { Button } from '@/Components/ui/button'
  import * as Dialog from '@/Components/ui/dialog'
  import { Input } from '@/Components/ui/input'
  import { routes } from '@/routes'

  let {
    applicationId,
    environmentId,
    environmentName,
  }: {
    applicationId: string
    environmentId: string
    environmentName: string
  } = $props()

  let open = $state(false)
  let confirmation = $state('')
  let processing = $state(false)
  let requestError = $state('')
  const confirmed = $derived(confirmation === environmentName)

  function setOpen(next: boolean) {
    if (processing) return
    open = next
    if (!next) {
      confirmation = ''
      requestError = ''
    }
  }

  function destroy(event: SubmitEvent) {
    event.preventDefault()
    if (!confirmed || processing) return

    processing = true
    requestError = ''
    router.delete(routes.environmentDestroy(applicationId, environmentId), {
      preserveScroll: true,
      onError: (errors) => {
        requestError = Object.values(errors).map(String).join('\n') || 'The Environment could not be deleted.'
      },
      onFinish: () => (processing = false),
    })
  }
</script>

<Dialog.Root {open} onOpenChange={setOpen}>
  <Dialog.Trigger>
    {#snippet child({ props })}
      <Button {...props} variant="destructive">Delete environment</Button>
    {/snippet}
  </Dialog.Trigger>
  <Dialog.Content showCloseButton={!processing}>
    <form class="grid gap-4" onsubmit={destroy}>
      <Dialog.Header>
        <Dialog.Title>Permanently delete {environmentName}?</Dialog.Title>
        <Dialog.Description>This removes the Environment, its builds, deployments, releases, secrets, routes, containers, network, and background jobs. This cannot be undone.</Dialog.Description>
      </Dialog.Header>
      <label class="grid gap-2 text-xs">
        Type <span class="font-mono text-foreground">{environmentName}</span> to confirm
        <Input bind:value={confirmation} autocomplete="off" autofocus disabled={processing} />
      </label>
      {#if requestError}<p class="whitespace-pre-wrap border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive" role="alert">{requestError}</p>{/if}
      <Dialog.Footer>
        <Button type="button" variant="outline" disabled={processing} onclick={() => setOpen(false)}>Cancel</Button>
        <Button type="submit" variant="destructive" disabled={!confirmed || processing}>{processing ? 'Deleting...' : 'Delete permanently'}</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
