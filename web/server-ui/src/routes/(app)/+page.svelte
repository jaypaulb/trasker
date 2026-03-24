<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Timesheet, ReportSummary } from '$lib/types';
  import SummaryCard from '$lib/components/SummaryCard.svelte';
  import TagChart from '$lib/components/TagChart.svelte';

  let timesheets = $state<Timesheet[]>([]);
  let tagSummary = $state<ReportSummary[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Default: current week (Mon-Sun)
  function getCurrentWeekRange(): { from: string; to: string } {
    const now = new Date();
    const dayOfWeek = now.getDay();
    const monday = new Date(now);
    monday.setDate(now.getDate() - ((dayOfWeek + 6) % 7));
    monday.setHours(0, 0, 0, 0);
    const sunday = new Date(monday);
    sunday.setDate(monday.getDate() + 6);
    sunday.setHours(23, 59, 59, 999);
    return {
      from: monday.toISOString(),
      to: sunday.toISOString(),
    };
  }

  onMount(async () => {
    try {
      const range = getCurrentWeekRange();
      const [ts, summary] = await Promise.all([
        api.timesheets.list(range),
        api.reports.summary({ ...range, group_by: 'tag' }),
      ]);
      timesheets = ts;
      tagSummary = summary;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load dashboard data.';
    } finally {
      loading = false;
    }
  });

  function formatDuration(seconds: number): string {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return `${h}h ${m}m`;
  }

  const totalSeconds = $derived(
    tagSummary.reduce((sum, t) => sum + t.total_duration_s, 0)
  );
  const totalEntries = $derived(
    tagSummary.reduce((sum, t) => sum + t.entry_count, 0)
  );
  const topTag = $derived(
    tagSummary.length > 0
      ? tagSummary.reduce((a, b) => (a.total_duration_s > b.total_duration_s ? a : b)).tag
      : 'N/A'
  );
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900">Dashboard</h1>

  {#if loading}
    <div class="flex items-center gap-2 text-gray-500">
      <div class="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600"></div>
      Loading...
    </div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4">{error}</div>
  {:else}
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <SummaryCard
        title="Total Time (This Week)"
        value={formatDuration(totalSeconds)}
        color="blue"
      />
      <SummaryCard
        title="Submissions"
        value={String(totalEntries)}
        subtitle="time blocks submitted"
        color="green"
      />
      <SummaryCard
        title="Top Activity"
        value={topTag}
        subtitle="most time spent"
        color="purple"
      />
    </div>

    <TagChart data={tagSummary} />

    <div class="bg-white rounded-lg shadow-sm border">
      <div class="px-5 py-4 border-b">
        <h3 class="text-sm font-medium text-gray-500">Recent Submissions</h3>
      </div>
      <div class="divide-y">
        {#each timesheets.slice(0, 10) as ts}
          {#each ts.entries as entry}
            <div class="px-5 py-3 flex items-center justify-between">
              <div>
                <span class="font-medium text-gray-900">{entry.tag}</span>
                <span class="text-xs text-gray-400 ml-2">
                  {new Date(entry.started_at).toLocaleDateString()}
                </span>
              </div>
              <span class="text-sm text-gray-600">{formatDuration(entry.duration_s)}</span>
            </div>
          {/each}
        {/each}
        {#if timesheets.length === 0}
          <div class="px-5 py-8 text-center text-gray-400">
            No submissions this week. Submit time from your Trasker client.
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
