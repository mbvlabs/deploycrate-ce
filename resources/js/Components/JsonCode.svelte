<script lang="ts">
  import CopyIcon from '@lucide/svelte/icons/copy'
  import { toast } from 'svelte-sonner'

  import { Button } from '@/Components/ui/button'
  import * as ScrollArea from '@/Components/ui/scroll-area'
  import { cn } from '@/lib/utils.js'

  type TokenKind = 'key' | 'string' | 'number' | 'literal' | 'punctuation' | 'plain'
  type Token = { value: string; kind: TokenKind }

  let { value, class: className }: { value: unknown; class?: string } = $props()

  const formatted = $derived(JSON.stringify(value ?? {}, null, 2) ?? '{}')
  const tokens = $derived(tokenize(formatted))

  function tokenize(input: string): Token[] {
    const pattern = /"(?:\\.|[^"\\])*"|\b(?:true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|[{}\[\],:]|\s+|./g
    const matches = [...input.matchAll(pattern)]

    return matches.map((match) => {
      const token = match[0]
      const offset = (match.index ?? 0) + token.length

      if (/^\s+$/.test(token)) return { value: token, kind: 'plain' }
      if (token.startsWith('"')) {
        const following = input.slice(offset).match(/^\s*/)?.[0].length ?? 0
        return {
          value: token,
          kind: input[offset + following] === ':' ? 'key' : 'string',
        }
      }
      if (/^(true|false|null)$/.test(token)) return { value: token, kind: 'literal' }
      if (/^-?\d/.test(token)) return { value: token, kind: 'number' }
      if (/^[{}\[\],:]$/.test(token)) return { value: token, kind: 'punctuation' }
      return { value: token, kind: 'plain' }
    })
  }

  function tokenClass(kind: TokenKind) {
    return {
      key: 'text-primary',
      string: 'text-emerald-400',
      number: 'text-amber-400',
      literal: 'text-violet-400',
      punctuation: 'text-muted-foreground',
      plain: '',
    }[kind]
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(formatted)
      toast.success('JSON copied to clipboard')
    } catch {
      toast.error('JSON could not be copied')
    }
  }
</script>

<div class={cn('relative min-w-0 max-w-full border border-border bg-muted/20', className)}>
  <Button type="button" variant="ghost" size="icon-xs" class="absolute right-2 top-2 z-10 bg-background/80" aria-label="Copy JSON" title="Copy JSON" onclick={copy}><CopyIcon /></Button>
  <ScrollArea.Root orientation="both" class="max-h-96 w-full">
    <pre class="min-w-max whitespace-pre p-4 pr-10 text-xs leading-5"><code>{#each tokens as token}<span class={tokenClass(token.kind)}>{token.value}</span>{/each}</code></pre>
  </ScrollArea.Root>
</div>
