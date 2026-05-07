import { api } from '$lib/api';
import type { PageLoad } from './$types';
import type { LayoutSnapshot, LayoutTimelineEntry, Device } from '$lib/types';

export type LayoutPageData =
  | { mode: 'no-device' }
  | { mode: 'timeline'; deviceId: string; timeline: LayoutTimelineEntry[]; from: string; to: string }
  | { mode: 'permalink'; deviceId: string; t: string; snapshot: LayoutSnapshot | null };

export const load: PageLoad<LayoutPageData> = async ({ url, parent }) => {
  await parent();

  const devices: Device[] = await api.devices.list();
  if (devices.length === 0) {
    return { mode: 'no-device' };
  }
  const deviceId = devices[0].id;

  const tParam = url.searchParams.get('t');
  if (tParam) {
    try {
      const snapshot = await api.layout.at(deviceId, tParam);
      return { mode: 'permalink', deviceId, t: tParam, snapshot };
    } catch (_err) {
      // 404 means "no snapshot at-or-before" — render empty permalink state
      return { mode: 'permalink', deviceId, t: tParam, snapshot: null };
    }
  }

  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const from = today.toISOString();
  const to = new Date().toISOString();
  const timeline = await api.layout.timeline({ device_id: deviceId, from, to });
  return { mode: 'timeline', deviceId, timeline, from, to };
};
