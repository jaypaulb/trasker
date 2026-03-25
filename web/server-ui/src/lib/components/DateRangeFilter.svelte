<script lang="ts">
  interface Props {
    from: string;
    to: string;
    onchange: (from: string, to: string) => void;
  }

  let { from = $bindable(), to = $bindable(), onchange }: Props = $props();

  function handleChange() {
    onchange(from, to);
  }

  function setThisWeek() {
    const now = new Date();
    const dayOfWeek = now.getDay();
    const monday = new Date(now);
    monday.setDate(now.getDate() - ((dayOfWeek + 6) % 7));
    from = monday.toISOString().slice(0, 10);
    to = new Date().toISOString().slice(0, 10);
    handleChange();
  }

  function setThisMonth() {
    const now = new Date();
    from = new Date(now.getFullYear(), now.getMonth(), 1).toISOString().slice(0, 10);
    to = now.toISOString().slice(0, 10);
    handleChange();
  }

  function setLast30Days() {
    const now = new Date();
    const past = new Date(now);
    past.setDate(now.getDate() - 30);
    from = past.toISOString().slice(0, 10);
    to = now.toISOString().slice(0, 10);
    handleChange();
  }
</script>

<div class="flex items-center gap-3 flex-wrap">
  <div class="flex items-center gap-2">
    <label class="text-sm text-gray-500 dark:text-gray-400">From</label>
    <input type="date" bind:value={from} onchange={handleChange}
      class="border dark:border-slate-600 rounded px-2 py-1 text-sm bg-white dark:bg-slate-700 dark:text-gray-200" />
  </div>
  <div class="flex items-center gap-2">
    <label class="text-sm text-gray-500 dark:text-gray-400">To</label>
    <input type="date" bind:value={to} onchange={handleChange}
      class="border dark:border-slate-600 rounded px-2 py-1 text-sm bg-white dark:bg-slate-700 dark:text-gray-200" />
  </div>
  <div class="flex gap-1">
    <button onclick={setThisWeek} class="px-2 py-1 text-xs bg-gray-100 dark:bg-slate-700 hover:bg-gray-200 dark:hover:bg-slate-600 dark:text-gray-300 rounded">This Week</button>
    <button onclick={setThisMonth} class="px-2 py-1 text-xs bg-gray-100 dark:bg-slate-700 hover:bg-gray-200 dark:hover:bg-slate-600 dark:text-gray-300 rounded">This Month</button>
    <button onclick={setLast30Days} class="px-2 py-1 text-xs bg-gray-100 dark:bg-slate-700 hover:bg-gray-200 dark:hover:bg-slate-600 dark:text-gray-300 rounded">Last 30 Days</button>
  </div>
</div>
