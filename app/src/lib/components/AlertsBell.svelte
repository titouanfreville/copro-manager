<!--
	Bell with unread badge. Mounted in TopBar. Resolves the current
	user's foyer from the auth UID + foyer member_ids, then subscribes
	to that foyer's alerts via Firestore. The badge counts non-read,
	non-resolved, non-dismissed alerts. Tapping the bell opens an
	inline modal rendering AlertsPanel — the /alerts route still
	exists as a service-worker deep-link fallback.
-->
<script lang="ts">
	import { page } from '$app/stores';
	import { authState } from '$lib/auth';
	import { subscribeAlerts, subscribeAlertsForFoyer, subscribeFoyers } from '$lib/live';
	import type { Alert, Foyer } from '$lib/api';
	import AlertsPanel from './AlertsPanel.svelte';
	import IconButton from './IconButton.svelte';

	let foyers = $state<Foyer[]>([]);
	let foyersLoaded = $state(false);
	let alerts = $state<Alert[]>([]);
	let modalOpen = $state(false);
	let bellEl = $state<HTMLButtonElement | null>(null);
	let modalEl = $state<HTMLDivElement | null>(null);
	let prevPathname = $state<string | null>(null);

	let currentFoyer = $derived.by(() => {
		if ($authState.status !== 'signed-in') return null;
		const uid = $authState.user.uid;
		return foyers.find((f) => f.member_ids.includes(uid)) ?? null;
	});

	// True when foyers have arrived but none lists the signed-in UID in
	// member_ids. Indicates a data-link issue (foyer doc out of sync with
	// Firebase Auth) — surface in the panel so the user knows the bell
	// isn't broken, and fall back to an unfiltered alerts feed so they at
	// least see what's addressed to whichever foyer their data points at.
	let foyerLinkMissing = $derived(
		$authState.status === 'signed-in' && foyersLoaded && currentFoyer === null
	);

	// Badge counts every undismissed alert the user hasn't read. We
	// intentionally do NOT exclude resolved alerts — once the server
	// auto-resolves an entry (e.g. missing-receipt cleared by an
	// attachment), it should still register as "new" until the user
	// has actually opened the panel. Resolved + read filters
	// independently in the panel below.
	let unreadCount = $derived(
		alerts.filter((a) => !a.read_at && !a.dismissed_at).length
	);

	$effect(() => {
		if ($authState.status !== 'signed-in') return;
		foyersLoaded = false;
		// Pass an onError callback so a permission-denied (rules
		// misconfigured, signed-out mid-flight, etc.) doesn't silently
		// hide the bell with no diagnostic — at least leave a console
		// breadcrumb for the next debugger.
		const unsub = subscribeFoyers(
			(rows) => {
				foyers = rows;
				foyersLoaded = true;
			},
			(err) => console.warn('AlertsBell foyers error', err)
		);
		return () => unsub();
	});

	$effect(() => {
		if ($authState.status !== 'signed-in') {
			alerts = [];
			return;
		}
		const foyerID = currentFoyer?.id ?? '';
		if (foyerID) {
			const unsub = subscribeAlertsForFoyer(
				foyerID,
				(rows) => (alerts = rows),
				(err) => console.warn('AlertsBell alerts error', err)
			);
			return () => unsub();
		}
		if (!foyersLoaded) {
			alerts = [];
			return;
		}
		// Fallback: foyers loaded but no foyer claims this UID. Most
		// likely a data-link miss between Firebase Auth and the foyer
		// doc — log the diagnostic and read alerts unfiltered so the
		// user still sees anything the API addressed to a foyer they
		// SHOULD belong to. Firestore rules already gate by auth, so
		// this isn't a leak — just a lookup that's wider than ideal.
		console.warn(
			'AlertsBell: signed-in UID is not in any foyer.member_ids — falling back to unfiltered alerts feed',
			{
				uid: $authState.user.uid,
				foyer_member_ids: foyers.map((f) => ({ id: f.id, member_ids: f.member_ids }))
			}
		);
		const unsub = subscribeAlerts(
			(rows) => (alerts = rows),
			(err) => console.warn('AlertsBell alerts error', err)
		);
		return () => unsub();
	});

	// Close the modal whenever the route ACTUALLY changes — `$page` ticks
	// on intra-page reactivity (search params, store revs) too, so a naive
	// effect would close the modal at mount and on every tick. Track the
	// previous path explicitly and only close on a real navigation.
	let pathname = $derived($page.url.pathname);
	$effect(() => {
		const next = pathname;
		if (prevPathname !== null && prevPathname !== next) {
			modalOpen = false;
		}
		prevPathname = next;
	});

	// Focus management: trap Tab inside the modal while open and restore
	// focus to the bell when closed.
	$effect(() => {
		if (!modalOpen || !modalEl) return;
		const root = modalEl;
		const opener = bellEl;
		// Move focus into the modal on open.
		const first = root.querySelector<HTMLElement>(
			'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
		);
		(first ?? root).focus();
		const onTrap = (e: KeyboardEvent) => {
			if (e.key !== 'Tab') return;
			const focusables = Array.from(
				root.querySelectorAll<HTMLElement>(
					'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
				)
			).filter((el) => !el.hasAttribute('disabled'));
			if (focusables.length === 0) {
				e.preventDefault();
				return;
			}
			const firstEl = focusables[0];
			const lastEl = focusables[focusables.length - 1];
			const active = document.activeElement as HTMLElement | null;
			if (e.shiftKey && active === firstEl) {
				e.preventDefault();
				lastEl.focus();
			} else if (!e.shiftKey && active === lastEl) {
				e.preventDefault();
				firstEl.focus();
			}
		};
		root.addEventListener('keydown', onTrap);
		return () => {
			root.removeEventListener('keydown', onTrap);
			opener?.focus();
		};
	});

	function openModal() {
		modalOpen = true;
	}
	function closeModal() {
		modalOpen = false;
	}
	function onKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && modalOpen) closeModal();
	}
