<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { FocusEvent } from '$lib/types';

  let events: FocusEvent[] = [];
  let selected = new Set<number>();
  let loading = true;
  let submitting = false;
  let showConfirm = false;
  let submitResult: string | null = null;

  onMount(async () => {
    try {
      events = await api.getEvents();
      // Filter to only tagged, non-idle events
      events = events.filter(e => !e.is_idle && e.tag_name);
    } catch (e) {
      console.error('Failed to load:', e);
    } finally {
      loading = false;
    }
  });

  function toggleSelect(id: number) {
    if (selected.has(id)) {
      selected.delete(id);
    } else {
      selected.add(id);
    }
    selected = new Set(selected); // trigger reactivity
  }

  function selectAll() {
    if (selected.size === events.length) {
      selected = new Set();
    } else {
      selected = new Set(events.map(e => e.id));
    }
  }

  async function confirmSubmit() {
    submitting = true;
    try {
      const result = await api.submit(Array.from(selected));
      submitResult = `Submitted successfully! Submission ID: ${result.submission_id}`;
      selected = new Set();
      showConfirm = false;
    } catch (e) {
      submitResult = `Submission failed: ${e}`;
    } finally {
      submitting = false;
    }
  }

  function formatTime(iso: string): string {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function formatDuration(seconds: number | undefined): string {
    if (!seconds) return '-';
    const m = Math.floor(seconds / 60);
    return `${m}m`;
  }
</script>

<svelte:head>
  <title>Trasker — Submit</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Submit Entries</h1>

  {#if submitResult}
    <div class="mb-4 p-3 rounded {submitResult.includes('failed') ? 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-300' : 'bg-green-50 dark:bg-green-900/30 text-green-700 dark:text-green-300'}">
      {submitResult}
    </div>
  {/if}

  {#if loading}
    <p class="text-gray-500 dark:text-gray-400">Loading...</p>
  {:else}
    <!-- Actions -->
    <div class="flex items-center gap-4 mb-4">
      <button class="text-sm text-blue-500 dark:text-blue-400 hover:underline" onclick={selectAll}>
        {selected.size === events.length ? 'Deselect All' : 'Select All'}
      </button>
      <span class="text-sm text-gray-500 dark:text-gray-400">{selected.size} selected</span>
      <button class="ml-auto px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 disabled:opacity-50"
              disabled={selected.size === 0}
              onclick={() => showConfirm = true}>
        Submit Selected
      </button>
    </div>

    <!-- Event list -->
    <div class="space-y-1">
      {#each events as event}
        <div class="flex items-center gap-3 p-2 rounded hover:bg-gray-50 dark:hover:bg-gray-800 cursor-pointer"
             onclick={() => toggleSelect(event.id)}
             onkeydown={(e) => e.key === 'Enter' && toggleSelect(event.id)}
             role="checkbox"
             aria-checked={selected.has(event.id)}
             tabindex="0">
          <input type="checkbox" checked={selected.has(event.id)}
                 onclick={(e) => { e.stopPropagation(); toggleSelect(event.id); }}
                 class="rounded" />
          <span class="text-sm text-gray-500 dark:text-gray-400 w-16">{formatTime(event.started_at)}</span>
          <span class="flex-1 truncate">{event.app_name}</span>
          <span class="px-2 py-0.5 rounded text-xs text-white"
                style="background-color: {event.tag_color}">
            {event.tag_name}
          </span>
          <span class="text-sm text-gray-500 dark:text-gray-400 w-12 text-right">{formatDuration(event.duration_s)}</span>
        </div>
      {/each}
    </div>

    {#if events.length === 0}
      <p class="text-gray-400 dark:text-gray-500 text-center py-8">No tagged entries to submit. Tag your events first.</p>
    {/if}
  {/if}

  <!-- Confirmation dialog -->
  {#if showConfirm}
    <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div class="bg-white dark:bg-gray-800 rounded-lg p-6 max-w-md mx-4">
        <h2 class="text-lg font-bold mb-3">Confirm Submission</h2>
        <p class="text-gray-600 dark:text-gray-300 mb-4">
          Submitted entries cannot be edited or deleted from your client.
          Only your org admin can modify or remove submitted entries.
          Are you sure?
        </p>
        <p class="text-sm text-gray-500 dark:text-gray-400 mb-4">{selected.size} entries will be submitted.</p>
        <div class="flex gap-3 justify-end">
          <button class="px-4 py-2 border dark:border-gray-600 rounded hover:bg-gray-50 dark:hover:bg-gray-700"
                  onclick={() => showConfirm = false}>
            Cancel
          </button>
          <button class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 disabled:opacity-50"
                  disabled={submitting}
                  onclick={confirmSubmit}>
            {submitting ? 'Submitting...' : 'Submit'}
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
