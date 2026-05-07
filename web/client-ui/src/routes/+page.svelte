<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { FocusEvent, TagSummary } from '$lib/types';

  let events: FocusEvent[] = [];
  let tagSummaries: TagSummary[] = [];
  let currentFocus: FocusEvent | null = null;
  let totalTrackedSeconds = 0;
  let loading = true;

  onMount(async () => {
    try {
      events = await api.getEvents();
      computeSummaries();
      if (events.length > 0) {
        currentFocus = events[0]; // Most recent
      }
    } catch (e) {
      console.error('Failed to load events:', e);
    } finally {
      loading = false;
    }
  });

  function computeSummaries() {
    const byTag = new Map<string, { color: string; seconds: number }>();
    let total = 0;

    for (const event of events) {
      if (event.is_idle) continue;
      const seconds = event.duration_s ?? 0;
      total += seconds;
      const tag = event.tag_name || 'Untagged';
      const color = event.tag_color || '#999999';
      const existing = byTag.get(tag) ?? { color, seconds: 0 };
      existing.seconds += seconds;
      byTag.set(tag, existing);
    }

    totalTrackedSeconds = total;
    tagSummaries = Array.from(byTag.entries())
      .map(([tag, { color, seconds }]) => ({
        tag,
        color,
        totalSeconds: seconds,
        percentage: total > 0 ? Math.round((seconds / total) * 100) : 0,
      }))
      .sort((a, b) => b.totalSeconds - a.totalSeconds);
  }

  function formatDuration(seconds: number): string {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return h > 0 ? `${h}h ${m}m` : `${m}m`;
  }
</script>

<svelte:head>
  <title>Trasker — Dashboard</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Today's Activity</h1>

  {#if loading}
    <p class="text-gray-500 dark:text-gray-400">Loading...</p>
  {:else}
    <!-- Current Focus -->
    {#if currentFocus}
      <div class="bg-blue-50 dark:bg-blue-900/30 border border-blue-200 dark:border-blue-800 rounded-lg p-4 mb-6">
        <p class="text-sm text-blue-600 dark:text-blue-400 font-medium">Currently Focused On</p>
        <p class="text-lg font-semibold">{currentFocus.app_name}</p>
        <p class="text-gray-600 dark:text-gray-400 text-sm truncate">{currentFocus.window_title}</p>
        {#if currentFocus.tag_name}
          <span class="inline-block mt-1 px-2 py-0.5 rounded text-xs text-white"
                style="background-color: {currentFocus.tag_color}">
            {currentFocus.tag_name}
          </span>
        {/if}
      </div>
    {/if}

    <!-- Total Time -->
    <div class="mb-6">
      <p class="text-gray-500 dark:text-gray-400 text-sm">Total tracked today</p>
      <p class="text-3xl font-bold">{formatDuration(totalTrackedSeconds)}</p>
    </div>

    <!-- Tag Breakdown -->
    <div class="space-y-3">
      <h2 class="text-lg font-semibold">By Tag</h2>
      {#each tagSummaries as summary}
        <div class="flex items-center gap-3">
          <div class="w-3 h-3 rounded-full" style="background-color: {summary.color}"></div>
          <span class="flex-1 font-medium">{summary.tag}</span>
          <span class="text-gray-600 dark:text-gray-400">{formatDuration(summary.totalSeconds)}</span>
          <div class="w-24 bg-gray-200 dark:bg-gray-700 rounded-full h-2">
            <div class="h-2 rounded-full" style="width: {summary.percentage}%; background-color: {summary.color}"></div>
          </div>
          <span class="text-sm text-gray-500 dark:text-gray-400 w-10 text-right">{summary.percentage}%</span>
        </div>
      {/each}
      {#if tagSummaries.length === 0}
        <p class="text-gray-400 dark:text-gray-500">No activity tracked yet today.</p>
      {/if}
    </div>
  {/if}
</div>
