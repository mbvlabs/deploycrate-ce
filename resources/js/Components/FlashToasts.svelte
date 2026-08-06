<script lang="ts">
  import { router } from "@inertiajs/svelte";
  import { toast } from "svelte-sonner";
  import { onMount } from "svelte";

  import { Toaster } from "@/Components/ui/sonner";

  type FlashMessage = {
    id?: string;
    type: string;
    message: string;
  };

  let { initialFlashes }: { initialFlashes?: unknown } = $props();
  const seenFlashIds = new Set<string>();

  function flashMessage(value: unknown): FlashMessage | null {
    if (!value || typeof value !== "object") return null;

    const flash = value as Record<string, unknown>;
    const type = typeof flash.type === "string" ? flash.type : flash.Type;
    const message =
      typeof flash.message === "string" ? flash.message : flash.Message;
    const id = typeof flash.id === "string" ? flash.id : flash.ID;
    if (typeof type !== "string" || typeof message !== "string") return null;
    return { id: typeof id === "string" ? id : undefined, type, message };
  }

  function pushFlashes(flashes: unknown) {
    if (!Array.isArray(flashes)) return;

    for (const value of flashes) {
      const flash = flashMessage(value);
      if (!flash) continue;
      if (flash.id && seenFlashIds.has(flash.id)) continue;
      if (flash.id) seenFlashIds.add(flash.id);

      const notify =
        flash.type === "success"
          ? toast.success
          : flash.type === "error"
            ? toast.error
            : flash.type === "warning"
              ? toast.warning
              : toast.info;
      notify(flash.message, { id: flash.id });
    }
  }

  onMount(() => {
    pushFlashes(initialFlashes);
    return router.on("success", (event) => {
      pushFlashes(event.detail.page.props.flash);
    });
  });
</script>

<Toaster position="bottom-right" richColors closeButton />
