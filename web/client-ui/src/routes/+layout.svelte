<script lang="ts">
  import '../app.css';
  import { page } from '$app/stores';

  let { children } = $props();

  const navItems = [
    { href: '/', label: 'Dashboard' },
    { href: '/timeline', label: 'Timeline' },
    { href: '/submit', label: 'Submit' },
    { href: '/pomodoro', label: 'Pomodoro' },
    { href: '/tags', label: 'Tags' },
    { href: '/settings', label: 'Settings' },
  ];
</script>

<div class="min-h-screen bg-white">
  <nav class="border-b">
    <div class="max-w-4xl mx-auto px-6 flex items-center gap-6 h-14">
      <span class="font-bold text-lg">Trasker</span>
      {#each navItems as item}
        <a href={item.href}
           class="text-sm hover:text-blue-500 {$page.url.pathname === item.href ? 'text-blue-500 font-medium' : 'text-gray-600'}">
          {item.label}
        </a>
      {/each}
    </div>
  </nav>

  {@render children()}
</div>
