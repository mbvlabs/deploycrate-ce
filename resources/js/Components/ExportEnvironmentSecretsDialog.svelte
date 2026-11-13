<script lang="ts">
  import { onDestroy } from "svelte";
  import { Button } from "@/Components/ui/button";
  import * as Dialog from "@/Components/ui/dialog";
  import { Input } from "@/Components/ui/input";
  import { Textarea } from "@/Components/ui/textarea";
  import { routes } from "@/routes";
  import { toast } from "svelte-sonner";

  let {
    applicationId,
    environmentId,
  }: {
    applicationId: string;
    environmentId: string;
  } = $props();

  let open = $state(false);
  let password = $state("");
  let text = $state<string | null>(null);
  let error = $state("");
  let processing = $state(false);
  let request: AbortController | undefined;

  onDestroy(() => request?.abort());

  function changeOpen(value: boolean) {
    open = value;
    request?.abort();
    request = undefined;
    password = "";
    text = null;
    error = "";
    processing = false;
  }

  async function reveal(event: SubmitEvent) {
    event.preventDefault();
    if (!password || processing) return;
    const controller = new AbortController();
    request = controller;
    processing = true;
    error = "";
    try {
      const response = await window.fetch(
        routes.environmentSecretsExport(applicationId, environmentId),
        {
          method: "POST",
          credentials: "same-origin",
          cache: "no-store",
          signal: controller.signal,
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ password }),
        },
      );
      const payload = await response.json().catch(() => ({}));
      if (controller.signal.aborted) return;
      if (!response.ok || typeof payload.text !== "string")
        throw new Error(
          payload.error ||
            "Secrets could not be exported. Please try again later.",
        );
      text = payload.text;
    } catch (cause) {
      if (!controller.signal.aborted)
        error =
          cause instanceof Error
            ? cause.message
            : "Secrets could not be exported";
    } finally {
      if (request === controller) {
        password = "";
        processing = false;
        request = undefined;
      }
    }
  }

  async function copy() {
    if (text === null) return;
    try {
      await navigator.clipboard.writeText(text);
      toast.success("Secrets copied");
    } catch {
      error = "Copy failed. Select the text below and copy it manually.";
    }
  }
</script>

<Dialog.Root {open} onOpenChange={changeOpen}>
  <Dialog.Trigger>
    {#snippet child({ props })}
      <Button {...props} type="button" variant="outline">Export secrets</Button>
    {/snippet}
  </Dialog.Trigger>
  <Dialog.Content class="sm:max-w-2xl">
    <Dialog.Header>
      <Dialog.Title>Export Environment secrets</Dialog.Title>
      <Dialog.Description>
        {#if text === null}
          Enter your account password to reveal this environment’s current
          secrets, including Resource-managed secrets.
        {:else}
          Copy these .env values into your new setup. Keep them private. Closing
          this dialog clears the revealed values, but not your clipboard.
        {/if}
      </Dialog.Description>
    </Dialog.Header>
    {#if text === null}
      <form class="space-y-4" onsubmit={reveal}>
        <label class="grid gap-2 text-sm">
          Your password
          <Input
            type="password"
            bind:value={password}
            autocomplete="current-password"
            required
            disabled={processing}
          />
        </label>
        {#if error}<p role="alert" class="text-sm text-destructive">
            {error}
          </p>{/if}
        <Dialog.Footer>
          <Button
            type="button"
            variant="outline"
            onclick={() => changeOpen(false)}>Cancel</Button
          >
          <Button
            type="submit"
            disabled={processing || !password}
            aria-busy={processing}
          >
            {processing ? "Verifying…" : "Reveal secrets"}
          </Button>
        </Dialog.Footer>
      </form>
    {:else}
      <Textarea
        value={text}
        readonly
        aria-label="Exported secrets"
        class="min-h-80 font-mono"
        autocomplete="off"
        spellcheck="false"
      />
      {#if text === ""}<p class="text-sm text-muted-foreground">
          This environment has no secrets.
        </p>{/if}
      {#if error}<p role="alert" class="text-sm text-destructive">
          {error}
        </p>{/if}
      <Dialog.Footer>
        <Button
          type="button"
          variant="outline"
          onclick={() => changeOpen(false)}>Close</Button
        >
        <Button type="button" onclick={copy} disabled={text === ""}
          >Copy secrets</Button
        >
      </Dialog.Footer>
    {/if}
  </Dialog.Content>
</Dialog.Root>
