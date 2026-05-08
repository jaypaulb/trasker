<script lang="ts">
  import Sidebar from '$lib/components/Sidebar.svelte';
  import Header from '$lib/components/Header.svelte';
  import { goto } from '$app/navigation';
  import { isAuthenticated, currentUser } from '$lib/stores/auth';

  let { children } = $props();

  // Route guard: enforce authentication and force_password_change.
  // The change-password page lives under /auth/* (outside this layout),
  // so redirecting from inside (app)/* will not loop.
  $effect(() => {
    if (!$isAuthenticated) {
      goto('/login');
      return;
    }
    if ($currentUser?.force_password_change) {
      goto('/auth/change-password');
    }
  });
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
