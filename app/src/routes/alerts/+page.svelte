<script lang="ts">
	import { goto } from '$app/navigation';
	import { authState } from '$lib/auth';
	import AlertsPanel from '$lib/components/AlertsPanel.svelte';
	import { subscribeAlertsForFoyer, subscribeFoyers } from '$lib/live';
	import type { Alert, Foyer } from '$lib/api';

	let foyers = $state<Foyer[]>([]);
	let alerts = $state<Alert[]>([]);
	let liveError = $state('');

	let currentFoyer = $derived.by(() => {
		if ($authState.status !== 'signed-in') return null;
		const uid = $authState.user.uid;
		return foyers.find((f) => f.member_ids.includes(uid)) ?? null;
	});

	$effect(() => {
		if ($authState.status === 'signed-out') goto('/login');
		if ($authState.status !== 'signed-in') return;
		const unsub = subscribeFoyers(
			(rows) => (foyers = rows),
			(err) => (liveError = err.message)
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
			(err) => (liveError = err.message)
		);
		return () => unsub();
	});

	let activeAlerts = $derived(alerts.filter((a) => !a.dismissed_at && !a.resolved_at));
	let unread = $derived(activeAlerts.filter((a) => !a.read_at));
</script>

<div class="page">
	{#if $authState.status !== 'signed-in'}
		<main class="main">
			<p class="muted center">Chargement…</p>
		</main>
	{:else}
		<main class="main">
			<section class="hero">
				<p class="hero-eyebrow">Notifications</p>
				<h1 class="hero-title">Alertes</h1>
				<p class="hero-sub">
					{unread.length} non lue{unread.length > 1 ? 's' : ''} ·
					{activeAlerts.length} active{activeAlerts.length > 1 ? 's' : ''}
				</p>
				<a class="hero-back" href="/expenses">← retour au registre</a>
			</section>

			{#if liveError}
				<div class="error-card" role="alert">{liveError}</div>
			{/if}

			<AlertsPanel {alerts} />
		</main>
	{/if}
</div>

<style>
	:global(html),
	:global(body) {
		background: #faf8f4;
	}
	.page {
		--bg: #faf8f4;
		--bg-warm: #f5f0e6;
		--surface: #ffffff;
		--ink: #161310;
		--ink-2: #44403a;
		--ink-3: #7a7268;
		--ink-4: #aea69a;
		--hairline: #e8e2d6;
		--hairline-2: #d8d0c1;
		--accent: #c24e2a;
		--accent-deep: #8f3a1f;
		--accent-soft: #f4e2d8;
		--danger: #b73223;
		--display: 'Fraunces', 'Hoefler Text', Georgia, serif;
		--ui:
			'Hanken Grotesk', -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
		min-height: 100vh;
		font-family: var(--ui);
		color: var(--ink);
		background: var(--bg);
	}
	.main {
		max-width: 720px;
		margin: 0 auto;
		padding: 1.5rem 1.25rem 6rem;
	}
	.hero {
		padding: 1.5rem 0 1.4rem;
	}
	.hero-eyebrow {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.18em;
		color: var(--ink-4);
		margin: 0 0 0.4rem;
	}
	.hero-title {
		font-family: var(--display);
		font-weight: 400;
		font-size: clamp(2rem, 4.5vw, 2.7rem);
		margin: 0 0 0.6rem;
	}
	.hero-sub {
		color: var(--ink-3);
		font-size: 0.95rem;
		margin: 0 0 0.8rem;
	}
	.hero-back {
		font-size: 0.78rem;
		color: var(--accent);
		text-decoration: none;
	}
	.hero-back:hover {
		text-decoration: underline;
	}
	.error-card {
		background: rgba(183, 50, 35, 0.05);
		border: 1px solid rgba(183, 50, 35, 0.2);
		color: var(--danger);
		border-radius: 0.6rem;
		padding: 0.8rem 1rem;
		font-size: 0.85rem;
		margin-bottom: 1rem;
	}
	.center {
		text-align: center;
	}
	.muted {
		color: var(--ink-3);
	}
</style>
