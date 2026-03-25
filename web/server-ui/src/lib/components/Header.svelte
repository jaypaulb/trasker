<script lang="ts">
  import { currentUser, clearAuth } from '$lib/stores/auth';
  import { theme, toggleTheme } from '$lib/stores/theme';
  import { goto } from '$app/navigation';

  let showMenu = $state(false);

  function handleLogout() {
    clearAuth();
    goto('/login');
  }
</script>

<header class="bg-white dark:bg-slate-800 border-b border-gray-200 dark:border-slate-700 px-6 py-3 flex items-center justify-between">
  <div>
    <!-- Breadcrumb or page title slot could go here -->
  </div>

  <div class="flex items-center gap-3">
    <!-- Theme toggle -->
    <button
      onclick={toggleTheme}
      class="p-2 rounded-lg text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
      aria-label="Toggle dark mode"
    >
      {#if $theme === 'dark'}
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"/>
        </svg>
      {:else}
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"/>
        </svg>
      {/if}
    </button>

    <!-- User menu -->
    <div class="relative">
      <button
        onclick={() => showMenu = !showMenu}
        class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white"
      >
        <div class="w-8 h-8 bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 rounded-full flex items-center justify-center font-medium">
          {$currentUser?.display_name?.charAt(0) ?? '?'}
        </div>
        <span>{$currentUser?.display_name ?? 'User'}</span>
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/>
        </svg>
      </button>

      {#if showMenu}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="fixed inset-0 z-10"
          onclick={() => showMenu = false}
          onkeydown={() => {}}
        ></div>
        <div class="absolute right-0 mt-2 w-48 bg-white dark:bg-slate-800 rounded-lg shadow-lg border dark:border-slate-700 z-20 py-1">
          <div class="px-4 py-2 text-xs text-gray-500 dark:text-gray-400 border-b dark:border-slate-700">
            {$currentUser?.email}
            <br/>
            <span class="capitalize">{$currentUser?.role}</span>
          </div>
          <button
            onclick={handleLogout}
            class="w-full text-left px-4 py-2 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20"
          >
            Sign out
          </button>
        </div>
      {/if}
    </div>
  </div>
</header>
