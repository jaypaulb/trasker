<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { FocusEvent, Tag } from '$lib/types';

  let events: FocusEvent[] = [];
  let tags: Tag[] = [];
  let loading = true;
  let editingNote: number | null = null;
  let noteText = '';

  onMount(async () => {
    try {
      [events, tags] = await Promise.all([api.getEvents(), api.getTags()]);
    } catch (e) {
      console.error('Failed to load:', e);
    } finally {
      loading = false;
    }
  });

  async function setTag(eventId: number, tagId: number) {
    try {
      await api.tagEvent(eventId, tagId);
      events = await api.getEvents();
    } catch (e) {
      console.error('Failed to tag:', e);
    }
  }

  async function saveNote(eventId: number) {
    if (!noteText.trim()) return;
    try {
      await api.addNote(eventId, noteText.trim());
      events = await api.getEvents();
      editingNote = null;
      noteText = '';
    } catch (e) {
      console.error('Failed to save note:', e);
    }
  }

  function formatTime(iso: string): string {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function formatDuration(seconds: number | undefined): string {
    if (!seconds) return '-';
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return m > 0 ? `${m}m ${s}s` : `${s}s`;
  }
</script>

<svelte:head>
  <title>Trasker — Timeline</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Timeline</h1>

  {#if loading}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <div class="space-y-2">
      {#each events as event}
        <div class="border rounded-lg p-3 hover:bg-gray-50 {event.is_idle ? 'opacity-50' : ''}">
          <div class="flex items-center gap-3">
            <!-- Time -->
            <span class="text-sm text-gray-500 w-16 shrink-0">
              {formatTime(event.started_at)}
            </span>

            <!-- App + Title -->
            <div class="flex-1 min-w-0">
              <p class="font-medium truncate">{event.app_name}</p>
              <p class="text-sm text-gray-600 truncate">{event.window_title}</p>
            </div>

            <!-- Duration -->
            <span class="text-sm text-gray-500 w-16 text-right">
              {formatDuration(event.duration_s)}
            </span>

            <!-- Tag -->
            <div class="w-32">
              {#if event.tag_name}
                <span class="px-2 py-0.5 rounded text-xs text-white"
                      style="background-color: {event.tag_color}">
                  {event.tag_name}
                </span>
              {:else}
                <select class="text-xs border rounded p-1 w-full"
                        onchange={(e) => setTag(event.id, parseInt((e.target as HTMLSelectElement).value))}>
                  <option value="">Tag...</option>
                  {#each tags as tag}
                    <option value={tag.id}>{tag.name}</option>
                  {/each}
                </select>
              {/if}
            </div>

            <!-- Note button -->
            <button class="text-gray-400 hover:text-blue-500 text-sm"
                    onclick={() => { editingNote = editingNote === event.id ? null : event.id; noteText = event.note_text ?? ''; }}>
              {event.note_text ? '📝' : '+note'}
            </button>
          </div>

          <!-- Note editor -->
          {#if editingNote === event.id}
            <div class="mt-2 flex gap-2">
              <input type="text" bind:value={noteText}
                     class="flex-1 border rounded px-2 py-1 text-sm"
                     placeholder="Add a note..."
                     onkeydown={(e) => e.key === 'Enter' && saveNote(event.id)} />
              <button class="px-3 py-1 bg-blue-500 text-white rounded text-sm hover:bg-blue-600"
                      onclick={() => saveNote(event.id)}>
                Save
              </button>
            </div>
          {/if}

          <!-- Existing note display -->
          {#if event.note_text && editingNote !== event.id}
            <p class="mt-1 text-sm text-gray-600 italic ml-19">{event.note_text}</p>
          {/if}
        </div>
      {/each}

      {#if events.length === 0}
        <p class="text-gray-400 text-center py-8">No events tracked today.</p>
      {/if}
    </div>
  {/if}
</div>
