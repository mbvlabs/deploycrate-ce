<script lang="ts">
  import TriangleAlertIcon from "@lucide/svelte/icons/triangle-alert";

  import { Button } from "@/Components/ui/button";
  import * as Empty from "@/Components/ui/empty";
  import { routes } from "@/routes";

  let {
    status,
    title,
    description,
  }: { status: string; title: string; description: string } = $props();
</script>

<Empty.Root class="w-full max-w-md border border-border bg-card">
  <Empty.Header>
    <Empty.Media
      variant="icon"
      class="border-destructive/40 bg-destructive/10 text-destructive"
      ><TriangleAlertIcon /></Empty.Media
    >
    <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">
      Error {status}
    </p>
    <Empty.Title>{title}</Empty.Title>
    <Empty.Description>{description}</Empty.Description>
  </Empty.Header>
  <Empty.Content
    ><Button href={routes.homePage()} variant="outline" class="w-full"
      >Return home</Button
    ></Empty.Content
  >
</Empty.Root>
