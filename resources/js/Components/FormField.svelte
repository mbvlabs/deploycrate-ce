<script lang="ts">
  import type { Snippet } from 'svelte'
  import { Label } from '@/Components/ui/label'
  let { label, error, children }: { label: string; error?: string; children: Snippet } = $props()
</script>
<div class="grid gap-2">
  <Label class={error ? 'text-destructive' : undefined}>{label}</Label>
  {@render children()}
  {#if error}<p class="text-xs font-medium text-destructive">{error}</p>{/if}
</div>
