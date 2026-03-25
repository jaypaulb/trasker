<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { User, ApiKey } from '$lib/types';

  let users = $state<User[]>([]);
  let apiKeys = $state<Map<string, ApiKey[]>>(new Map());
  let loading = $state(true);
  let error = $state<string | null>(null);
  let actionError = $state<string | null>(null);

  onMount(async () => {
    try {
      users = await api.users.listTeam();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load users.';
    } finally {
      loading = false;
    }
  });

  async function changeRole(userId: string, newRole: string) {
    actionError = null;
    try {
      const updated = await api.users.updateRole(userId, newRole);
      users = users.map((u) => (u.id === userId ? updated : u));
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to update role.';
    }
  }

  async function loadKeys(userId: string) {
    try {
      const keys = await api.admin.listApiKeys(userId);
      apiKeys = new Map([...apiKeys, [userId, keys]]);
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to load API keys.';
    }
  }

  async function revokeKey(keyId: string, userId: string) {
    if (!confirm('Revoke this API key? The associated client will stop working until the user downloads a new one.')) return;
    actionError = null;
    try {
      await api.admin.revokeKey(keyId);
      // Reload keys for this user
      await loadKeys(userId);
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to revoke key.';
    }
  }

  function formatDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, {
      year: 'numeric', month: 'short', day: 'numeric',
    });
  }

  const roleBadgeClasses: Record<string, string> = {
    admin: 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-400',
    manager: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
    member: 'bg-gray-100 text-gray-700 dark:bg-slate-700 dark:text-gray-300',
  };
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900 dark:text-white">User Management</h1>

  {#if actionError}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 rounded-lg p-3 text-sm">{actionError}</div>
  {/if}

  {#if loading}
    <div class="text-gray-500 dark:text-gray-400">Loading users...</div>
  {:else if error}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 rounded-lg p-4">{error}</div>
  {:else}
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 dark:bg-slate-700/50 text-left text-gray-500 dark:text-gray-400">
          <tr>
            <th class="px-5 py-3 font-medium">Name</th>
            <th class="px-5 py-3 font-medium">Email</th>
            <th class="px-5 py-3 font-medium">Role</th>
            <th class="px-5 py-3 font-medium">Joined</th>
            <th class="px-5 py-3 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y dark:divide-slate-700">
          {#each users as user}
            <tr>
              <td class="px-5 py-3 text-gray-900 dark:text-white font-medium">{user.display_name}</td>
              <td class="px-5 py-3 text-gray-600 dark:text-gray-300">{user.email}</td>
              <td class="px-5 py-3">
                <select
                  value={user.role}
                  onchange={(e) => changeRole(user.id, (e.target as HTMLSelectElement).value)}
                  class="text-xs px-2 py-1 rounded border dark:border-slate-600 {roleBadgeClasses[user.role] ?? ''}"
                >
                  <option value="member">Member</option>
                  <option value="manager">Manager</option>
                  <option value="admin">Admin</option>
                </select>
              </td>
              <td class="px-5 py-3 text-gray-600 dark:text-gray-300">{formatDate(user.created_at)}</td>
              <td class="px-5 py-3">
                <button
                  onclick={() => loadKeys(user.id)}
                  class="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300 text-xs"
                >
                  View API Keys
                </button>
              </td>
            </tr>

            <!-- Expandable API Keys row -->
            {#if apiKeys.has(user.id)}
              <tr class="bg-gray-50 dark:bg-slate-700/30">
                <td colspan="5" class="px-5 py-3">
                  <div class="text-xs text-gray-500 dark:text-gray-400 mb-2">API Keys for {user.display_name}:</div>
                  {#if (apiKeys.get(user.id) ?? []).length === 0}
                    <span class="text-xs text-gray-400 dark:text-gray-500">No API keys.</span>
                  {:else}
                    <div class="space-y-1">
                      {#each apiKeys.get(user.id) ?? [] as key}
                        <div class="flex items-center gap-3 text-xs">
                          <span class="font-mono text-gray-500 dark:text-gray-400">{key.key_prefix}...</span>
                          <span class={key.revoked ? 'text-red-500 dark:text-red-400' : 'text-green-600 dark:text-green-400'}>
                            {key.revoked ? 'Revoked' : 'Active'}
                          </span>
                          <span class="text-gray-400 dark:text-gray-500">Last used: {formatDate(key.last_used_at)}</span>
                          <span class="text-gray-400 dark:text-gray-500">Expires: {formatDate(key.expires_at)}</span>
                          {#if !key.revoked}
                            <button
                              onclick={() => revokeKey(key.id, user.id)}
                              class="text-red-600 dark:text-red-400 hover:text-red-800 dark:hover:text-red-300"
                            >
                              Revoke
                            </button>
                          {/if}
                        </div>
                      {/each}
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
