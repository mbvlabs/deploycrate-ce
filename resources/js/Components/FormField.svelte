<script lang="ts">
  import type { Snippet } from 'svelte'
  import * as Field from '@/Components/ui/field'

  let { label, description, error, children }: { label: string; description?: string; error?: string; children: Snippet } = $props()
</script>

<Field.Field data-invalid={Boolean(error)}>
  <Field.Label class="w-full flex-col items-start">
    <span>{label}</span>
    {@render children()}
  </Field.Label>
  {#if description}<Field.Description>{description}</Field.Description>{/if}
  {#if error}<Field.Error>{error}</Field.Error>{/if}
</Field.Field>
