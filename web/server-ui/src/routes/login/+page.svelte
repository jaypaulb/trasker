<script lang="ts">
  import { getLoginUrl } from '$lib/auth';
  import { isAuthenticated, setAuth } from '$lib/stores/auth';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let oidcEnabled = $state(false);
  let localEnabled = $state(true);
  let configLoaded = $state(false);
  let adminEmail = $state('');
  let adminPassword = $state('');
  let loginError = $state<string | null>(null);
  let loading = $state(false);

  onMount(async () => {
    if ($isAuthenticated) {
      goto('/');
      return;
    }

    try {
      const config = await api.auth.config();
      oidcEnabled = config.oidc_enabled;
      localEnabled = config.local_enabled;
    } catch {
      // If config fails, default to local login enabled (server bootstraps an admin)
      oidcEnabled = false;
      localEnabled = true;
    } finally {
      configLoaded = true;
    }
  });

  function handleOidcLogin() {
    window.location.href = getLoginUrl();
  }

  async function handleLocalLogin() {
    loginError = null;
    loading = true;
    try {
      const result = await api.auth.localLogin(adminEmail, adminPassword);
      setAuth(result.tokens, result.user);

      if (result.user.force_password_change) {
        goto('/auth/change-password');
      } else {
        goto('/');
      }
    } catch (e: unknown) {
      if (e && typeof e === 'object' && 'status' in e && (e as { status: number }).status === 401) {
        loginError = 'Invalid email or password';
      } else {
        loginError = 'Login failed. Please try again.';
      }
    } finally {
      loading = false;
    }
  }
</script>

<div class="min-h-screen flex items-center justify-center bg-slate-50 dark:bg-slate-950 relative">
  <div class="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-8 max-w-sm w-full">
    <h1 class="text-2xl font-bold text-gray-900 dark:text-white mb-2 text-center">Trasker</h1>
    <p class="text-gray-500 dark:text-gray-400 mb-6 text-center">Sign in to your organization's time tracking dashboard</p>

    {#if !configLoaded}
      <p class="text-gray-400 dark:text-gray-500 text-sm text-center">Loading...</p>
    {:else}
      {#if oidcEnabled}
        <button
          onclick={handleOidcLogin}
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
      {/if}

      {#if oidcEnabled && localEnabled}
        <div class="flex items-center gap-3 my-6">
          <div class="flex-1 h-px bg-gray-200 dark:bg-slate-600"></div>
          <span class="text-xs text-gray-400 dark:text-gray-500">or</span>
          <div class="flex-1 h-px bg-gray-200 dark:bg-slate-600"></div>
        </div>
      {/if}

      {#if localEnabled}
        <form onsubmit={(e) => { e.preventDefault(); handleLocalLogin(); }} class="text-left space-y-4">
          <div>
            <label for="email" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Email</label>
            <input
              id="email"
              type="email"
              bind:value={adminEmail}
              autocomplete="username"
              class="w-full px-3 py-2 border border-gray-300 dark:border-slate-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm bg-white dark:bg-slate-700 dark:text-gray-200"
              placeholder="admin@localhost"
              required
            />
          </div>
          <div>
            <label for="password" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Password</label>
            <input
              id="password"
              type="password"
              bind:value={adminPassword}
              autocomplete="current-password"
              class="w-full px-3 py-2 border border-gray-300 dark:border-slate-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm bg-white dark:bg-slate-700 dark:text-gray-200"
              required
            />
          </div>

          {#if loginError}
            <p class="text-red-600 dark:text-red-400 text-sm">{loginError}</p>
          {/if}

          <button
            type="submit"
            disabled={loading}
            class="w-full bg-gray-800 dark:bg-gray-700 hover:bg-gray-900 dark:hover:bg-gray-600 text-white font-medium py-2.5 px-4 rounded-lg transition-colors text-sm disabled:opacity-50"
          >
            {loading ? 'Signing in...' : 'Sign in'}
          </button>
        </form>
      {/if}

      {#if !oidcEnabled && !localEnabled}
        <p class="text-gray-400 dark:text-gray-500 text-sm text-center">
          No sign-in methods are enabled. Contact your administrator.
        </p>
      {/if}

      {#if !oidcEnabled && localEnabled}
        <p class="text-xs text-gray-400 dark:text-gray-500 mt-6 text-center">
          Microsoft Entra ID is not configured. Configure it in admin settings to enable organization sign-in.
        </p>
      {/if}
    {/if}
  </div>
</div>
