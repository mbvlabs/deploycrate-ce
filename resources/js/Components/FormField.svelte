<script lang="ts">
  import type { Snippet } from 'svelte'
  import { Label } from '@/Components/ui/label'
  let { label, children }: { label: string; children: Snippet } = $props()
</script>
<div class="grid gap-2"><Label>{label}</Label>{@render children()}</div>
