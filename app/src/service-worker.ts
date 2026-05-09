/// <reference lib="webworker" />

// Custom service worker for copro-manager.
//
// Two responsibilities beyond Workbox's precache manifest:
//
//  1. `push` handler — required by iOS Safari + Chrome's `userVisibleOnly`
//     contract: every push must surface a visible notification, otherwise
//     the browser revokes the subscription after a few silent pushes.
//  2. `notificationclick` handler — opens (or focuses) the deep-link the
//     server attached to the alert, so a tap on the toast lands the user
//     on the right entity (FR59).
//
// Workbox replaces `self.__WB_MANIFEST` with the precache list at build
// time when the `injectManifest` strategy is used.

import { cleanupOutdatedCaches, precacheAndRoute } from 'workbox-precaching';
import { NetworkFirst } from 'workbox-strategies';
import { registerRoute } from 'workbox-routing';

declare const self: ServiceWorkerGlobalScope;

interface PushPayload {
	title?: string;
	body?: string;
	deep_link?: string;
	alert_id?: string;
}

cleanupOutdatedCaches();
precacheAndRoute(self.__WB_MANIFEST);

// GCS signed URLs are short-lived (≤1h per NFR12); falling back to a
// stale cache would return a 403, not a useful response. NetworkFirst
// keeps us snappy when online, useful when offline for the brief
// signed-URL window.
registerRoute(
	({ url }) => url.origin === 'https://storage.googleapis.com',
	new NetworkFirst({
		cacheName: 'gcs-documents',
		networkTimeoutSeconds: 5
	})
);

self.addEventListener('push', (event: PushEvent) => {
	let payload: PushPayload = {};
	if (event.data) {
		try {
			payload = event.data.json() as PushPayload;
		} catch {
			payload = { body: event.data.text() };
		}
	}
	const title = payload.title ?? 'Copro';
	const body = payload.body ?? '';
	const deepLink = payload.deep_link ?? '/alerts';

	event.waitUntil(
		self.registration.showNotification(title, {
			body,
			icon: '/icon-192.png',
			badge: '/icon-192.png',
			// Tag dedupes back-to-back pushes about the same entity so the
			// user doesn't see ten stacked toasts.
			tag: payload.alert_id ?? deepLink,
			data: { deep_link: deepLink, alert_id: payload.alert_id }
		})
	);
});

self.addEventListener('notificationclick', (event: NotificationEvent) => {
	event.notification.close();
	const data = (event.notification.data ?? {}) as { deep_link?: string };
	const deepLink = data.deep_link ?? '/alerts';

	event.waitUntil(
		(async () => {
			const allClients = await self.clients.matchAll({
				type: 'window',
				includeUncontrolled: true
			});
			// If a window is already open on our origin, focus it and
			// navigate it to the deep link rather than opening a new tab.
			for (const client of allClients) {
				const url = new URL(client.url);
				if (url.origin === self.location.origin) {
					await client.focus();
					if ('navigate' in client && typeof client.navigate === 'function') {
						await client.navigate(deepLink);
					}
					return;
				}
			}
			await self.clients.openWindow(deepLink);
		})()
	);
});

// Skip-waiting wired to autoUpdate from useRegisterSW — the page tells
// us when to take over.
self.addEventListener('message', (event: ExtendableMessageEvent) => {
	if (event.data && (event.data as { type?: string }).type === 'SKIP_WAITING') {
		self.skipWaiting();
	}
});
