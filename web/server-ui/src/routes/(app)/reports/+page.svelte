<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { ReportSummary } from '$lib/types';
  import DateRangeFilter from '$lib/components/DateRangeFilter.svelte';
  import TagChart from '$lib/components/TagChart.svelte';
  import BarChart from '$lib/components/BarChart.svelte';

  let tagData = $state<ReportSummary[]>([]);
  let dayData = $state<ReportSummary[]>([]);
  let personData = $state<ReportSummary[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let from = $state(new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10));
  let to = $state(new Date().toISOString().slice(0, 10));

  async function loadData() {
    loading = true;
    error = null;
    try {
      const params = {
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
      };
      const [byTag, byDay, byPerson] = await Promise.all([
        api.reports.summary({ ...params, group_by: 'tag' }),
        api.reports.summary({ ...params, group_by: 'day' }),
        api.reports.summary({ ...params, group_by: 'person' }),
      ]);
      tagData = byTag;
      dayData = byDay;
      personData = byPerson;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load report data.';
    } finally {
      loading = false;
    }
  }

  onMount(loadData);

  function handleDateChange() {
    loadData();
  }

  async function handleExportCsv() {
    try {
      const blob = await api.reports.exportCsv({
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `trasker-report-${from}-to-${to}.csv`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (e) {
      error = e instanceof Error ? e.message : 'CSV export failed.';
    }
  }

  const TAG_COLORS = [
    '#2563eb', '#16a34a', '#d97706', '#dc2626', '#7c3aed',
    '#0891b2', '#be185d', '#65a30d', '#ea580c', '#4f46e5',
  ];

  const dayChartLabels = $derived(dayData.map((d) => d.tag)); // tag field reused as day label
  const dayChartDatasets = $derived([{
    label: 'Hours',
    data: dayData.map((d) => d.total_duration_s / 3600),
    color: '#2563eb',
  }]);

  const personChartLabels = $derived(personData.map((d) => d.tag)); // tag field reused as person name
  const personChartDatasets = $derived([{
    label: 'Hours',
    data: personData.map((d) => d.total_duration_s / 3600),
    color: '#16a34a',
  }]);
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Reports</h1>
    <button
      onclick={handleExportCsv}
      class="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 text-sm"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
      </svg>
      Export CSV
    </button>
  </div>

  <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-4">
    <DateRangeFilter bind:from bind:to onchange={handleDateChange} />
  </div>

  {#if loading}
    <div class="text-gray-500 dark:text-gray-400">Loading reports...</div>
  {:else if error}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 rounded-lg p-4">{error}</div>
  {:else}
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-5">
        <h3 class="text-sm font-medium text-gray-500 dark:text-gray-400 mb-4">Time by Tag</h3>
        <TagChart data={tagData} />
      </div>

      <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-5">
        <h3 class="text-sm font-medium text-gray-500 dark:text-gray-400 mb-4">Time by Day</h3>
        <BarChart labels={dayChartLabels} datasets={dayChartDatasets} />
      </div>

      <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-5 lg:col-span-2">
        <h3 class="text-sm font-medium text-gray-500 dark:text-gray-400 mb-4">Time by Person</h3>
        <BarChart labels={personChartLabels} datasets={personChartDatasets} />
      </div>
    </div>
  {/if}
</div>
