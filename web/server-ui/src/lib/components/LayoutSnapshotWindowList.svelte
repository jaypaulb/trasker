<script lang="ts">
  import type { LayoutWindow } from '$lib/types';

  interface Props {
    windows: LayoutWindow[];
  }

  const { windows }: Props = $props();
</script>

<ul class="divide-y dark:divide-slate-700">
  {#each windows as w}
    <li class="px-5 py-3 text-sm text-gray-700 dark:text-gray-300">
      <span class="truncate" title={w.window_title || w.app_name}>
        {#if w.app_name && w.window_title}
          <strong class="font-medium text-gray-900 dark:text-white">{w.app_name}</strong> &mdash; {w.window_title}
        {:else if w.window_title}
          {w.window_title}
        {:else}
          <strong class="font-medium text-gray-900 dark:text-white">{w.app_name}</strong>
        {/if}
      </span>
      <span class="text-xs text-gray-400 dark:text-gray-500"> · {w.w}×{w.h} @ ({w.x}, {w.y})</span>
    </li>
  {/each}
</ul>
