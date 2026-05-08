import { get } from 'svelte/store';
import { tokens, clearAuth } from '$lib/stores/auth';
import type {
  User, Device, Timesheet, TimesheetEntry,
  ReportSummary, AuditLogEntry, OrgSettings, ApiKey, BuildStatus, AuthTokens,
  LayoutSnapshot, LayoutTimelineEntry
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
    /** Local admin login with email + password */
    localLogin(email: string, password: string): Promise<{ tokens: AuthTokens; user: User & { force_password_change?: boolean } }> {
      return request('POST', '/auth/local-login', { email, password }, { skipAuth: true });
    },
    /** Get server auth configuration */
    config(): Promise<{ oidc_enabled: boolean; local_enabled: boolean }> {
      return request('GET', '/auth/config', undefined, { skipAuth: true });
    },
    /** Change password (requires auth). Returns the updated user with
     *  force_password_change cleared, so callers can refresh the local store. */
    changePassword(current_password: string, new_password: string): Promise<{ status: string; user: User }> {
      return request('POST', '/auth/change-password', { current_password, new_password });
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

  // Layout snapshots (Phase 7)
  layout: {
    timeline(params: { device_id: string; from: string; to: string }): Promise<LayoutTimelineEntry[]> {
      const q = new URLSearchParams(params).toString();
      return request('GET', `/layout-snapshots/timeline?${q}`);
    },
    at(device_id: string, t: string): Promise<LayoutSnapshot> {
      return request('GET', `/layout-snapshots?device_id=${device_id}&t=${encodeURIComponent(t)}`);
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
