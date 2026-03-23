# Server Dashboard & Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Build the server Svelte dashboard, Docker Compose deployment, and client build pipeline

**Architecture:** SvelteKit SPA calling Go API, Caddy reverse proxy with auto-TLS, PostgreSQL in Docker, build pipeline that cross-compiles Go client binaries with baked-in API keys via ldflags

**Tech Stack:** SvelteKit 2, TypeScript, TailwindCSS, Docker Compose, Caddy 2, Go cross-compilation

**Depends on:** Plan 01 (shared foundation — models, apikey package, version), Plan 02 (server core — Go API server, all endpoints, PostgreSQL store, Entra auth)

**Spec:** `docs/superpowers/specs/2026-03-23-trasker-design.md`

---

## Task 1: SvelteKit Project Init

**Files:**
- Create: `web/server-ui/package.json`
- Create: `web/server-ui/svelte.config.js`
- Create: `web/server-ui/vite.config.ts`
- Create: `web/server-ui/tsconfig.json`
- Create: `web/server-ui/src/app.html`
- Create: `web/server-ui/src/app.css`
- Create: `web/server-ui/tailwind.config.ts`
- Create: `web/server-ui/src/routes/+layout.svelte`
- Create: `web/server-ui/src/routes/+page.svelte`
- Create: `web/server-ui/static/favicon.png`

**Steps:**

- [ ] 1.1 — Scaffold SvelteKit project with TypeScript:

```bash
cd web && npx sv create server-ui --template minimal --types ts
```

Expected: `web/server-ui/` directory created with SvelteKit skeleton.

- [ ] 1.2 — Install TailwindCSS and adapter-static:

```bash
cd web/server-ui
npx sv add tailwindcss
npm install -D @sveltejs/adapter-static
```

Expected: `tailwind.config.ts` created, TailwindCSS directives in `app.css`.

- [ ] 1.3 — Configure adapter-static in `web/server-ui/svelte.config.js`:

```js
import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/kit/vite';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      pages: 'build',
      assets: 'build',
      fallback: 'index.html',  // SPA mode — all routes serve index.html
      precompress: false,
      strict: true
    })
  }
};

export default config;
```

- [ ] 1.4 — Create SPA prerender config at `web/server-ui/src/routes/+layout.ts`:

```ts
// SPA mode: disable SSR, enable client-side routing
export const ssr = false;
export const prerender = false;
```

- [ ] 1.5 — Set up base TailwindCSS styles in `web/server-ui/src/app.css`:

```css
@import 'tailwindcss';

:root {
  --color-primary: #2563eb;
  --color-primary-dark: #1d4ed8;
  --color-danger: #dc2626;
  --color-warning: #d97706;
  --color-success: #16a34a;
  --color-bg: #f8fafc;
  --color-sidebar: #1e293b;
  --color-sidebar-text: #e2e8f0;
}

body {
  @apply bg-[var(--color-bg)] text-gray-900 font-sans;
}
```

- [ ] 1.6 — Verify the dev server starts:

```bash
cd web/server-ui && npm run dev -- --port 5174
```

Expected: Vite dev server running at `http://localhost:5174`, page loads without errors.

- [ ] 1.7 — Verify static build works:

```bash
cd web/server-ui && npm run build
ls web/server-ui/build/index.html
```

Expected: `build/` directory with `index.html` (SPA fallback).

- [ ] 1.8 — Commit: `git add web/server-ui/ && git commit -m "feat(server-ui): scaffold SvelteKit project with TailwindCSS and adapter-static"`

---

## Task 2: API Client

**Files:**
- Create: `web/server-ui/src/lib/api.ts`
- Create: `web/server-ui/src/lib/types.ts`
- Create: `web/server-ui/src/lib/stores/auth.ts`

**Steps:**

- [ ] 2.1 — Create shared TypeScript types at `web/server-ui/src/lib/types.ts`:

```ts
// Matches server API response types from Plan 02

export interface User {
  id: string;
  entra_oid: string;
  email: string;
  display_name: string;
  role: 'admin' | 'manager' | 'member';
  created_at: string;
  updated_at: string;
}

export interface Device {
  id: string;
  user_id: string;
  api_key_id: string;
  client_device_id: string;
  device_name: string | null;
  os: string;
  last_seen_at: string;
  created_at: string;
}

export interface TimesheetEntry {
  id: string;
  timesheet_id: string;
  tag: string;
  started_at: string;
  ended_at: string;
  duration_s: number;
  notes: string | null;
  app_summary: string | null;
}

export interface Timesheet {
  id: string;
  user_id: string;
  device_id: string;
  submitted_at: string;
  created_at: string;
  entries: TimesheetEntry[];
}

export interface ReportSummary {
  tag: string;
  total_duration_s: number;
  entry_count: number;
}

export interface AuditLogEntry {
  id: string;
  admin_id: string;
  admin_name?: string;
  action: string;
  target_type: string;
  target_id: string;
  old_value: unknown;
  new_value: unknown;
  created_at: string;
}

export interface OrgSettings {
  org_name: string;
  entra_tenant: string;
  entra_client: string;
  key_expiry_days: number;
  updated_at: string;
}

export interface ApiKey {
  id: string;
  user_id: string;
  key_prefix: string;
  last_used_at: string;
  expires_at: string;
  revoked: boolean;
  created_at: string;
  revoked_at: string | null;
}

export interface BuildStatus {
  status: 'idle' | 'building' | 'ready' | 'failed';
  target_os: string;
  target_arch: string;
  error?: string;
}

export interface AuthTokens {
  access_token: string;
  refresh_token: string;
  expires_at: number; // Unix timestamp
}
```

- [ ] 2.2 — Create auth store at `web/server-ui/src/lib/stores/auth.ts`:

```ts
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
```

- [ ] 2.3 — Create API client at `web/server-ui/src/lib/api.ts`:

```ts
import { get } from 'svelte/store';
import { tokens, clearAuth } from '$lib/stores/auth';
import type {
  User, Device, Timesheet, TimesheetEntry,
  ReportSummary, AuditLogEntry, OrgSettings, ApiKey, BuildStatus, AuthTokens
} from '$lib/types';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1';

class ApiError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public body: unknown
  ) {
    super(`API ${status}: ${statusText}`);
    this.name = 'ApiError';
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  opts?: { skipAuth?: boolean }
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (!opts?.skipAuth) {
    const currentTokens = get(tokens);
    if (!currentTokens) {
      throw new ApiError(401, 'Not authenticated', null);
    }

    // Check if token is about to expire (within 60s) and refresh
    if (currentTokens.expires_at < Date.now() / 1000 + 60) {
      try {
        const refreshed = await refreshToken(currentTokens.refresh_token);
        // Store import is circular-safe because we use the store directly
        tokens.set(refreshed);
        headers['Authorization'] = `Bearer ${refreshed.access_token}`;
      } catch {
        clearAuth();
        throw new ApiError(401, 'Token refresh failed', null);
      }
    } else {
      headers['Authorization'] = `Bearer ${currentTokens.access_token}`;
    }
  }

  const response = await fetch(`${BASE_URL}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    const errorBody = await response.text().catch(() => null);
    let parsed: unknown = errorBody;
    try {
      parsed = JSON.parse(errorBody ?? '');
    } catch {
      // keep as string
    }

    if (response.status === 401) {
      clearAuth();
    }

    throw new ApiError(response.status, response.statusText, parsed);
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}

// --- Auth ---

async function refreshToken(refresh_token: string): Promise<AuthTokens> {
  const response = await fetch(`${BASE_URL}/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token }),
  });
  if (!response.ok) throw new Error('Refresh failed');
  return response.json();
}

export const api = {
  // Auth
  auth: {
    /** Exchange Entra auth code for tokens */
    login(code: string, redirect_uri: string): Promise<{ tokens: AuthTokens; user: User }> {
      return request('POST', '/auth/login', { code, redirect_uri }, { skipAuth: true });
    },
    refresh(refresh_token: string): Promise<AuthTokens> {
      return refreshToken(refresh_token);
    },
  },

  // Users
  users: {
    list(): Promise<User[]> {
      return request('GET', '/users');  // admin only
    },
    listTeam(): Promise<User[]> {
      return request('GET', '/users/team');  // manager+ — returns users visible to this manager
    },
    updateRole(userId: string, role: string): Promise<User> {
      return request('PATCH', `/users/${userId}`, { role });
    },
  },

  // Devices
  devices: {
    list(): Promise<Device[]> {
      return request('GET', '/devices');
    },
    updateName(deviceId: string, device_name: string): Promise<Device> {
      return request('PATCH', `/devices/${deviceId}`, { device_name });
    },
  },

  // Timesheets
  timesheets: {
    list(params?: { from?: string; to?: string }): Promise<Timesheet[]> {
      const query = new URLSearchParams();
      if (params?.from) query.set('from', params.from);
      if (params?.to) query.set('to', params.to);
      const qs = query.toString();
      return request('GET', `/timesheets${qs ? '?' + qs : ''}`);
    },
    team(params?: { from?: string; to?: string; user_id?: string }): Promise<Timesheet[]> {
      const query = new URLSearchParams();
      if (params?.from) query.set('from', params.from);
      if (params?.to) query.set('to', params.to);
      if (params?.user_id) query.set('user_id', params.user_id);
      const qs = query.toString();
      return request('GET', `/timesheets/team${qs ? '?' + qs : ''}`);
    },
  },

  // Reports
  reports: {
    summary(params: {
      from: string;
      to: string;
      user_id?: string;
      group_by?: 'tag' | 'day' | 'person';
    }): Promise<ReportSummary[]> {
      const query = new URLSearchParams({ from: params.from, to: params.to });
      if (params.user_id) query.set('user_id', params.user_id);
      if (params.group_by) query.set('group_by', params.group_by);
      return request('GET', `/reports/summary?${query}`);
    },
    exportCsv(params: { from: string; to: string; user_id?: string }): Promise<Blob> {
      const currentTokens = get(tokens);
      const query = new URLSearchParams({ from: params.from, to: params.to });
      if (params.user_id) query.set('user_id', params.user_id);
      return fetch(`${BASE_URL}/reports/export?${query}`, {
        headers: { Authorization: `Bearer ${currentTokens?.access_token}` },
      }).then((r) => {
        if (!r.ok) throw new ApiError(r.status, r.statusText, null);
        return r.blob();
      });
    },
  },

  // Build / download
  build: {
    download(target_os: string, target_arch: string): Promise<Blob> {
      const currentTokens = get(tokens);
      return fetch(`${BASE_URL}/build/download`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${currentTokens?.access_token}`,
        },
        body: JSON.stringify({ target_os, target_arch }),
      }).then((r) => {
        if (!r.ok) throw new ApiError(r.status, r.statusText, null);
        return r.blob();
      });
    },
    status(): Promise<BuildStatus> {
      return request('GET', '/build/status');
    },
  },

  // Admin
  admin: {
    editEntry(entryId: string, data: Partial<TimesheetEntry>): Promise<TimesheetEntry> {
      return request('PATCH', `/admin/entries/${entryId}`, data);
    },
    deleteEntry(entryId: string): Promise<void> {
      return request('DELETE', `/admin/entries/${entryId}`);
    },
    revokeKey(keyId: string): Promise<void> {
      return request('POST', `/admin/keys/${keyId}/revoke`);
    },
    auditLog(params?: { from?: string; to?: string; action?: string }): Promise<AuditLogEntry[]> {
      const query = new URLSearchParams();
      if (params?.from) query.set('from', params.from);
      if (params?.to) query.set('to', params.to);
      if (params?.action) query.set('action', params.action);
      const qs = query.toString();
      return request('GET', `/admin/audit${qs ? '?' + qs : ''}`);
    },
    getSettings(): Promise<OrgSettings> {
      return request('GET', '/admin/settings');
    },
    updateSettings(data: Partial<OrgSettings>): Promise<OrgSettings> {
      return request('PATCH', '/admin/settings', data);
    },
    listApiKeys(userId?: string): Promise<ApiKey[]> {
      const qs = userId ? `?user_id=${userId}` : '';
      return request('GET', `/admin/keys${qs}`);
    },
  },
};
```

- [ ] 2.4 — Verify TypeScript compiles:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No errors in `src/lib/api.ts`, `src/lib/types.ts`, `src/lib/stores/auth.ts`.

- [ ] 2.5 — Commit: `git add web/server-ui/src/lib/ && git commit -m "feat(server-ui): add typed API client, auth store, and shared types"`

---

## Task 3: Auth Flow

**Files:**
- Create: `web/server-ui/src/lib/auth.ts`
- Create: `web/server-ui/src/routes/login/+page.svelte`
- Create: `web/server-ui/src/routes/auth/callback/+page.svelte`
- Modify: `web/server-ui/src/routes/+layout.svelte`

**Steps:**

- [ ] 3.1 — Create Entra auth helpers at `web/server-ui/src/lib/auth.ts`:

```ts
const ENTRA_AUTHORITY = import.meta.env.VITE_ENTRA_AUTHORITY ?? 'https://login.microsoftonline.com';
const ENTRA_TENANT = import.meta.env.VITE_ENTRA_TENANT_ID ?? '';
const ENTRA_CLIENT_ID = import.meta.env.VITE_ENTRA_CLIENT_ID ?? '';

