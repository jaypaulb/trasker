<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { AutoSubmit, FocusEvent, Tag } from '$lib/types';

  const intervalOptions = [
    { hours: 0, label: 'Off' },
    { hours: 1, label: 'Every hour' },
    { hours: 2, label: 'Every 2 hours' },
    { hours: 4, label: 'Every 4 hours' },
    { hours: 6, label: 'Every 6 hours' },
    { hours: 8, label: 'Every 8 hours' },
    { hours: 12, label: 'Every 12 hours' },
    { hours: 24, label: 'Every 24 hours' },
  ];

  let events: FocusEvent[] = [];
  let selected = new Set<number>();
  let loading = true;
  let submitting = false;
  let showConfirm = false;
  let submitResult: string | null = null;

  // Auto-submit
  let tags: Tag[] = [];
  let autoSaved: AutoSubmit | null = null;
  let autoInterval = 0;
  let autoTagIds: number[] = [];
  let autoSaving = false;
  let autoMessage = '';

  onMount(async () => {
    try {
      const [allEvents, allTags, auto] = await Promise.all([
        api.getEvents(),
        api.getTags(),
        api.getAutoSubmit(),
      ]);
      // Filter to only tagged, non-idle events
      events = allEvents.filter(e => !e.is_idle && e.tag_name);
      tags = allTags;
      applyAutoSubmit(auto);
    } catch (e) {
      console.error('Failed to load:', e);
    } finally {
      loading = false;
    }
  });

  function applyAutoSubmit(auto: AutoSubmit) {
    autoSaved = auto;
    autoInterval = auto.interval_hours;
    autoTagIds = [...auto.tag_ids];
  }

  function toggleAutoTag(id: number) {
    autoTagIds = autoTagIds.includes(id)
      ? autoTagIds.filter(t => t !== id)
      : [...autoTagIds, id];
  }

  function autoDirty(): boolean {
    if (!autoSaved) return false;
    const a = [...autoTagIds].sort((x, y) => x - y).join(',');
    const b = [...autoSaved.tag_ids].sort((x, y) => x - y).join(',');
    return autoInterval !== autoSaved.interval_hours || a !== b;
  }

  async function saveAutoSubmit() {
    if (autoInterval > 0 && autoTagIds.length === 0) {
      autoMessage = 'Tick at least one tag to auto-submit.';
      return;
    }
    autoSaving = true;
    try {
      applyAutoSubmit(await api.saveAutoSubmit(autoInterval, autoTagIds));
      autoMessage = autoSaved?.enabled ? 'Auto-submit saved.' : 'Auto-submit is off.';
    } catch (e) {
      autoMessage = `Save failed: ${e}`;
    } finally {
      autoSaving = false;
    }
  }

  function formatWhen(iso?: string): string {
    if (!iso) return '';
    return new Date(iso).toLocaleString([], { weekday: 'short', hour: '2-digit', minute: '2-digit' });
  }

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
    <!-- Auto-submit -->
    <section class="mb-8 p-4 border dark:border-gray-700 rounded">
      <h2 class="text-lg font-semibold mb-3">Auto-submit</h2>

      <div class="flex flex-wrap items-center gap-3 mb-4">
        <label class="text-sm text-gray-600 dark:text-gray-300" for="auto-interval">Submit</label>
        <select id="auto-interval" bind:value={autoInterval}
                class="border dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200 rounded px-2 py-2 text-sm">
          {#each intervalOptions as opt}
            <option value={opt.hours}>{opt.label}</option>
          {/each}
        </select>
        <span class="text-sm text-gray-500 dark:text-gray-400">the ticked tags below</span>
      </div>

      {#if tags.length === 0}
        <p class="text-sm text-gray-400 dark:text-gray-500">Create a tag first, then tick it here.</p>
      {:else}
        <div class="flex flex-wrap gap-2 mb-4">
          {#each tags as tag}
            <label class="flex items-center gap-2 px-3 py-1.5 border dark:border-gray-600 rounded cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800 text-sm">
              <input type="checkbox" checked={autoTagIds.includes(tag.id)}
                     onchange={() => toggleAutoTag(tag.id)} class="rounded" />
              <span class="w-3 h-3 rounded" style="background-color: {tag.color}"></span>
              {tag.name}
            </label>
          {/each}
        </div>
      {/if}

      <div class="flex flex-wrap items-center gap-3">
        <button class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 disabled:opacity-50 text-sm"
                disabled={autoSaving || !autoDirty()}
                onclick={saveAutoSubmit}>
          {autoSaving ? 'Saving...' : 'Save auto-submit'}
        </button>
        {#if autoDirty()}
          <span class="text-sm text-yellow-600 dark:text-yellow-400">Unsaved changes</span>
        {/if}
        {#if autoMessage}
          <span class="text-sm text-gray-600 dark:text-gray-300">{autoMessage}</span>
        {/if}
      </div>

      {#if autoSaved?.enabled}
        <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">
          Next run {formatWhen(autoSaved.next_run)}. Covers finished entries on the ticked tags that start after {formatWhen(autoSaved.enabled_at)}.
        </p>
      {/if}
      <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
        Auto-submitted entries cannot be edited or deleted from your client, the same as manual submissions.
        Only your org admin can change them.
      </p>
    </section>

    <!-- Manual submit -->
    <h2 class="text-lg font-semibold mb-3">Submit manually</h2>

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
