<script lang="ts">
  import { currentUser, clearAuth } from '$lib/stores/auth';
  import { goto } from '$app/navigation';

  let showMenu = $state(false);

  function handleLogout() {
    clearAuth();
    goto('/login');
  }
</script>

<header class="bg-white border-b border-gray-200 px-6 py-3 flex items-center justify-between">
  <div>
    <!-- Breadcrumb or page title slot could go here -->
  </div>

  <div class="relative">
    <button
      onclick={() => showMenu = !showMenu}
      class="flex items-center gap-2 text-sm text-gray-700 hover:text-gray-900"
    >
      <div class="w-8 h-8 bg-blue-100 text-blue-700 rounded-full flex items-center justify-center font-medium">
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
      <div class="absolute right-0 mt-2 w-48 bg-white rounded-lg shadow-lg border z-20 py-1">
        <div class="px-4 py-2 text-xs text-gray-500 border-b">
          {$currentUser?.email}
          <br/>
          <span class="capitalize">{$currentUser?.role}</span>
        </div>
        <button
          onclick={handleLogout}
          class="w-full text-left px-4 py-2 text-sm text-red-600 hover:bg-red-50"
        >
          Sign out
        </button>
      </div>
    {/if}
  </div>
</header>
