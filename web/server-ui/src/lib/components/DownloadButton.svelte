<script lang="ts">
  import { api } from '$lib/api';

  interface Props {
    targetOs: string;
    targetArch: string;
    label: string;
  }

  let { targetOs, targetArch, label }: Props = $props();

  let downloading = $state(false);
  let error = $state<string | null>(null);

  async function handleDownload() {
    downloading = true;
    error = null;
    try {
      const blob = await api.build.download(targetOs, targetArch);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      const ext = targetOs === 'windows' ? '.exe' : '';
      a.href = url;
      a.download = `trasker-client-${targetOs}-${targetArch}${ext}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Download failed.';
    } finally {
      downloading = false;
    }
  }
</script>

<button
  onclick={handleDownload}
  disabled={downloading}
  class="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors text-sm"
>
  {#if downloading}
    <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>
    Building...
  {:else}
    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
    </svg>
    {label}
  {/if}
</button>
{#if error}
  <p class="text-xs text-red-600 dark:text-red-400 mt-1">{error}</p>
{/if}
