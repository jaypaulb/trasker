<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Timesheet, TimesheetEntry, User } from '$lib/types';
  import DateRangeFilter from '$lib/components/DateRangeFilter.svelte';

  let timesheets = $state<Timesheet[]>([]);
  let users = $state<User[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let actionError = $state<string | null>(null);

  let from = $state(new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10));
  let to = $state(new Date().toISOString().slice(0, 10));

  let editingEntry = $state<TimesheetEntry | null>(null);
  let editTag = $state('');
  let editNotes = $state('');
  let editDuration = $state(0);

  async function loadData() {
    loading = true;
    error = null;
    try {
      const params = {
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
      };
      const [ts, userList] = await Promise.all([
        api.timesheets.team(params),
        users.length === 0 ? api.users.listTeam() : Promise.resolve(users),
      ]);
      timesheets = ts;
      if (users.length === 0) users = userList;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load data.';
    } finally {
      loading = false;
    }
  }

  onMount(loadData);

  function handleDateChange() {
    loadData();
  }

  function startEdit(entry: TimesheetEntry) {
    editingEntry = entry;
    editTag = entry.tag;
    editNotes = entry.notes ?? '';
    editDuration = entry.duration_s;
  }

  async function saveEdit() {
    if (!editingEntry) return;
    actionError = null;
    try {
      await api.admin.editEntry(editingEntry.id, {
        tag: editTag,
        notes: editNotes,
        duration_s: editDuration,
      });
      editingEntry = null;
      await loadData();
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to save changes.';
    }
  }

  async function deleteEntry(entryId: string) {
    if (!confirm(
      'Delete this timesheet entry? This action is audit-logged and cannot be undone.'
    )) return;
    actionError = null;
    try {
      await api.admin.deleteEntry(entryId);
      await loadData();
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to delete entry.';
    }
  }

  function cancelEdit() {
    editingEntry = null;
  }

  function formatDuration(s: number): string {
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    return `${h}h ${m}m`;
  }

  const userMap = $derived(new Map(users.map((u) => [u.id, u])));
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Timesheet Corrections</h1>

  <div class="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 text-amber-800 dark:text-amber-300 rounded-lg p-3 text-sm">
    All edits and deletions are recorded in the audit log. Only modify entries when there is a legitimate correction needed.
  </div>

  {#if actionError}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 rounded-lg p-3 text-sm">{actionError}</div>
  {/if}

  <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-4">
    <DateRangeFilter bind:from bind:to onchange={handleDateChange} />
  </div>

  <!-- Edit Modal -->
  {#if editingEntry}
    <div class="fixed inset-0 bg-black/50 z-40 flex items-center justify-center">
      <div class="bg-white dark:bg-slate-800 rounded-lg shadow-xl p-6 w-full max-w-md">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Edit Entry</h3>
        <div class="space-y-3">
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Tag</label>
            <input type="text" bind:value={editTag} class="border dark:border-slate-600 rounded px-3 py-2 w-full text-sm bg-white dark:bg-slate-700 dark:text-gray-200" />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Duration (seconds)</label>
            <input type="number" bind:value={editDuration} class="border dark:border-slate-600 rounded px-3 py-2 w-full text-sm bg-white dark:bg-slate-700 dark:text-gray-200" />
            <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">{formatDuration(editDuration)}</p>
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Notes</label>
            <textarea bind:value={editNotes} rows="3" class="border dark:border-slate-600 rounded px-3 py-2 w-full text-sm bg-white dark:bg-slate-700 dark:text-gray-200"></textarea>
          </div>
        </div>
        <div class="flex justify-end gap-2 mt-4">
          <button onclick={cancelEdit} class="px-4 py-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white">Cancel</button>
          <button onclick={saveEdit} class="px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700">Save Changes</button>
        </div>
      </div>
    </div>
  {/if}

  {#if loading}
    <div class="text-gray-500 dark:text-gray-400">Loading entries...</div>
  {:else if error}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 rounded-lg p-4">{error}</div>
  {:else}
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 dark:bg-slate-700/50 text-left text-gray-500 dark:text-gray-400">
          <tr>
            <th class="px-5 py-3 font-medium">Person</th>
            <th class="px-5 py-3 font-medium">Tag</th>
            <th class="px-5 py-3 font-medium">Date</th>
            <th class="px-5 py-3 font-medium">Duration</th>
            <th class="px-5 py-3 font-medium">Notes</th>
            <th class="px-5 py-3 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y dark:divide-slate-700">
          {#each timesheets as ts}
            {#each ts.entries as entry}
              <tr>
                <td class="px-5 py-3 text-gray-900 dark:text-white">
                  {userMap.get(ts.user_id)?.display_name ?? 'Unknown'}
                </td>
                <td class="px-5 py-3">
                  <span class="inline-block px-2 py-0.5 bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 text-xs rounded-full">
                    {entry.tag}
                  </span>
                </td>
                <td class="px-5 py-3 text-gray-600 dark:text-gray-300">
                  {new Date(entry.started_at).toLocaleDateString()}
                </td>
                <td class="px-5 py-3 text-gray-600 dark:text-gray-300">{formatDuration(entry.duration_s)}</td>
                <td class="px-5 py-3 text-gray-500 dark:text-gray-400 text-xs max-w-xs truncate">{entry.notes ?? ''}</td>
                <td class="px-5 py-3 space-x-2">
                  <button onclick={() => startEdit(entry)} class="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300 text-xs">Edit</button>
                  <button onclick={() => deleteEntry(entry.id)} class="text-red-600 dark:text-red-400 hover:text-red-800 dark:hover:text-red-300 text-xs">Delete</button>
                </td>
              </tr>
            {/each}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
