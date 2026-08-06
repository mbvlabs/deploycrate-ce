<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    eyebrow,
    title,
    description,
    actions,
  }: {
    eyebrow: string;
    title: string;
    description: string;
    actions?: Snippet;
  } = $props();
</script>

<header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
  <div class="max-w-3xl">
    <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">
      {eyebrow}
    </p>
    <h1 class="mt-3 text-3xl font-semibold tracking-tight">{title}</h1>
    <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
      {description}
    </p>
  </div>
  {#if actions}
    <div class="flex shrink-0 flex-col gap-2 sm:flex-row">
      {@render actions()}
    </div>
  {/if}
</header>
