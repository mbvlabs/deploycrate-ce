<script lang="ts">
  import { onMount } from "svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import { Spinner } from "@/Components/ui/spinner";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";

  let {
    auth,
    handoff,
  }: {
    auth: { email: string };
    handoff: { action: string; manifest: string };
  } = $props();
  let form: HTMLFormElement;
  onMount(() => form.submit());
</script>

<svelte:head><title>Continue to GitHub</title></svelte:head>

<DashboardLayout email={auth.email}>
  <Card.Root class="mx-auto max-w-xl text-center">
    <Card.Header
      ><div class="mb-2 flex justify-center"><Spinner class="size-6" /></div>
      <Card.Title>Opening GitHub</Card.Title><Card.Description
        >DeployCrate is securely handing the private App manifest to GitHub.</Card.Description
      ></Card.Header
    >
    <Card.Content
      ><form bind:this={form} method="post" action={handoff.action}>
        <input type="hidden" name="manifest" value={handoff.manifest} />
        <Button type="submit" variant="outline"
          >Continue if you are not redirected</Button
        >
      </form></Card.Content
    >
  </Card.Root>
</DashboardLayout>
