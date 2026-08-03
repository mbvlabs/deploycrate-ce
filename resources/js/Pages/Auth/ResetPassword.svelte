<script lang="ts">
  import { useForm } from '@inertiajs/svelte'

  import AuthCard from '@/Components/Auth/AuthCard.svelte'
  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import { Input } from '@/Components/ui/input'
  import { Label } from '@/Components/ui/label'
  import { Spinner } from '@/Components/ui/spinner'
  import Layout from '@/Layouts/Layout.svelte'
  import { routes } from '@/routes'

  let { token, errors = {} }: { token: string; errors?: Record<string, string> } = $props()

  function initialToken() {
    return token
  }

  const form = useForm({ resetPasswordToken: initialToken(), password: '', confirmPassword: '' })

  function submit(event: SubmitEvent) {
    event.preventDefault()
    $form.put(routes.passwordUpdate())
  }
</script>

<svelte:head>
  <title>Choose a new password</title>
</svelte:head>

<Layout>
  <AuthCard title="Choose a new password" description="Enter and confirm the new password for your account.">
    <form class="grid gap-5" onsubmit={submit}>
      {#if errors.resetPasswordToken}
        <Alert.Root variant="destructive" class="border-destructive/50 bg-destructive/10">
          <Alert.Title>Invalid reset link</Alert.Title>
          <Alert.Description class="text-current/80">{errors.resetPasswordToken}</Alert.Description>
        </Alert.Root>
      {/if}
      <div class="grid gap-2">
        <Label for="password">New password</Label>
        <Input
          id="password"
          bind:value={$form.password}
          type="password"
          autocomplete="new-password"
          aria-invalid={errors.password ? 'true' : undefined}
          aria-describedby={errors.password ? 'password-error' : undefined}
          required
        />
        {#if errors.password}<p id="password-error" class="text-xs font-medium text-destructive">{errors.password}</p>{/if}
      </div>
      <div class="grid gap-2">
        <Label for="confirmPassword">Confirm new password</Label>
        <Input
          id="confirmPassword"
          bind:value={$form.confirmPassword}
          type="password"
          autocomplete="new-password"
          aria-invalid={errors.confirmPassword ? 'true' : undefined}
          aria-describedby={errors.confirmPassword ? 'confirm-password-error' : undefined}
          required
        />
        {#if errors.confirmPassword}<p id="confirm-password-error" class="text-xs font-medium text-destructive">{errors.confirmPassword}</p>{/if}
      </div>
      <Button type="submit" size="lg" disabled={$form.processing} aria-busy={$form.processing} class="w-full">
        {#if $form.processing}<Spinner />{/if}
        Update password
      </Button>
    </form>
  </AuthCard>
</Layout>
