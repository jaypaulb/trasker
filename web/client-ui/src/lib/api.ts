const BASE = '';  // Same origin — Go serves both SPA and API

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) throw new Error(`GET ${path}: ${res.status}`);
  return res.json();
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`POST ${path}: ${res.status}`);
  return res.json();
}

async function patch<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`PATCH ${path}: ${res.status}`);
  return res.json();
}

import type { FocusEvent, Tag, Submission, PomodoroSession, Config } from './types';

export const api = {
  getEvents: (date?: string) =>
    get<FocusEvent[]>(`/api/events${date ? `?date=${date}` : ''}`),

  getTags: () => get<Tag[]>('/api/tags'),

  createTag: (name: string, color: string) =>
    post<Tag>('/api/tags', { name, color }),

  tagEvent: (eventId: number, tagId: number) =>
    post<{ status: string }>(`/api/events/${eventId}/tag`, { tag_id: tagId }),

  addNote: (eventId: number, text: string) =>
    post<{ id: number }>(`/api/events/${eventId}/note`, { text }),

  getSubmissions: () => get<Submission[]>('/api/submissions'),

  submit: (eventIds: number[]) =>
    post<{ submission_id: number; status: string }>('/api/submit', { event_ids: eventIds }),

  getPomodoro: () => get<PomodoroSession[]>('/api/pomodoro'),

  startPomodoro: (workMins: number, breakMins: number, tagId?: number) =>
    post<{ id: number; status: string }>('/api/pomodoro/start', {
      work_mins: workMins,
      break_mins: breakMins,
      tag_id: tagId,
    }),

  getConfig: () => get<Config>('/api/config'),

  updateConfig: (updates: Partial<Config>) =>
    patch<{ status: string }>('/api/config', updates),
};
