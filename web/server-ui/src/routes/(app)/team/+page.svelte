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
  <h1 class="text-2xl font-bold text-gray-900">Team Submissions</h1>

  <div class="bg-white rounded-lg shadow-sm border p-4 flex items-center gap-4 flex-wrap">
    <DateRangeFilter bind:from bind:to onchange={handleDateChange} />

    <div class="flex items-center gap-2">
      <label class="text-sm text-gray-500">Person</label>
      <select onchange={handleUserFilter} class="border rounded px-2 py-1 text-sm">
        <option value="">All</option>
        {#each users as user}
          <option value={user.id}>{user.display_name}</option>
        {/each}
      </select>
    </div>
  </div>

  {#if loading}
    <div class="text-gray-500">Loading team data...</div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4">{error}</div>
  {:else if timesheets.length === 0}
    <div class="bg-white rounded-lg shadow-sm border p-8 text-center text-gray-400">
      No team submissions found for the selected period.
    </div>
  {:else}
    <div class="bg-white rounded-lg shadow-sm border overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-left text-gray-500">
          <tr>
            <th class="px-5 py-3 font-medium">Person</th>
            <th class="px-5 py-3 font-medium">Tag</th>
            <th class="px-5 py-3 font-medium">Date</th>
            <th class="px-5 py-3 font-medium">Duration</th>
            <th class="px-5 py-3 font-medium">Notes</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each timesheets as ts}
            {#each ts.entries as entry}
              <tr>
                <td class="px-5 py-3 text-gray-900">
                  {userMap.get(ts.user_id)?.display_name ?? 'Unknown'}
                </td>
                <td class="px-5 py-3">
                  <span class="inline-block px-2 py-0.5 bg-blue-50 text-blue-700 text-xs rounded-full">
                    {entry.tag}
                  </span>
                </td>
                <td class="px-5 py-3 text-gray-600">
                  {new Date(entry.started_at).toLocaleDateString()}
                </td>
                <td class="px-5 py-3 text-gray-600">{formatDuration(entry.duration_s)}</td>
                <td class="px-5 py-3 text-gray-500 text-xs max-w-xs truncate">
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
