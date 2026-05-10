<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { ApiError, type MeterReading } from '$lib/api';
	import { authState } from '$lib/auth';
	import Button from '$lib/components/Button.svelte';
	import IconButton from '$lib/components/IconButton.svelte';
	import PhotoCapture from '$lib/components/PhotoCapture.svelte';
	import { subscribeMeters } from '$lib/live';
	import {
		SANITY_CHECK_THRESHOLD,
		attachMeterPhoto,
		computeDeltas,
		createMeter,
		driftPct,
		suggestRawMeterPhotoValues
	} from '$lib/meters';

	let meters = $state<MeterReading[]>([]);
	let liveError = $state('');

	$effect(() => {
		if ($authState.status === 'signed-out') goto('/login');
		if ($authState.status !== 'signed-in') return;
		const unsub = subscribeMeters(
			(rows) => (meters = rows),
			(err) => (liveError = err.message)
		);
		return () => unsub();
	});

	function defaultPeriod(): string {
		const url = new URL(window.location.href);
		const fromQs = url.searchParams.get('period');
		if (fromQs && /^\d{4}-(0[1-9]|1[0-2])$/.test(fromQs)) return fromQs;
		const d = new Date();
		const y = d.getFullYear();
		const mo = String(d.getMonth() + 1).padStart(2, '0');
		return `${y}-${mo}`;
	}

	let period = $state('');
	let qsPrimed = $state(false);
	$effect(() => {
		if (!period && typeof window !== 'undefined') period = defaultPeriod();
	});
	// Read ?period=YYYY-MM ONCE at mount. After the user touches the
	// field the URL no longer dictates the value — earlier behavior
	// snapped the input back on every $page tick.
	$effect(() => {
		if (qsPrimed) return;
		const fromQs = $page.url.searchParams.get('period');
		if (fromQs && /^\d{4}-(0[1-9]|1[0-2])$/.test(fromQs)) period = fromQs;
		qsPrimed = true;
	});

	let globalM3 = $state('');
	let commonM3 = $state('');
	let rdcM3 = $state('');
	let premierM3 = $state('');

	let globalFile = $state<File | null>(null);
	let detailFile = $state<File | null>(null);

	let saving = $state(false);
	let formError = $state('');

	let prior = $derived(
		meters.find((m) => m.period < period && m.period !== period) ?? null
	);
	let priorDeltas = $derived.by(() => {
		const g = parseFloat(globalM3);
		const c = parseFloat(commonM3);
		const r = parseFloat(rdcM3);
		const p1 = parseFloat(premierM3);
		if (!prior || isNaN(g) || isNaN(c) || isNaN(r) || isNaN(p1)) return null;
		return computeDeltas(
			{
				...prior,
				period,
				global_m3: g,
				common_m3: c,
				rdc_m3: r,
				premier_m3: p1
			},
			prior
		);
	});
	let drift = $derived(driftPct(priorDeltas));

	function setFile(kind: 'global' | 'detail', e: Event) {
		const input = e.target as HTMLInputElement;
		const f = input.files?.[0] ?? null;
		if (kind === 'global') globalFile = f;
		else detailFile = f;
	}

	let ocrBusy = $state<{ global?: boolean; detail?: boolean }>({});
	let ocrInfo = $state('');

	function fmtM3Field(v: number): string {
		// Match the form input precision (3 decimals = 1 L resolution).
		// String form so the type=number input accepts it cleanly.
		return v.toFixed(3);
	}

	// Confidence threshold below which OCR results are NOT auto-pasted
	// into the form. The user still sees the value in the info banner so
	// they can type it manually after eyeballing the photo. A 0 from the
	// server signals "no detection" — leave the field as-is.
	const OCR_CONFIDENCE_GATE = 0.5;

	async function autoReadGlobal() {
		if (!globalFile || ocrBusy.global) return;
		ocrInfo = '';
		ocrBusy = { ...ocrBusy, global: true };
		try {
			const res = await suggestRawMeterPhotoValues('global', globalFile);
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
		if (!detailFile || ocrBusy.detail) return;
		ocrInfo = '';
		ocrBusy = { ...ocrBusy, detail: true };
		try {
			const res = await suggestRawMeterPhotoValues('detail', detailFile);
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

	async function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (saving) return;
		formError = '';
		const g = parseFloat(globalM3);
		const c = parseFloat(commonM3);
		const r = parseFloat(rdcM3);
		const p1 = parseFloat(premierM3);
		if (!/^\d{4}-(0[1-9]|1[0-2])$/.test(period)) {
			formError = 'Période invalide (format YYYY-MM).';
			return;
		}
		if ([g, c, r, p1].some((v) => isNaN(v) || v < 0)) {
			formError = 'Toutes les valeurs doivent être ≥ 0.';
			return;
		}
		if (!detailFile) {
			formError = 'La photo des sous-compteurs est requise (preuve des deltas).';
			return;
		}
		saving = true;
		try {
			await createMeter({
				period,
				global_m3: g,
				common_m3: c,
				rdc_m3: r,
				premier_m3: p1
			});
			// Fire both photo uploads in parallel — independent operations,
			// no need to serialize. The metadata write already succeeded;
			// surface partial failures so the user knows to retry from the
			// edit page rather than silently landing on an incomplete row.
			const uploads: Promise<unknown>[] = [];
			const labels: string[] = [];
			if (globalFile) {
				uploads.push(attachMeterPhoto(period, 'global', globalFile));
				labels.push('global');
			}
			if (detailFile) {
				uploads.push(attachMeterPhoto(period, 'detail', detailFile));
				labels.push('détails');
			}
			const results = await Promise.allSettled(uploads);
			const failed = results
				.map((r, i) => (r.status === 'rejected' ? labels[i] : null))
				.filter((v): v is string => v !== null);
			if (failed.length > 0) {
				formError = `Lecture créée, mais l'envoi de la photo (${failed.join(', ')}) a échoué — réessaie depuis l'édition.`;
				saving = false;
				return;
			}
			goto('/meters');
		} catch (err) {
			formError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			saving = false;
		}
	}
</script>

<div class="page">
	{#if $authState.status !== 'signed-in'}
		<main class="main"><p class="muted center">Chargement…</p></main>
	{:else}
		<main class="main">
			<section class="hero">
				<p class="hero-eyebrow">Capture</p>
				<h1 class="hero-title">Nouvelle lecture</h1>
				<p class="hero-sub">
					Photographie le compteur principal puis le panneau des trois sous-compteurs,
					et saisis les valeurs en m³ (3 décimales = précision au litre).
				</p>
				<IconButton
					icon="chevron-left"
					href="/meters"
					variant="text"
					size="sm"
					aria-label="Retour aux compteurs"
				/>
			</section>

			{#if liveError}
				<div class="error-card" role="alert">{liveError}</div>
			{/if}

			<form class="form" onsubmit={onSubmit}>
				<label class="field">
					<span class="lbl">Période (YYYY-MM)</span>
					<input
						type="month"
						required
						bind:value={period}
						placeholder="2026-05"
					/>
					{#if prior}
						<span class="hint">Période précédente : {prior.period}</span>
					{:else}
						<span class="hint">Première lecture — pas de delta calculable.</span>
					{/if}
				</label>

				<fieldset class="block">
					<legend>Compteur global</legend>
					<label class="field">
						<span class="lbl">Photo du compteur principal</span>
						<input
							type="file"
							accept="image/*"
							onchange={(e) => setFile('global', e)}
						/>
						<div class="capture-row">
							<PhotoCapture
								filename="compteur-global"
								onCapture={(f) => (globalFile = f)}
								label="Webcam"
							/>
						</div>
						{#if globalFile}
							<span class="hint">{globalFile.name}</span>
						{/if}
					</label>
					<div class="ocr-row">
						<button
							type="button"
							class="ocr-btn"
							disabled={!globalFile || ocrBusy.global}
							onclick={autoReadGlobal}
						>
							{ocrBusy.global ? 'Lecture…' : '🔎 Auto-lire la photo'}
						</button>
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
							placeholder="1234.567"
						/>
					</label>
				</fieldset>

				<fieldset class="block">
					<legend>Sous-compteurs</legend>
					<label class="field">
						<span class="lbl">Photo des trois sous-compteurs</span>
						<input
							type="file"
							accept="image/*"
							onchange={(e) => setFile('detail', e)}
						/>
						<div class="capture-row">
							<PhotoCapture
								filename="sous-compteurs"
								onCapture={(f) => (detailFile = f)}
								label="Webcam"
							/>
						</div>
						{#if detailFile}
							<span class="hint">{detailFile.name}</span>
						{/if}
					</label>
					<div class="ocr-row">
						<button
							type="button"
							class="ocr-btn"
							disabled={!detailFile || ocrBusy.detail}
							onclick={autoReadDetail}
						>
							{ocrBusy.detail ? 'Lecture…' : '🔎 Auto-lire les trois valeurs'}
						</button>
					</div>
					<div class="row">
						<label class="field flex">
							<span class="lbl">Commun (m³)</span>
							<input
								type="number"
								inputmode="decimal"
								step="0.001"
								min="0"
								required
								bind:value={commonM3}
							/>
						</label>
						<label class="field flex">
							<span class="lbl">RDC (m³)</span>
							<input
								type="number"
								inputmode="decimal"
								step="0.001"
								min="0"
								required
								bind:value={rdcM3}
							/>
						</label>
						<label class="field flex">
							<span class="lbl">1er (m³)</span>
							<input
								type="number"
								inputmode="decimal"
								step="0.001"
								min="0"
								required
								bind:value={premierM3}
							/>
						</label>
					</div>
				</fieldset>

				{#if priorDeltas}
					<div class="check" class:warn={drift !== null && drift > SANITY_CHECK_THRESHOLD}>
						<span class="check-label">Cohérence vs. {prior?.period}</span>
						<span>
							Δglobal {priorDeltas.dGlobal.toFixed(3)} m³ · Σ détails {priorDeltas.totalDetail.toFixed(3)} m³
							{#if drift !== null}
								· écart {(drift * 100).toFixed(1)}%
							{/if}
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
					<Button type="submit" variant="primary" mark disabled={saving}>
						{saving ? 'Enregistrement…' : 'Enregistrer'}
					</Button>
				</div>
			</form>
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
	.hero-sub {
		color: var(--ink-3);
		font-size: 0.95rem;
		margin: 0 0 0.8rem;
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
	.hint {
		color: var(--ink-3);
		font-size: 0.8rem;
	}
	.capture-row {
		display: flex;
		gap: 0.5rem;
	}
	.form input[type='number'],
	.form input[type='month'],
	.form input[type='file'] {
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
	.ocr-row {
		display: flex;
		justify-content: flex-start;
	}
	.ocr-btn {
		font-size: 0.85rem;
		color: var(--accent);
		font-weight: 600;
		cursor: pointer;
		border: 1px solid var(--accent);
		border-radius: 999px;
		padding: 0.4rem 0.95rem;
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
	.center {
		text-align: center;
	}
	.muted {
		color: var(--ink-3);
	}
</style>
