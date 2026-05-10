<!--
	Alerts list + push toggle. Used by the bell modal and the /alerts
	route (the service-worker deep-link fallback). Caller owns the
	subscription so we don't duplicate Firestore listeners between the
	bell badge and the panel.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError } from '$lib/api';
	import { dismissAlert, markAlertRead, markAllAlertsRead } from '$lib/alerts';
	import Button from './Button.svelte';
	import { formatDate, formatEUR } from '$lib/format';

	// Whitelist relative paths only. The deep_link comes from a server-side
	// alert payload but is stored in Firestore — defense-in-depth against a
	// future bug that would let an absolute URL slip in.
	function safeDeepLink(raw: string | null | undefined): string | null {
		if (!raw || typeof raw !== 'string') return null;
		if (!raw.startsWith('/')) return null;
		if (raw.startsWith('//') || raw.startsWith('/\\')) return null;
		return raw;
	}
	import {
		getAvailability,
		subscribePush,
		unsubscribePush,
		type PushAvailability
	} from '$lib/push';
	import type { Alert } from '$lib/api';

	let { alerts = [] }: { alerts?: Alert[] } = $props();

	let actionError = $state('');
	let pushState = $state<PushAvailability>({
		supported: false,
		permission: 'unsupported',
		subscribed: false
	});
	let pushBusy = $state(false);

	$effect(() => {
		void getAvailability().then((a) => (pushState = a));
	});

	// Show every alert that the user hasn't dismissed — including
	// auto-resolved ones (e.g. a missing-receipt alert the server marked
	// resolved when an attachment finally landed). The user explicitly
	// asked for "stays in view until I delete it"; the resolved flag is
	// system-level metadata, not a UI signal.
	let activeAlerts = $derived(alerts.filter((a) => !a.dismissed_at));
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
			case 'monthly_meter_reading':
				return 'Relevé de compteur';
			case 'contract_expiring':
				return 'Contrat à renouveler';
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
				return `« ${name} » · ${formatEUR(cents)}`;
			}
			case 'balance_seasonal': {
				const cents = Math.abs(Number(p.net_cents ?? 0));
				return `Solde non équilibré (${formatEUR(cents)}).`;
			}
			case 'monthly_meter_reading': {
				const period = String(p.period ?? '');
				return `Relevé attendu pour ${period}.`;
			}
			case 'contract_expiring': {
				const name = String(p.contract_name ?? '?');
				const endRaw = String(p.end_date ?? '');
				const end = endRaw ? new Date(endRaw) : null;
				const endLabel =
					end && !Number.isNaN(end.getTime()) ? end.toLocaleDateString('fr-FR') : endRaw;
				return `« ${name} » expire le ${endLabel}.`;
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
		const target = safeDeepLink(a.deep_link);
		if (target) goto(target);
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
						<button type="button" class="card-action" onclick={() => onMarkRead(a)}>
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

<style>
	.error-card {
		background: rgba(183, 50, 35, 0.05);
		border: 1px solid rgba(183, 50, 35, 0.2);
		color: var(--danger, #b73223);
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
		background: var(--surface, #fff);
		border: 1px solid var(--hairline, #e8e2d6);
		border-radius: 0.85rem;
		padding: 0.85rem 1rem;
		margin: 0 0 1.4rem;
	}
	.push-toggle-body {
		flex: 1;
		min-width: 0;
	}
	.push-toggle-title {
		font-family: var(--display, 'Fraunces', Georgia, serif);
		font-weight: 500;
		font-size: 1rem;
		margin: 0 0 0.2rem;
	}
	.push-toggle-sub {
		font-size: 0.78rem;
		color: var(--ink-3, #7a7268);
		margin: 0;
	}
	.empty {
		background: var(--surface, #fff);
		border: 1px dashed var(--hairline-2, #d8d0c1);
		border-radius: 1rem;
		padding: 2rem 1.5rem;
		text-align: center;
	}
	.empty-title {
		font-family: var(--display, 'Fraunces', Georgia, serif);
		font-size: 1.2rem;
		margin: 0 0 0.3rem;
	}
	.empty-sub {
		color: var(--ink-3, #7a7268);
		margin: 0;
	}
	.section-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin: 1rem 0 0.5rem;
	}
	.section-title {
		font-family: var(--display, 'Fraunces', Georgia, serif);
		font-weight: 500;
		font-size: 1.1rem;
		margin: 0;
	}
	.section-link {
		background: transparent;
		border: 0;
		color: var(--accent, #c24e2a);
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
		background: var(--surface, #fff);
		border: 1px solid var(--hairline, #e8e2d6);
		border-radius: 0.7rem;
		padding: 0.7rem 0.9rem;
	}
	.card.unread {
		border-color: var(--accent, #c24e2a);
		background: var(--accent-soft, #f4e2d8);
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
		color: var(--accent-deep, #8f3a1f);
		font-weight: 700;
	}
	.card-time {
		font-size: 0.7rem;
		color: var(--ink-4, #aea69a);
	}
	.card-body {
		margin: 0.3rem 0;
		color: var(--ink, #161310);
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
		color: var(--ink-2, #44403a);
		background: transparent;
		border: 1px solid var(--hairline-2, #d8d0c1);
		border-radius: 999px;
		padding: 0.22rem 0.6rem;
		cursor: pointer;
	}
	.card-action:hover {
		background: var(--bg, #faf8f4);
	}
	.card-action-danger:hover {
		color: var(--danger, #b73223);
		border-color: rgba(183, 50, 35, 0.3);
		background: rgba(183, 50, 35, 0.06);
	}
</style>
