<script lang="ts">
  import { router } from '@inertiajs/svelte'

  import * as AlertDialog from '@/Components/ui/alert-dialog'
  import { Button } from '@/Components/ui/button'
  import * as Field from '@/Components/ui/field'
  import { Input } from '@/Components/ui/input'
  import { Spinner } from '@/Components/ui/spinner'
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

<AlertDialog.Root {open} onOpenChange={setOpen}>
  <AlertDialog.Trigger>
    {#snippet child({ props })}
      <Button {...props} variant="destructive">Delete environment</Button>
    {/snippet}
  </AlertDialog.Trigger>
  <AlertDialog.Content>
    <form class="grid gap-4" onsubmit={destroy}>
      <AlertDialog.Header>
        <AlertDialog.Title>Permanently delete {environmentName}?</AlertDialog.Title>
        <AlertDialog.Description>This removes the Environment, its builds, deployments, releases, secrets, routes, containers, network, and background jobs. This cannot be undone.</AlertDialog.Description>
      </AlertDialog.Header>
      <Field.Field data-invalid={Boolean(requestError)}>
        <Field.Label class="w-full flex-col items-start">
          <span>Type <span class="font-mono text-foreground">{environmentName}</span> to confirm</span>
        <Input bind:value={confirmation} autocomplete="off" autofocus disabled={processing} />
        </Field.Label>
        {#if requestError}<Field.Error class="whitespace-pre-wrap border border-destructive/50 bg-destructive/10 p-3">{requestError}</Field.Error>{/if}
      </Field.Field>
      <AlertDialog.Footer>
        <Button type="button" variant="outline" disabled={processing} onclick={() => setOpen(false)}>Cancel</Button>
        <Button type="submit" variant="destructive" disabled={!confirmed || processing} aria-busy={processing}>
          {#if processing}<Spinner />{/if}
          Delete permanently
        </Button>
      </AlertDialog.Footer>
    </form>
  </AlertDialog.Content>
</AlertDialog.Root>
