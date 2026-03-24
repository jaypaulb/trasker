<script lang="ts">
  import { getLoginUrl } from '$lib/auth';
  import { isAuthenticated } from '$lib/stores/auth';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  onMount(() => {
    if ($isAuthenticated) {
      goto('/');
    }
  });

  function handleLogin() {
    window.location.href = getLoginUrl();
  }
</script>

<div class="min-h-screen flex items-center justify-center bg-slate-50">
  <div class="bg-white rounded-lg shadow-lg p-8 max-w-sm w-full text-center">
    <h1 class="text-2xl font-bold text-gray-900 mb-2">Trasker</h1>
    <p class="text-gray-500 mb-8">Sign in to your organization's time tracking dashboard</p>

    <button
      onclick={handleLogin}
      class="w-full flex items-center justify-center gap-3 bg-[var(--color-primary)] hover:bg-[var(--color-primary-dark)] text-white font-medium py-3 px-4 rounded-lg transition-colors"
    >
      <svg class="w-5 h-5" viewBox="0 0 21 21" xmlns="http://www.w3.org/2000/svg">
        <rect x="1" y="1" width="9" height="9" fill="#f25022"/>
        <rect x="11" y="1" width="9" height="9" fill="#7fba00"/>
        <rect x="1" y="11" width="9" height="9" fill="#00a4ef"/>
        <rect x="11" y="11" width="9" height="9" fill="#ffb900"/>
      </svg>
      Sign in with Microsoft
    </button>

    <p class="text-xs text-gray-400 mt-6">
      Your organization uses Microsoft Entra ID for authentication.
    </p>
  </div>
</div>
