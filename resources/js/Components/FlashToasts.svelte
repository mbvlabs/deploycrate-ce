<script lang="ts">
  import CircleCheckIcon from '@lucide/svelte/icons/circle-check'
  import CircleXIcon from '@lucide/svelte/icons/circle-x'
  import InfoIcon from '@lucide/svelte/icons/info'
  import { router } from '@inertiajs/svelte'
  import { onMount } from 'svelte'

  import * as Alert from '@/Components/ui/alert'

  type FlashMessage = {
    ID?: string
    Type: string
    Message: string
  }

  type Toast = FlashMessage & { id: number }

  let { initialFlashes }: { initialFlashes?: unknown } = $props()
  let toasts = $state<Toast[]>([])
  let nextId = 0
  const seenFlashIds = new Set<string>()

  function isFlashMessage(value: unknown): value is FlashMessage {
    if (!value || typeof value !== 'object') return false

    const flash = value as Record<string, unknown>
    return typeof flash.Type === 'string' && typeof flash.Message === 'string'
  }

  function pushFlashes(value: unknown) {
    if (!Array.isArray(value)) return

    for (const flash of value.filter(isFlashMessage)) {
      if (flash.ID && seenFlashIds.has(flash.ID)) continue
      if (flash.ID) seenFlashIds.add(flash.ID)

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
        variant={toast.Type === 'error' ? 'destructive' : 'default'}
        class={`${toast.Type === 'success'
          ? 'border-success/50 bg-success/10 text-success'
          : toast.Type === 'error'
            ? 'border-destructive/50 bg-destructive/10'
            : 'border-primary/50 bg-primary/10 text-primary'} mb-2`}
      >
        {#if toast.Type === 'success'}
          <CircleCheckIcon />
        {:else if toast.Type === 'error'}
          <CircleXIcon />
        {:else}
          <InfoIcon />
        {/if}
        <Alert.Title class="capitalize">{toast.Type || 'Notice'}</Alert.Title>
        <Alert.Description class="text-current/80">{toast.Message}</Alert.Description>
      </Alert.Root>
    {/each}
  </div>
{/if}
