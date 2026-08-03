<script lang="ts">
  import MoonIcon from '@lucide/svelte/icons/moon'
  import SunIcon from '@lucide/svelte/icons/sun'

  import { Button } from '@/Components/ui/button'

  const storageKey = 'deploycrate-theme'

  function initialTheme() {
    if (typeof document === 'undefined') return true
    return document.documentElement.classList.contains('dark')
  }

  let dark = $state(initialTheme())

  function toggleTheme() {
    dark = !dark
    document.documentElement.classList.toggle('dark', dark)

    try {
      localStorage.setItem(storageKey, dark ? 'dark' : 'light')
    } catch {
      // The selected theme still applies for this page when storage is unavailable.
    }
  }
</script>

<Button
  variant="outline"
  size="icon-sm"
  aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
  aria-pressed={dark}
  title={dark ? 'Switch to light theme' : 'Switch to dark theme'}
  onclick={toggleTheme}
>
  {#if dark}
    <SunIcon />
  {:else}
    <MoonIcon />
  {/if}
</Button>
