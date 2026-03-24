<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Chart, DoughnutController, ArcElement, Tooltip, Legend } from 'chart.js';
  import type { ReportSummary } from '$lib/types';

  Chart.register(DoughnutController, ArcElement, Tooltip, Legend);

  interface Props {
    data: ReportSummary[];
  }

  let { data }: Props = $props();

  let canvas: HTMLCanvasElement;
  let chart: Chart | null = null;

  const TAG_COLORS = [
    '#2563eb', '#16a34a', '#d97706', '#dc2626', '#7c3aed',
    '#0891b2', '#be185d', '#65a30d', '#ea580c', '#4f46e5',
  ];

  function buildChart() {
    if (chart) chart.destroy();
    if (!canvas || data.length === 0) return;

    chart = new Chart(canvas, {
      type: 'doughnut',
      data: {
        labels: data.map((d) => d.tag),
        datasets: [
          {
            data: data.map((d) => Math.round(d.total_duration_s / 60)),
            backgroundColor: data.map((_, i) => TAG_COLORS[i % TAG_COLORS.length]),
            borderWidth: 0,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { position: 'right' },
          tooltip: {
            callbacks: {
              label: (ctx) => {
                const mins = ctx.parsed as number;
                const h = Math.floor(mins / 60);
                const m = mins % 60;
                return `${ctx.label}: ${h}h ${m}m`;
              },
            },
          },
        },
      },
    });
  }

  onMount(() => buildChart());

  $effect(() => {
    // Rebuild when data changes
    data;
    buildChart();
  });

  onDestroy(() => chart?.destroy());
</script>

<div class="bg-white rounded-lg shadow-sm border p-5">
  <h3 class="text-sm font-medium text-gray-500 mb-4">Time by Tag</h3>
  <div class="h-64">
    {#if data.length === 0}
      <div class="flex items-center justify-center h-full text-gray-400">
        No submitted time data yet.
      </div>
    {:else}
      <canvas bind:this={canvas}></canvas>
    {/if}
  </div>
</div>
