<script lang="ts">
  interface Props {
    title: string;
    value: string;
    subtitle?: string;
    color?: string;
  }

  let { title, value, subtitle, color = 'blue' }: Props = $props();

  const colorClasses: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700',
    green: 'bg-green-50 text-green-700',
    amber: 'bg-amber-50 text-amber-700',
    purple: 'bg-purple-50 text-purple-700',
  };
</script>

<div class="bg-white rounded-lg shadow-sm border p-5">
  <p class="text-sm font-medium text-gray-500">{title}</p>
  <p class="text-2xl font-bold mt-1 {colorClasses[color] ?? colorClasses.blue}">{value}</p>
  {#if subtitle}
    <p class="text-xs text-gray-400 mt-1">{subtitle}</p>
  {/if}
</div>
