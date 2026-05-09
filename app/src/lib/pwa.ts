// PWA install detection + platform-aware install prompt support.
// Standalone detection drives the install banner (PRD FR63 / Story 1.5):
// when the app runs in browser mode, we surface a dismissible banner
// with platform-appropriate instructions until the user adds it to the
// home screen.

import { browser } from '$app/environment';

export type PwaPlatform = 'ios' | 'android' | 'desktop' | 'unknown';

/**
 * `display-mode: standalone` is true when the app was launched from the
 * home-screen icon (iOS) or installed via beforeinstallprompt (Android /
 * desktop Chrome). iOS Safari also sets `navigator.standalone` for
 * legacy reasons — we honor both.
 */
export function isStandalone(): boolean {
	if (!browser) return false;
	if (window.matchMedia('(display-mode: standalone)').matches) return true;
	const navStandalone = (window.navigator as Navigator & { standalone?: boolean }).standalone;
	return Boolean(navStandalone);
}

/**
 * Crude UA + matchMedia heuristic. Good enough to pick the right copy /
 * affordance — we never branch security-sensitive code on the result.
 */
export function detectPlatform(): PwaPlatform {
	if (!browser) return 'unknown';
	const ua = window.navigator.userAgent;
	const isIPhone = /iPhone|iPad|iPod/.test(ua);
	if (isIPhone) return 'ios';
	const isAndroid = /Android/.test(ua);
	if (isAndroid) return 'android';
	if (window.matchMedia('(pointer: fine)').matches) return 'desktop';
	return 'unknown';
}

// Captured `beforeinstallprompt` event so the user-facing button can
// trigger the native Android Chrome install flow at the moment they tap.
// The browser fires this exactly once per page load when install is
// possible — we hold the deferred event until used or until the user
// dismisses the banner.
type BeforeInstallPromptEvent = Event & {
	prompt: () => Promise<void>;
	userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
};

let deferredPrompt: BeforeInstallPromptEvent | null = null;
let promptListenerInstalled = false;
const listeners = new Set<(available: boolean) => void>();

function ensurePromptListener() {
	if (!browser || promptListenerInstalled) return;
	promptListenerInstalled = true;
	window.addEventListener('beforeinstallprompt', (e) => {
		// Block the browser's automatic mini-infobar so we drive the prompt
		// from our banner instead.
		e.preventDefault();
		deferredPrompt = e as BeforeInstallPromptEvent;
		for (const fn of listeners) fn(true);
	});
	window.addEventListener('appinstalled', () => {
		deferredPrompt = null;
		for (const fn of listeners) fn(false);
	});
}

/**
 * Subscribe to install-availability changes. Returns an unsubscribe.
 */
export function onInstallAvailability(fn: (available: boolean) => void): () => void {
	ensurePromptListener();
	listeners.add(fn);
	// Push current state synchronously so the consumer can render once
	// without waiting for the first event.
	fn(deferredPrompt !== null);
	return () => listeners.delete(fn);
}

/**
 * Fire the captured native install prompt (Android Chrome / desktop
 * Chrome). Returns the user choice. Throws if no prompt is available
 * (caller should fall back to text instructions).
 */
export async function promptInstall(): Promise<'accepted' | 'dismissed'> {
	if (!deferredPrompt) throw new Error('Install prompt unavailable');
	await deferredPrompt.prompt();
	const choice = await deferredPrompt.userChoice;
	deferredPrompt = null;
	for (const fn of listeners) fn(false);
	return choice.outcome;
}

const DISMISS_KEY = 'pwa.bannerDismissed';

export function isBannerDismissed(): boolean {
	if (!browser) return false;
	try {
		return sessionStorage.getItem(DISMISS_KEY) === '1';
	} catch {
		return false;
	}
}

export function dismissBanner() {
	if (!browser) return;
	try {
		sessionStorage.setItem(DISMISS_KEY, '1');
	} catch {
		/* quota / private mode — best-effort */
	}
}
