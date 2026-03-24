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
