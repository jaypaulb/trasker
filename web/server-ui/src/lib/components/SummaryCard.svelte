<script lang="ts">
  interface Props {
    title: string;
    value: string;
    subtitle?: string;
    color?: string;
  }

  let { title, value, subtitle, color = 'blue' }: Props = $props();

  const colorClasses: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    green: 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300',
    amber: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    purple: 'bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
  };
</script>

<div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700 p-5">
  <p class="text-sm font-medium text-gray-500 dark:text-gray-400">{title}</p>
  <p class="text-2xl font-bold mt-1 {colorClasses[color] ?? colorClasses.blue}">{value}</p>
  {#if subtitle}
    <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">{subtitle}</p>
  {/if}
</div>
