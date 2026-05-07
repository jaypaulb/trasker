<script lang="ts">
  import '../app.css';
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import { theme, toggleTheme } from '$lib/theme';

  let { children } = $props();

  const navItems = [
    { href: '/', label: 'Dashboard' },
    { href: '/timeline', label: 'Timeline' },
    { href: '/submit', label: 'Submit' },
    { href: '/pomodoro', label: 'Pomodoro' },
    { href: '/tags', label: 'Tags' },
    { href: '/settings', label: 'Settings' },
  ];

  let quitting = $state(false);

  async function quit() {
    if (!confirm('Shut down Trasker client? You will need to restart it manually.')) return;
    quitting = true;
    try {
      await api.quit();
    } catch {
      // Connection will drop as the server shuts down — expected
    }
  }
</script>

<div class="min-h-screen bg-white dark:bg-gray-900 dark:text-gray-100 transition-colors">
  <nav class="border-b dark:border-gray-700">
    <div class="max-w-4xl mx-auto px-6 flex items-center gap-6 h-14">
      <span class="font-bold text-lg">Trasker</span>
      {#each navItems as item}
        <a href={item.href}
           class="text-sm hover:text-blue-500 {$page.url.pathname === item.href ? 'text-blue-500 font-medium' : 'text-gray-600 dark:text-gray-400'}">
          {item.label}
        </a>
      {/each}
      <div class="ml-auto flex items-center gap-3">
        <button class="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
                onclick={toggleTheme}
                title="Toggle dark mode">
          {$theme === 'dark' ? '☀️' : '🌙'}
        </button>
        <button class="text-sm text-red-500 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300"
                onclick={quit}
                disabled={quitting}>
          {quitting ? 'Shutting down...' : 'Quit'}
        </button>
      </div>
    </div>
  </nav>

  {@render children()}
</div>
