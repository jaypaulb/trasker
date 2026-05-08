<script lang="ts">
  import LayoutSnapshotWindowList from './LayoutSnapshotWindowList.svelte';
  import type { LayoutWindow } from '$lib/types';

  interface Props {
    id: string;
    capturedAt: string;
    windowsCount: number;
    windows?: LayoutWindow[] | null;
    expanded?: boolean;
    onToggle: () => void;
  }

  const { id, capturedAt, windowsCount, windows = null, expanded = false, onToggle }: Props = $props();

  const timeStr = $derived(
    new Date(capturedAt).toLocaleTimeString(undefined, {
      hour: '2-digit', minute: '2-digit', second: '2-digit',
    })
  );
  const countLabel = $derived(windowsCount === 1 ? '1 window' : `${windowsCount} windows`);
</script>

<button
  type="button"
  class="w-full px-5 py-3 flex items-center gap-3 text-left hover:bg-gray-50 dark:hover:bg-slate-700/50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-600"
  aria-expanded={expanded}
  aria-controls="snapshot-row-{id}-windows"
  onclick={onToggle}
>
  <svg
    aria-hidden="true"
    class="w-4 h-4 text-gray-400 dark:text-gray-500 transition-transform {expanded ? 'rotate-90' : ''}"
    fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24"
  >
    <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
  </svg>
  <span class="font-mono text-sm text-gray-900 dark:text-white">{timeStr}</span>
  <span class="text-xs text-gray-500 dark:text-gray-400 ml-auto">{countLabel}</span>
</button>

{#if expanded && windows}
  <div id="snapshot-row-{id}-windows">
    <LayoutSnapshotWindowList windows={windows} />
  </div>
{/if}
