<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import { setAuth } from '$lib/stores/auth';
  import { getCallbackRedirectUri } from '$lib/auth';

  let error = $state<string | null>(null);
  let loading = $state(true);

  onMount(async () => {
    const code = $page.url.searchParams.get('code');
    const errorParam = $page.url.searchParams.get('error');
    const errorDesc = $page.url.searchParams.get('error_description');

    if (errorParam) {
      error = errorDesc ?? errorParam;
      loading = false;
      return;
    }

    if (!code) {
      error = 'No authorization code received.';
      loading = false;
      return;
    }

    try {
      const result = await api.auth.login(code, getCallbackRedirectUri());
      setAuth(result.tokens, result.user);
      goto('/');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Authentication failed.';
      loading = false;
    }
  });
</script>

<div class="min-h-screen flex items-center justify-center bg-slate-50">
  <div class="bg-white rounded-lg shadow-lg p-8 max-w-sm w-full text-center">
    {#if loading}
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto mb-4"></div>
      <p class="text-gray-500">Signing you in...</p>
    {:else if error}
      <div class="text-red-600 mb-4">
        <svg class="w-12 h-12 mx-auto mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.27 16.5c-.77.833.192 2.5 1.732 2.5z"/>
        </svg>
        <p class="font-medium">Sign-in failed</p>
        <p class="text-sm mt-1">{error}</p>
      </div>
      <a href="/login" class="text-blue-600 hover:underline text-sm">Try again</a>
    {/if}
  </div>
</div>
