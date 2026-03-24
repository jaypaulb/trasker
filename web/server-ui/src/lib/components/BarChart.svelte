<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Chart, BarController, BarElement, CategoryScale, LinearScale, Tooltip, Legend } from 'chart.js';

  Chart.register(BarController, BarElement, CategoryScale, LinearScale, Tooltip, Legend);

  interface Props {
    labels: string[];
    datasets: { label: string; data: number[]; color: string }[];
    yLabel?: string;
  }

  let { labels, datasets, yLabel = 'Hours' }: Props = $props();

  let canvas: HTMLCanvasElement;
  let chart: Chart | null = null;

  function buildChart() {
    if (chart) chart.destroy();
    if (!canvas) return;

    chart = new Chart(canvas, {
      type: 'bar',
      data: {
        labels,
        datasets: datasets.map((ds) => ({
          label: ds.label,
          data: ds.data,
          backgroundColor: ds.color,
          borderRadius: 4,
        })),
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          y: {
            beginAtZero: true,
            title: { display: true, text: yLabel },
          },
        },
        plugins: {
          tooltip: {
            callbacks: {
              label: (ctx) => {
                const val = ctx.parsed.y;
                return `${ctx.dataset.label}: ${val.toFixed(1)}h`;
              },
            },
          },
        },
      },
    });
  }

  onMount(() => buildChart());

  $effect(() => {
    // Rebuild on data changes
    labels;
    datasets;
    buildChart();
  });

  onDestroy(() => chart?.destroy());
</script>

<div class="h-80">
  <canvas bind:this={canvas}></canvas>
</div>
