<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Tag, TagRule } from '$lib/types';

  let tags: Tag[] = [];
  let rules: TagRule[] = [];
  let loading = true;
  let newName = '';
  let newColor = '#3b82f6';
  let message = '';

  // New rule form
  let ruleTagId: number | undefined;
  let ruleAppPattern = '';
  let ruleTitlePattern = '';

  onMount(async () => {
    try {
      [tags, rules] = await Promise.all([api.getTags(), api.getTagRules()]);
    } catch (e) {
      console.error('Failed to load:', e);
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

  async function createRule() {
    if (!ruleTagId || !ruleAppPattern.trim()) return;
    try {
      const titlePat = ruleTitlePattern.trim() || undefined;
      await api.createTagRule(ruleTagId, ruleAppPattern.trim(), titlePat);
      rules = await api.getTagRules();
      ruleAppPattern = '';
      ruleTitlePattern = '';
      message = 'Rule created';
      setTimeout(() => message = '', 3000);
    } catch (e) {
      message = `Failed: ${e}`;
    }
  }

  async function deleteRule(id: number) {
    try {
      await api.deleteTagRule(id);
      rules = await api.getTagRules();
    } catch (e) {
      message = `Failed: ${e}`;
    }
  }

  async function acceptRule(id: number) {
    try {
      await api.acceptTagRule(id);
      rules = await api.getTagRules();
      message = 'Rule accepted';
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

    <!-- Auto-tag rules section -->
    <h2 class="text-lg font-semibold mt-8 mb-3">Auto-Tag Rules</h2>

    <!-- Create new rule -->
    <div class="flex items-center gap-3 mb-4">
      <select bind:value={ruleTagId} class="border rounded px-2 py-2 text-sm">
        <option value={undefined}>Select tag...</option>
        {#each tags as tag}
          <option value={tag.id}>{tag.name}</option>
        {/each}
      </select>
      <input type="text" bind:value={ruleAppPattern}
             class="flex-1 border rounded px-3 py-2 text-sm"
             placeholder="App pattern (e.g. firefox, code)" />
      <input type="text" bind:value={ruleTitlePattern}
             class="flex-1 border rounded px-3 py-2 text-sm"
             placeholder="Title pattern (optional)" />
      <button class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 text-sm"
              onclick={createRule}
              disabled={!ruleTagId || !ruleAppPattern.trim()}>
        Add Rule
      </button>
    </div>

    <!-- Rule list -->
    <div class="space-y-2">
      {#each rules as rule}
        <div class="flex items-center gap-3 p-3 border rounded text-sm {rule.suggested ? 'border-yellow-300 bg-yellow-50' : ''}">
          <span class="font-medium text-blue-600">{rule.tag_name}</span>
          <span class="text-gray-600">app: <code class="bg-gray-100 px-1 rounded">{rule.app_pattern}</code></span>
          {#if rule.title_pattern}
            <span class="text-gray-600">title: <code class="bg-gray-100 px-1 rounded">{rule.title_pattern}</code></span>
          {/if}
          <span class="text-gray-400">hits: {rule.hit_count}</span>
          {#if rule.suggested}
            <span class="text-yellow-600 text-xs font-medium">SUGGESTED</span>
            <button class="text-green-600 hover:text-green-800 text-xs" onclick={() => acceptRule(rule.id)}>Accept</button>
          {/if}
          <button class="ml-auto text-red-500 hover:text-red-700 text-xs" onclick={() => deleteRule(rule.id)}>Delete</button>
        </div>
      {/each}

      {#if rules.length === 0}
        <p class="text-gray-400 text-sm">No auto-tag rules yet. Rules will be suggested as you tag events, or you can create them manually above.</p>
      {/if}
    </div>
  {/if}
</div>
