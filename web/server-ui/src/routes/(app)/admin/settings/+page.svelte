<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { OrgSettings } from '$lib/types';

  let settings = $state<OrgSettings | null>(null);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state<string | null>(null);
  let success = $state(false);

  // Editable fields
  let orgName = $state('');
  let entraTenant = $state('');
  let entraClient = $state('');
  let entraSecret = $state('');
  let keyExpiryDays = $state(60);

  onMount(async () => {
    try {
      settings = await api.admin.getSettings();
      orgName = settings.org_name;
      entraTenant = settings.entra_tenant;
      entraClient = settings.entra_client;
      entraSecret = settings.entra_secret ?? '';
      keyExpiryDays = settings.key_expiry_days;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load settings.';
    } finally {
      loading = false;
    }
  });

  async function handleSave() {
    saving = true;
    error = null;
    success = false;
    try {
      settings = await api.admin.updateSettings({
        org_name: orgName,
        entra_tenant: entraTenant,
        entra_client: entraClient,
        entra_secret: entraSecret,
        key_expiry_days: keyExpiryDays,
      });
      entraSecret = settings.entra_secret ?? '';
      success = true;
      setTimeout(() => (success = false), 3000);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save settings.';
    } finally {
      saving = false;
    }
  }
</script>

<div class="space-y-6 max-w-2xl">
  <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Organization Settings</h1>

  {#if loading}
    <div class="text-gray-500 dark:text-gray-400">Loading settings...</div>
  {:else if error}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 rounded-lg p-4">{error}</div>
  {:else}
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-6 space-y-5">
      <div>
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Organization Name</label>
        <input type="text" bind:value={orgName}
          class="border dark:border-slate-600 rounded-lg px-3 py-2 w-full text-sm bg-white dark:bg-slate-700 dark:text-gray-200" />
      </div>

      <hr class="border-gray-200 dark:border-slate-700" />

      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">Microsoft Entra ID Configuration</h3>

      <div>
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Tenant ID</label>
        <input type="text" bind:value={entraTenant}
          class="border dark:border-slate-600 rounded-lg px-3 py-2 w-full text-sm font-mono bg-white dark:bg-slate-700 dark:text-gray-200"
          placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
        <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">
          Your Microsoft Entra (Azure AD) tenant ID. Found in Azure Portal &gt; App registrations.
        </p>
      </div>

      <div>
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Client ID (Application ID)</label>
        <input type="text" bind:value={entraClient}
          class="border dark:border-slate-600 rounded-lg px-3 py-2 w-full text-sm font-mono bg-white dark:bg-slate-700 dark:text-gray-200"
          placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
        <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">
          The application (client) ID of your registered Entra app.
        </p>
      </div>

      <div>
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Client Secret</label>
        <input type="password" bind:value={entraSecret}
          class="border dark:border-slate-600 rounded-lg px-3 py-2 w-full text-sm font-mono bg-white dark:bg-slate-700 dark:text-gray-200"
          placeholder="Enter client secret" />
        <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">
          The client secret for your Entra app registration. Shown as masked after saving.
        </p>
      </div>

      <hr class="border-gray-200 dark:border-slate-700" />

      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">API Key Policy</h3>

      <div>
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Key Expiry (days of inactivity)</label>
        <input type="number" bind:value={keyExpiryDays} min="1" max="365"
          class="border dark:border-slate-600 rounded-lg px-3 py-2 w-32 text-sm bg-white dark:bg-slate-700 dark:text-gray-200" />
        <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">
          API keys expire after this many days without any client communication. Default: 60.
        </p>
      </div>

      <div class="flex items-center gap-3 pt-2">
        <button
          onclick={handleSave}
          disabled={saving}
          class="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 text-sm"
        >
          {saving ? 'Saving...' : 'Save Settings'}
        </button>
        {#if success}
          <span class="text-green-600 dark:text-green-400 text-sm">Settings saved.</span>
        {/if}
      </div>
    </div>

    {#if settings}
      <div class="text-xs text-gray-400 dark:text-gray-500">
        Last updated: {new Date(settings.updated_at).toLocaleString()}
      </div>
    {/if}
  {/if}
</div>
