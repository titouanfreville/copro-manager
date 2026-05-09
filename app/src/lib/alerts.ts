// Foyer-facing alert helpers. Reads come live from Firestore via
// $lib/live; only mutations go through the API.

import { api } from './api';

export function markAlertRead(id: string): Promise<void> {
	return api<void>(`/alerts/${encodeURIComponent(id)}/read`, { method: 'POST' });
}

export function dismissAlert(id: string): Promise<void> {
	return api<void>(`/alerts/${encodeURIComponent(id)}/dismiss`, { method: 'POST' });
}

export function markAllAlertsRead(): Promise<void> {
	return api<void>('/alerts/mark-all-read', { method: 'POST' });
}
