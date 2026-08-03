<script lang="ts">
  import { useForm } from '@inertiajs/svelte'

  import AuthCard from '@/Components/Auth/AuthCard.svelte'
  import { Button } from '@/Components/ui/button'
  import * as InputOTP from '@/Components/ui/input-otp'
  import { Label } from '@/Components/ui/label'
  import { Spinner } from '@/Components/ui/spinner'
  import Layout from '@/Layouts/Layout.svelte'
  import { routes } from '@/routes'

  let { errors = {} }: { errors?: Record<string, string> } = $props()
  const form = useForm({ code: '' })

  function submit(event: SubmitEvent) {
    event.preventDefault()
    $form.post(routes.confirmationCreate())
  }
</script>

<svelte:head>
  <title>Verify email</title>
</svelte:head>

<Layout>
  <AuthCard title="Verify your email" description="Enter the six-digit verification code sent to your email address.">
    <form class="grid gap-5" onsubmit={submit}>
      <div class="grid gap-2">
        <Label for="code">Verification code</Label>
        <InputOTP.Root id="code" bind:value={$form.code} maxlength={6} inputmode="numeric" autocomplete="one-time-code" aria-invalid={errors.code ? 'true' : undefined} aria-describedby={errors.code ? 'code-error' : undefined} required>
          {#snippet children({ cells })}
            <InputOTP.Group class="w-full justify-center">
              {#each cells as cell (cell)}<InputOTP.Slot {cell} class="size-10" />{/each}
            </InputOTP.Group>
          {/snippet}
        </InputOTP.Root>
        {#if errors.code}<p id="code-error" class="text-xs font-medium text-destructive">{errors.code}</p>{/if}
      </div>
      <Button type="submit" size="lg" disabled={$form.processing} aria-busy={$form.processing} class="w-full">
        {#if $form.processing}<Spinner />{/if}
        Verify email
      </Button>
    </form>
  </AuthCard>
</Layout>
