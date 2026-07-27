<script lang="ts">
  import { onMount } from 'svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'

  let { auth, handoff }: { auth: { email: string }; handoff: { action: string; manifest: string } } = $props()
  let form: HTMLFormElement
  onMount(() => form.submit())
</script>

<svelte:head><title>Continue to GitHub</title></svelte:head>

<DashboardLayout email={auth.email}>
  <section class="mx-auto max-w-xl border border-border p-8 text-center">
    <h1 class="text-2xl font-semibold">Opening GitHub</h1>
    <p class="mt-3 text-sm text-muted-foreground">DeployCrate is securely handing the private App manifest to GitHub.</p>
    <form bind:this={form} method="post" action={handoff.action} class="mt-6">
      <input type="hidden" name="manifest" value={handoff.manifest} />
      <button type="submit" class="text-sm text-primary hover:underline">Continue if you are not redirected</button>
    </form>
  </section>
</DashboardLayout>
