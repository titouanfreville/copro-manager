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

import { cleanupOutdatedCaches, precacheAndRoute } from "workbox-precaching";
import { CacheableResponsePlugin } from "workbox-cacheable-response";
import { NetworkFirst } from "workbox-strategies";
import { registerRoute } from "workbox-routing";

declare const self: ServiceWorkerGlobalScope;

interface PushPayload {
  title?: string;
  body?: string;
  deep_link?: string;
  alert_id?: string;
}

// Whitelist relative app paths only. A malicious push payload could
// otherwise navigate the user to an attacker-controlled origin via
// clients.openWindow / client.navigate.
function safeDeepLink(raw: string | undefined): string {
  if (!raw || typeof raw !== "string") return "/alerts";
  if (!raw.startsWith("/")) return "/alerts";
  // Block protocol-relative (//evil.example) and backslash variants.
  if (raw.startsWith("//") || raw.startsWith("/\\")) return "/alerts";
  return raw;
}

cleanupOutdatedCaches();
precacheAndRoute(self.__WB_MANIFEST);

// GCS signed URLs are short-lived (≤1h per NFR12); falling back to a
// stale cache would return a 403, not a useful response. NetworkFirst
// keeps us snappy when online, useful when offline for the brief
// signed-URL window. Only 200s are cacheable — a 403 from an expired
// signed URL must not be served back from cache as if valid.
registerRoute(
  ({ url }) => url.origin === "https://storage.googleapis.com",
  new NetworkFirst({
    cacheName: "gcs-documents",
    networkTimeoutSeconds: 5,
    plugins: [new CacheableResponsePlugin({ statuses: [200] })],
  }),
);

self.addEventListener("push", (event: PushEvent) => {
  let payload: PushPayload = {};
  if (event.data) {
    try {
      payload = event.data.json() as PushPayload;
    } catch {
      // Inner text() can also throw on non-UTF-8 binary payloads — wrap
      // separately so we never escape the listener with an unhandled
      // rejection (Chrome drops the subscription after a few of those).
      try {
        payload = { body: event.data.text() };
      } catch {
        payload = {};
      }
    }
  }
  const title = payload.title ?? "Copro";
  const body = payload.body ?? "";
  const deepLink = safeDeepLink(payload.deep_link);

  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      icon: "/icon-192.png",
      badge: "/icon-192.png",
      // Tag dedupes back-to-back pushes about the same entity so the
      // user doesn't see ten stacked toasts.
      tag: payload.alert_id ?? deepLink,
      data: { deep_link: deepLink, alert_id: payload.alert_id },
    }),
  );
});

self.addEventListener("notificationclick", (event: NotificationEvent) => {
  event.notification.close();
  const data = (event.notification.data ?? {}) as { deep_link?: string };
  const deepLink = safeDeepLink(data.deep_link);

  event.waitUntil(
    (async () => {
      const allClients = await self.clients.matchAll({
        type: "window",
        includeUncontrolled: true,
      });
      // If a window is already open on our origin, focus it and
      // navigate it to the deep link rather than opening a new tab.
      for (const client of allClients) {
        const url = new URL(client.url);
        if (url.origin === self.location.origin) {
          await client.focus();
          if ("navigate" in client && typeof client.navigate === "function") {
            await client.navigate(deepLink);
          }
          return;
        }
      }
      await self.clients.openWindow(deepLink);
    })(),
  );
});

// Skip-waiting wired to autoUpdate from useRegisterSW — the page tells
// us when to take over.
self.addEventListener("message", (event: ExtendableMessageEvent) => {
  if (event.data && (event.data as { type?: string }).type === "SKIP_WAITING") {
    self.skipWaiting();
  }
});
