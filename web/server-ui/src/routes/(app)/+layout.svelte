<script lang="ts">
  import Sidebar from '$lib/components/Sidebar.svelte';
  import Header from '$lib/components/Header.svelte';

  let { children } = $props();
</script>

<div class="flex min-h-screen bg-slate-50 dark:bg-slate-950">
  <Sidebar />
  <div class="flex-1 flex flex-col">
    <Header />
    <main class="flex-1 p-6">
      {@render children()}
    </main>
  </div>
</div>
