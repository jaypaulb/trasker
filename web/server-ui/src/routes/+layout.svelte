<script lang="ts">
  import '../app.css';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { isAuthenticated } from '$lib/stores/auth';
  import { onMount } from 'svelte';

  let { children } = $props();

  const publicRoutes = ['/login', '/auth/callback'];

  onMount(() => {
    // Reactive auth guard
    const unsubPage = page.subscribe(($page) => {
      const unsubAuth = isAuthenticated.subscribe(($isAuth) => {
        const isPublic = publicRoutes.some((r) => $page.url.pathname.startsWith(r));
        if (!$isAuth && !isPublic) {
          goto('/login');
        }
      });
      // Only check once per navigation
      unsubAuth();
    });

    return unsubPage;
  });
</script>

{@render children()}
