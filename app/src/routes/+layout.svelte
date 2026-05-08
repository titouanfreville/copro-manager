<script lang="ts">
	import '../app.css';
	import { pwaInfo } from 'virtual:pwa-info';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';

	import { authState } from '$lib/auth';
	import { computeBalance, formatBalanceEUR, type Balance } from '$lib/balance';
	import { subscribeExpenses, subscribeFoyers } from '$lib/live';
	import type { Expense, Foyer } from '$lib/api';

	let { children } = $props();

	// ─── Live data for the global balance chip ───
	// Subscribed only when the user is signed in. Unsubscribed on sign-out
	// so we don't keep a Firestore listener open against a missing token.
	let foyers = $state<Foyer[]>([]);
	let expenses = $state<Expense[]>([]);
	let balance = $derived<Balance | null>(computeBalance(expenses, foyers));
	let liveError = $state('');

	let unsubFoyers: (() => void) | null = null;
	let unsubExpenses: (() => void) | null = null;

	// $effect re-runs only when the auth STATUS string flips — not on every
	// re-emit of the store (e.g. token refresh) which would otherwise tear
	// down + recreate listeners on every refresh.
	$effect(() => {
		const status = $authState.status;
		if (status === 'signed-in') {
			liveError = '';
			const onErr = (err: Error) => {
				// Permission-denied (rules misconfigured, signed-out mid-flight)
				// would otherwise leave the chip silently absent. Log so a
				// "balance never shows" report has a breadcrumb.
				console.warn('layout subscription error', err);
				liveError = err.message || String(err);
			};
			if (!unsubFoyers) unsubFoyers = subscribeFoyers((rows) => (foyers = rows), onErr);
			if (!unsubExpenses) unsubExpenses = subscribeExpenses((rows) => (expenses = rows), onErr);
		} else {
			unsubFoyers?.();
			unsubExpenses?.();
			unsubFoyers = null;
			unsubExpenses = null;
			foyers = [];
			expenses = [];
			liveError = '';
		}
		return () => {
			unsubFoyers?.();
			unsubExpenses?.();
			unsubFoyers = null;
			unsubExpenses = null;
		};
	});

	// Hide on:
	//  - /login (no auth → no data)
	//  - / (transient redirect, would flash)
	//  - /expenses (the page already shows a full-size hero balance, the
	//    chip would also collide with that page's user-block on mobile).
	let showChip = $derived(
		$authState.status === 'signed-in' &&
			balance !== null &&
			$page.url.pathname !== '/login' &&
			$page.url.pathname !== '/' &&
			$page.url.pathname !== '/expenses'
	);

	function chipText(b: Balance): string {
		if (b.net === 0) return 'Comptes équilibrés';
		const amount = formatBalanceEUR(b.net);
		// Positive net → 1er owes RDC; negative → RDC owes 1er.
		return b.net > 0
			? `${b.premier.name} doit ${amount}`
			: `${b.rdc.name} doit ${amount}`;
	}

	function chipTitle(b: Balance): string {
		if (b.net === 0) return 'Tout est à jour entre les deux foyers';
		return b.net > 0
			? `${b.premier.name} doit ${formatBalanceEUR(b.net)} à ${b.rdc.name}`
			: `${b.rdc.name} doit ${formatBalanceEUR(b.net)} à ${b.premier.name}`;
	}

	function gotoExpenses() {
		if ($page.url.pathname !== '/expenses') goto('/expenses');
	}

	onMount(async () => {
		if (pwaInfo) {
			const { useRegisterSW } = await import('virtual:pwa-register/svelte');
			useRegisterSW({ immediate: true });
		}
	});
</script>

<svelte:head>
	{@html pwaInfo ? pwaInfo.webManifest.linkTag : ''}
</svelte:head>

{#if showChip && balance}
	<button
		type="button"
		class="balance-chip"
		class:balance-even={balance.net === 0}
		class:balance-rdc-creditor={balance.net > 0}
		class:balance-1er-creditor={balance.net < 0}
		onclick={gotoExpenses}
		title={chipTitle(balance)}
		aria-label={chipTitle(balance)}
	>
		<span class="balance-mark" aria-hidden="true">⌇</span>
		<span class="balance-text">{chipText(balance)}</span>
	</button>
{/if}

{@render children?.()}

<style>
	.balance-chip {
		position: fixed;
		top: 0.85rem;
		right: 0.9rem;
		z-index: 50;
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0.42rem 0.85rem 0.42rem 0.7rem;
		font-family:
			'Hanken Grotesk',
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			system-ui,
			sans-serif;
		font-size: 0.78rem;
		font-weight: 600;
		letter-spacing: 0.01em;
		color: #44403a;
		background: rgba(255, 255, 255, 0.92);
		backdrop-filter: blur(6px);
		-webkit-backdrop-filter: blur(6px);
		border: 1px solid #d8d0c1;
		border-radius: 999px;
		box-shadow:
			0 6px 18px rgba(20, 16, 12, 0.06),
			0 1px 2px rgba(20, 16, 12, 0.04);
		cursor: pointer;
		transition:
			transform 160ms ease,
			box-shadow 160ms ease,
			border-color 160ms ease,
			background 160ms ease;
	}
	.balance-chip:hover {
		transform: translateY(-1px);
		box-shadow:
			0 10px 24px rgba(20, 16, 12, 0.08),
			0 2px 4px rgba(20, 16, 12, 0.05);
		border-color: #c24e2a;
	}
	.balance-chip:focus-visible {
		outline: 2px solid #c24e2a;
		outline-offset: 2px;
	}

	.balance-mark {
		font-family: 'Fraunces', 'Hoefler Text', Georgia, serif;
		font-size: 1rem;
		line-height: 1;
		font-style: italic;
		transform: rotate(-30deg);
		display: inline-block;
		color: #c24e2a;
	}

	/* Color cues without shouting — terracotta accent for either creditor,
	   sage when even. The text already says who owes whom. */
	.balance-even .balance-mark {
		color: #5a7461;
	}
	.balance-even {
		color: #4f6e5c;
	}
	.balance-rdc-creditor .balance-text,
	.balance-1er-creditor .balance-text {
		color: #161310;
	}

	@media (max-width: 480px) {
		.balance-chip {
			top: 0.55rem;
			right: 0.55rem;
			padding: 0.35rem 0.7rem 0.35rem 0.55rem;
			font-size: 0.72rem;
		}
	}
</style>
