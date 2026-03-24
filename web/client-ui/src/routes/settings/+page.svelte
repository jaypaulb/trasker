<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Config } from '$lib/types';

  let config: Config | null = null;
  let loading = true;
  let saving = false;
  let message = '';

  onMount(async () => {
    try {
      config = await api.getConfig();
    } catch (e) {
      console.error('Failed to load config:', e);
    } finally {
      loading = false;
    }
  });

  async function toggleTracking() {
    if (!config) return;
    saving = true;
    try {
      const newVal = !config.tracking_on;
      await api.updateConfig({ tracking_on: newVal } as any);
      config.tracking_on = newVal;
      message = `Tracking ${newVal ? 'enabled' : 'disabled'}`;
      setTimeout(() => message = '', 3000);
    } catch (e) {
      message = `Failed: ${e}`;
    } finally {
      saving = false;
    }
  }

  async function toggleAutostart() {
    if (!config) return;
    saving = true;
    try {
      const newVal = !config.autostart;
      await api.updateConfig({ autostart: newVal } as any);
      config.autostart = newVal;
      message = `Autostart ${newVal ? 'enabled' : 'disabled'}`;
      setTimeout(() => message = '', 3000);
    } catch (e) {
      message = `Failed: ${e}`;
    } finally {
      saving = false;
    }
  }

  async function savePomodoro(workMins: number, breakMins: number) {
    saving = true;
    try {
      const defaults = JSON.stringify({ work: workMins, break: breakMins });
      await api.updateConfig({ pomodoro_defaults: defaults } as any);
      message = 'Pomodoro defaults saved';
      setTimeout(() => message = '', 3000);
    } catch (e) {
      message = `Failed: ${e}`;
    } finally {
      saving = false;
    }
  }

  $: pomodoroDefaults = config?.pomodoro_defaults ? JSON.parse(config.pomodoro_defaults) : { work: 25, break: 5 };
</script>

<svelte:head>
  <title>Trasker — Settings</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Settings</h1>

  {#if message}
    <div class="mb-4 p-3 bg-blue-50 text-blue-700 rounded">{message}</div>
  {/if}

  {#if loading || !config}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <div class="space-y-6">
      <!-- Device Info -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Device</h2>
        <div class="bg-gray-50 rounded-lg p-4 space-y-1 text-sm">
          <p><span class="text-gray-500">Device ID:</span> {config.device_id}</p>
          <p><span class="text-gray-500">Server:</span> {config.server_url}</p>
        </div>
      </section>

      <!-- Tracking -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Tracking</h2>
        <div class="flex items-center gap-4">
          <button class="px-4 py-2 rounded {config.tracking_on ? 'bg-green-500 text-white' : 'bg-gray-200 text-gray-700'}"
                  disabled={saving}
                  onclick={toggleTracking}>
            {config.tracking_on ? 'Tracking ON' : 'Tracking OFF'}
          </button>
        </div>
      </section>

      <!-- Autostart -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Autostart</h2>
        <div class="flex items-center gap-4">
          <button class="px-4 py-2 rounded {config.autostart ? 'bg-green-500 text-white' : 'bg-gray-200 text-gray-700'}"
                  disabled={saving}
                  onclick={toggleAutostart}>
            {config.autostart ? 'Start with OS: ON' : 'Start with OS: OFF'}
          </button>
        </div>
      </section>

      <!-- Pomodoro Defaults -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Pomodoro Defaults</h2>
        <div class="flex items-center gap-4">
          <label class="text-sm">
            Work: <input type="number" value={pomodoroDefaults.work} min="1" max="90"
                         class="w-16 border rounded px-2 py-1"
                         onchange={(e) => { pomodoroDefaults.work = parseInt((e.target as HTMLInputElement).value); }} /> min
          </label>
          <label class="text-sm">
            Break: <input type="number" value={pomodoroDefaults.break} min="1" max="30"
                          class="w-16 border rounded px-2 py-1"
                          onchange={(e) => { pomodoroDefaults.break = parseInt((e.target as HTMLInputElement).value); }} /> min
          </label>
          <button class="px-3 py-1 bg-blue-500 text-white rounded text-sm"
                  onclick={() => savePomodoro(pomodoroDefaults.work, pomodoroDefaults.break)}>
            Save
          </button>
        </div>
      </section>

      <!-- Presence Intervals -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Presence Check Intervals</h2>
        <p class="text-sm text-gray-600">{config.presence_intervals}</p>
        <p class="text-xs text-gray-400 mt-1">Minutes before each "Still there?" check when no focus change occurs.</p>
      </section>
    </div>
  {/if}
</div>
