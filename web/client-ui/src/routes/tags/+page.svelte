<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Tag } from '$lib/types';

  let tags: Tag[] = [];
  let loading = true;
  let newName = '';
  let newColor = '#3b82f6';
  let message = '';

  // TODO: Wire tag_rules via additional API endpoints when tagger store API is added
  // For now, this page manages tags only. Rule management will be added in a follow-up.

  onMount(async () => {
    try {
      tags = await api.getTags();
    } catch (e) {
      console.error('Failed to load tags:', e);
    } finally {
      loading = false;
    }
  });

  async function createTag() {
    if (!newName.trim()) return;
    try {
      await api.createTag(newName.trim(), newColor);
      tags = await api.getTags();
      newName = '';
      message = 'Tag created';
      setTimeout(() => message = '', 3000);
    } catch (e) {
      message = `Failed: ${e}`;
    }
  }
</script>

<svelte:head>
  <title>Trasker — Tags</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Tags</h1>

  {#if message}
    <div class="mb-4 p-3 bg-blue-50 text-blue-700 rounded">{message}</div>
  {/if}

  <!-- Create new tag -->
  <div class="flex items-center gap-3 mb-6">
    <input type="text" bind:value={newName}
           class="flex-1 border rounded px-3 py-2"
           placeholder="New tag name..."
           onkeydown={(e) => e.key === 'Enter' && createTag()} />
    <input type="color" bind:value={newColor}
           class="w-10 h-10 rounded cursor-pointer border-0" />
    <button class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600"
            onclick={createTag}>
      Create
    </button>
  </div>

  {#if loading}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <!-- Tag list -->
    <div class="space-y-2">
      {#each tags as tag}
        <div class="flex items-center gap-3 p-3 border rounded">
          <div class="w-6 h-6 rounded" style="background-color: {tag.color}"></div>
          <span class="font-medium flex-1">{tag.name}</span>
          <span class="text-xs text-gray-400">Created {new Date(tag.created_at).toLocaleDateString()}</span>
        </div>
      {/each}

      {#if tags.length === 0}
        <p class="text-gray-400 text-center py-8">No tags yet. Create your first tag above.</p>
      {/if}
    </div>

    <!-- Auto-tag rules section (placeholder for tagger integration) -->
    <h2 class="text-lg font-semibold mt-8 mb-3">Auto-Tag Rules</h2>
    <p class="text-gray-400 text-sm">Auto-tag rule management will be available in a future update. Rules are currently managed through the tagger engine.</p>
  {/if}
</div>