</script>

<svelte:window onkeydown={onKey} />

<button
	type="button"
	class="bell"
	bind:this={bellEl}
	onclick={openModal}
	aria-label={unreadCount > 0
		? unreadCount === 1
			? '1 alerte non lue'
			: `${unreadCount} alertes non lues`
		: 'Aucune alerte non lue'}
	aria-haspopup="dialog"
	aria-expanded={modalOpen}
	title="Alertes"
>
	<span class="bell-glyph" aria-hidden="true">⚐</span>
	{#if unreadCount > 0}
		<span class="bell-badge">{unreadCount > 9 ? '9+' : unreadCount}</span>
	{/if}
</button>

{#if modalOpen}
	<div
		class="modal-backdrop"
		role="presentation"
		onclick={closeModal}
	></div>
	<div
		class="modal"
		role="dialog"
		aria-modal="true"
		aria-label="Alertes"
		tabindex="-1"
		bind:this={modalEl}
	>
		<header class="modal-head">
			<div>
				<p class="modal-eyebrow">Notifications</p>
				<h2 class="modal-title">Alertes</h2>
			</div>
			<IconButton
				icon="close"
				aria-label="Fermer"
				variant="text"
				onclick={closeModal}
			/>
		</header>
		<div class="modal-body">
			{#if foyerLinkMissing}
				<p class="link-warning" role="alert">
					Aucun foyer ne référence ton compte. Affichage de toutes les alertes le temps
					qu'un admin recroise les liens (foyer.member_ids ↔ Firebase UID).
				</p>
			{/if}
			<AlertsPanel {alerts} />
		</div>
	</div>
{/if}

<style>
	.bell {
		position: relative;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 2.75rem;
		height: 2.75rem;
		background: transparent;
		border: 1px solid var(--hairline-2, #d8d0c1);
		border-radius: 999px;
		cursor: pointer;
		color: var(--ink-2, #44403a);
		transition:
			background 160ms,
			border-color 160ms,
			color 160ms;
	}
	.bell:hover {
		background: var(--accent-soft, #f4e2d8);
		color: var(--accent-deep, #8f3a1f);
		border-color: var(--accent, #c24e2a);
	}
	.bell:focus-visible {
		outline: 2px solid var(--accent, #c24e2a);
		outline-offset: 2px;
	}
	.bell-glyph {
		font-family: var(--display, 'Fraunces', Georgia, serif);
		font-size: 1.05rem;
		font-style: italic;
		line-height: 1;
	}
	.bell-badge {
		position: absolute;
		top: -3px;
		right: -3px;
		min-width: 1.05rem;
		height: 1.05rem;
		padding: 0 0.25rem;
		background: var(--accent, #c24e2a);
		color: var(--bg, #faf8f4);
		border-radius: 999px;
		font-size: 0.62rem;
		font-weight: 700;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		font-feature-settings: 'tnum';
	}

	.modal-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(20, 16, 12, 0.32);
		backdrop-filter: blur(4px);
		-webkit-backdrop-filter: blur(4px);
		z-index: 110;
		animation: fade-in var(--dur-base, 200ms) var(--ease-out, ease-out);
	}
	.modal {
		position: fixed;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		z-index: 120;
		width: min(560px, calc(100vw - 2rem));
		max-height: min(80vh, 720px);
		background: var(--surface, #fff);
		border: 1px solid var(--hairline, #e8e2d6);
		border-radius: 1rem;
		box-shadow: 0 24px 48px rgba(20, 16, 12, 0.18);
		display: flex;
		flex-direction: column;
		animation: pop-in var(--dur-base, 200ms) var(--ease-out, ease-out);
	}
	.modal-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		padding: 1rem 1.2rem 0.6rem;
		border-bottom: 1px solid var(--hairline, #e8e2d6);
	}
	.modal-eyebrow {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.18em;
		color: var(--ink-4, #aea69a);
		margin: 0 0 0.25rem;
		font-weight: 600;
	}
	.modal-title {
		font-family: var(--display, 'Fraunces', Georgia, serif);
		font-weight: 400;
		font-size: 1.5rem;
		margin: 0;
	}
	.modal-body {
		overflow-y: auto;
		padding: 1rem 1.2rem 1.2rem;
	}
	.link-warning {
		margin: 0 0 0.9rem;
		padding: 0.7rem 0.85rem;
		background: rgba(194, 78, 42, 0.07);
		border: 1px solid rgba(194, 78, 42, 0.25);
		border-radius: 0.55rem;
		color: var(--accent-deep, #8f3a1f);
		font-size: 0.85rem;
		line-height: 1.4;
	}

	@media (max-width: 560px) {
		.modal {
			top: auto;
			bottom: 0;
			left: 0;
			transform: none;
			width: 100%;
			max-height: 90vh;
			border-radius: 1rem 1rem 0 0;
			animation: slide-up var(--dur-base, 200ms) var(--ease-out, ease-out);
		}
	}

	@keyframes fade-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
	@keyframes pop-in {
		from {
			opacity: 0;
			transform: translate(-50%, -48%) scale(0.97);
		}
		to {
			opacity: 1;
			transform: translate(-50%, -50%) scale(1);
		}
	}
	@keyframes slide-up {
		from {
			transform: translateY(8%);
			opacity: 0;
		}
		to {
			transform: translateY(0);
			opacity: 1;
		}
	}
</style>
