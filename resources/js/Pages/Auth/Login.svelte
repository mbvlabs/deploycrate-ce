<script lang="ts">
  import EyeIcon from "@lucide/svelte/icons/eye";
  import EyeOffIcon from "@lucide/svelte/icons/eye-off";
  import { Link, useForm } from "@inertiajs/svelte";

  import AuthCard from "@/Components/Auth/AuthCard.svelte";
  import { Button } from "@/Components/ui/button";
  import { Input } from "@/Components/ui/input";
  import * as InputGroup from "@/Components/ui/input-group";
  import { Label } from "@/Components/ui/label";
  import { Spinner } from "@/Components/ui/spinner";
  import Layout from "@/Layouts/Layout.svelte";
  import { routes } from "@/routes";

  let { errors = {} }: { errors?: Record<string, string> } = $props();
  const form = useForm({ email: "", password: "" });
  let showPassword = $state(false);

  function submit(event: SubmitEvent) {
    event.preventDefault();
    $form.post(routes.sessionCreate());
  }
</script>

<svelte:head>
  <title>Sign in</title>
</svelte:head>

<Layout>
  <AuthCard
    title="Sign in to your account"
    description="Enter your credentials to continue to your workspace."
  >
    <form class="grid gap-5" onsubmit={submit}>
      <div class="grid gap-2">
        <Label for="email">Email</Label>
        <Input
          id="email"
          bind:value={$form.email}
          type="email"
          autocomplete="email"
          placeholder="you@example.com"
          aria-invalid={errors.email ? "true" : undefined}
          aria-describedby={errors.email ? "email-error" : undefined}
          required
        />
        {#if errors.email}<p
            id="email-error"
            class="text-xs font-medium text-destructive"
          >
            {errors.email}
          </p>{/if}
      </div>
      <div class="grid gap-2">
        <div class="flex items-center justify-between gap-3">
          <Label for="password">Password</Label>
          <Link
            class="text-xs text-primary underline-offset-4 hover:underline"
            href={routes.passwordNew()}>Forgot password?</Link
          >
        </div>
        <InputGroup.Root>
          <InputGroup.Input
            id="password"
            bind:value={$form.password}
            type={showPassword ? "text" : "password"}
            autocomplete="current-password"
            aria-invalid={errors.password ? "true" : undefined}
            aria-describedby={errors.password ? "password-error" : undefined}
            required
          />
          <InputGroup.Addon align="inline-end">
            <InputGroup.Button
              size="icon-xs"
              aria-label={showPassword ? "Hide password" : "Show password"}
              onclick={() => (showPassword = !showPassword)}
            >
              {#if showPassword}<EyeOffIcon />{:else}<EyeIcon />{/if}
            </InputGroup.Button>
          </InputGroup.Addon>
        </InputGroup.Root>
        {#if errors.password}<p
            id="password-error"
            class="text-xs font-medium text-destructive"
          >
            {errors.password}
          </p>{/if}
      </div>
      <Button
        type="submit"
        size="lg"
        disabled={$form.processing}
        aria-busy={$form.processing}
        class="w-full"
      >
        {#if $form.processing}<Spinner />{/if}
        Sign in
      </Button>
    </form>
  </AuthCard>
</Layout>
