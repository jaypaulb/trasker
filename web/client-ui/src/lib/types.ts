export interface FocusEvent {
  id: number;
  app_name: string;
  window_title: string;
  started_at: string;
  ended_at?: string;
  duration_s?: number;
  is_idle: boolean;
  tag_name?: string;
  tag_color?: string;
  note_text?: string;
}

export interface Tag {
  id: number;
  name: string;
  color: string;
  created_at: string;
}

export interface Submission {
  id: number;
  server_id?: string;
  submitted_at: string;
  status: string;
  retry_count: number;
}

export interface PomodoroSession {
  id: number;
  started_at: string;
  ended_at?: string;
  work_mins: number;
  break_mins: number;
  status: string;
  tag_id?: number;
}

export interface Config {
  device_id: string;
  server_url: string;
  tracking_on: boolean;
  autostart: boolean;
  presence_intervals: string;
  pomodoro_defaults: string;
}

export interface TagRule {
  id: number;
  tag_id: number;
  tag_name: string;
  app_pattern: string;
  title_pattern?: string;
  priority: number;
  suggested: boolean;
  hit_count: number;
}

export interface TagSummary {
  tag: string;
  color: string;
  totalSeconds: number;
  percentage: number;
}
