<script lang="ts">
  import CircleCheckIcon from '@lucide/svelte/icons/circle-check'
  import CircleXIcon from '@lucide/svelte/icons/circle-x'
  import InfoIcon from '@lucide/svelte/icons/info'
  import { router } from '@inertiajs/svelte'
  import { onMount } from 'svelte'

  import * as Alert from '@/Components/ui/alert'

  type FlashMessage = {
    id?: string
    type: string
    message: string
  }

  type Toast = FlashMessage & { id: number }

  let { initialFlashes }: { initialFlashes?: unknown } = $props()
  let toasts = $state<Toast[]>([])
  let nextId = 0
  const seenFlashIds = new Set<string>()

  function flashMessage(value: unknown): FlashMessage | null {
    if (!value || typeof value !== 'object') return null

    const flash = value as Record<string, unknown>
    const type = typeof flash.type === 'string' ? flash.type : flash.Type
    const message = typeof flash.message === 'string' ? flash.message : flash.Message
    const id = typeof flash.id === 'string' ? flash.id : flash.ID
    if (typeof type !== 'string' || typeof message !== 'string') return null
    return { id: typeof id === 'string' ? id : undefined, type, message }
  }

  function pushFlashes(flashes: unknown) {
    if (!Array.isArray(flashes)) return

    for (const value of flashes) {
      const flash = flashMessage(value)
      if (!flash) continue
      if (flash.id && seenFlashIds.has(flash.id)) continue
      if (flash.id) seenFlashIds.add(flash.id)

      const id = nextId++
      toasts.push({ ...flash, id })
      window.setTimeout(() => {
        toasts = toasts.filter((toast) => toast.id !== id)
      }, 5000)
    }
  }

  onMount(() => {
    pushFlashes(initialFlashes)
    return router.on('success', (event) => {
      pushFlashes(event.detail.page.props.flash)
    })
  })
</script>

{#if toasts.length > 0}
  <div class="fixed bottom-4 right-4 z-50 w-[min(24rem,calc(100vw-2rem))]" aria-live="polite">
    {#each toasts as toast (toast.id)}
      <Alert.Root
        variant={toast.type === 'error' ? 'destructive' : 'default'}
        class={`${toast.type === 'success'
          ? 'border-success/50 bg-success/10 text-success'
          : toast.type === 'error'
            ? 'border-destructive/50 bg-destructive/10'
            : 'border-primary/50 bg-primary/10 text-primary'} mb-2`}
      >
        {#if toast.type === 'success'}
          <CircleCheckIcon />
        {:else if toast.type === 'error'}
          <CircleXIcon />
        {:else}
          <InfoIcon />
        {/if}
        <Alert.Title class="capitalize">{toast.type || 'Notice'}</Alert.Title>
        <Alert.Description class="text-current/80">{toast.message}</Alert.Description>
      </Alert.Root>
    {/each}
  </div>
{/if}
