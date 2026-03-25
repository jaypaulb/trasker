<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { AuditLogEntry } from '$lib/types';
  import DateRangeFilter from '$lib/components/DateRangeFilter.svelte';

  let entries = $state<AuditLogEntry[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let from = $state(new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10));
  let to = $state(new Date().toISOString().slice(0, 10));
  let actionFilter = $state('');

  const actionTypes = [
    '', 'entry.edit', 'entry.delete', 'user.role_change',
    'key.revoke', 'settings.update',
  ];

  async function loadData() {
    loading = true;
    error = null;
    try {
      entries = await api.admin.auditLog({
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
        action: actionFilter || undefined,
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load audit log.';
    } finally {
      loading = false;
    }
  }

  onMount(loadData);

  function handleDateChange() {
    loadData();
  }

  function handleActionFilter(e: Event) {
    actionFilter = (e.target as HTMLSelectElement).value;
    loadData();
  }

  function formatTimestamp(iso: string): string {
    return new Date(iso).toLocaleString(undefined, {
      year: 'numeric', month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
    });
  }

  function formatJson(val: unknown): string {
    if (val === null || val === undefined) return '-';
    try {
      return JSON.stringify(val, null, 2);
    } catch {
      return String(val);
    }
  }

  const actionLabels: Record<string, string> = {
    'entry.edit': 'Edit Entry',
    'entry.delete': 'Delete Entry',
    'user.role_change': 'Change Role',
    'key.revoke': 'Revoke API Key',
    'settings.update': 'Update Settings',
  };

  const actionColors: Record<string, string> = {
    'entry.edit': 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    'entry.delete': 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300',
    'user.role_change': 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    'key.revoke': 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300',
    'settings.update': 'bg-gray-100 text-gray-700 dark:bg-slate-700 dark:text-gray-300',
  };
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Audit Log</h1>

  <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-4 flex items-center gap-4 flex-wrap">
    <DateRangeFilter bind:from bind:to onchange={handleDateChange} />

    <div class="flex items-center gap-2">
      <label class="text-sm text-gray-500 dark:text-gray-400">Action</label>
      <select onchange={handleActionFilter} class="border dark:border-slate-600 rounded px-2 py-1 text-sm bg-white dark:bg-slate-700 dark:text-gray-200">
        {#each actionTypes as action}
          <option value={action}>{action ? (actionLabels[action] ?? action) : 'All Actions'}</option>
        {/each}
      </select>
    </div>
  </div>

  {#if loading}
    <div class="text-gray-500 dark:text-gray-400">Loading audit log...</div>
  {:else if error}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 rounded-lg p-4">{error}</div>
  {:else if entries.length === 0}
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-8 text-center text-gray-400 dark:text-gray-500">
      No audit log entries for the selected period and filter.
    </div>
  {:else}
    <div class="space-y-3">
      {#each entries as entry}
        <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-4">
          <div class="flex items-center gap-3 mb-2">
            <span class="inline-block px-2 py-0.5 text-xs rounded-full {actionColors[entry.action] ?? 'bg-gray-100 text-gray-700 dark:bg-slate-700 dark:text-gray-300'}">
              {actionLabels[entry.action] ?? entry.action}
            </span>
            <span class="text-sm text-gray-900 dark:text-white font-medium">
              {entry.admin_name ?? entry.admin_id.slice(0, 8)}
            </span>
            <span class="text-xs text-gray-400 dark:text-gray-500">{formatTimestamp(entry.created_at)}</span>
          </div>

          <div class="text-xs text-gray-500 dark:text-gray-400 mb-1">
            Target: <span class="font-mono">{entry.target_type} / {entry.target_id.slice(0, 8)}...</span>
          </div>

          {#if entry.old_value || entry.new_value}
            <div class="grid grid-cols-2 gap-3 mt-2">
              <div>
                <span class="text-xs text-gray-400 dark:text-gray-500">Before:</span>
                <pre class="text-xs bg-red-50 dark:bg-red-900/20 dark:text-red-300 rounded p-2 mt-1 overflow-auto max-h-32">{formatJson(entry.old_value)}</pre>
              </div>
              <div>
                <span class="text-xs text-gray-400 dark:text-gray-500">After:</span>
                <pre class="text-xs bg-green-50 dark:bg-green-900/20 dark:text-green-300 rounded p-2 mt-1 overflow-auto max-h-32">{formatJson(entry.new_value)}</pre>
              </div>
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
