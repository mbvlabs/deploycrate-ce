<script lang="ts">
  import { Button } from '@/Components/ui/button'
  import * as Dialog from '@/Components/ui/dialog'

  type Secret = { key: string; value: string }

  let {
    open = $bindable(false),
    existingSecrets = [],
    reservedKeys = [],
    onImport,
  }: {
    open?: boolean
    existingSecrets?: Secret[]
    reservedKeys?: string[]
    onImport: (secrets: Secret[]) => void
  } = $props()

  let text = $state('')
  let errors = $state<string[]>([])

  $effect(() => {
    if (!open) {
      text = ''
      errors = []
    }
  })

  function close() {
    open = false
    text = ''
    errors = []
  }

  function submit(event: SubmitEvent) {
    event.preventDefault()
    const parsed: Secret[] = []
    const nextErrors: string[] = []
    const keys = new Set(existingSecrets.map((secret) => secret.key.trim().toUpperCase()).filter(Boolean))
    const reserved = new Set(reservedKeys.map((key) => key.trim().toUpperCase()).filter(Boolean))

    for (const [index, line] of text.split(/\r?\n/).entries()) {
      if (line.trim() === '') continue
      const separator = line.indexOf('=')
      if (separator < 1) {
        nextErrors.push(`Line ${index + 1} must use KEY=VALUE.`)
        continue
      }
      const key = line.slice(0, separator).trim().toUpperCase()
      const value = line.slice(separator + 1)
      if (!/^[A-Z_][A-Z0-9_]*$/.test(key)) nextErrors.push(`Line ${index + 1} has an invalid key: ${key || '(empty)'}.`)
      else if (reserved.has(key)) nextErrors.push(`Line ${index + 1} uses reserved key ${key}.`)
      else if (keys.has(key)) nextErrors.push(`Line ${index + 1} duplicates ${key}.`)
      else if (value === '') nextErrors.push(`Line ${index + 1} must have a value.`)
      else {
        keys.add(key)
        parsed.push({ key, value })
      }
    }

    errors = nextErrors
    if (errors.length > 0) return
    onImport(parsed)
    close()
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-2xl">
    <form class="space-y-5" onsubmit={submit}>
      <div><h2 class="text-lg font-semibold">Import Environment secrets</h2><p class="mt-1 text-sm text-muted-foreground">Paste one secret per line using KEY=VALUE. Values may contain additional equals signs.</p></div>
      <label class="grid gap-2 text-xs">Secrets<textarea class="min-h-64 w-full border border-input bg-background px-3 py-2 font-mono text-xs" bind:value={text} placeholder={'DATABASE_URL=postgres://...\nAPI_TOKEN=...'} autocomplete="off" spellcheck="false"></textarea></label>
      {#if errors.length > 0}<div class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">{#each errors as error}<p>{error}</p>{/each}</div>{/if}
      <div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={close}>Cancel</Button><Button type="submit">Import secrets</Button></div>
    </form>
  </Dialog.Content>
</Dialog.Root>
