<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { ApiError, type Expense, type MeterReading } from '$lib/api';
	import { authState } from '$lib/auth';
	import Button from '$lib/components/Button.svelte';
	import IconButton from '$lib/components/IconButton.svelte';
	import PhotoCapture from '$lib/components/PhotoCapture.svelte';
	import { subscribeExpenses, subscribeMeters } from '$lib/live';
	import {
		SANITY_CHECK_THRESHOLD,
		attachMeterPhoto,
		computeDeltas,
		deleteMeter,
		deleteMeterPhoto,
		driftPct,
		getMeterPhotoDownloadUrl,
		suggestMeterPhotoValues,
		updateMeter
	} from '$lib/meters';

	let period = $derived($page.params.period ?? '');

	let meters = $state<MeterReading[]>([]);
	let expenses = $state<Expense[]>([]);
	let liveError = $state('');

	$effect(() => {
		if ($authState.status === 'signed-out') goto('/login');
		if ($authState.status !== 'signed-in') return;
		const u1 = subscribeMeters(
			(rows) => (meters = rows),
			(err) => (liveError = err.message)
		);
		const u2 = subscribeExpenses(
			(rows) => (expenses = rows),
			(err) => (liveError = err.message)
		);
		return () => {
			u1();
			u2();
		};
	});

	let current = $derived(meters.find((m) => m.period === period) ?? null);
	let prior = $derived(
		meters.find((m) => m.period < period && m.period !== period) ?? null
	);
	let referencingExpenses = $derived(
		expenses.filter(
			(e) => e.distribution_mode === 'water_3_meters' && e.meter_reading_period === period
		)
	);

	let globalM3 = $state('');
	let commonM3 = $state('');
	let rdcM3 = $state('');
	let premierM3 = $state('');
	let primed = $state(false);

	$effect(() => {
		if (current && !primed) {
			globalM3 = String(current.global_m3);
			commonM3 = String(current.common_m3);
			rdcM3 = String(current.rdc_m3);
			premierM3 = String(current.premier_m3);
			primed = true;
		}
	});

	let saving = $state(false);
	let formError = $state('');
	let deletingMeter = $state(false);

	let ocrBusy = $state<{ global?: boolean; detail?: boolean }>({});
	let ocrInfo = $state('');

	function fmtM3Field(v: number): string {
		return v.toFixed(3);
	}

	const OCR_CONFIDENCE_GATE = 0.5;

	async function autoReadGlobal() {
		if (ocrBusy.global || !current?.global_photo_object) return;
		ocrInfo = '';
		ocrBusy = { ...ocrBusy, global: true };
		try {
			const res = await suggestMeterPhotoValues(period, 'global');
			const c0 = res.confidence?.[0] ?? 0;
			if (res.values.length === 0 || c0 <= 0) {
				ocrInfo = 'Aucun chiffre détecté sur la photo globale.';
			} else if (c0 < OCR_CONFIDENCE_GATE) {
				ocrInfo = `Lecture peu fiable (${res.values[0].toFixed(3)} m³, confiance ${(c0 * 100).toFixed(0)} %) — saisis manuellement.`;
			} else {
				globalM3 = fmtM3Field(res.values[0]);
				ocrInfo = `Photo globale lue : ${globalM3} m³ (vérifie avant d'enregistrer).`;
			}
		} catch (err) {
			ocrInfo = err instanceof ApiError ? `OCR : ${err.message}` : `OCR : ${String(err)}`;
		} finally {
			ocrBusy = { ...ocrBusy, global: false };
		}
	}

	async function autoReadDetail() {
		if (ocrBusy.detail || !current?.detail_photo_object) return;
		ocrInfo = '';
		ocrBusy = { ...ocrBusy, detail: true };
		try {
			const res = await suggestMeterPhotoValues(period, 'detail');
			const conf = res.confidence ?? [];
			const slots: [number, (s: string) => void, string][] = [
				[0, (s) => (commonM3 = s), 'commun'],
				[1, (s) => (rdcM3 = s), 'RDC'],
				[2, (s) => (premierM3 = s), '1er']
			];
			let filled = 0;
			const skipped: string[] = [];
			for (const [i, set, label] of slots) {
				const v = res.values[i];
				const c = conf[i] ?? 0;
				if (v === undefined || c <= 0) continue;
				if (c < OCR_CONFIDENCE_GATE) {
					skipped.push(`${label} (~${v.toFixed(3)} m³)`);
					continue;
				}
				set(fmtM3Field(v));
				filled++;
			}
			if (filled === 0 && skipped.length === 0) {
				ocrInfo = 'Aucun chiffre détecté sur la photo des sous-compteurs.';
			} else {
				const parts: string[] = [];
				if (filled > 0)
					parts.push(`${filled} valeur${filled > 1 ? 's' : ''} pré-remplie${filled > 1 ? 's' : ''}`);
				if (skipped.length > 0)
					parts.push(`peu fiable : ${skipped.join(', ')} — saisis manuellement`);
				ocrInfo =
					parts.join(' · ') +
					'. Ordre attendu : commun (compteur bleu) / RDC (le plus éloigné) / 1er (proche du bleu).';
			}
		} catch (err) {
			ocrInfo = err instanceof ApiError ? `OCR : ${err.message}` : `OCR : ${String(err)}`;
		} finally {
			ocrBusy = { ...ocrBusy, detail: false };
		}
	}

	let priorDeltas = $derived.by(() => {
		const g = parseFloat(globalM3);
		const c = parseFloat(commonM3);
		const r = parseFloat(rdcM3);
		const p1 = parseFloat(premierM3);
		if (!prior || !current || isNaN(g) || isNaN(c) || isNaN(r) || isNaN(p1)) return null;
		return computeDeltas(
			{
				...current,
				global_m3: g,
				common_m3: c,
				rdc_m3: r,
				premier_m3: p1
			},
			prior
		);
	});
	let drift = $derived(driftPct(priorDeltas));

	let photoUrls = $state<{ global?: string; detail?: string }>({});
	$effect(() => {
		if (!current) return;
		if (current.global_photo_object && !photoUrls.global) {
			void getMeterPhotoDownloadUrl(period, 'global').then(
				({ url }) => (photoUrls = { ...photoUrls, global: url })
			).catch(() => {});
		}
		if (current.detail_photo_object && !photoUrls.detail) {
			void getMeterPhotoDownloadUrl(period, 'detail').then(
				({ url }) => (photoUrls = { ...photoUrls, detail: url })
			).catch(() => {});
		}
	});

	async function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (saving) return;
		formError = '';
		const g = parseFloat(globalM3);
		const c = parseFloat(commonM3);
		const r = parseFloat(rdcM3);
		const p1 = parseFloat(premierM3);
		if ([g, c, r, p1].some((v) => isNaN(v) || v < 0)) {
			formError = 'Toutes les valeurs doivent être ≥ 0.';
			return;
		}
		saving = true;
		try {
			await updateMeter(period, {
				period,
				global_m3: g,
				common_m3: c,
				rdc_m3: r,
				premier_m3: p1
			});
			goto('/meters');
		} catch (err) {
			formError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			saving = false;
		}
	}

	async function uploadReplacement(kind: 'global' | 'detail', f: File) {
		try {
			await attachMeterPhoto(period, kind, f);
			photoUrls = { ...photoUrls, [kind]: undefined };
		} catch (err) {
			formError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		}
	}

	async function onReplacePhoto(kind: 'global' | 'detail', e: Event) {
		const input = e.target as HTMLInputElement;
		const f = input.files?.[0];
		if (!f) return;
		await uploadReplacement(kind, f);
		input.value = '';
	}

	async function onDeletePhoto(kind: 'global' | 'detail') {
		if (!confirm(`Supprimer la photo ${kind} ?`)) return;
		try {
			await deleteMeterPhoto(period, kind);
			photoUrls = { ...photoUrls, [kind]: undefined };
		} catch (err) {
			formError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		}
	}

	async function onDeleteMeter() {
		if (deletingMeter) return;
		const ok = confirm(
			`Supprimer la lecture ${period} ? Cette action ne peut pas être annulée.`
		);
		if (!ok) return;
		deletingMeter = true;
		try {
			await deleteMeter(period);
			goto('/meters');
		} catch (err) {
			formError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			deletingMeter = false;
		}
	}
