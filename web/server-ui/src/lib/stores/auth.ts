import { writable, derived } from 'svelte/store';
import type { User, AuthTokens } from '$lib/types';

const STORAGE_KEY_TOKENS = 'trasker_tokens';
const STORAGE_KEY_USER = 'trasker_user';

function loadFromStorage<T>(key: string): T | null {
  if (typeof window === 'undefined') return null;
  const raw = localStorage.getItem(key);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as T;
  } catch {
    return null;
  }
}

function saveToStorage(key: string, value: unknown): void {
  localStorage.setItem(key, JSON.stringify(value));
}

function clearStorage(): void {
  localStorage.removeItem(STORAGE_KEY_TOKENS);
  localStorage.removeItem(STORAGE_KEY_USER);
}

export const tokens = writable<AuthTokens | null>(loadFromStorage(STORAGE_KEY_TOKENS));
export const currentUser = writable<User | null>(loadFromStorage(STORAGE_KEY_USER));

// Persist on change
tokens.subscribe((val) => {
  if (val) saveToStorage(STORAGE_KEY_TOKENS, val);
  else localStorage.removeItem(STORAGE_KEY_TOKENS);
});

currentUser.subscribe((val) => {
  if (val) saveToStorage(STORAGE_KEY_USER, val);
  else localStorage.removeItem(STORAGE_KEY_USER);
});

export const isAuthenticated = derived(tokens, ($tokens) => {
  if (!$tokens) return false;
  return $tokens.expires_at > Date.now() / 1000;
});

export const userRole = derived(currentUser, ($user) => $user?.role ?? null);

export function setAuth(newTokens: AuthTokens, user: User): void {
  tokens.set(newTokens);
  currentUser.set(user);
}

export function clearAuth(): void {
  tokens.set(null);
  currentUser.set(null);
  clearStorage();
}
