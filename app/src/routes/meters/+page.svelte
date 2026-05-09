<script lang="ts">
	import { goto } from '$app/navigation';
	import type { MeterReading } from '$lib/api';
	import { authState } from '$lib/auth';
	import Fab from '$lib/components/Fab.svelte';
	import IconButton from '$lib/components/IconButton.svelte';
	import { subscribeMeters } from '$lib/live';
	import {
		SANITY_CHECK_THRESHOLD,
		computeDeltas,
		driftPct,
		getMeterPhotoDownloadUrl
	} from '$lib/meters';

	let meters = $state<MeterReading[]>([]);
	let liveError = $state('');
	let photoUrls = $state<Record<string, { global?: string; detail?: string }>>({});

	$effect(() => {
		if ($authState.status === 'signed-out') goto('/login');
		if ($authState.status !== 'signed-in') return;
		const unsub = subscribeMeters(
			(rows) => (meters = rows),
			(err) => (liveError = err.message)
		);
		return () => unsub();
	});

	$effect(() => {
		if ($authState.status !== 'signed-in') return;
		// Resolve a thumbnail URL per (period, kind) once per row that has a
		// photo recorded. Cache keys by `period:kind`. Re-fetches on a 9-min
		// timer so URLs don't expire under a long-lived session.
		for (const m of meters) {
			ensurePhoto(m, 'global');
			ensurePhoto(m, 'detail');
		}
	});

	async function ensurePhoto(m: MeterReading, kind: 'global' | 'detail') {
		const has = kind === 'global' ? m.global_photo_object : m.detail_photo_object;
		if (!has) return;
		const cached = photoUrls[m.period]?.[kind];
		if (cached) return;
		try {
			const { url } = await getMeterPhotoDownloadUrl(m.period, kind);
			photoUrls = {
				...photoUrls,
				[m.period]: { ...(photoUrls[m.period] ?? {}), [kind]: url }
			};
		} catch (err) {
			console.warn('photo url fetch failed', m.period, kind, err);
		}
	}

	function periodLabel(period: string): string {
		const [y, mo] = period.split('-');
		const months = [
			'Janvier',
			'Février',
			'Mars',
			'Avril',
			'Mai',
			'Juin',
			'Juillet',
			'Août',
			'Septembre',
			'Octobre',
			'Novembre',
			'Décembre'
		];
		const idx = Number(mo) - 1;
		if (idx < 0 || idx > 11 || !y) return period;
		return `${months[idx]} ${y}`;
	}

	function fmtM3(v: number): string {
		return v.toLocaleString('fr-FR', { maximumFractionDigits: 3 });
	}

	function fmtDelta(v: number | undefined): string {
		if (v === undefined) return '—';
		return `+${fmtM3(v)} m³`;
	}

	function priorOf(currentPeriod: string): MeterReading | null {
		const idx = meters.findIndex((m) => m.period === currentPeriod);
		if (idx === -1) return null;
		// `meters` is sorted desc by Period, so the prior is the next index.
		return meters[idx + 1] ?? null;
	}
</script>

