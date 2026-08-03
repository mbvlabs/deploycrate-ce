<script lang="ts">
  let { label, value }: { label: string; value: string } = $props()
</script>
<dl><dt class="text-xs text-muted-foreground">{label}</dt><dd class="mt-1 break-all font-mono text-xs">{value}</dd></dl>
