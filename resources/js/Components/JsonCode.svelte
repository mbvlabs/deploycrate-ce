<script lang="ts">
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
</script>

<pre
  class={cn(
    'min-w-0 max-w-full overflow-hidden whitespace-pre-wrap break-all border border-border bg-muted/20 p-4 text-xs leading-5 [overflow-wrap:anywhere]',
    className,
  )}
><code>{#each tokens as token}<span class={tokenClass(token.kind)}>{token.value}</span>{/each}</code></pre>
