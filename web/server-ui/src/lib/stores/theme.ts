import { writable } from 'svelte/store';
import { browser } from '$app/environment';

function getInitialTheme(): 'light' | 'dark' {
  if (!browser) return 'dark';
  const stored = localStorage.getItem('trasker-theme');
  if (stored === 'dark' || stored === 'light') return stored;
  return 'dark';
}

export const theme = writable<'light' | 'dark'>(getInitialTheme());

export function toggleTheme() {
  theme.update((t) => {
    const next = t === 'dark' ? 'light' : 'dark';
    if (browser) localStorage.setItem('trasker-theme', next);
    return next;
  });
}
