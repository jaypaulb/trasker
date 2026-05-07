<script lang="ts">
  import { api } from '$lib/api';
  import LayoutSnapshotRow from '$lib/components/LayoutSnapshotRow.svelte';
  import LayoutSnapshotWindowList from '$lib/components/LayoutSnapshotWindowList.svelte';
  import type { LayoutSnapshot } from '$lib/types';
  import type { LayoutPageData } from './+page';

  interface Props {
    data: LayoutPageData;
  }

  const { data }: Props = $props();

  let expandedId = $state<string | null>(null);
  let expandedSnapshot = $state<LayoutSnapshot | null>(null);
  let expandedError = $state<string | null>(null);
  let expandedLoading = $state(false);
  let copyLabel = $state('Copy permalink');

  async function toggleRow(id: string, capturedAt: string) {
    if (data.mode !== 'timeline') return;
    const deviceId = data.deviceId;
    if (expandedId === id) {
      expandedId = null;
      expandedSnapshot = null;
      return;
    }
    expandedId = id;
    expandedSnapshot = null;
    expandedError = null;
    expandedLoading = true;
    try {
      expandedSnapshot = await api.layout.at(deviceId, capturedAt);
    } catch (err) {
      expandedError = err instanceof Error ? err.message : 'Failed to load snapshot.';
    } finally {
      expandedLoading = false;
    }
  }

  async function copyPermalink() {
    try {
      await navigator.clipboard.writeText(window.location.href);
      copyLabel = 'Copied';
      setTimeout(() => (copyLabel = 'Copy permalink'), 2000);
    } catch (_) {
      copyLabel = 'Copy failed';
      setTimeout(() => (copyLabel = 'Copy permalink'), 2000);
    }
  }
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Layout Snapshots</h1>

  {#if data.mode === 'no-device'}
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700">
      <div class="px-5 py-8 text-center text-gray-400 dark:text-gray-500">
        <p class="text-gray-700 dark:text-gray-300 font-medium">No client devices registered yet.</p>
        <p class="mt-2 text-sm">
          Layout snapshots come from your Trasker client. Download and run the client from the
          <a href="/devices" class="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300 underline">Devices</a>
          page, then reload this view.
        </p>
      </div>
    </div>
  {:else if data.mode === 'timeline'}
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700">
      <div class="px-5 py-4 border-b dark:border-slate-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">Today's snapshots</h2>
      </div>
      {#if data.timeline.length === 0}
        <div class="px-5 py-8 text-center text-gray-400 dark:text-gray-500">
          <p class="font-medium">No snapshots yet today.</p>
          <p class="mt-2 text-sm">Your client hasn't reported a layout change since midnight. Snapshots are written only when something on your desktop changes — open a window or switch focus, then come back.</p>
        </div>
      {:else}
        <div class="divide-y dark:divide-slate-700">
          {#each data.timeline as entry (entry.id)}
            <LayoutSnapshotRow
              id={entry.id}
              capturedAt={entry.captured_at}
              windowsCount={entry.windows_count}
              expanded={expandedId === entry.id}
              windows={expandedId === entry.id && expandedSnapshot ? expandedSnapshot.windows : null}
              onToggle={() => toggleRow(entry.id, entry.captured_at)}
            />
            {#if expandedId === entry.id}
              {#if expandedLoading}
                <div class="px-5 py-4 text-sm text-gray-500 dark:text-gray-400">Loading snapshot...</div>
              {:else if expandedError}
                <div class="px-5 py-4 bg-red-50 dark:bg-red-900/20 border-y border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 text-sm">
                  {expandedError}
                  <p class="text-xs text-red-600 dark:text-red-300 mt-1">Refresh the page to retry.</p>
                </div>
              {/if}
            {/if}
          {/each}
        </div>
      {/if}
    </div>
  {:else if data.mode === 'permalink'}
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700">
      <div class="px-5 py-4 border-b dark:border-slate-700 flex items-center justify-between gap-3">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">Snapshot at {data.t}</h2>
        <button type="button" class="text-xs text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300" onclick={copyPermalink}>
          {copyLabel}
        </button>
      </div>
      {#if data.snapshot === null}
        <div class="px-5 py-8 text-center text-gray-400 dark:text-gray-500">
          <p class="font-medium">No snapshot for this moment.</p>
          <p class="mt-2 text-sm">This device may have reported no changes at-or-before this timestamp. Try a later timestamp.</p>
        </div>
      {:else}
        <LayoutSnapshotWindowList windows={data.snapshot.windows} />
      {/if}
    </div>
  {/if}
</div>