<div class="page">
	{#if $authState.status !== 'signed-in'}
		<main class="main"><p class="muted center">Chargement…</p></main>
	{:else}
		<main class="main">
			<section class="hero">
				<p class="hero-eyebrow">Consommation</p>
				<h1 class="hero-title">Compteurs d'eau</h1>
				<p class="hero-sub">
					{meters.length} lecture{meters.length > 1 ? 's' : ''} · une par mois,
					quatre valeurs (global + 3 sous-compteurs).
				</p>
				<IconButton
					icon="chevron-left"
					href="/expenses"
					variant="text"
					size="sm"
					aria-label="Retour aux dépenses"
				/>
			</section>

			{#if liveError}
				<div class="error-card" role="alert">{liveError}</div>
			{/if}

			{#if meters.length === 0}
				<section class="empty">
					<p class="empty-title">Aucune lecture</p>
					<p class="empty-sub">
						Capture la première lecture pour activer la répartition « eau (3
						sous-compteurs) » dans tes dépenses.
					</p>
					<a class="empty-cta" href="/meters/new">Nouvelle lecture →</a>
				</section>
			{:else}
				<section class="cards">
					{#each meters as m (m.period)}
						{@const prev = priorOf(m.period)}
						{@const deltas = computeDeltas(m, prev)}
						{@const drift = driftPct(deltas)}
						<article class="card">
							<header class="card-head">
								<h2 class="card-period">{periodLabel(m.period)}</h2>
								<span class="card-when">{m.period}</span>
							</header>

							<div class="thumbs">
								{#if photoUrls[m.period]?.global}
									<a
										class="thumb"
										href={photoUrls[m.period]?.global}
										target="_blank"
										rel="noopener"
										aria-label="Photo du compteur global"
									>
										<img src={photoUrls[m.period]?.global} alt="" />
										<span class="thumb-tag">global</span>
									</a>
								{:else if m.global_photo_object}
									<span class="thumb thumb-loading">global</span>
								{/if}
								{#if photoUrls[m.period]?.detail}
									<a
										class="thumb"
										href={photoUrls[m.period]?.detail}
										target="_blank"
										rel="noopener"
										aria-label="Photo des sous-compteurs"
									>
										<img src={photoUrls[m.period]?.detail} alt="" />
										<span class="thumb-tag">détails</span>
									</a>
								{:else if m.detail_photo_object}
									<span class="thumb thumb-loading">détails</span>
								{/if}
							</div>

							<div class="grid">
								<div class="cell">
									<span class="cell-label">Global</span>
									<span class="cell-value">{fmtM3(m.global_m3)} m³</span>
									<span class="cell-delta">{fmtDelta(deltas?.dGlobal)}</span>
								</div>
								<div class="cell">
									<span class="cell-label">Commun</span>
									<span class="cell-value">{fmtM3(m.common_m3)} m³</span>
									<span class="cell-delta">{fmtDelta(deltas?.dCommon)}</span>
								</div>
								<div class="cell">
									<span class="cell-label">RDC</span>
									<span class="cell-value">{fmtM3(m.rdc_m3)} m³</span>
									<span class="cell-delta">{fmtDelta(deltas?.dRDC)}</span>
								</div>
								<div class="cell">
									<span class="cell-label">1er</span>
									<span class="cell-value">{fmtM3(m.premier_m3)} m³</span>
									<span class="cell-delta">{fmtDelta(deltas?.d1er)}</span>
								</div>
							</div>

							{#if drift !== null && drift > SANITY_CHECK_THRESHOLD}
								<p class="drift" role="status">
									⚠️ Écart {(drift * 100).toFixed(1)}% entre Δglobal et Σ détails.
								</p>
							{/if}

							<footer class="card-foot">
								<a class="link" href={`/meters/${m.period}`}>Modifier</a>
							</footer>
						</article>
					{/each}
				</section>
			{/if}
		</main>

		<Fab onclick={() => goto('/meters/new')} aria-label="Nouvelle lecture">
			Nouvelle lecture
		</Fab>
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
		--rdc: #5a7461;
		--clay: #9e6a4d;
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
	.error-card {
		background: rgba(183, 50, 35, 0.05);
		border: 1px solid rgba(183, 50, 35, 0.2);
		color: var(--danger);
		border-radius: 0.6rem;
		padding: 0.8rem 1rem;
		font-size: 0.85rem;
		margin-bottom: 1rem;
	}
	.empty {
		background: var(--surface);
		border: 1px dashed var(--hairline-2);
		border-radius: 1rem;
		padding: 2.5rem 1.5rem;
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.8rem;
	}
	.empty-title {
		font-family: var(--display);
		font-size: 1.3rem;
		margin: 0;
	}
	.empty-sub {
		color: var(--ink-3);
		max-width: 32rem;
		margin: 0 0 0.4rem;
	}
	.empty-cta {
		color: var(--accent);
		font-weight: 600;
		text-decoration: none;
		font-size: 0.95rem;
	}
	.empty-cta:hover {
		color: var(--accent-deep);
	}
	.cards {
		display: grid;
		gap: 0.85rem;
	}
	.card {
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 1rem;
		padding: 1rem 1.1rem;
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}
	.card-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.6rem;
	}
	.card-period {
		font-family: var(--display);
		font-weight: 500;
		font-size: 1.2rem;
		margin: 0;
		color: var(--ink);
	}
	.card-when {
		font-family: var(--ui);
		color: var(--ink-4);
		font-size: 0.78rem;
		letter-spacing: 0.08em;
	}
	.thumbs {
		display: flex;
		gap: 0.5rem;
	}
	.thumb {
		position: relative;
		width: 72px;
		height: 72px;
		border-radius: 0.5rem;
		overflow: hidden;
		background: var(--bg-warm);
		border: 1px solid var(--hairline);
		display: inline-flex;
		align-items: flex-end;
		justify-content: flex-start;
		padding: 0;
	}
	.thumb img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.thumb-tag {
		position: relative;
		font-size: 0.6rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: white;
		background: rgba(0, 0, 0, 0.45);
		padding: 0.1rem 0.4rem;
		border-radius: 999px;
		margin: 0.3rem;
	}
	.thumb-loading {
		font-family: var(--ui);
		font-size: 0.6rem;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--ink-3);
		font-weight: 600;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		text-align: center;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.55rem 0.85rem;
	}
	.cell {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}
	.cell-label {
		font-size: 0.66rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--ink-4);
		font-weight: 600;
	}
	.cell-value {
		font-family: var(--display);
		font-size: 1.05rem;
		color: var(--ink);
	}
	.cell-delta {
		font-size: 0.78rem;
		color: var(--ink-3);
	}
	.drift {
		margin: 0;
		padding: 0.45rem 0.7rem;
		font-size: 0.82rem;
		color: var(--accent-deep);
		background: var(--accent-soft);
		border-radius: 0.5rem;
	}
	.card-foot {
		display: flex;
		justify-content: flex-end;
	}
	.link {
		color: var(--accent);
		text-decoration: none;
		font-weight: 600;
		font-size: 0.86rem;
	}
	.link:hover {
		color: var(--accent-deep);
	}
	.center {
		text-align: center;
	}
	.muted {
		color: var(--ink-3);
	}
	@media (min-width: 540px) {
		.grid {
			grid-template-columns: repeat(4, minmax(0, 1fr));
		}
	}
</style>
