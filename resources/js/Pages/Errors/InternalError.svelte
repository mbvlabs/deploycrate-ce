<script lang="ts">
  import ErrorCard from '@/Components/Errors/ErrorCard.svelte'
  import Layout from '@/Layouts/Layout.svelte'
</script>

<svelte:head>
  <title>Something went wrong</title>
</svelte:head>

<Layout>
  <ErrorCard status="500" title="Something went wrong" description="DeployCrate CE encountered an unexpected error." />
</Layout>
