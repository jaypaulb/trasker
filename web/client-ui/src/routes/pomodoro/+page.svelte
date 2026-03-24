<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api } from '$lib/api';
  import type { PomodoroSession, Tag } from '$lib/types';

  let sessions: PomodoroSession[] = [];
  let tags: Tag[] = [];
  let loading = true;
  let workMins = 25;
  let breakMins = 5;
  let selectedTagId: number | undefined;

  // Live timer state (polled from API or computed locally)
  let activePomo: PomodoroSession | null = null;
  let timerDisplay = '00:00';
  let timerInterval: ReturnType<typeof setInterval> | null = null;

  onMount(async () => {
    try {
      [sessions, tags] = await Promise.all([api.getPomodoro(), api.getTags()]);
      activePomo = sessions.find(s => s.status === 'work' || s.status === 'break') ?? null;
      if (activePomo) startTimerDisplay();
    } catch (e) {
      console.error('Failed to load:', e);
    } finally {
      loading = false;
    }
  });

  onDestroy(() => {
    if (timerInterval) clearInterval(timerInterval);
  });

  async function startPomodoro() {
    try {
      await api.startPomodoro(workMins, breakMins, selectedTagId);
      sessions = await api.getPomodoro();
      activePomo = sessions.find(s => s.status === 'work' || s.status === 'break') ?? null;
      if (activePomo) startTimerDisplay();
    } catch (e) {
      console.error('Failed to start:', e);
    }
  }

  function startTimerDisplay() {
    if (timerInterval) clearInterval(timerInterval);
    timerInterval = setInterval(updateTimer, 1000);
    updateTimer();
  }

  function updateTimer() {
    if (!activePomo) {
      timerDisplay = '00:00';
      return;
    }
    const started = new Date(activePomo.started_at).getTime();
    const durationMs = (activePomo.status === 'work' ? activePomo.work_mins : activePomo.break_mins) * 60 * 1000;
    const remaining = Math.max(0, durationMs - (Date.now() - started));
    const mins = Math.floor(remaining / 60000);
    const secs = Math.floor((remaining % 60000) / 1000);
    timerDisplay = `${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;

    if (remaining === 0) {
      // Refresh from server
      api.getPomodoro().then(s => {
        sessions = s;
        activePomo = s.find(sess => sess.status === 'work' || sess.status === 'break') ?? null;
        if (!activePomo && timerInterval) {
          clearInterval(timerInterval);
          timerInterval = null;
        }
      });
    }
  }

  function formatTime(iso: string): string {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  $: completedCount = sessions.filter(s => s.status === 'done').length;
  $: totalWorkMins = sessions.filter(s => s.status === 'done').reduce((sum, s) => sum + s.work_mins, 0);
</script>

<svelte:head>
  <title>Trasker — Pomodoro</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Pomodoro Timer</h1>

  {#if loading}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <!-- Timer display -->
    <div class="text-center mb-8">
      <p class="text-6xl font-mono font-bold mb-2">{timerDisplay}</p>
      {#if activePomo}
        <p class="text-lg text-gray-600 capitalize">{activePomo.status} phase</p>
      {:else}
        <p class="text-lg text-gray-400">Ready to start</p>
      {/if}
    </div>

    <!-- Controls -->
    {#if !activePomo}
      <div class="flex items-center gap-4 justify-center mb-8">
        <label class="text-sm">
          Work: <input type="number" bind:value={workMins} min="1" max="90"
                       class="w-16 border rounded px-2 py-1" /> min
        </label>
        <label class="text-sm">
          Break: <input type="number" bind:value={breakMins} min="1" max="30"
                        class="w-16 border rounded px-2 py-1" /> min
        </label>
        <select bind:value={selectedTagId} class="border rounded px-2 py-1 text-sm">
          <option value={undefined}>No tag</option>
          {#each tags as tag}
            <option value={tag.id}>{tag.name}</option>
          {/each}
        </select>
        <button class="px-6 py-2 bg-green-500 text-white rounded-lg hover:bg-green-600 font-medium"
                onclick={startPomodoro}>
          Start
        </button>
      </div>
    {/if}

    <!-- Stats -->
    <div class="grid grid-cols-2 gap-4 mb-8">
      <div class="bg-gray-50 rounded-lg p-4 text-center">
        <p class="text-3xl font-bold">{completedCount}</p>
        <p class="text-sm text-gray-500">Completed today</p>
      </div>
      <div class="bg-gray-50 rounded-lg p-4 text-center">
        <p class="text-3xl font-bold">{totalWorkMins}m</p>
        <p class="text-sm text-gray-500">Focus time</p>
      </div>
    </div>

    <!-- History -->
    <h2 class="text-lg font-semibold mb-3">Recent Sessions</h2>
    <div class="space-y-2">
      {#each sessions.slice(0, 10) as session}
        <div class="flex items-center gap-3 p-2 border rounded text-sm">
          <span class="text-gray-500">{formatTime(session.started_at)}</span>
          <span class="capitalize font-medium {session.status === 'done' ? 'text-green-600' : session.status === 'cancelled' ? 'text-red-500' : 'text-blue-500'}">
            {session.status}
          </span>
          <span class="text-gray-500">{session.work_mins}m / {session.break_mins}m</span>
        </div>
      {/each}
    </div>
  {/if}
</div>
