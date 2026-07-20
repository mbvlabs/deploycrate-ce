<script lang="ts">
  import ErrorCard from '@/Components/Errors/ErrorCard.svelte'
  import Layout from '@/Layouts/Layout.svelte'
</script>

<svelte:head>
  <title>Not found</title>
</svelte:head>

<Layout>
  <ErrorCard status="404" title="Not found" description="The page you are looking for could not be found." />
</Layout>
