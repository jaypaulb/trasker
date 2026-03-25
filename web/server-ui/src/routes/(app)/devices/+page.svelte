<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Device } from '$lib/types';
  import DownloadButton from '$lib/components/DownloadButton.svelte';

  let devices = $state<Device[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let editingId = $state<string | null>(null);
  let editName = $state('');

  onMount(async () => {
    try {
      devices = await api.devices.list();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load devices.';
    } finally {
      loading = false;
    }
  });

  function startEdit(device: Device) {
    editingId = device.id;
    editName = device.device_name ?? '';
  }

  async function saveEdit(deviceId: string) {
    try {
      const updated = await api.devices.updateName(deviceId, editName);
      devices = devices.map((d) => (d.id === deviceId ? updated : d));
      editingId = null;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update device name.';
    }
  }

  function cancelEdit() {
    editingId = null;
  }

  function formatDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, {
      year: 'numeric', month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit',
    });
  }

  const osLabels: Record<string, string> = {
    linux: 'Linux',
    darwin: 'macOS',
    windows: 'Windows',
  };

  const buildTargets = [
    { os: 'linux', arch: 'amd64', label: 'Linux (x64)' },
    { os: 'linux', arch: 'arm64', label: 'Linux (ARM64)' },
    { os: 'darwin', arch: 'amd64', label: 'macOS (Intel)' },
    { os: 'darwin', arch: 'arm64', label: 'macOS (Apple Silicon)' },
    { os: 'windows', arch: 'amd64', label: 'Windows (x64)' },
  ];
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Devices</h1>

  <!-- Download Section -->
  <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-6">
    <h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-2">Download Trasker Client</h2>
    <p class="text-sm text-gray-500 dark:text-gray-400 mb-4">
      Each download generates a unique API key baked into the binary. No configuration needed.
    </p>
    <div class="flex flex-wrap gap-3">
      {#each buildTargets as target}
        <DownloadButton
          targetOs={target.os}
          targetArch={target.arch}
          label={target.label}
        />
      {/each}
    </div>
    <details class="mt-4 text-sm text-gray-500 dark:text-gray-400">
      <summary class="cursor-pointer hover:text-gray-700 dark:hover:text-gray-300">After downloading</summary>
      <div class="mt-2 space-y-2 pl-4 border-l-2 border-gray-200 dark:border-slate-600">
        <p><strong class="text-gray-700 dark:text-gray-300">Linux / macOS:</strong></p>
        <code class="block bg-gray-100 dark:bg-slate-700 rounded px-3 py-2 text-xs font-mono">chmod +x trasker-client-* && ./trasker-client-*</code>
        <p><strong class="text-gray-700 dark:text-gray-300">Windows:</strong> Double-click the downloaded <code>.exe</code> file.</p>
        <p>The client opens a local dashboard at <code>http://127.0.0.1:9746</code> and registers with the server automatically.</p>
      </div>
    </details>
  </div>

  <!-- Registered Devices -->
  <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700">
    <div class="px-5 py-4 border-b dark:border-slate-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">Registered Devices</h2>
    </div>

    {#if loading}
      <div class="p-5 text-gray-500 dark:text-gray-400">Loading devices...</div>
    {:else if error}
      <div class="p-5 text-red-600 dark:text-red-400">{error}</div>
    {:else if devices.length === 0}
      <div class="p-8 text-center text-gray-400 dark:text-gray-500">
        No devices registered yet. Download and run the client on a machine to register it.
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead class="bg-gray-50 dark:bg-slate-700/50 text-left text-gray-500 dark:text-gray-400">
            <tr>
              <th class="px-5 py-3 font-medium">Name</th>
              <th class="px-5 py-3 font-medium">OS</th>
              <th class="px-5 py-3 font-medium">Device ID</th>
              <th class="px-5 py-3 font-medium">Last Seen</th>
              <th class="px-5 py-3 font-medium">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y dark:divide-slate-700">
            {#each devices as device}
              <tr>
                <td class="px-5 py-3">
                  {#if editingId === device.id}
                    <div class="flex items-center gap-2">
                      <input
                        type="text"
                        bind:value={editName}
                        class="border dark:border-slate-600 rounded px-2 py-1 text-sm w-40 bg-white dark:bg-slate-700 dark:text-gray-200"
                        onkeydown={(e) => { if (e.key === 'Enter') saveEdit(device.id); if (e.key === 'Escape') cancelEdit(); }}
                      />
                      <button onclick={() => saveEdit(device.id)} class="text-green-600 dark:text-green-400 hover:text-green-800 dark:hover:text-green-300 text-xs">Save</button>
                      <button onclick={cancelEdit} class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-xs">Cancel</button>
                    </div>
                  {:else}
                    <span class="text-gray-900 dark:text-white">{device.device_name ?? 'Unnamed'}</span>
                  {/if}
                </td>
                <td class="px-5 py-3 text-gray-600 dark:text-gray-300">{osLabels[device.os] ?? device.os}</td>
                <td class="px-5 py-3 font-mono text-xs text-gray-400 dark:text-gray-500">{device.client_device_id.slice(0, 8)}...</td>
                <td class="px-5 py-3 text-gray-600 dark:text-gray-300">{formatDate(device.last_seen_at)}</td>
                <td class="px-5 py-3">
                  {#if editingId !== device.id}
                    <button
                      onclick={() => startEdit(device)}
                      class="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300 text-xs"
                    >
                      Rename
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>
