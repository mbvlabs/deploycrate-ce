<script lang="ts">
  import { Link, useForm } from "@inertiajs/svelte";

  import AuthCard from "@/Components/Auth/AuthCard.svelte";
  import { Button } from "@/Components/ui/button";
  import { Input } from "@/Components/ui/input";
  import { Label } from "@/Components/ui/label";
  import { Spinner } from "@/Components/ui/spinner";
  import Layout from "@/Layouts/Layout.svelte";
  import { routes } from "@/routes";

  let { errors = {} }: { errors?: Record<string, string> } = $props();
  const form = useForm({ email: "" });

  function submit(event: SubmitEvent) {
    event.preventDefault();
    $form.post(routes.passwordCreate());
  }
</script>

<svelte:head>
  <title>Reset password</title>
</svelte:head>

<Layout>
  <AuthCard
    title="Reset your password"
    description="Enter your email address and we will send you a reset code."
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
      <Button
        type="submit"
        size="lg"
        disabled={$form.processing}
        aria-busy={$form.processing}
        class="w-full"
      >
        {#if $form.processing}<Spinner />{/if}
        Send reset code
      </Button>
    </form>

    {#snippet footer()}
      <p class="text-xs">
        Remember your password?
        <Link
          class="text-primary underline-offset-4 hover:underline"
          href={routes.sessionNew()}>Sign in</Link
        >
      </p>
    {/snippet}
  </AuthCard>
</Layout>