</script>

<div class="page">
	{#if $authState.status !== 'signed-in'}
		<main class="main"><p class="muted center">Chargement…</p></main>
	{:else if !current}
		<main class="main">
			<p class="muted center">Lecture introuvable…</p>
			<p class="center">
				<a class="link" href="/meters">← Retour aux compteurs</a>
			</p>
		</main>
	{:else}
		<main class="main">
			<section class="hero">
				<p class="hero-eyebrow">Édition</p>
				<h1 class="hero-title">Lecture {period}</h1>
				<IconButton
					icon="chevron-left"
					href="/meters"
					variant="text"
					size="sm"
					aria-label="Retour aux compteurs"
				/>
			</section>

			{#if referencingExpenses.length > 0}
				<div class="ref" role="alert">
					⚠️ {referencingExpenses.length} dépense(s) « eau (3 sous-compteurs) »
					utilisent cette période. Les parts déjà calculées ne seront pas recomputées.
				</div>
			{/if}

			{#if liveError}
				<div class="error-card" role="alert">{liveError}</div>
			{/if}

			<form class="form" onsubmit={onSubmit}>
				<fieldset class="block">
					<legend>Compteur global</legend>
					<div class="photo-row">
						{#if photoUrls.global}
							<a
								class="thumb"
								href={photoUrls.global}
								target="_blank"
								rel="noopener"
							>
								<img src={photoUrls.global} alt="Compteur global" />
							</a>
						{:else if current.global_photo_object}
							<div class="thumb thumb-loading">…</div>
						{:else}
							<div class="thumb thumb-empty">aucune</div>
						{/if}
						<div class="photo-actions">
							<label class="file-btn">
								<input
									type="file"
									accept="image/*"
									capture="environment"
									onchange={(e) => onReplacePhoto('global', e)}
								/>
								<span>Remplacer</span>
							</label>
							<PhotoCapture
								filename="compteur-global"
								onCapture={(f) => uploadReplacement('global', f)}
								label="Webcam"
							/>
							{#if current.global_photo_object}
								<button
									type="button"
									class="ocr-btn"
									disabled={ocrBusy.global}
									onclick={autoReadGlobal}
								>
									{ocrBusy.global ? 'Lecture…' : '🔎 Auto-lire'}
								</button>
								<button
									type="button"
									class="link-danger"
									onclick={() => onDeletePhoto('global')}
								>
									Supprimer
								</button>
							{/if}
						</div>
					</div>
					<label class="field">
						<span class="lbl">Index global (m³)</span>
						<input
							type="number"
							inputmode="decimal"
							step="0.001"
							min="0"
							required
							bind:value={globalM3}
						/>
					</label>
				</fieldset>

				<fieldset class="block">
					<legend>Sous-compteurs</legend>
					<div class="photo-row">
						{#if photoUrls.detail}
							<a
								class="thumb"
								href={photoUrls.detail}
								target="_blank"
								rel="noopener"
							>
								<img src={photoUrls.detail} alt="Sous-compteurs" />
							</a>
						{:else if current.detail_photo_object}
							<div class="thumb thumb-loading">…</div>
						{:else}
							<div class="thumb thumb-empty">aucune</div>
						{/if}
						<div class="photo-actions">
							<label class="file-btn">
								<input
									type="file"
									accept="image/*"
									capture="environment"
									onchange={(e) => onReplacePhoto('detail', e)}
								/>
								<span>Remplacer</span>
							</label>
							<PhotoCapture
								filename="sous-compteurs"
								onCapture={(f) => uploadReplacement('detail', f)}
								label="Webcam"
							/>
							{#if current.detail_photo_object}
								<button
									type="button"
									class="ocr-btn"
									disabled={ocrBusy.detail}
									onclick={autoReadDetail}
								>
									{ocrBusy.detail ? 'Lecture…' : '🔎 Auto-lire'}
								</button>
								<button
									type="button"
									class="link-danger"
									onclick={() => onDeletePhoto('detail')}
								>
									Supprimer
								</button>
							{/if}
						</div>
					</div>
					<div class="row">
						<label class="field flex">
							<span class="lbl">Commun (m³)</span>
							<input type="number" inputmode="decimal" step="0.001" min="0" required bind:value={commonM3} />
						</label>
						<label class="field flex">
							<span class="lbl">RDC (m³)</span>
							<input type="number" inputmode="decimal" step="0.001" min="0" required bind:value={rdcM3} />
						</label>
						<label class="field flex">
							<span class="lbl">1er (m³)</span>
							<input type="number" inputmode="decimal" step="0.001" min="0" required bind:value={premierM3} />
						</label>
					</div>
				</fieldset>

				{#if priorDeltas}
					<div class="check" class:warn={drift !== null && drift > SANITY_CHECK_THRESHOLD}>
						<span class="check-label">Cohérence vs. {prior?.period}</span>
						<span>
							Δglobal {priorDeltas.dGlobal.toFixed(3)} m³ · Σ détails {priorDeltas.totalDetail.toFixed(3)} m³
							{#if drift !== null}· écart {(drift * 100).toFixed(1)}%{/if}
						</span>
					</div>
				{/if}

				{#if ocrInfo}
					<p class="ocr-info" role="status">{ocrInfo}</p>
				{/if}
				{#if formError}
					<p class="form-error" role="alert">{formError}</p>
				{/if}

				<div class="actions">
					<Button variant="ghost" onclick={() => goto('/meters')}>Annuler</Button>
					<Button type="submit" variant="primary" mark disabled={saving || deletingMeter}>
						{saving ? 'Enregistrement…' : 'Mettre à jour'}
					</Button>
				</div>
			</form>

			<section class="danger-zone">
				<h2>Zone dangereuse</h2>
				<p class="muted">
					Une lecture référencée par une dépense ne peut pas être supprimée tant que
					les dépenses concernées sont actives.
				</p>
				<Button
					variant="ghost"
					onclick={onDeleteMeter}
					disabled={deletingMeter || referencingExpenses.length > 0}
				>
					{deletingMeter ? 'Suppression…' : 'Supprimer la lecture'}
				</Button>
			</section>
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
		max-width: 640px;
		margin: 0 auto;
		padding: 1.5rem 1.25rem 6rem;
	}
	.hero {
		padding: 1.5rem 0 1rem;
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
		font-size: clamp(1.8rem, 4.5vw, 2.4rem);
		margin: 0 0 0.6rem;
	}
	.ref {
		background: var(--accent-soft);
		border: 1px solid var(--accent);
		color: var(--accent-deep);
		padding: 0.7rem 0.9rem;
		border-radius: 0.6rem;
		font-size: 0.88rem;
		margin-bottom: 1rem;
	}
	.error-card,
	.form-error {
		color: var(--danger);
		background: rgba(183, 50, 35, 0.05);
		border: 1px solid rgba(183, 50, 35, 0.2);
		border-radius: 0.6rem;
		padding: 0.7rem 0.9rem;
		font-size: 0.85rem;
		margin: 0;
	}
	.form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}
	.block {
		border: 1px solid var(--hairline);
		border-radius: 0.85rem;
		padding: 0.9rem 1rem 1rem;
		background: var(--surface);
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}
	.block legend {
		font-family: var(--display);
		font-style: italic;
		color: var(--ink);
		padding: 0 0.4rem;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.field.flex {
		flex: 1 1 0;
		min-width: 0;
	}
	.row {
		display: flex;
		gap: 0.55rem;
	}
	.lbl {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--ink-4);
		font-weight: 600;
	}
	.form input[type='number'] {
		font-family: var(--ui);
		font-size: 0.95rem;
		padding: 0.55rem 0.7rem;
		border: 1px solid var(--hairline-2);
		border-radius: 0.45rem;
		background: var(--surface);
		color: var(--ink);
	}
	.form input:focus {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-soft);
	}
	.photo-row {
		display: flex;
		gap: 0.7rem;
		align-items: center;
	}
	.thumb {
		position: relative;
		width: 80px;
		height: 80px;
		border-radius: 0.5rem;
		overflow: hidden;
		background: var(--bg-warm);
		border: 1px solid var(--hairline);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	.thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.thumb-loading,
	.thumb-empty {
		font-size: 0.7rem;
		color: var(--ink-3);
	}
	.photo-actions {
		display: flex;
		gap: 0.6rem;
		align-items: center;
	}
	.file-btn {
		font-size: 0.85rem;
		color: var(--accent);
		font-weight: 600;
		cursor: pointer;
		border: 1px solid var(--accent);
		border-radius: 999px;
		padding: 0.35rem 0.85rem;
		background: var(--surface);
	}
	.file-btn input {
		display: none;
	}
	.file-btn:hover {
		background: var(--accent-soft);
	}
	.link-danger {
		background: transparent;
		border: 0;
		color: var(--danger);
		text-decoration: underline;
		font-size: 0.82rem;
		cursor: pointer;
	}
	.ocr-btn {
		font-size: 0.82rem;
		color: var(--accent);
		font-weight: 600;
		cursor: pointer;
		border: 1px solid var(--accent);
		border-radius: 999px;
		padding: 0.32rem 0.85rem;
		background: var(--surface);
		font-family: var(--ui);
	}
	.ocr-btn:hover:not(:disabled) {
		background: var(--accent-soft);
	}
	.ocr-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.ocr-info {
		margin: 0;
		padding: 0.55rem 0.85rem;
		font-size: 0.85rem;
		color: var(--accent-deep);
		background: var(--accent-soft);
		border: 1px solid var(--accent);
		border-radius: 0.5rem;
	}
	.check {
		background: var(--bg-warm);
		border: 1px solid var(--hairline);
		border-radius: 0.6rem;
		padding: 0.6rem 0.85rem;
		font-size: 0.85rem;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}
	.check.warn {
		background: var(--accent-soft);
		border-color: var(--accent);
		color: var(--accent-deep);
	}
	.check-label {
		font-size: 0.7rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--ink-4);
		font-weight: 600;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
	.danger-zone {
		margin-top: 2.5rem;
		padding: 1rem 1.1rem;
		border: 1px dashed var(--danger);
		border-radius: 0.85rem;
		background: rgba(183, 50, 35, 0.03);
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		align-items: flex-start;
	}
	.danger-zone h2 {
		font-family: var(--display);
		font-size: 1rem;
		margin: 0;
		color: var(--danger);
	}
	.center {
		text-align: center;
	}
	.muted {
		color: var(--ink-3);
	}
	.link {
		color: var(--accent);
		text-decoration: none;
		font-weight: 600;
	}
</style>