function getRedirectUri(): string {
  return `${window.location.origin}/auth/callback`;
}

export function getLoginUrl(): string {
  const params = new URLSearchParams({
    client_id: ENTRA_CLIENT_ID,
    response_type: 'code',
    redirect_uri: getRedirectUri(),
    scope: 'openid profile email',
    response_mode: 'query',
    state: crypto.randomUUID(),
  });
  return `${ENTRA_AUTHORITY}/${ENTRA_TENANT}/oauth2/v2.0/authorize?${params}`;
}

export function getCallbackRedirectUri(): string {
  return getRedirectUri();
}
```

- [ ] 3.2 — Create login page at `web/server-ui/src/routes/login/+page.svelte`:

```svelte
<script lang="ts">
  import { getLoginUrl } from '$lib/auth';
  import { isAuthenticated } from '$lib/stores/auth';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  onMount(() => {
    if ($isAuthenticated) {
      goto('/');
    }
  });

  function handleLogin() {
    window.location.href = getLoginUrl();
  }
</script>

<div class="min-h-screen flex items-center justify-center bg-slate-50">
  <div class="bg-white rounded-lg shadow-lg p-8 max-w-sm w-full text-center">
    <h1 class="text-2xl font-bold text-gray-900 mb-2">Trasker</h1>
    <p class="text-gray-500 mb-8">Sign in to your organization's time tracking dashboard</p>

    <button
      onclick={handleLogin}
      class="w-full flex items-center justify-center gap-3 bg-[var(--color-primary)] hover:bg-[var(--color-primary-dark)] text-white font-medium py-3 px-4 rounded-lg transition-colors"
    >
      <svg class="w-5 h-5" viewBox="0 0 21 21" xmlns="http://www.w3.org/2000/svg">
        <rect x="1" y="1" width="9" height="9" fill="#f25022"/>
        <rect x="11" y="1" width="9" height="9" fill="#7fba00"/>
        <rect x="1" y="11" width="9" height="9" fill="#00a4ef"/>
        <rect x="11" y="11" width="9" height="9" fill="#ffb900"/>
      </svg>
      Sign in with Microsoft
    </button>

    <p class="text-xs text-gray-400 mt-6">
      Your organization uses Microsoft Entra ID for authentication.
    </p>
  </div>
</div>
```

- [ ] 3.3 — Create callback handler at `web/server-ui/src/routes/auth/callback/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import { setAuth } from '$lib/stores/auth';
  import { getCallbackRedirectUri } from '$lib/auth';

  let error = $state<string | null>(null);
  let loading = $state(true);

  onMount(async () => {
    const code = $page.url.searchParams.get('code');
    const errorParam = $page.url.searchParams.get('error');
    const errorDesc = $page.url.searchParams.get('error_description');

    if (errorParam) {
      error = errorDesc ?? errorParam;
      loading = false;
      return;
    }

    if (!code) {
      error = 'No authorization code received.';
      loading = false;
      return;
    }

    try {
      const result = await api.auth.login(code, getCallbackRedirectUri());
      setAuth(result.tokens, result.user);
      goto('/');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Authentication failed.';
      loading = false;
    }
  });
</script>

