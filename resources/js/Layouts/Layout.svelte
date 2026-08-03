<script lang="ts">
  import type { Snippet } from 'svelte'

  import ThemeToggle from '@/Components/ThemeToggle.svelte'

  let { children }: { children: Snippet } = $props()
</script>

<main class="relative flex min-h-screen items-center justify-center overflow-hidden bg-background px-4 py-12 text-foreground sm:px-6">
  <div class="absolute right-4 top-4 z-10">
    <ThemeToggle />
  </div>
  <div class="relative flex w-full items-center justify-center">
    {@render children()}
  </div>
</main>
