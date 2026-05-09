<!--
	Bell with unread badge. Mounted in TopBar. Resolves the current
	user's foyer from the auth UID + foyer member_ids, then subscribes
	to that foyer's alerts via Firestore. The badge counts non-read,
	non-resolved, non-dismissed alerts; tapping the bell navigates to
	/alerts.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { authState } from '$lib/auth';
	import { subscribeAlertsForFoyer, subscribeFoyers } from '$lib/live';
	import type { Alert, Foyer } from '$lib/api';

	let foyers = $state<Foyer[]>([]);
	let alerts = $state<Alert[]>([]);

	let currentFoyer = $derived.by(() => {
		if ($authState.status !== 'signed-in') return null;
		const uid = $authState.user.uid;
		return foyers.find((f) => f.member_ids.includes(uid)) ?? null;
	});

	let unreadCount = $derived(
		alerts.filter((a) => !a.read_at && !a.resolved_at && !a.dismissed_at).length
	);

	$effect(() => {
		if ($authState.status !== 'signed-in') return;
		// Pass an onError callback so a permission-denied (rules
		// misconfigured, signed-out mid-flight, etc.) doesn't silently
		// hide the bell with no diagnostic — at least leave a console
		// breadcrumb for the next debugger.
		const unsub = subscribeFoyers(
			(rows) => (foyers = rows),
			(err) => console.warn('AlertsBell foyers error', err)
		);
		return () => unsub();
	});

	$effect(() => {
		const foyerID = currentFoyer?.id ?? '';
		if (!foyerID) {
			alerts = [];
			return;
		}
		const unsub = subscribeAlertsForFoyer(
			foyerID,
			(rows) => (alerts = rows),
			(err) => console.warn('AlertsBell alerts error', err)
		);
		return () => unsub();
	});

	function open() {
		goto('/alerts');
	}
</script>

<button
	type="button"
	class="bell"
	onclick={open}
	aria-label={unreadCount > 0 ? `${unreadCount} alertes non lues` : 'Aucune alerte non lue'}
	title="Alertes"
>
	<span class="bell-glyph" aria-hidden="true">⚐</span>
	{#if unreadCount > 0}
		<span class="bell-badge">{unreadCount > 9 ? '9+' : unreadCount}</span>
	{/if}
</button>

<style>
	.bell {
		position: relative;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 2.1rem;
		height: 2.1rem;
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
</style>