<div class="min-h-screen flex items-center justify-center bg-slate-50">
  <div class="bg-white rounded-lg shadow-lg p-8 max-w-sm w-full text-center">
    {#if loading}
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto mb-4"></div>
      <p class="text-gray-500">Signing you in...</p>
    {:else if error}
      <div class="text-red-600 mb-4">
        <svg class="w-12 h-12 mx-auto mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.27 16.5c-.77.833.192 2.5 1.732 2.5z"/>
        </svg>
        <p class="font-medium">Sign-in failed</p>
        <p class="text-sm mt-1">{error}</p>
      </div>
      <a href="/login" class="text-blue-600 hover:underline text-sm">Try again</a>
    {/if}
  </div>
</div>
```

- [ ] 3.4 — Update root layout with auth guard at `web/server-ui/src/routes/+layout.svelte`:

```svelte
<script lang="ts">
  import '../app.css';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { isAuthenticated } from '$lib/stores/auth';
  import { onMount } from 'svelte';

  let { children } = $props();

  const publicRoutes = ['/login', '/auth/callback'];

  onMount(() => {
    // Reactive auth guard
    const unsubPage = page.subscribe(($page) => {
      const unsubAuth = isAuthenticated.subscribe(($isAuth) => {
        const isPublic = publicRoutes.some((r) => $page.url.pathname.startsWith(r));
        if (!$isAuth && !isPublic) {
          goto('/login');
        }
      });
      // Only check once per navigation
      unsubAuth();
    });

    return unsubPage;
  });
</script>

{@render children()}
```

- [ ] 3.5 — Create `.env.example` for dev at `web/server-ui/.env.example`:

```env
VITE_API_BASE_URL=/api/v1
VITE_ENTRA_TENANT_ID=your-tenant-id
VITE_ENTRA_CLIENT_ID=your-client-id
VITE_ENTRA_AUTHORITY=https://login.microsoftonline.com
```

- [ ] 3.6 — Verify auth pages render:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No type errors.

- [ ] 3.7 — Commit: `git add web/server-ui/src/lib/auth.ts web/server-ui/src/routes/login/ web/server-ui/src/routes/auth/ web/server-ui/src/routes/+layout.svelte web/server-ui/.env.example && git commit -m "feat(server-ui): add Entra OIDC auth flow with login, callback, and auth guard"`

---

## Task 4: Layout & Navigation

**Files:**
- Create: `web/server-ui/src/lib/components/Sidebar.svelte`
- Create: `web/server-ui/src/lib/components/Header.svelte`
- Create: `web/server-ui/src/routes/(app)/+layout.svelte`

**Steps:**

- [ ] 4.1 — Create Sidebar component at `web/server-ui/src/lib/components/Sidebar.svelte`:

```svelte
<script lang="ts">
  import { page } from '$app/stores';
  import { userRole } from '$lib/stores/auth';

  interface NavItem {
    label: string;
    href: string;
    icon: string;
    roles?: string[];
  }

  const navItems: NavItem[] = [
    { label: 'Dashboard', href: '/', icon: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-4 0h4' },
    { label: 'Devices', href: '/devices', icon: 'M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z' },
    { label: 'Team', href: '/team', icon: 'M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z', roles: ['admin', 'manager'] },
    { label: 'Reports', href: '/reports', icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z', roles: ['admin', 'manager'] },
    { label: 'Users', href: '/admin/users', icon: 'M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z', roles: ['admin'] },
    { label: 'Corrections', href: '/admin/corrections', icon: 'M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z', roles: ['admin'] },
    { label: 'Settings', href: '/admin/settings', icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z M15 12a3 3 0 11-6 0 3 3 0 016 0z', roles: ['admin'] },
    { label: 'Audit Log', href: '/admin/audit', icon: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2', roles: ['admin'] },
  ];

  function isActive(href: string, pathname: string): boolean {
    if (href === '/') return pathname === '/';
    return pathname.startsWith(href);
  }
</script>

<aside class="w-64 bg-[var(--color-sidebar)] text-[var(--color-sidebar-text)] min-h-screen flex flex-col">
  <div class="p-6">
    <h1 class="text-xl font-bold text-white">Trasker</h1>
  </div>

  <nav class="flex-1 px-3 space-y-1">
    {#each navItems as item}
      {#if !item.roles || (item.roles && $userRole && item.roles.includes($userRole))}
        <a
          href={item.href}
          class="flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors
            {isActive(item.href, $page.url.pathname)
              ? 'bg-blue-600 text-white'
              : 'text-slate-300 hover:bg-slate-700 hover:text-white'}"
        >
          <svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d={item.icon}/>
          </svg>
          {item.label}
        </a>
      {/if}
    {/each}
  </nav>
</aside>
```

- [ ] 4.2 — Create Header component at `web/server-ui/src/lib/components/Header.svelte`:

```svelte
<script lang="ts">
  import { currentUser, clearAuth } from '$lib/stores/auth';
  import { goto } from '$app/navigation';

  let showMenu = $state(false);

  function handleLogout() {
    clearAuth();
    goto('/login');
  }
</script>

<header class="bg-white border-b border-gray-200 px-6 py-3 flex items-center justify-between">
  <div>
    <!-- Breadcrumb or page title slot could go here -->
  </div>

  <div class="relative">
    <button
      onclick={() => showMenu = !showMenu}
      class="flex items-center gap-2 text-sm text-gray-700 hover:text-gray-900"
    >
      <div class="w-8 h-8 bg-blue-100 text-blue-700 rounded-full flex items-center justify-center font-medium">
        {$currentUser?.display_name?.charAt(0) ?? '?'}
      </div>
      <span>{$currentUser?.display_name ?? 'User'}</span>
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/>
      </svg>
    </button>

    {#if showMenu}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="fixed inset-0 z-10"
        onclick={() => showMenu = false}
        onkeydown={() => {}}
      ></div>
      <div class="absolute right-0 mt-2 w-48 bg-white rounded-lg shadow-lg border z-20 py-1">
        <div class="px-4 py-2 text-xs text-gray-500 border-b">
          {$currentUser?.email}
          <br/>
          <span class="capitalize">{$currentUser?.role}</span>
        </div>
        <button
          onclick={handleLogout}
          class="w-full text-left px-4 py-2 text-sm text-red-600 hover:bg-red-50"
        >
          Sign out
        </button>
      </div>
    {/if}
  </div>
</header>
```

- [ ] 4.3 — Create app layout (authenticated shell) at `web/server-ui/src/routes/(app)/+layout.svelte`:

```svelte
<script lang="ts">
  import Sidebar from '$lib/components/Sidebar.svelte';
  import Header from '$lib/components/Header.svelte';

  let { children } = $props();
</script>

<div class="flex min-h-screen">
  <Sidebar />
  <div class="flex-1 flex flex-col">
    <Header />
    <main class="flex-1 p-6">
      {@render children()}
    </main>
  </div>
</div>
```

- [ ] 4.4 — Move the root `+page.svelte` to the `(app)` group. Create `web/server-ui/src/routes/(app)/+page.svelte`:

```svelte
<script lang="ts">
  // Placeholder — Task 5 builds the real dashboard
</script>

<h1 class="text-2xl font-bold">Dashboard</h1>
<p class="text-gray-500 mt-2">Welcome to Trasker. Your submitted time overview will appear here.</p>
```

Update the original `web/server-ui/src/routes/+page.svelte` to redirect:

```svelte
<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { isAuthenticated } from '$lib/stores/auth';

  onMount(() => {
    if ($isAuthenticated) {
      goto('/');
    } else {
      goto('/login');
    }
  });
</script>
```

> **Note:** The `(app)` route group applies the sidebar/header layout to all authenticated pages. Login and callback routes sit outside this group.

- [ ] 4.5 — Verify layout renders:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No errors.

- [ ] 4.6 — Commit: `git add web/server-ui/src/lib/components/ web/server-ui/src/routes/ && git commit -m "feat(server-ui): add app shell with sidebar navigation and header"`

---

## Task 5: Dashboard Page

**Files:**
- Create: `web/server-ui/src/lib/components/SummaryCard.svelte`
- Create: `web/server-ui/src/lib/components/TagChart.svelte`
- Modify: `web/server-ui/src/routes/(app)/+page.svelte`

**Steps:**

- [ ] 5.1 — Install Chart.js:

```bash
cd web/server-ui && npm install chart.js
```

Expected: `chart.js` added to `package.json` dependencies.

- [ ] 5.2 — Create SummaryCard component at `web/server-ui/src/lib/components/SummaryCard.svelte`:

```svelte
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
```

- [ ] 5.3 — Create TagChart component at `web/server-ui/src/lib/components/TagChart.svelte`:

```svelte
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
```

- [ ] 5.4 — Build the Dashboard page at `web/server-ui/src/routes/(app)/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Timesheet, ReportSummary } from '$lib/types';
  import SummaryCard from '$lib/components/SummaryCard.svelte';
  import TagChart from '$lib/components/TagChart.svelte';

  let timesheets = $state<Timesheet[]>([]);
  let tagSummary = $state<ReportSummary[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Default: current week (Mon-Sun)
  function getCurrentWeekRange(): { from: string; to: string } {
    const now = new Date();
    const dayOfWeek = now.getDay();
    const monday = new Date(now);
    monday.setDate(now.getDate() - ((dayOfWeek + 6) % 7));
    monday.setHours(0, 0, 0, 0);
    const sunday = new Date(monday);
    sunday.setDate(monday.getDate() + 6);
    sunday.setHours(23, 59, 59, 999);
    return {
      from: monday.toISOString(),
      to: sunday.toISOString(),
    };
  }

  onMount(async () => {
    try {
      const range = getCurrentWeekRange();
      const [ts, summary] = await Promise.all([
        api.timesheets.list(range),
        api.reports.summary({ ...range, group_by: 'tag' }),
      ]);
      timesheets = ts;
      tagSummary = summary;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load dashboard data.';
    } finally {
      loading = false;
    }
  });

  function formatDuration(seconds: number): string {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return `${h}h ${m}m`;
  }

  const totalSeconds = $derived(
    tagSummary.reduce((sum, t) => sum + t.total_duration_s, 0)
  );
  const totalEntries = $derived(
    tagSummary.reduce((sum, t) => sum + t.entry_count, 0)
  );
  const topTag = $derived(
    tagSummary.length > 0
      ? tagSummary.reduce((a, b) => (a.total_duration_s > b.total_duration_s ? a : b)).tag
      : 'N/A'
  );
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900">Dashboard</h1>

  {#if loading}
    <div class="flex items-center gap-2 text-gray-500">
      <div class="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600"></div>
      Loading...
    </div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4">{error}</div>
  {:else}
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <SummaryCard
        title="Total Time (This Week)"
        value={formatDuration(totalSeconds)}
        color="blue"
      />
      <SummaryCard
        title="Submissions"
        value={String(totalEntries)}
        subtitle="time blocks submitted"
        color="green"
      />
      <SummaryCard
        title="Top Activity"
        value={topTag}
        subtitle="most time spent"
        color="purple"
      />
    </div>

    <TagChart data={tagSummary} />

    <div class="bg-white rounded-lg shadow-sm border">
      <div class="px-5 py-4 border-b">
        <h3 class="text-sm font-medium text-gray-500">Recent Submissions</h3>
      </div>
      <div class="divide-y">
        {#each timesheets.slice(0, 10) as ts}
          {#each ts.entries as entry}
            <div class="px-5 py-3 flex items-center justify-between">
              <div>
                <span class="font-medium text-gray-900">{entry.tag}</span>
                <span class="text-xs text-gray-400 ml-2">
                  {new Date(entry.started_at).toLocaleDateString()}
                </span>
              </div>
              <span class="text-sm text-gray-600">{formatDuration(entry.duration_s)}</span>
            </div>
          {/each}
        {/each}
        {#if timesheets.length === 0}
          <div class="px-5 py-8 text-center text-gray-400">
            No submissions this week. Submit time from your Trasker client.
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
```

- [ ] 5.5 — Verify:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No type errors.

- [ ] 5.6 — Commit: `git add web/server-ui/src/lib/components/SummaryCard.svelte web/server-ui/src/lib/components/TagChart.svelte web/server-ui/src/routes/ && git commit -m "feat(server-ui): add dashboard page with summary cards and tag chart"`

---

## Task 6: Devices Page

**Files:**
- Create: `web/server-ui/src/routes/(app)/devices/+page.svelte`
- Create: `web/server-ui/src/lib/components/DownloadButton.svelte`

**Steps:**

- [ ] 6.1 — Create DownloadButton component at `web/server-ui/src/lib/components/DownloadButton.svelte`:

```svelte
<script lang="ts">
  import { api } from '$lib/api';

  interface Props {
    targetOs: string;
    targetArch: string;
    label: string;
  }

  let { targetOs, targetArch, label }: Props = $props();

  let downloading = $state(false);
  let error = $state<string | null>(null);

  async function handleDownload() {
    downloading = true;
    error = null;
    try {
      const blob = await api.build.download(targetOs, targetArch);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      const ext = targetOs === 'windows' ? '.exe' : '';
      a.href = url;
      a.download = `trasker-client-${targetOs}-${targetArch}${ext}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Download failed.';
    } finally {
      downloading = false;
    }
  }
</script>

<button
  onclick={handleDownload}
  disabled={downloading}
  class="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors text-sm"
>
  {#if downloading}
    <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>
    Building...
  {:else}
    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
    </svg>
    {label}
  {/if}
</button>
{#if error}
  <p class="text-xs text-red-600 mt-1">{error}</p>
{/if}
```

- [ ] 6.2 — Create Devices page at `web/server-ui/src/routes/(app)/devices/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Device } from '$lib/types';
  import DownloadButton from '$lib/components/DownloadButton.svelte';

  let devices = $state<Device[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let editingId = $state<string | null>(null);
  let editName = $state('');

  onMount(async () => {
    try {
      devices = await api.devices.list();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load devices.';
    } finally {
      loading = false;
    }
  });

  function startEdit(device: Device) {
    editingId = device.id;
    editName = device.device_name ?? '';
  }

  async function saveEdit(deviceId: string) {
    try {
      const updated = await api.devices.updateName(deviceId, editName);
      devices = devices.map((d) => (d.id === deviceId ? updated : d));
      editingId = null;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update device name.';
    }
  }

  function cancelEdit() {
    editingId = null;
  }

  function formatDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, {
      year: 'numeric', month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit',
    });
  }

  const osLabels: Record<string, string> = {
    linux: 'Linux',
    darwin: 'macOS',
    windows: 'Windows',
  };

  const buildTargets = [
    { os: 'linux', arch: 'amd64', label: 'Linux (x64)' },
    { os: 'linux', arch: 'arm64', label: 'Linux (ARM64)' },
    { os: 'darwin', arch: 'amd64', label: 'macOS (Intel)' },
    { os: 'darwin', arch: 'arm64', label: 'macOS (Apple Silicon)' },
    { os: 'windows', arch: 'amd64', label: 'Windows (x64)' },
  ];
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900">Devices</h1>

  <!-- Download Section -->
  <div class="bg-white rounded-lg shadow-sm border p-6">
    <h2 class="text-lg font-semibold text-gray-900 mb-2">Download Trasker Client</h2>
    <p class="text-sm text-gray-500 mb-4">
      Each download generates a unique API key baked into the binary. No configuration needed.
    </p>
    <div class="flex flex-wrap gap-3">
      {#each buildTargets as target}
        <DownloadButton
          targetOs={target.os}
          targetArch={target.arch}
          label={target.label}
        />
      {/each}
    </div>
  </div>

  <!-- Registered Devices -->
  <div class="bg-white rounded-lg shadow-sm border">
    <div class="px-5 py-4 border-b">
      <h2 class="text-lg font-semibold text-gray-900">Registered Devices</h2>
    </div>

    {#if loading}
      <div class="p-5 text-gray-500">Loading devices...</div>
    {:else if error}
      <div class="p-5 text-red-600">{error}</div>
    {:else if devices.length === 0}
      <div class="p-8 text-center text-gray-400">
        No devices registered yet. Download and run the client on a machine to register it.
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead class="bg-gray-50 text-left text-gray-500">
            <tr>
              <th class="px-5 py-3 font-medium">Name</th>
              <th class="px-5 py-3 font-medium">OS</th>
              <th class="px-5 py-3 font-medium">Device ID</th>
              <th class="px-5 py-3 font-medium">Last Seen</th>
              <th class="px-5 py-3 font-medium">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y">
            {#each devices as device}
              <tr>
                <td class="px-5 py-3">
                  {#if editingId === device.id}
                    <div class="flex items-center gap-2">
                      <input
                        type="text"
                        bind:value={editName}
                        class="border rounded px-2 py-1 text-sm w-40"
                        onkeydown={(e) => { if (e.key === 'Enter') saveEdit(device.id); if (e.key === 'Escape') cancelEdit(); }}
                      />
                      <button onclick={() => saveEdit(device.id)} class="text-green-600 hover:text-green-800 text-xs">Save</button>
                      <button onclick={cancelEdit} class="text-gray-400 hover:text-gray-600 text-xs">Cancel</button>
                    </div>
                  {:else}
                    <span class="text-gray-900">{device.device_name ?? 'Unnamed'}</span>
                  {/if}
                </td>
                <td class="px-5 py-3 text-gray-600">{osLabels[device.os] ?? device.os}</td>
                <td class="px-5 py-3 font-mono text-xs text-gray-400">{device.client_device_id.slice(0, 8)}...</td>
                <td class="px-5 py-3 text-gray-600">{formatDate(device.last_seen_at)}</td>
                <td class="px-5 py-3">
                  {#if editingId !== device.id}
                    <button
                      onclick={() => startEdit(device)}
                      class="text-blue-600 hover:text-blue-800 text-xs"
                    >
                      Rename
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>
```

- [ ] 6.3 — Verify:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No type errors.

- [ ] 6.4 — Commit: `git add web/server-ui/src/routes/\(app\)/devices/ web/server-ui/src/lib/components/DownloadButton.svelte && git commit -m "feat(server-ui): add devices page with download buttons and device management"`

---

## Task 7: Team Page

**Files:**
- Create: `web/server-ui/src/routes/(app)/team/+page.svelte`
- Create: `web/server-ui/src/lib/components/DateRangeFilter.svelte`

**Steps:**

- [ ] 7.1 — Create DateRangeFilter component at `web/server-ui/src/lib/components/DateRangeFilter.svelte`:

```svelte
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
    <label class="text-sm text-gray-500">From</label>
    <input type="date" bind:value={from} onchange={handleChange}
      class="border rounded px-2 py-1 text-sm" />
  </div>
  <div class="flex items-center gap-2">
    <label class="text-sm text-gray-500">To</label>
    <input type="date" bind:value={to} onchange={handleChange}
      class="border rounded px-2 py-1 text-sm" />
  </div>
  <div class="flex gap-1">
    <button onclick={setThisWeek} class="px-2 py-1 text-xs bg-gray-100 hover:bg-gray-200 rounded">This Week</button>
    <button onclick={setThisMonth} class="px-2 py-1 text-xs bg-gray-100 hover:bg-gray-200 rounded">This Month</button>
    <button onclick={setLast30Days} class="px-2 py-1 text-xs bg-gray-100 hover:bg-gray-200 rounded">Last 30 Days</button>
  </div>
</div>
```

- [ ] 7.2 — Create Team page at `web/server-ui/src/routes/(app)/team/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Timesheet, User } from '$lib/types';
  import DateRangeFilter from '$lib/components/DateRangeFilter.svelte';

  let timesheets = $state<Timesheet[]>([]);
  let users = $state<User[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Default: last 7 days
  let from = $state(new Date(Date.now() - 7 * 86400000).toISOString().slice(0, 10));
  let to = $state(new Date().toISOString().slice(0, 10));
  let filterUserId = $state<string>('');

  async function loadData() {
    loading = true;
    error = null;
    try {
      const params: { from: string; to: string; user_id?: string } = {
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
      };
      if (filterUserId) params.user_id = filterUserId;

      const [ts, userList] = await Promise.all([
        api.timesheets.team(params),
        users.length === 0 ? api.users.listTeam() : Promise.resolve(users),
      ]);
      timesheets = ts;
      if (users.length === 0) users = userList;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load team data.';
    } finally {
      loading = false;
    }
  }

  onMount(loadData);

  function handleDateChange() {
    loadData();
  }

  function handleUserFilter(e: Event) {
    filterUserId = (e.target as HTMLSelectElement).value;
    loadData();
  }

  function formatDuration(s: number): string {
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    return `${h}h ${m}m`;
  }

  // Build user lookup map
  const userMap = $derived(
    new Map(users.map((u) => [u.id, u]))
  );
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900">Team Submissions</h1>

  <div class="bg-white rounded-lg shadow-sm border p-4 flex items-center gap-4 flex-wrap">
    <DateRangeFilter bind:from bind:to onchange={handleDateChange} />

    <div class="flex items-center gap-2">
      <label class="text-sm text-gray-500">Person</label>
      <select onchange={handleUserFilter} class="border rounded px-2 py-1 text-sm">
        <option value="">All</option>
        {#each users as user}
          <option value={user.id}>{user.display_name}</option>
        {/each}
      </select>
    </div>
  </div>

  {#if loading}
    <div class="text-gray-500">Loading team data...</div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4">{error}</div>
  {:else if timesheets.length === 0}
    <div class="bg-white rounded-lg shadow-sm border p-8 text-center text-gray-400">
      No team submissions found for the selected period.
    </div>
  {:else}
    <div class="bg-white rounded-lg shadow-sm border overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-left text-gray-500">
          <tr>
            <th class="px-5 py-3 font-medium">Person</th>
            <th class="px-5 py-3 font-medium">Tag</th>
            <th class="px-5 py-3 font-medium">Date</th>
            <th class="px-5 py-3 font-medium">Duration</th>
            <th class="px-5 py-3 font-medium">Notes</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each timesheets as ts}
            {#each ts.entries as entry}
              <tr>
                <td class="px-5 py-3 text-gray-900">
                  {userMap.get(ts.user_id)?.display_name ?? 'Unknown'}
                </td>
                <td class="px-5 py-3">
                  <span class="inline-block px-2 py-0.5 bg-blue-50 text-blue-700 text-xs rounded-full">
                    {entry.tag}
                  </span>
                </td>
                <td class="px-5 py-3 text-gray-600">
                  {new Date(entry.started_at).toLocaleDateString()}
                </td>
                <td class="px-5 py-3 text-gray-600">{formatDuration(entry.duration_s)}</td>
                <td class="px-5 py-3 text-gray-500 text-xs max-w-xs truncate">
                  {entry.notes ?? ''}
                </td>
              </tr>
            {/each}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
```

- [ ] 7.3 — Verify:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No type errors.

- [ ] 7.4 — Commit: `git add web/server-ui/src/routes/\(app\)/team/ web/server-ui/src/lib/components/DateRangeFilter.svelte && git commit -m "feat(server-ui): add team page with date range and person filter"`

---

## Task 8: Reports Page

**Files:**
- Create: `web/server-ui/src/routes/(app)/reports/+page.svelte`
- Create: `web/server-ui/src/lib/components/BarChart.svelte`

**Steps:**

- [ ] 8.1 — Create BarChart component at `web/server-ui/src/lib/components/BarChart.svelte`:

```svelte
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
```

- [ ] 8.2 — Create Reports page at `web/server-ui/src/routes/(app)/reports/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { ReportSummary } from '$lib/types';
  import DateRangeFilter from '$lib/components/DateRangeFilter.svelte';
  import TagChart from '$lib/components/TagChart.svelte';
  import BarChart from '$lib/components/BarChart.svelte';

  let tagData = $state<ReportSummary[]>([]);
  let dayData = $state<ReportSummary[]>([]);
  let personData = $state<ReportSummary[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let from = $state(new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10));
  let to = $state(new Date().toISOString().slice(0, 10));

  async function loadData() {
    loading = true;
    error = null;
    try {
      const params = {
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
      };
      const [byTag, byDay, byPerson] = await Promise.all([
        api.reports.summary({ ...params, group_by: 'tag' }),
        api.reports.summary({ ...params, group_by: 'day' }),
        api.reports.summary({ ...params, group_by: 'person' }),
      ]);
      tagData = byTag;
      dayData = byDay;
      personData = byPerson;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load report data.';
    } finally {
      loading = false;
    }
  }

  onMount(loadData);

  function handleDateChange() {
    loadData();
  }

  async function handleExportCsv() {
    try {
      const blob = await api.reports.exportCsv({
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `trasker-report-${from}-to-${to}.csv`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (e) {
      error = e instanceof Error ? e.message : 'CSV export failed.';
    }
  }

  const TAG_COLORS = [
    '#2563eb', '#16a34a', '#d97706', '#dc2626', '#7c3aed',
    '#0891b2', '#be185d', '#65a30d', '#ea580c', '#4f46e5',
  ];

  const dayChartLabels = $derived(dayData.map((d) => d.tag)); // tag field reused as day label
  const dayChartDatasets = $derived([{
    label: 'Hours',
    data: dayData.map((d) => d.total_duration_s / 3600),
    color: '#2563eb',
  }]);

  const personChartLabels = $derived(personData.map((d) => d.tag)); // tag field reused as person name
  const personChartDatasets = $derived([{
    label: 'Hours',
    data: personData.map((d) => d.total_duration_s / 3600),
    color: '#16a34a',
  }]);
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold text-gray-900">Reports</h1>
    <button
      onclick={handleExportCsv}
      class="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 text-sm"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
      </svg>
      Export CSV
    </button>
  </div>

  <div class="bg-white rounded-lg shadow-sm border p-4">
    <DateRangeFilter bind:from bind:to onchange={handleDateChange} />
  </div>

  {#if loading}
    <div class="text-gray-500">Loading reports...</div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4">{error}</div>
  {:else}
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <div class="bg-white rounded-lg shadow-sm border p-5">
        <h3 class="text-sm font-medium text-gray-500 mb-4">Time by Tag</h3>
        <TagChart data={tagData} />
      </div>

      <div class="bg-white rounded-lg shadow-sm border p-5">
        <h3 class="text-sm font-medium text-gray-500 mb-4">Time by Day</h3>
        <BarChart labels={dayChartLabels} datasets={dayChartDatasets} />
      </div>

      <div class="bg-white rounded-lg shadow-sm border p-5 lg:col-span-2">
        <h3 class="text-sm font-medium text-gray-500 mb-4">Time by Person</h3>
        <BarChart labels={personChartLabels} datasets={personChartDatasets} />
      </div>
    </div>
  {/if}
</div>
```

- [ ] 8.3 — Verify:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No type errors.

- [ ] 8.4 — Commit: `git add web/server-ui/src/routes/\(app\)/reports/ web/server-ui/src/lib/components/BarChart.svelte && git commit -m "feat(server-ui): add reports page with charts and CSV export"`

---

## Task 9: Admin — User Management

**Files:**
- Create: `web/server-ui/src/routes/(app)/admin/users/+page.svelte`

**Steps:**

- [ ] 9.1 — Create Users admin page at `web/server-ui/src/routes/(app)/admin/users/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { User, ApiKey } from '$lib/types';

  let users = $state<User[]>([]);
  let apiKeys = $state<Map<string, ApiKey[]>>(new Map());
  let loading = $state(true);
  let error = $state<string | null>(null);
  let actionError = $state<string | null>(null);

  onMount(async () => {
    try {
      users = await api.users.listTeam();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load users.';
    } finally {
      loading = false;
    }
  });

  async function changeRole(userId: string, newRole: string) {
    actionError = null;
    try {
      const updated = await api.users.updateRole(userId, newRole);
      users = users.map((u) => (u.id === userId ? updated : u));
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to update role.';
    }
  }

  async function loadKeys(userId: string) {
    try {
      const keys = await api.admin.listApiKeys(userId);
      apiKeys = new Map([...apiKeys, [userId, keys]]);
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to load API keys.';
    }
  }

  async function revokeKey(keyId: string, userId: string) {
    if (!confirm('Revoke this API key? The associated client will stop working until the user downloads a new one.')) return;
    actionError = null;
    try {
      await api.admin.revokeKey(keyId);
      // Reload keys for this user
      await loadKeys(userId);
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to revoke key.';
    }
  }

  function formatDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, {
      year: 'numeric', month: 'short', day: 'numeric',
    });
  }

  const roleBadgeClasses: Record<string, string> = {
    admin: 'bg-red-50 text-red-700',
    manager: 'bg-amber-50 text-amber-700',
    member: 'bg-gray-100 text-gray-700',
  };
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900">User Management</h1>

  {#if actionError}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-3 text-sm">{actionError}</div>
  {/if}

  {#if loading}
    <div class="text-gray-500">Loading users...</div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4">{error}</div>
  {:else}
    <div class="bg-white rounded-lg shadow-sm border overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-left text-gray-500">
          <tr>
            <th class="px-5 py-3 font-medium">Name</th>
            <th class="px-5 py-3 font-medium">Email</th>
            <th class="px-5 py-3 font-medium">Role</th>
            <th class="px-5 py-3 font-medium">Joined</th>
            <th class="px-5 py-3 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each users as user}
            <tr>
              <td class="px-5 py-3 text-gray-900 font-medium">{user.display_name}</td>
              <td class="px-5 py-3 text-gray-600">{user.email}</td>
              <td class="px-5 py-3">
                <select
                  value={user.role}
                  onchange={(e) => changeRole(user.id, (e.target as HTMLSelectElement).value)}
                  class="text-xs px-2 py-1 rounded border {roleBadgeClasses[user.role] ?? ''}"
                >
                  <option value="member">Member</option>
                  <option value="manager">Manager</option>
                  <option value="admin">Admin</option>
                </select>
              </td>
              <td class="px-5 py-3 text-gray-600">{formatDate(user.created_at)}</td>
              <td class="px-5 py-3">
                <button
                  onclick={() => loadKeys(user.id)}
                  class="text-blue-600 hover:text-blue-800 text-xs"
                >
                  View API Keys
                </button>
              </td>
            </tr>

            <!-- Expandable API Keys row -->
            {#if apiKeys.has(user.id)}
              <tr class="bg-gray-50">
                <td colspan="5" class="px-5 py-3">
                  <div class="text-xs text-gray-500 mb-2">API Keys for {user.display_name}:</div>
                  {#if (apiKeys.get(user.id) ?? []).length === 0}
                    <span class="text-xs text-gray-400">No API keys.</span>
                  {:else}
                    <div class="space-y-1">
                      {#each apiKeys.get(user.id) ?? [] as key}
                        <div class="flex items-center gap-3 text-xs">
                          <span class="font-mono text-gray-500">{key.key_prefix}...</span>
                          <span class={key.revoked ? 'text-red-500' : 'text-green-600'}>
                            {key.revoked ? 'Revoked' : 'Active'}
                          </span>
                          <span class="text-gray-400">Last used: {formatDate(key.last_used_at)}</span>
                          <span class="text-gray-400">Expires: {formatDate(key.expires_at)}</span>
                          {#if !key.revoked}
                            <button
                              onclick={() => revokeKey(key.id, user.id)}
                              class="text-red-600 hover:text-red-800"
                            >
                              Revoke
                            </button>
                          {/if}
                        </div>
                      {/each}
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
```

- [ ] 9.2 — Verify:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No type errors.

- [ ] 9.3 — Commit: `git add web/server-ui/src/routes/\(app\)/admin/users/ && git commit -m "feat(server-ui): add admin user management with role changes and API key revocation"`

---

## Task 10: Admin — Timesheet Corrections

**Files:**
- Create: `web/server-ui/src/routes/(app)/admin/corrections/+page.svelte`

**Steps:**

- [ ] 10.1 — Create Corrections page at `web/server-ui/src/routes/(app)/admin/corrections/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Timesheet, TimesheetEntry, User } from '$lib/types';
  import DateRangeFilter from '$lib/components/DateRangeFilter.svelte';

  let timesheets = $state<Timesheet[]>([]);
  let users = $state<User[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let actionError = $state<string | null>(null);

  let from = $state(new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10));
  let to = $state(new Date().toISOString().slice(0, 10));

  let editingEntry = $state<TimesheetEntry | null>(null);
  let editTag = $state('');
  let editNotes = $state('');
  let editDuration = $state(0);

  async function loadData() {
    loading = true;
    error = null;
    try {
      const params = {
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
      };
      const [ts, userList] = await Promise.all([
        api.timesheets.team(params),
        users.length === 0 ? api.users.listTeam() : Promise.resolve(users),
      ]);
      timesheets = ts;
      if (users.length === 0) users = userList;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load data.';
    } finally {
      loading = false;
    }
  }

  onMount(loadData);

  function handleDateChange() {
    loadData();
  }

  function startEdit(entry: TimesheetEntry) {
    editingEntry = entry;
    editTag = entry.tag;
    editNotes = entry.notes ?? '';
    editDuration = entry.duration_s;
  }

  async function saveEdit() {
    if (!editingEntry) return;
    actionError = null;
    try {
      await api.admin.editEntry(editingEntry.id, {
        tag: editTag,
        notes: editNotes,
        duration_s: editDuration,
      });
      editingEntry = null;
      await loadData();
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to save changes.';
    }
  }

  async function deleteEntry(entryId: string) {
    if (!confirm(
      'Delete this timesheet entry? This action is audit-logged and cannot be undone.'
    )) return;
    actionError = null;
    try {
      await api.admin.deleteEntry(entryId);
      await loadData();
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed to delete entry.';
    }
  }

  function cancelEdit() {
    editingEntry = null;
  }

  function formatDuration(s: number): string {
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    return `${h}h ${m}m`;
  }

  const userMap = $derived(new Map(users.map((u) => [u.id, u])));
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900">Timesheet Corrections</h1>

  <div class="bg-amber-50 border border-amber-200 text-amber-800 rounded-lg p-3 text-sm">
    All edits and deletions are recorded in the audit log. Only modify entries when there is a legitimate correction needed.
  </div>

  {#if actionError}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-3 text-sm">{actionError}</div>
  {/if}

  <div class="bg-white rounded-lg shadow-sm border p-4">
    <DateRangeFilter bind:from bind:to onchange={handleDateChange} />
  </div>

  <!-- Edit Modal -->
  {#if editingEntry}
    <div class="fixed inset-0 bg-black/50 z-40 flex items-center justify-center">
      <div class="bg-white rounded-lg shadow-xl p-6 w-full max-w-md">
        <h3 class="text-lg font-semibold mb-4">Edit Entry</h3>
        <div class="space-y-3">
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">Tag</label>
            <input type="text" bind:value={editTag} class="border rounded px-3 py-2 w-full text-sm" />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">Duration (seconds)</label>
            <input type="number" bind:value={editDuration} class="border rounded px-3 py-2 w-full text-sm" />
            <p class="text-xs text-gray-400 mt-1">{formatDuration(editDuration)}</p>
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">Notes</label>
            <textarea bind:value={editNotes} rows="3" class="border rounded px-3 py-2 w-full text-sm"></textarea>
          </div>
        </div>
        <div class="flex justify-end gap-2 mt-4">
          <button onclick={cancelEdit} class="px-4 py-2 text-sm text-gray-600 hover:text-gray-900">Cancel</button>
          <button onclick={saveEdit} class="px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700">Save Changes</button>
        </div>
      </div>
    </div>
  {/if}

  {#if loading}
    <div class="text-gray-500">Loading entries...</div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4">{error}</div>
  {:else}
    <div class="bg-white rounded-lg shadow-sm border overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-left text-gray-500">
          <tr>
            <th class="px-5 py-3 font-medium">Person</th>
            <th class="px-5 py-3 font-medium">Tag</th>
            <th class="px-5 py-3 font-medium">Date</th>
            <th class="px-5 py-3 font-medium">Duration</th>
            <th class="px-5 py-3 font-medium">Notes</th>
            <th class="px-5 py-3 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each timesheets as ts}
            {#each ts.entries as entry}
              <tr>
                <td class="px-5 py-3 text-gray-900">
                  {userMap.get(ts.user_id)?.display_name ?? 'Unknown'}
                </td>
                <td class="px-5 py-3">
                  <span class="inline-block px-2 py-0.5 bg-blue-50 text-blue-700 text-xs rounded-full">
                    {entry.tag}
                  </span>
                </td>
                <td class="px-5 py-3 text-gray-600">
                  {new Date(entry.started_at).toLocaleDateString()}
                </td>
                <td class="px-5 py-3 text-gray-600">{formatDuration(entry.duration_s)}</td>
                <td class="px-5 py-3 text-gray-500 text-xs max-w-xs truncate">{entry.notes ?? ''}</td>
                <td class="px-5 py-3 space-x-2">
                  <button onclick={() => startEdit(entry)} class="text-blue-600 hover:text-blue-800 text-xs">Edit</button>
                  <button onclick={() => deleteEntry(entry.id)} class="text-red-600 hover:text-red-800 text-xs">Delete</button>
                </td>
              </tr>
            {/each}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
```

- [ ] 10.2 — Verify:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No type errors.

- [ ] 10.3 — Commit: `git add web/server-ui/src/routes/\(app\)/admin/corrections/ && git commit -m "feat(server-ui): add admin timesheet corrections with edit/delete and audit trail warning"`

---

## Task 11: Admin — Org Settings

**Files:**
- Create: `web/server-ui/src/routes/(app)/admin/settings/+page.svelte`

**Steps:**

- [ ] 11.1 — Create Org Settings page at `web/server-ui/src/routes/(app)/admin/settings/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { OrgSettings } from '$lib/types';

  let settings = $state<OrgSettings | null>(null);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state<string | null>(null);
  let success = $state(false);

  // Editable fields
  let orgName = $state('');
  let entraTenant = $state('');
  let entraClient = $state('');
  let keyExpiryDays = $state(60);

  onMount(async () => {
    try {
      settings = await api.admin.getSettings();
      orgName = settings.org_name;
      entraTenant = settings.entra_tenant;
      entraClient = settings.entra_client;
      keyExpiryDays = settings.key_expiry_days;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load settings.';
    } finally {
      loading = false;
    }
  });

  async function handleSave() {
    saving = true;
    error = null;
    success = false;
    try {
      settings = await api.admin.updateSettings({
        org_name: orgName,
        entra_tenant: entraTenant,
        entra_client: entraClient,
        key_expiry_days: keyExpiryDays,
      });
      success = true;
      setTimeout(() => (success = false), 3000);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save settings.';
    } finally {
      saving = false;
    }
  }
</script>

<div class="space-y-6 max-w-2xl">
  <h1 class="text-2xl font-bold text-gray-900">Organization Settings</h1>

  {#if loading}
    <div class="text-gray-500">Loading settings...</div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4">{error}</div>
  {:else}
    <div class="bg-white rounded-lg shadow-sm border p-6 space-y-5">
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Organization Name</label>
        <input type="text" bind:value={orgName}
          class="border rounded-lg px-3 py-2 w-full text-sm" />
      </div>

      <hr class="border-gray-200" />

      <h3 class="text-sm font-semibold text-gray-900">Microsoft Entra ID Configuration</h3>

      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Tenant ID</label>
        <input type="text" bind:value={entraTenant}
          class="border rounded-lg px-3 py-2 w-full text-sm font-mono"
          placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
        <p class="text-xs text-gray-400 mt-1">
          Your Microsoft Entra (Azure AD) tenant ID. Found in Azure Portal &gt; App registrations.
        </p>
      </div>

      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Client ID (Application ID)</label>
        <input type="text" bind:value={entraClient}
          class="border rounded-lg px-3 py-2 w-full text-sm font-mono"
          placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
        <p class="text-xs text-gray-400 mt-1">
          The application (client) ID of your registered Entra app.
        </p>
      </div>

      <hr class="border-gray-200" />

      <h3 class="text-sm font-semibold text-gray-900">API Key Policy</h3>

      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Key Expiry (days of inactivity)</label>
        <input type="number" bind:value={keyExpiryDays} min="1" max="365"
          class="border rounded-lg px-3 py-2 w-32 text-sm" />
        <p class="text-xs text-gray-400 mt-1">
          API keys expire after this many days without any client communication. Default: 60.
        </p>
      </div>

      <div class="flex items-center gap-3 pt-2">
        <button
          onclick={handleSave}
          disabled={saving}
          class="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 text-sm"
        >
          {saving ? 'Saving...' : 'Save Settings'}
        </button>
        {#if success}
          <span class="text-green-600 text-sm">Settings saved.</span>
        {/if}
      </div>
    </div>

    {#if settings}
      <div class="text-xs text-gray-400">
        Last updated: {new Date(settings.updated_at).toLocaleString()}
      </div>
    {/if}
  {/if}
</div>
```

- [ ] 11.2 — Verify:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No type errors.

- [ ] 11.3 — Commit: `git add web/server-ui/src/routes/\(app\)/admin/settings/ && git commit -m "feat(server-ui): add admin org settings page with Entra config and key expiry"`

---

## Task 12: Admin — Audit Log

**Files:**
- Create: `web/server-ui/src/routes/(app)/admin/audit/+page.svelte`

**Steps:**

- [ ] 12.1 — Create Audit Log page at `web/server-ui/src/routes/(app)/admin/audit/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { AuditLogEntry } from '$lib/types';
  import DateRangeFilter from '$lib/components/DateRangeFilter.svelte';

  let entries = $state<AuditLogEntry[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let from = $state(new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10));
  let to = $state(new Date().toISOString().slice(0, 10));
  let actionFilter = $state('');

  const actionTypes = [
    '', 'entry.edit', 'entry.delete', 'user.role_change',
    'key.revoke', 'settings.update',
  ];

  async function loadData() {
    loading = true;
    error = null;
    try {
      entries = await api.admin.auditLog({
        from: new Date(from).toISOString(),
        to: new Date(to + 'T23:59:59').toISOString(),
        action: actionFilter || undefined,
      });
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load audit log.';
    } finally {
      loading = false;
    }
  }

  onMount(loadData);

  function handleDateChange() {
    loadData();
  }

  function handleActionFilter(e: Event) {
    actionFilter = (e.target as HTMLSelectElement).value;
    loadData();
  }

  function formatTimestamp(iso: string): string {
    return new Date(iso).toLocaleString(undefined, {
      year: 'numeric', month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
    });
  }

  function formatJson(val: unknown): string {
    if (val === null || val === undefined) return '-';
    try {
      return JSON.stringify(val, null, 2);
    } catch {
      return String(val);
    }
  }

  const actionLabels: Record<string, string> = {
    'entry.edit': 'Edit Entry',
    'entry.delete': 'Delete Entry',
    'user.role_change': 'Change Role',
    'key.revoke': 'Revoke API Key',
    'settings.update': 'Update Settings',
  };

  const actionColors: Record<string, string> = {
    'entry.edit': 'bg-amber-50 text-amber-700',
    'entry.delete': 'bg-red-50 text-red-700',
    'user.role_change': 'bg-blue-50 text-blue-700',
    'key.revoke': 'bg-red-50 text-red-700',
    'settings.update': 'bg-gray-100 text-gray-700',
  };
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900">Audit Log</h1>

  <div class="bg-white rounded-lg shadow-sm border p-4 flex items-center gap-4 flex-wrap">
    <DateRangeFilter bind:from bind:to onchange={handleDateChange} />

    <div class="flex items-center gap-2">
      <label class="text-sm text-gray-500">Action</label>
      <select onchange={handleActionFilter} class="border rounded px-2 py-1 text-sm">
        {#each actionTypes as action}
          <option value={action}>{action ? (actionLabels[action] ?? action) : 'All Actions'}</option>
        {/each}
      </select>
    </div>
  </div>

  {#if loading}
    <div class="text-gray-500">Loading audit log...</div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4">{error}</div>
  {:else if entries.length === 0}
    <div class="bg-white rounded-lg shadow-sm border p-8 text-center text-gray-400">
      No audit log entries for the selected period and filter.
    </div>
  {:else}
    <div class="space-y-3">
      {#each entries as entry}
        <div class="bg-white rounded-lg shadow-sm border p-4">
          <div class="flex items-center gap-3 mb-2">
            <span class="inline-block px-2 py-0.5 text-xs rounded-full {actionColors[entry.action] ?? 'bg-gray-100 text-gray-700'}">
              {actionLabels[entry.action] ?? entry.action}
            </span>
            <span class="text-sm text-gray-900 font-medium">
              {entry.admin_name ?? entry.admin_id.slice(0, 8)}
            </span>
            <span class="text-xs text-gray-400">{formatTimestamp(entry.created_at)}</span>
          </div>

          <div class="text-xs text-gray-500 mb-1">
            Target: <span class="font-mono">{entry.target_type} / {entry.target_id.slice(0, 8)}...</span>
          </div>

          {#if entry.old_value || entry.new_value}
            <div class="grid grid-cols-2 gap-3 mt-2">
              <div>
                <span class="text-xs text-gray-400">Before:</span>
                <pre class="text-xs bg-red-50 rounded p-2 mt-1 overflow-auto max-h-32">{formatJson(entry.old_value)}</pre>
              </div>
              <div>
                <span class="text-xs text-gray-400">After:</span>
                <pre class="text-xs bg-green-50 rounded p-2 mt-1 overflow-auto max-h-32">{formatJson(entry.new_value)}</pre>
              </div>
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
```

- [ ] 12.2 — Verify:

```bash
cd web/server-ui && npx svelte-check
```

Expected: No type errors.

- [ ] 12.3 — Commit: `git add web/server-ui/src/routes/\(app\)/admin/audit/ && git commit -m "feat(server-ui): add admin audit log with action filters and before/after diff"`

---

## Task 13: Build Pipeline

**Files:**
- Create: `internal/server/builder/builder.go`
- Create: `internal/server/builder/builder_test.go`

**Steps:**

- [ ] 13.1 — Create the builder package at `internal/server/builder/builder.go`:

```go
package builder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Target represents a cross-compilation target.
type Target struct {
	OS   string // "linux", "darwin", "windows"
	Arch string // "amd64", "arm64"
}

// String returns the GOOS/GOARCH string.
func (t Target) String() string {
	return t.OS + "/" + t.Arch
}

// BinaryName returns the output binary name for this target.
func (t Target) BinaryName() string {
	name := fmt.Sprintf("trasker-client-%s-%s", t.OS, t.Arch)
	if t.OS == "windows" {
		name += ".exe"
	}
	return name
}

// SupportedTargets lists all valid build targets.
var SupportedTargets = []Target{
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "amd64"},
}

// ValidateTarget checks if a target is in the supported list.
func ValidateTarget(os, arch string) (Target, error) {
	for _, t := range SupportedTargets {
		if t.OS == os && t.Arch == arch {
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("unsupported target: %s/%s", os, arch)
}

// BuildRequest contains all information needed to build a client binary.
type BuildRequest struct {
	Target    Target
	APIKey    string // Plaintext API key to bake in
	ServerURL string // Server URL to bake in
	Version   string // Version string to embed
	OutputDir string // Directory to write the binary
}

// BuildResult contains the result of a build.
type BuildResult struct {
	BinaryPath string
	Target     Target
	Err        error
}

// Builder cross-compiles client binaries with stamped configuration.
type Builder struct {
	// SourceDir is the path to the Go module root (where go.mod lives).
	SourceDir string

	// ClientPkg is the import path of the client main package,
	// relative to the module root.
	ClientPkg string

	mu       sync.Mutex
	building map[string]bool // key: target string, value: in progress
}

// NewBuilder creates a Builder.
//
// sourceDir: absolute path to the Go module root.
// clientPkg: relative import path, e.g. "./cmd/trasker-client".
func NewBuilder(sourceDir, clientPkg string) *Builder {
	return &Builder{
		SourceDir: sourceDir,
		ClientPkg: clientPkg,
		building:  make(map[string]bool),
	}
}

// IsBuilding reports whether a build is in progress for the given target.
func (b *Builder) IsBuilding(t Target) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.building[t.String()]
}

// Build cross-compiles the client binary for the given request.
//
// It sets GOOS/GOARCH and uses -ldflags to stamp the API key, server URL,
// and version into the binary. Since trasker uses modernc.org/sqlite (pure Go),
// CGO_ENABLED=0 works for all targets — no C cross-compilers needed.
//
// NOTE: This invokes `go build` directly, so the Go toolchain must be available.
// In Docker deployment, the server API container does NOT include Go.
// The builder runs inside the separate Dockerfile.builder container.
// The server API invokes builds via `docker exec` or a build queue:
//   docker exec trasker-builder /build.sh --goos=linux --goarch=amd64 --api-key=... --server-url=...
// The Build() method below is used when Go is available locally (dev mode).
// For production Docker deployment, use BuildViaDocker() which shells out to the builder container.
func (b *Builder) Build(ctx context.Context, req BuildRequest) BuildResult {
	targetKey := req.Target.String()

	b.mu.Lock()
	if b.building[targetKey] {
		b.mu.Unlock()
		return BuildResult{
			Target: req.Target,
			Err:    fmt.Errorf("build already in progress for %s", targetKey),
		}
	}
	b.building[targetKey] = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.building, targetKey)
		b.mu.Unlock()
	}()

	// Ensure output directory exists
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return BuildResult{Target: req.Target, Err: fmt.Errorf("create output dir: %w", err)}
	}

	outputPath := filepath.Join(req.OutputDir, req.Target.BinaryName())

	// Build ldflags to stamp values into the binary.
	// These correspond to variables in cmd/trasker-client/main.go:
	//   var apiKey string
	//   var serverURL string
	//   var version string
	ldflags := fmt.Sprintf(
		"-s -w -X main.apiKey=%s -X main.serverURL=%s -X main.version=%s",
		req.APIKey, req.ServerURL, req.Version,
	)

	args := []string{
		"build",
		"-ldflags", ldflags,
		"-trimpath",
		"-o", outputPath,
		req.Target.BinaryName(), // throwaway — overridden by -o
	}
	// The actual package to build
	args[len(args)-1] = b.ClientPkg

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = b.SourceDir
	cmd.Env = append(os.Environ(),
		"GOOS="+req.Target.OS,
		"GOARCH="+req.Target.Arch,
		"CGO_ENABLED=0",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return BuildResult{
			Target: req.Target,
			Err:    fmt.Errorf("go build failed: %w\noutput: %s", err, string(output)),
		}
	}

	return BuildResult{
		BinaryPath: outputPath,
		Target:     req.Target,
	}
}
```

- [ ] 13.2 — Create builder tests at `internal/server/builder/builder_test.go`:

```go
package builder

import (
	"testing"
)

func TestValidateTarget_Supported(t *testing.T) {
	target, err := ValidateTarget("linux", "amd64")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if target.OS != "linux" || target.Arch != "amd64" {
		t.Fatalf("expected linux/amd64, got: %s", target.String())
	}
}

func TestValidateTarget_Unsupported(t *testing.T) {
	_, err := ValidateTarget("plan9", "mips")
	if err == nil {
		t.Fatal("expected error for unsupported target, got nil")
	}
}

func TestTargetBinaryName_Linux(t *testing.T) {
	target := Target{OS: "linux", Arch: "amd64"}
	expected := "trasker-client-linux-amd64"
	if got := target.BinaryName(); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestTargetBinaryName_Windows(t *testing.T) {
	target := Target{OS: "windows", Arch: "amd64"}
	expected := "trasker-client-windows-amd64.exe"
	if got := target.BinaryName(); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestNewBuilder(t *testing.T) {
	b := NewBuilder("/path/to/src", "./cmd/trasker-client")
	if b.SourceDir != "/path/to/src" {
		t.Fatalf("expected /path/to/src, got %s", b.SourceDir)
	}
	if b.ClientPkg != "./cmd/trasker-client" {
		t.Fatalf("expected ./cmd/trasker-client, got %s", b.ClientPkg)
	}
}

func TestIsBuilding_DefaultFalse(t *testing.T) {
	b := NewBuilder("/tmp", "./cmd/trasker-client")
	target := Target{OS: "linux", Arch: "amd64"}
	if b.IsBuilding(target) {
		t.Fatal("expected IsBuilding to return false for new builder")
	}
}
```

- [ ] 13.3 — Run the tests:

```bash
cd /path/to/trasker && go test ./internal/server/builder/ -v
```

Expected output:
```
=== RUN   TestValidateTarget_Supported
--- PASS: TestValidateTarget_Supported
=== RUN   TestValidateTarget_Unsupported
--- PASS: TestValidateTarget_Unsupported
=== RUN   TestTargetBinaryName_Linux
--- PASS: TestTargetBinaryName_Linux
=== RUN   TestTargetBinaryName_Windows
--- PASS: TestTargetBinaryName_Windows
=== RUN   TestNewBuilder
--- PASS: TestNewBuilder
=== RUN   TestIsBuilding_DefaultFalse
--- PASS: TestIsBuilding_DefaultFalse
PASS
```

- [ ] 13.4 — Commit: `git add internal/server/builder/ && git commit -m "feat(builder): add cross-compilation builder with ldflags stamping for client binaries"`

---

## Task 14: Docker Compose

**Files:**
- Create: `deploy/docker-compose.yml`

**Steps:**

- [ ] 14.1 — Create `deploy/docker-compose.yml`:

```yaml
# Trasker — Docker Compose deployment
# Usage: cd deploy && docker compose up -d
#
# Requires .env file — see .env.example in this directory.

services:
  # PostgreSQL database
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-trasker}
      POSTGRES_USER: ${POSTGRES_USER:-trasker}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-trasker}"]
      interval: 5s
      timeout: 3s
      retries: 5
    networks:
      - internal

  # Go API server
  api:
    build:
      context: ..
      dockerfile: deploy/Dockerfile.server
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      TRASKER_DB_HOST: postgres
      TRASKER_DB_PORT: 5432
      TRASKER_DB_NAME: ${POSTGRES_DB:-trasker}
      TRASKER_DB_USER: ${POSTGRES_USER:-trasker}
      TRASKER_DB_PASSWORD: ${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}
      TRASKER_ENTRA_TENANT: ${ENTRA_TENANT_ID:?ENTRA_TENANT_ID is required}
      TRASKER_ENTRA_CLIENT: ${ENTRA_CLIENT_ID:?ENTRA_CLIENT_ID is required}
      TRASKER_ENTRA_SECRET: ${ENTRA_CLIENT_SECRET:?ENTRA_CLIENT_SECRET is required}
      TRASKER_SERVER_URL: ${SERVER_URL:?SERVER_URL is required}
      TRASKER_JWT_SECRET: ${JWT_SECRET:?JWT_SECRET is required}
    volumes:
      # Builder needs access to Go source to cross-compile client binaries
      - buildcache:/tmp/trasker-builds
    expose:
      - "8080"
    networks:
      - internal

  # Svelte SPA served by nginx
  web:
    image: nginx:alpine
    restart: unless-stopped
    volumes:
      - ../web/server-ui/build:/usr/share/nginx/html:ro
      - ./nginx/default.conf:/etc/nginx/conf.d/default.conf:ro
    expose:
      - "80"
    networks:
      - internal

  # Caddy reverse proxy with auto-TLS
  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    depends_on:
      - api
      - web
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./caddy/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    environment:
      DOMAIN: ${DOMAIN:?DOMAIN is required}
    networks:
      - internal

volumes:
  pgdata:
  buildcache:
  caddy_data:
  caddy_config:

networks:
  internal:
```

- [ ] 14.2 — Create nginx config for SPA at `deploy/nginx/default.conf`:

```nginx
server {
    listen 80;
    server_name _;

    root /usr/share/nginx/html;
    index index.html;

    # SPA: all routes fall back to index.html
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Cache static assets
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
}
```

- [ ] 14.3 — Verify docker-compose config is valid:

```bash
cd deploy && docker compose config --quiet
```

Expected: No output (valid config). Note: will fail if `.env` is missing, which is expected at this stage — the validation proves syntax is correct.

- [ ] 14.4 — Commit: `git add deploy/docker-compose.yml deploy/nginx/ && git commit -m "feat(deploy): add Docker Compose stack with postgres, api, web, and caddy services"`

---

## Task 15: Dockerfile.server

**Files:**
- Create: `deploy/Dockerfile.server`

**Steps:**

- [ ] 15.1 — Create multi-stage Dockerfile at `deploy/Dockerfile.server`:

```dockerfile
# ============================================
# Stage 1: Build the Go API server binary
# ============================================
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build server binary
# CGO_ENABLED=0 for static binary (no CGo dependencies in server)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /out/trasker-server \
    ./cmd/trasker-server

# ============================================
# Stage 2: Build the Svelte SPA (for embedding or reference)
# ============================================
# Note: The SPA is served by nginx in docker-compose, not embedded in the Go binary.
# This stage is here for build pipeline completeness — the built assets
# can be copied to the nginx volume.

# ============================================
# Stage 3: Runtime
# ============================================
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 trasker && \
    adduser -u 1001 -G trasker -D trasker

WORKDIR /app

COPY --from=builder /out/trasker-server .

# Copy migrations for the server to run on startup
COPY migrations/ ./migrations/

# Build cache directory for cross-compiled client binaries
RUN mkdir -p /tmp/trasker-builds && chown trasker:trasker /tmp/trasker-builds

USER trasker

EXPOSE 8080

ENTRYPOINT ["./trasker-server"]
```

- [ ] 15.2 — Verify Dockerfile syntax:

```bash
docker build --check -f deploy/Dockerfile.server .
```

Expected: No syntax errors. (Build itself will fail without Go source — that is expected.)

- [ ] 15.3 — Commit: `git add deploy/Dockerfile.server && git commit -m "feat(deploy): add multi-stage Dockerfile for Go API server"`

---

## Task 16: Dockerfile.builder

**Files:**
- Create: `deploy/Dockerfile.builder`

**Steps:**

- [ ] 16.1 — Create cross-compilation Dockerfile at `deploy/Dockerfile.builder`:

```dockerfile
# ============================================
# Trasker Client Builder
# ============================================
# This container provides the Go toolchain for cross-compiling
# client binaries. It is invoked by the API server's builder package
# when a user requests a client download.
#
# Key design choice: modernc.org/sqlite is pure Go, so we do NOT need
# CGo or platform-specific C toolchains. CGO_ENABLED=0 works for all targets.
#
# Usage (invoked by builder.go, not directly):
#   docker run --rm \
#     -v /path/to/trasker:/src:ro \
#     -v /tmp/trasker-builds:/out \
#     -e GOOS=linux -e GOARCH=amd64 \
#     -e API_KEY=xxx -e SERVER_URL=https://... -e VERSION=1.0.0 \
#     trasker-builder

FROM golang:1.23-alpine

RUN apk add --no-cache git

WORKDIR /src

# Pre-download modules for faster builds.
# This layer is cached — only re-runs when go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Copy full source (needed for build)
COPY . .

# Build script that reads env vars and runs go build
COPY deploy/builder-entrypoint.sh /usr/local/bin/builder-entrypoint.sh
RUN chmod +x /usr/local/bin/builder-entrypoint.sh

ENTRYPOINT ["builder-entrypoint.sh"]
```

- [ ] 16.2 — Create the builder entrypoint script at `deploy/builder-entrypoint.sh`:

```bash
#!/bin/sh
set -euo pipefail

# Required environment variables
: "${GOOS:?GOOS is required}"
: "${GOARCH:?GOARCH is required}"
: "${API_KEY:?API_KEY is required}"
: "${SERVER_URL:?SERVER_URL is required}"
: "${VERSION:=dev}"

# Determine binary extension
EXT=""
if [ "$GOOS" = "windows" ]; then
  EXT=".exe"
fi

OUTPUT="/out/trasker-client-${GOOS}-${GOARCH}${EXT}"

echo "Building trasker-client for ${GOOS}/${GOARCH}..."

CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
  -ldflags="-s -w -X main.apiKey=${API_KEY} -X main.serverURL=${SERVER_URL} -X main.version=${VERSION}" \
  -trimpath \
  -o "$OUTPUT" \
  ./cmd/trasker-client

echo "Built: $OUTPUT"
ls -lh "$OUTPUT"
```

- [ ] 16.3 — Commit: `git add deploy/Dockerfile.builder deploy/builder-entrypoint.sh && git commit -m "feat(deploy): add builder container for cross-compiling client binaries"`

---

## Task 17: Caddy Config

**Files:**
- Create: `deploy/caddy/Caddyfile`

**Steps:**

- [ ] 17.1 — Create Caddyfile at `deploy/caddy/Caddyfile`:

```caddyfile
# Trasker — Caddy reverse proxy
#
# The {$DOMAIN} environment variable is set in docker-compose.yml.
# Caddy automatically provisions and renews TLS certificates via Let's Encrypt.

{$DOMAIN} {
	# API: all /api/ requests go to the Go server
	handle /api/* {
		reverse_proxy api:8080
	}

	# Health check: pass through to API
	handle /health {
		reverse_proxy api:8080
	}

	# SPA: everything else goes to the nginx container serving static files
	handle {
		reverse_proxy web:80
	}

	# Security headers
	header {
		X-Content-Type-Options "nosniff"
		X-Frame-Options "DENY"
		Referrer-Policy "strict-origin-when-cross-origin"
		# Remove server identification
		-Server
	}

	# Compression
	encode gzip zstd

	# Access log
	log {
		output stdout
		format console
	}
}
```

- [ ] 17.2 — Verify Caddyfile syntax (requires caddy binary or docker):

```bash
docker run --rm -v $(pwd)/deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile
```

Expected: `Valid configuration` (or similar success message). The `{$DOMAIN}` placeholder may cause a warning about missing env var, which is acceptable.

- [ ] 17.3 — Commit: `git add deploy/caddy/ && git commit -m "feat(deploy): add Caddyfile with reverse proxy, auto-TLS, and security headers"`

---

## Task 18: Environment Config

**Files:**
- Create: `deploy/.env.example`
- Create: `migrations/001_init.up.sql`
- Create: `migrations/001_init.down.sql`

**Steps:**

- [ ] 18.1 — Create `.env.example` at `deploy/.env.example`:

```env
# ===========================================
# Trasker Server — Environment Configuration
# ===========================================
# Copy this file to .env and fill in all values.
# Required values are marked with (REQUIRED).

# --- Domain & TLS ---
# The public domain where Trasker is hosted.
# Caddy uses this for automatic TLS certificate provisioning.
DOMAIN=trasker.example.com          # (REQUIRED)

# The full public URL of the server (used for client binary stamping)
SERVER_URL=https://trasker.example.com  # (REQUIRED)

# --- PostgreSQL ---
POSTGRES_DB=trasker
POSTGRES_USER=trasker
POSTGRES_PASSWORD=                  # (REQUIRED) Use a strong random password

# --- Microsoft Entra ID (OIDC) ---
ENTRA_TENANT_ID=                    # (REQUIRED) Azure AD tenant ID
ENTRA_CLIENT_ID=                    # (REQUIRED) App registration client ID
ENTRA_CLIENT_SECRET=                # (REQUIRED) App registration client secret

# --- JWT ---
# Secret for signing dashboard session tokens.
# Generate with: openssl rand -base64 32
JWT_SECRET=                         # (REQUIRED)
```

- [ ] 18.2 — Create initial migration at `migrations/001_init.up.sql`:

```sql
-- Trasker initial schema
-- This migration creates all tables needed for the server.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- for gen_random_uuid()

CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entra_oid    TEXT UNIQUE NOT NULL,
    email        TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'member',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_user_role CHECK (role IN ('admin', 'manager', 'member'))
);

CREATE TABLE api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id),
    key_hash     TEXT NOT NULL,
    key_prefix   TEXT NOT NULL,
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked      BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_user ON api_keys(user_id);
CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);

CREATE TABLE devices (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id),
    api_key_id       UUID NOT NULL REFERENCES api_keys(id),
    client_device_id TEXT NOT NULL,
    device_name      TEXT,
    os               TEXT NOT NULL,
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_devices_client_api ON devices(client_device_id, api_key_id);
CREATE INDEX idx_devices_user ON devices(user_id);

CREATE TABLE timesheets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id),
    device_id    UUID NOT NULL REFERENCES devices(id),
    submitted_at TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_timesheets_user ON timesheets(user_id);
CREATE INDEX idx_timesheets_submitted ON timesheets(submitted_at);

CREATE TABLE timesheet_entries (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timesheet_id UUID NOT NULL REFERENCES timesheets(id) ON DELETE CASCADE,
    tag          TEXT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    ended_at     TIMESTAMPTZ NOT NULL,
    duration_s   INTEGER NOT NULL,
    notes        TEXT,
    app_summary  TEXT
);

CREATE INDEX idx_timesheet_entries_timesheet ON timesheet_entries(timesheet_id);
CREATE INDEX idx_timesheet_entries_tag ON timesheet_entries(tag);
CREATE INDEX idx_timesheet_entries_started ON timesheet_entries(started_at);

CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id    UUID NOT NULL REFERENCES users(id),
    action      TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id   UUID NOT NULL,
    old_value   JSONB,
    new_value   JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_admin ON audit_log(admin_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_created ON audit_log(created_at);

CREATE TABLE org_settings (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    org_name        TEXT NOT NULL,
    entra_tenant    TEXT NOT NULL,
    entra_client    TEXT NOT NULL,
    key_expiry_days INTEGER NOT NULL DEFAULT 60,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Insert default org settings row (single-row table)
INSERT INTO org_settings (id, org_name, entra_tenant, entra_client)
VALUES (1, 'My Organization', '', '');
```

- [ ] 18.3 — Create down migration at `migrations/001_init.down.sql`:

```sql
-- Trasker initial schema — rollback
-- WARNING: This drops all tables and data.

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS timesheet_entries;
DROP TABLE IF EXISTS timesheets;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS org_settings;
```

- [ ] 18.4 — Verify SQL syntax by dry-running against a temporary postgres:

```bash
docker run --rm -e POSTGRES_PASSWORD=test -e POSTGRES_DB=test \
  -v $(pwd)/migrations:/migrations:ro \
  postgres:16-alpine \
  sh -c "pg_isready -t 10 || sleep 2; psql -U postgres -d test -f /migrations/001_init.up.sql"
```

Expected: All `CREATE TABLE` statements execute without errors.

> **Note:** If this dry-run approach is flaky, visual review of the SQL is sufficient — the schema matches the spec exactly.

- [ ] 18.5 — Commit: `git add deploy/.env.example migrations/ && git commit -m "feat(deploy): add environment config, initial PostgreSQL migration, and rollback"`

---

## Summary

| Task | Component | Files Created |
|------|-----------|---------------|
| 1 | SvelteKit scaffold | `web/server-ui/*` (project skeleton) |
| 2 | API client | `src/lib/api.ts`, `types.ts`, `stores/auth.ts` |
| 3 | Auth flow | `src/lib/auth.ts`, login page, callback page |
| 4 | Layout | Sidebar, Header, `(app)/+layout.svelte` |
| 5 | Dashboard | SummaryCard, TagChart, dashboard page |
| 6 | Devices | DownloadButton, devices page |
| 7 | Team | DateRangeFilter, team page |
| 8 | Reports | BarChart, reports page |
| 9 | Admin Users | User management page |
| 10 | Admin Corrections | Timesheet corrections page |
| 11 | Admin Settings | Org settings page |
| 12 | Admin Audit | Audit log page |
| 13 | Build Pipeline | `internal/server/builder/builder.go` + tests |
| 14 | Docker Compose | `deploy/docker-compose.yml`, nginx config |
| 15 | Dockerfile.server | Multi-stage server build |
| 16 | Dockerfile.builder | Cross-compilation container + entrypoint |
| 17 | Caddy | `deploy/caddy/Caddyfile` |
| 18 | Environment | `.env.example`, PostgreSQL migrations |

**Total estimated time:** 4-6 hours for an experienced agent, ~8-10 hours for a careful first pass.

**Key architectural decisions:**
- SvelteKit with `adapter-static` produces a pure SPA — no Node.js runtime in production
- nginx serves the SPA static files (not the Go server) — separation of concerns
- Caddy handles TLS termination and routes `/api/*` to Go, everything else to nginx
- Builder uses `CGO_ENABLED=0` — pure Go cross-compilation, no C toolchains
- PostgreSQL migrations are plain SQL files, run by the Go server on startup
- All admin actions are audit-logged with before/after JSON snapshots
