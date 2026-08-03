<script lang="ts">
  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Dialog from '@/Components/ui/dialog'
  import * as Field from '@/Components/ui/field'
  import { Textarea } from '@/Components/ui/textarea'

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
    if (parsed.length === 0 && errors.length === 0) errors = ['Paste at least one secret to import.']
    if (errors.length > 0) return
    onImport(parsed)
    close()
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-2xl">
    <form class="space-y-5" onsubmit={submit}>
      <Dialog.Header>
        <Dialog.Title>Import Environment secrets</Dialog.Title>
        <Dialog.Description>Paste one secret per line using KEY=VALUE. Values may contain additional equals signs.</Dialog.Description>
      </Dialog.Header>
      <Field.Field data-invalid={errors.length > 0}>
        <Field.Label class="w-full flex-col items-start">
          <span>Secrets</span>
          <Textarea class="min-h-64 font-mono" bind:value={text} placeholder={'DATABASE_URL=postgres://...\nAPI_TOKEN=...'} autocomplete="off" spellcheck="false" />
        </Field.Label>
      </Field.Field>
      {#if errors.length > 0}
        <Alert.Root variant="destructive" role="alert">
          <Alert.Title>Import could not continue</Alert.Title>
          <Alert.Description>{#each errors as error}<p>{error}</p>{/each}</Alert.Description>
        </Alert.Root>
      {/if}
      <Dialog.Footer><Button type="button" variant="outline" onclick={close}>Cancel</Button><Button type="submit">Import secrets</Button></Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
