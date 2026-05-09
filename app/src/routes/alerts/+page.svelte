<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError } from '$lib/api';
	import { dismissAlert, markAlertRead, markAllAlertsRead } from '$lib/alerts';
	import { authState } from '$lib/auth';
	import Button from '$lib/components/Button.svelte';
	import { formatDate } from '$lib/format';
	import { subscribeAlertsForFoyer, subscribeFoyers } from '$lib/live';
	import {
		getAvailability,
		subscribePush,
		unsubscribePush,
		type PushAvailability
	} from '$lib/push';
	import type { Alert, Foyer } from '$lib/api';

	let foyers = $state<Foyer[]>([]);
	let alerts = $state<Alert[]>([]);
	let liveError = $state('');
	let actionError = $state('');

	let pushState = $state<PushAvailability>({
		supported: false,
		permission: 'unsupported',
		subscribed: false
	});
	let pushBusy = $state(false);

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

	$effect(() => {
		if ($authState.status !== 'signed-in') return;
		void getAvailability().then((a) => (pushState = a));
	});

	let activeAlerts = $derived(alerts.filter((a) => !a.dismissed_at && !a.resolved_at));
	let unread = $derived(activeAlerts.filter((a) => !a.read_at));
	let read = $derived(activeAlerts.filter((a) => a.read_at));

	function kindLabel(k: Alert['kind']): string {
		switch (k) {
			case 'pending_completion':
				return 'Dépense à compléter';
			case 'missing_receipt':
				return 'Justificatif manquant';
			case 'peer_expense_added':
				return 'Nouvelle dépense';
			case 'balance_seasonal':
				return 'Solde à équilibrer';
		}
	}

	function relativeTime(iso: string): string {
		if (!iso) return '';
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '';
		const diffMs = Date.now() - d.getTime();
		const days = Math.floor(diffMs / 86400000);
		if (days === 0) return "aujourd'hui";
		if (days === 1) return 'hier';
		if (days < 7) return `il y a ${days} j`;
		return formatDate(iso);
	}

	function bodyOf(a: Alert): string {
		const p = a.payload ?? {};
		switch (a.kind) {
			case 'pending_completion':
				return `« ${String(p.expense_name ?? '?')} » attend son montant.`;
			case 'missing_receipt': {
				const stage = String(p.stage ?? '');
				const name = String(p.expense_name ?? '?');
				return `« ${name} » est sans justificatif (${stage}).`;
			}
			case 'peer_expense_added': {
				const name = String(p.expense_name ?? '?');
				const cents = Number(p.amount_cents ?? 0);
				return `« ${name} » · ${(cents / 100).toFixed(2)} €`;
			}
			case 'balance_seasonal': {
				const cents = Math.abs(Number(p.net_cents ?? 0));
				return `Solde non équilibré (${(cents / 100).toFixed(2)} €).`;
			}
		}
	}

	async function onView(a: Alert) {
		if (!a.read_at) {
			try {
				await markAlertRead(a.id);
			} catch {
				/* best-effort */
			}
		}
		if (a.deep_link) goto(a.deep_link);
	}

	async function onMarkRead(a: Alert) {
		try {
			await markAlertRead(a.id);
		} catch (err) {
			actionError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		}
	}

	async function onDismiss(a: Alert) {
		try {
			await dismissAlert(a.id);
		} catch (err) {
			actionError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		}
	}

	async function onMarkAllRead() {
		try {
			await markAllAlertsRead();
		} catch (err) {
			actionError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		}
	}

	async function onTogglePush() {
		if (pushBusy) return;
		pushBusy = true;
		actionError = '';
		try {
			if (pushState.subscribed) {
				await unsubscribePush();
			} else {
				await subscribePush();
			}
			pushState = await getAvailability();
		} catch (err) {
			actionError = err instanceof Error ? err.message : String(err);
		} finally {
			pushBusy = false;
		}
	}
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
			{#if actionError}
				<div class="error-card" role="alert">{actionError}</div>
			{/if}

			<section class="push-toggle" aria-label="Notifications push">
				<div class="push-toggle-body">
					<p class="push-toggle-title">Notifications push</p>
					<p class="push-toggle-sub">
						{#if !pushState.supported}
							Non supportées par ce navigateur. Le flux ci-dessous reste actif.
						{:else if pushState.subscribed}
							Activées sur cet appareil.
						{:else if pushState.permission === 'denied'}
							Permission refusée — réinitialise dans les réglages du navigateur.
						{:else}
							Reçois les alertes même quand l'app est fermée.
						{/if}
					</p>
				</div>
				{#if pushState.supported && pushState.permission !== 'denied'}
					<Button
						variant={pushState.subscribed ? 'ghost' : 'primary'}
						onclick={onTogglePush}
						disabled={pushBusy}
					>
						{pushBusy ? '…' : pushState.subscribed ? 'Désactiver' : 'Activer'}
					</Button>
				{/if}
			</section>

			{#if activeAlerts.length === 0}
				<section class="empty">
					<p class="empty-title">Tout est sous contrôle.</p>
					<p class="empty-sub">Aucune alerte active.</p>
				</section>
			{:else}
				{#if unread.length > 0}
					<header class="section-head">
						<h2 class="section-title">Non lues</h2>
						<button type="button" class="section-link" onclick={onMarkAllRead}>
							Tout marquer comme lu
						</button>
					</header>
					<ul class="cards">
						{#each unread as a (a.id)}
							<li class="card unread">
								<header class="card-head">
									<span class="card-kind">{kindLabel(a.kind)}</span>
									<span class="card-time">{relativeTime(a.fired_at)}</span>
								</header>
								<p class="card-body">{bodyOf(a)}</p>
								<div class="card-actions">
									{#if a.deep_link}
										<button type="button" class="card-action" onclick={() => onView(a)}>
											Voir
										</button>
									{/if}
									<button
										type="button"
										class="card-action"
										onclick={() => onMarkRead(a)}
									>
										Lu
									</button>
									<button
										type="button"
										class="card-action card-action-danger"
										onclick={() => onDismiss(a)}
									>
										Ignorer
									</button>
								</div>
							</li>
						{/each}
					</ul>
				{/if}

				{#if read.length > 0}
					<header class="section-head">
						<h2 class="section-title">Lues</h2>
					</header>
					<ul class="cards">
						{#each read as a (a.id)}
							<li class="card">
								<header class="card-head">
									<span class="card-kind">{kindLabel(a.kind)}</span>
									<span class="card-time">{relativeTime(a.fired_at)}</span>
								</header>
								<p class="card-body">{bodyOf(a)}</p>
								<div class="card-actions">
									{#if a.deep_link}
										<button type="button" class="card-action" onclick={() => onView(a)}>
											Voir
										</button>
									{/if}
									<button
										type="button"
										class="card-action card-action-danger"
										onclick={() => onDismiss(a)}
									>
										Ignorer
									</button>
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			{/if}
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
	.push-toggle {
		display: flex;
		gap: 1rem;
		align-items: center;
		justify-content: space-between;
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.85rem;
		padding: 0.85rem 1rem;
		margin: 0 0 1.4rem;
	}
	.push-toggle-body {
		flex: 1;
		min-width: 0;
	}
	.push-toggle-title {
		font-family: var(--display);
		font-weight: 500;
		font-size: 1rem;
		margin: 0 0 0.2rem;
	}
	.push-toggle-sub {
		font-size: 0.78rem;
		color: var(--ink-3);
		margin: 0;
	}
	.empty {
		background: var(--surface);
		border: 1px dashed var(--hairline-2);
		border-radius: 1rem;
		padding: 2rem 1.5rem;
		text-align: center;
	}
	.empty-title {
		font-family: var(--display);
		font-size: 1.2rem;
		margin: 0 0 0.3rem;
	}
	.empty-sub {
		color: var(--ink-3);
		margin: 0;
	}
	.section-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin: 1rem 0 0.5rem;
	}
	.section-title {
		font-family: var(--display);
		font-weight: 500;
		font-size: 1.1rem;
		margin: 0;
	}
	.section-link {
		background: transparent;
		border: 0;
		color: var(--accent);
		font-size: 0.78rem;
		cursor: pointer;
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.cards {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.card {
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.7rem;
		padding: 0.7rem 0.9rem;
	}
	.card.unread {
		border-color: var(--accent);
		background: var(--accent-soft);
	}
	.card-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.6rem;
	}
	.card-kind {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--accent-deep);
		font-weight: 700;
	}
	.card-time {
		font-size: 0.7rem;
		color: var(--ink-4);
	}
	.card-body {
		margin: 0.3rem 0;
		color: var(--ink);
		font-size: 0.92rem;
	}
	.card-actions {
		display: flex;
		gap: 0.35rem;
		justify-content: flex-end;
	}
	.card-action {
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--ink-2);
		background: transparent;
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		padding: 0.22rem 0.6rem;
		cursor: pointer;
	}
	.card-action:hover {
		background: var(--bg);
	}
	.card-action-danger:hover {
		color: var(--danger);
		border-color: rgba(183, 50, 35, 0.3);
		background: rgba(183, 50, 35, 0.06);
	}
	.center {
		text-align: center;
	}
	.muted {
		color: var(--ink-3);
	}
</style>
