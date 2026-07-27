<script lang="ts">
  let { label, value }: { label: string; value: string } = $props()
</script>
<div><p class="text-xs text-muted-foreground">{label}</p><p class="mt-1 break-all font-mono text-xs">{value}</p></div>
