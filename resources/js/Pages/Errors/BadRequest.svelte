<script lang="ts">
  import ErrorCard from "@/Components/Errors/ErrorCard.svelte";
  import Layout from "@/Layouts/Layout.svelte";
</script>

<svelte:head>
  <title>Bad request</title>
</svelte:head>

<Layout>
  <ErrorCard
    status="400"
    title="Bad request"
    description="The request was invalid and could not be processed."
  />
</Layout>
