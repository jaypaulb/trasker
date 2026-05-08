<script lang="ts">
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';
  import { isAuthenticated, currentUser } from '$lib/stores/auth';
  import { onMount } from 'svelte';

  let currentPassword = $state('');
  let newPassword = $state('');
  let confirmPassword = $state('');
  let error = $state<string | null>(null);
  let loading = $state(false);

  onMount(() => {
    if (!$isAuthenticated) {
      goto('/login');
    }
  });

  async function handleSubmit() {
    error = null;

    if (newPassword !== confirmPassword) {
      error = 'Passwords do not match';
      return;
    }
    if (newPassword.length < 8) {
      error = 'Password must be at least 8 characters';
      return;
    }

    loading = true;
    try {
      const result = await api.auth.changePassword(currentPassword, newPassword);
      // Server clears force_password_change on success and returns the updated
      // user. Refresh the local store so the (app)/+layout.svelte route guard
      // stops redirecting back here.
      if (result.user) {
        currentUser.set(result.user);
      }
      goto('/');
    } catch (e: unknown) {
      if (e && typeof e === 'object' && 'status' in e && (e as { status: number }).status === 401) {
        error = 'Current password is incorrect';
      } else {
        error = 'Failed to change password';
      }
    } finally {
      loading = false;
    }
  }
</script>

<div class="min-h-screen flex items-center justify-center bg-slate-50 dark:bg-slate-950">
  <div class="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-8 max-w-sm w-full">
    <h1 class="text-xl font-bold text-gray-900 dark:text-white mb-2 text-center">Change Password</h1>
    <p class="text-gray-500 dark:text-gray-400 text-sm mb-6 text-center">
      You must change your password before continuing.
    </p>

    <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-4">
      <div>
        <label for="current" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Current password</label>
        <input
          id="current"
          type="password"
          bind:value={currentPassword}
          class="w-full px-3 py-2 border border-gray-300 dark:border-slate-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm bg-white dark:bg-slate-700 dark:text-gray-200"
          required
        />
      </div>
      <div>
        <label for="new" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">New password</label>
        <input
          id="new"
          type="password"
          bind:value={newPassword}
          class="w-full px-3 py-2 border border-gray-300 dark:border-slate-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm bg-white dark:bg-slate-700 dark:text-gray-200"
          minlength="8"
          required
        />
      </div>
      <div>
        <label for="confirm" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Confirm new password</label>
        <input
          id="confirm"
          type="password"
          bind:value={confirmPassword}
          class="w-full px-3 py-2 border border-gray-300 dark:border-slate-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent text-sm bg-white dark:bg-slate-700 dark:text-gray-200"
          minlength="8"
          required
        />
      </div>

      {#if error}
        <p class="text-red-600 dark:text-red-400 text-sm">{error}</p>
      {/if}

      <button
        type="submit"
        disabled={loading}
        class="w-full bg-gray-800 dark:bg-gray-700 hover:bg-gray-900 dark:hover:bg-gray-600 text-white font-medium py-2.5 px-4 rounded-lg transition-colors text-sm disabled:opacity-50"
      >
        {loading ? 'Changing...' : 'Change Password'}
      </button>
    </form>
  </div>
</div>
