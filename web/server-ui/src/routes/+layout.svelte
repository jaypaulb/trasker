<script lang="ts">
  import '../app.css';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { isAuthenticated } from '$lib/stores/auth';
  import { theme } from '$lib/stores/theme';
  import { onMount } from 'svelte';

  let { children } = $props();

  const publicRoutes = ['/login', '/auth/callback'];

  onMount(() => {
    // Apply dark class to html element reactively
    const unsubTheme = theme.subscribe(($theme) => {
      document.documentElement.classList.toggle('dark', $theme === 'dark');
    });

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

    return () => {
      unsubTheme();
      unsubPage();
    };
  });
</script>

{@render children()}
