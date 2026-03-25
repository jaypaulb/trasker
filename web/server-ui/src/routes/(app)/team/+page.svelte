<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Timesheet, User } from '$lib/types';
  import DateRangeFilter from '$lib/components/DateRangeFilter.svelte';

  let timesheets = $state<Timesheet[]>([]);
  let users = $state<User[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Default: last 7 days
  let from = $state(new Date(Date.now() - 7 * 86400000).toISOString().slice(0, 10));
  let to = $state(new Date().toISOString().slice(0, 10));
  let filterUserId = $state<string>('');

  async function loadData() {
    loading = true;
    error = null;
    try {
      const params: { from: string; to: string; user_id?: string } = {
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
      };
      if (filterUserId) params.user_id = filterUserId;

      const [ts, userList] = await Promise.all([
        api.timesheets.team(params),
        users.length === 0 ? api.users.listTeam() : Promise.resolve(users),
      ]);
      timesheets = ts;
      if (users.length === 0) users = userList;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load team data.';
    } finally {
      loading = false;
    }
  }

  onMount(loadData);

  function handleDateChange() {
    loadData();
  }

  function handleUserFilter(e: Event) {
    filterUserId = (e.target as HTMLSelectElement).value;
    loadData();
  }

  function formatDuration(s: number): string {
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    return `${h}h ${m}m`;
  }

  // Build user lookup map
  const userMap = $derived(
    new Map(users.map((u) => [u.id, u]))
  );
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Team Submissions</h1>

  <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-4 flex items-center gap-4 flex-wrap">
    <DateRangeFilter bind:from bind:to onchange={handleDateChange} />

    <div class="flex items-center gap-2">
      <label class="text-sm text-gray-500 dark:text-gray-400">Person</label>
      <select onchange={handleUserFilter} class="border dark:border-slate-600 rounded px-2 py-1 text-sm bg-white dark:bg-slate-700 dark:text-gray-200">
        <option value="">All</option>
        {#each users as user}
          <option value={user.id}>{user.display_name}</option>
        {/each}
      </select>
    </div>
  </div>

  {#if loading}
    <div class="text-gray-500 dark:text-gray-400">Loading team data...</div>
  {:else if error}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 rounded-lg p-4">{error}</div>
  {:else if timesheets.length === 0}
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-8 text-center text-gray-400 dark:text-gray-500">
      No team submissions found for the selected period.
    </div>
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
                <td class="px-5 py-3 text-gray-500 dark:text-gray-400 text-xs max-w-xs truncate">
                  {entry.notes ?? ''}
                </td>
              </tr>
            {/each}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
