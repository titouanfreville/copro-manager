// Web Push subscription wrangling. The SvelteKit vite-pwa plugin
// already registers the service worker; this module wires the
// subscribe/unsubscribe dance and the API round-trip that hands the
// resulting endpoint + keys to the Go server for fan-out.
//
// iOS Safari (≥ 16.4) only supports Push when the PWA is installed to
// home screen — `display-mode: standalone` is the gate. The InstallBanner
// (Story 1.5) covers that prerequisite; the consumer of this module
// should fall back gracefully when permission isn't grantable.

import { browser } from "$app/environment";
import { env } from "$env/dynamic/public";

import { api } from "./api";

// Read at runtime via $env/dynamic/public so the build doesn't error
// when the deployer hasn't generated VAPID keys yet — the in-app feed
// works regardless; only push fan-out is gated on this.
const VAPID_PUBLIC_KEY = env.PUBLIC_VAPID_PUBLIC_KEY ?? "";

export type PushPermission = "default" | "granted" | "denied" | "unsupported";

export interface PushAvailability {
  supported: boolean;
  permission: PushPermission;
  subscribed: boolean;
}

export async function getAvailability(): Promise<PushAvailability> {
  if (!browser)
    return { supported: false, permission: "unsupported", subscribed: false };
  if (
    typeof Notification === "undefined" ||
    !("serviceWorker" in navigator) ||
    !("PushManager" in window)
  ) {
    return { supported: false, permission: "unsupported", subscribed: false };
  }
  const reg = await navigator.serviceWorker.getRegistration();
  const sub = reg ? await reg.pushManager.getSubscription() : null;
  return {
    supported: true,
    permission: Notification.permission as PushPermission,
    subscribed: sub !== null,
  };
}

/**
 * Request notification permission, subscribe via the SW push manager,
 * and POST the result to the API. Throws on any failure so the caller
 * can show a friendly error.
 *
 * Rolls back the local PushManager subscription when the server-side
 * record fails — otherwise IndexedDB and the API would drift apart and
 * the next call to subscribePush() throws an `InvalidStateError`
 * because PushManager already has a subscription registered locally.
 */
export async function subscribePush(): Promise<void> {
  if (!browser) throw new Error("push: SSR");
  if (!VAPID_PUBLIC_KEY) {
    throw new Error("Clé VAPID publique non configurée.");
  }
  if (!("serviceWorker" in navigator) || !("PushManager" in window)) {
    throw new Error("Notifications non supportées par ce navigateur.");
  }
  const reg = await navigator.serviceWorker.ready;
  const permission = await Notification.requestPermission();
  if (permission !== "granted") {
    throw new Error("Permission refusée.");
  }
  const vapidBytes = urlBase64ToUint8Array(VAPID_PUBLIC_KEY);
  // If a stale subscription already exists with a DIFFERENT VAPID key
  // (rotated server-side), pushManager.subscribe throws InvalidStateError.
  // Detect and unsubscribe before re-subscribing so the user isn't
  // stranded with an opaque error after a key rotation.
  const existing = await reg.pushManager.getSubscription();
  if (existing) {
    const existingKey = existing.options?.applicationServerKey;
    const matches = existingKey
      ? compareKeys(existingKey, vapidBytes)
      : false;
    if (!matches) {
      await existing.unsubscribe().catch(() => {});
    }
  }
  const sub = await reg.pushManager.subscribe({
    userVisibleOnly: true,
    // Cast through BufferSource — TS lib.dom narrows applicationServerKey
    // to `BufferSource | string` in some versions; runtime accepts the
    // underlying ArrayBuffer.
    applicationServerKey: vapidBytes.buffer as ArrayBuffer,
  });
  const json = sub.toJSON();
  const endpoint = json.endpoint;
  const p256dh = json.keys?.p256dh;
  const auth = json.keys?.auth;
  if (!endpoint || !p256dh || !auth) {
    // Local subscribe succeeded but we can't ship the keys; roll back
    // so the next attempt isn't blocked by an existing PushManager sub.
    await sub.unsubscribe().catch(() => {});
    throw new Error("Échec de la souscription : clés manquantes.");
  }
  try {
    await api<void>("/push/subscribe", {
      method: "POST",
      body: {
        endpoint,
        keys: { p256dh, auth },
        user_agent: navigator.userAgent,
      },
    });
  } catch (err) {
    // Server refused (validation, network, 5xx). Drop the local
    // subscription so retry isn't poisoned by a stale registration.
    await sub.unsubscribe().catch(() => {});
    throw err;
  }
}

export async function unsubscribePush(): Promise<void> {
  if (!browser) return;
  const reg = await navigator.serviceWorker.getRegistration();
  const sub = reg ? await reg.pushManager.getSubscription() : null;
  if (!sub) return;
  const endpoint = sub.endpoint;
  try {
    await sub.unsubscribe();
  } catch {
    // Best-effort — even if the local unsubscribe fails, drop the
    // server-side row so push fan-out stops sending us nothing.
  }
  await api<void>("/push/unsubscribe", {
    method: "POST",
    body: { endpoint },
  }).catch(() => {
    /* server may already not have it; idempotent */
  });
}

// Byte-equality check between two key buffers. Used to detect a VAPID
// rotation; if the existing subscription was made against a different
// key, we must drop it before resubscribing or PushManager throws.
function compareKeys(a: ArrayBuffer | BufferSource, b: Uint8Array): boolean {
  const aBuf =
    a instanceof ArrayBuffer
      ? new Uint8Array(a)
      : new Uint8Array(
          (a as ArrayBufferView).buffer,
          (a as ArrayBufferView).byteOffset,
          (a as ArrayBufferView).byteLength,
        );
  if (aBuf.length !== b.length) return false;
  for (let i = 0; i < aBuf.length; i++) if (aBuf[i] !== b[i]) return false;
  return true;
}

// VAPID public keys arrive as URL-safe base64. The push manager wants a
// raw Uint8Array of bytes — standard helper, lifted from the MDN docs.
function urlBase64ToUint8Array(base64: string): Uint8Array {
  const padding = "=".repeat((4 - (base64.length % 4)) % 4);
  const b64 = (base64 + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(b64);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}
