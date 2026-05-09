<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError } from '$lib/api';
	import { authState } from '$lib/auth';
	import Button from '$lib/components/Button.svelte';
	import Fab from '$lib/components/Fab.svelte';
	import IconButton from '$lib/components/IconButton.svelte';
	import {
		subscribeCategories,
		subscribeFoyers,
		subscribeTemplates
	} from '$lib/live';
	import { createCategory } from '$lib/categories';
	import { createTemplate, deleteTemplate, updateTemplate } from '$lib/templates';
	import type {
		Category,
		CreateTemplateInput,
		DistributionMode,
		ExpenseTemplate,
		Foyer,
		Frequency
	} from '$lib/api';

	// ─── Live data ───────────────────────────────────────────────
	let foyers = $state<Foyer[]>([]);
	let categories = $state<Category[]>([]);
	let templates = $state<ExpenseTemplate[]>([]);
	let liveError = $state('');

	$effect(() => {
		if ($authState.status !== 'signed-in') return;
		const unsubF = subscribeFoyers(
			(rows) => (foyers = rows),
			(err) => (liveError = err.message)
		);
		const unsubC = subscribeCategories(
			(rows) => (categories = rows),
			(err) => (liveError = err.message)
		);
		const unsubT = subscribeTemplates(
			(rows) => (templates = rows),
			(err) => (liveError = err.message)
		);
		return () => {
			unsubF();
			unsubC();
			unsubT();
		};
	});

	$effect(() => {
		if ($authState.status === 'signed-out') {
			goto('/login');
		}
	});

	// ─── Modal / form state ──────────────────────────────────────
	let modalOpen = $state(false);
	let editingId = $state<string | null>(null);
	let isEditing = $derived(editingId !== null);

	let name = $state('');
	let amountDefaultEuros = $state(''); // '' means "à compléter"
	let payerFoyerId = $state('');
	let categoryId = $state('');
	let mode = $state<DistributionMode>('equal');
	let note = $state('');
	let scheduleActive = $state(false);
	let frequency = $state<Frequency>('monthly');
	let dayOfMonth = $state(1);
	let startDate = $state(new Date().toISOString().slice(0, 10));
	let endDate = $state('');
	let saving = $state(false);
	let formError = $state('');

	// Inline category creator (pick "+ Nouvelle catégorie" without leaving the modal).
	let inlineCatCreating = $state(false);
	let inlineCatName = $state('');
	let inlineCatSaving = $state(false);
	let inlineCatError = $state('');
	function openInlineCat() {
		inlineCatCreating = true;
		inlineCatName = '';
		inlineCatError = '';
	}
	function closeInlineCat() {
		inlineCatCreating = false;
		inlineCatName = '';
		inlineCatError = '';
	}
	async function confirmInlineCat() {
		if (inlineCatSaving) return;
		const name = inlineCatName.trim();
		if (name.length < 2) {
			inlineCatError = 'Au moins 2 caractères.';
			return;
		}
		inlineCatSaving = true;
		try {
			const c = await createCategory({ name });
			categoryId = c.id;
			closeInlineCat();
		} catch (err) {
			inlineCatError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			inlineCatSaving = false;
		}
	}
	let deletingId = $state<string | null>(null);

	function eurosToCents(v: string): number {
		const clean = v.trim().replace(',', '.').replace(/\s/g, '');
		if (clean === '') return 0;
		const n = Number(clean);
		if (!Number.isFinite(n)) return NaN;
		return Math.round(n * 100);
	}
	function centsToEuros(c: number): string {
		if (!c) return '';
		return (c / 100).toFixed(2);
	}
	function formatEUR(c: number): string {
		return (c / 100).toLocaleString('fr-FR', { style: 'currency', currency: 'EUR' });
	}

	function frequencyLabel(f?: Frequency): string {
		switch (f) {
			case 'monthly':
				return 'mensuel';
			case 'quarterly':
				return 'trimestriel';
			case 'yearly':
				return 'annuel';
			default:
				return '';
		}
	}

	function modeLabel(m: DistributionMode): string {
		return m === 'equal' ? '50/50' : m === 'tantiemes' ? 'Tantièmes' : 'Manuel';
	}

	function categoryName(id: string): string {
		return categories.find((c) => c.id === id)?.name ?? id;
	}

	function foyerName(id: string): string {
		return foyers.find((f) => f.id === id)?.name ?? id;
	}

	function resetForm() {
		editingId = null;
		name = '';
		amountDefaultEuros = '';
		payerFoyerId = '';
		categoryId = '';
		mode = 'equal';
		note = '';
		scheduleActive = false;
		frequency = 'monthly';
		dayOfMonth = 1;
		startDate = new Date().toISOString().slice(0, 10);
		endDate = '';
		formError = '';
	}

	function openCreate() {
		resetForm();
		modalOpen = true;
	}

	function openEdit(t: ExpenseTemplate) {
		editingId = t.id;
		name = t.name;
		amountDefaultEuros = centsToEuros(t.amount_default_cents);
		payerFoyerId = t.payer_foyer_id;
		categoryId = t.category_id;
		mode = t.distribution_mode;
		note = t.note ?? '';
		scheduleActive = t.schedule_active;
		frequency = t.frequency ?? 'monthly';
		dayOfMonth = t.day_of_month ?? 1;
		startDate = t.next_occurrence_at ? t.next_occurrence_at.slice(0, 10) : new Date().toISOString().slice(0, 10);
		endDate = t.end_date ? t.end_date.slice(0, 10) : '';
		formError = '';
		modalOpen = true;
	}

	function closeModal() {
		if (saving) return;
		modalOpen = false;
		setTimeout(resetForm, 220);
	}

	async function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (saving) return;
		formError = '';
		if (!name.trim()) {
			formError = 'Donne un nom au modèle.';
			return;
		}
		if (!payerFoyerId) {
			formError = 'Choisis le foyer payeur.';
			return;
		}
		if (!categoryId) {
			formError = 'Choisis une catégorie.';
			return;
		}
		const amount = eurosToCents(amountDefaultEuros);
		if (Number.isNaN(amount) || amount < 0) {
			formError = 'Montant invalide (laisse vide pour « à compléter »).';
			return;
		}
		if (scheduleActive) {
			if (dayOfMonth < 1 || dayOfMonth > 31) {
				formError = 'Le jour du mois doit être entre 1 et 31.';
				return;
			}
			if (!startDate) {
				formError = 'La date de début est requise quand la planification est active.';
				return;
			}
			// Day-of-month is the recurrence anchor; StartDate must agree
			// with it so the first fire and subsequent fires land on the
			// same calendar day.
			const startDay = Number(startDate.slice(8, 10));
			if (Number.isFinite(startDay) && startDay !== dayOfMonth) {
				formError = `Le jour du mois (${dayOfMonth}) doit correspondre au jour de la date de début (${startDay}).`;
				return;
			}
			if (endDate && endDate < startDate) {
				formError = 'La date de fin doit être postérieure à la date de début.';
				return;
			}
			// Reject obviously-wrong past start dates on CREATE (allow on
			// edit so the user can fix typos / backfill).
			if (!editingId && startDate < new Date().toISOString().slice(0, 10)) {
				formError = 'La date de début doit être aujourd’hui ou plus tard.';
				return;
			}
		}
		const body: CreateTemplateInput = {
			name: name.trim(),
			amount_default_cents: amount,
			category_id: categoryId,
			payer_foyer_id: payerFoyerId,
			distribution_mode: mode,
			note: note.trim() || undefined,
			schedule_active: scheduleActive
		};
		if (scheduleActive) {
			body.frequency = frequency;
			body.day_of_month = dayOfMonth;
			body.start_date = startDate;
			if (endDate) body.end_date = endDate;
		}
		saving = true;
		try {
			if (editingId) {
				await updateTemplate(editingId, body);
			} else {
				await createTemplate(body);
			}
			modalOpen = false;
			setTimeout(resetForm, 220);
		} catch (err) {
			formError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			saving = false;
		}
	}

	async function onDelete(t: ExpenseTemplate) {
		if (deletingId) return;
		const ok = window.confirm(`Supprimer le modèle « ${t.name} » ?`);
		if (!ok) return;
		deletingId = t.id;
		try {
			await deleteTemplate(t.id);
		} catch (err) {
			liveError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			deletingId = null;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && modalOpen) closeModal();
	}

	const dateFormatter = new Intl.DateTimeFormat('fr-FR', {
		day: '2-digit',
		month: 'long',
		year: 'numeric',
		timeZone: 'UTC'
	});
	function formatDateFR(iso: string): string {
		const datePart = iso.slice(0, 10);
		if (datePart.length < 10) return iso;
		try {
			return dateFormatter.format(new Date(`${datePart}T00:00:00Z`));
		} catch {
			return iso;
		}
	}
</script>

<svelte:window onkeydown={onKeydown} />

<div class="page">
	{#if $authState.status !== 'signed-in'}
		<main class="main">
			<p class="muted center">Chargement…</p>
		</main>
	{:else}
		<main class="main">
			<section class="hero">
				<p class="hero-eyebrow">Bibliothèque</p>
				<h1 class="hero-title">Modèles</h1>
				<p class="hero-sub">
					Pré-réglages de dépenses récurrentes — utilisés à la création d'une nouvelle ligne, et déclenchés
					automatiquement chaque jour à 06:00 (Paris) quand la planification est activée.
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
				<div class="error-card" role="alert">
					<strong>Erreur :</strong> {liveError}
				</div>
			{/if}

			{#if templates.length === 0}
				<section class="empty">
					<p class="empty-title">Aucun modèle</p>
					<p class="empty-sub">
						Crée un modèle pour les factures qui reviennent — eau, électricité, taxe foncière…
					</p>
					<Button variant="primary" mark onclick={openCreate}>Nouveau modèle</Button>
				</section>
			{:else}
				<section class="cards">
					{#each templates as t (t.id)}
						<article class="card">
							<header class="card-head">
								<h2 class="card-name">{t.name}</h2>
								{#if t.schedule_active}
									<span class="card-schedule">{frequencyLabel(t.frequency)} · jour {t.day_of_month ?? '—'}</span>
								{:else}
									<span class="card-schedule card-schedule-manual">manuel</span>
								{/if}
							</header>
							<dl class="card-meta">
								<div>
									<dt>Payeur</dt>
									<dd>
										<span class="foyer-tag foyer-{foyers.find((f) => f.id === t.payer_foyer_id)?.floor ?? 'rdc'}">
											{foyerName(t.payer_foyer_id)}
										</span>
									</dd>
								</div>
								<div>
									<dt>Catégorie</dt>
									<dd>{categoryName(t.category_id)}</dd>
								</div>
								<div>
									<dt>Répartition</dt>
									<dd>{modeLabel(t.distribution_mode)}</dd>
								</div>
								<div>
									<dt>Montant</dt>
									<dd>
										{#if t.amount_default_cents > 0}
											{formatEUR(t.amount_default_cents)}
										{:else}
											<span class="card-pending">à compléter</span>
										{/if}
									</dd>
								</div>
								{#if t.schedule_active && t.next_occurrence_at}
									<div>
										<dt>Prochaine</dt>
										<dd>{formatDateFR(t.next_occurrence_at)}</dd>
									</div>
								{/if}
							</dl>
							{#if t.note}
								<p class="card-note">{t.note}</p>
							{/if}
							<div class="card-actions">
								<IconButton
									icon="edit"
									aria-label="Modifier {t.name}"
									onclick={() => openEdit(t)}
								/>
								<IconButton
									icon="delete"
									variant="danger"
									aria-label="Supprimer {t.name}"
									aria-busy={deletingId === t.id}
									onclick={() => onDelete(t)}
									disabled={deletingId !== null}
								/>
							</div>
						</article>
					{/each}
				</section>
			{/if}
		</main>

		<Fab onclick={openCreate} aria-label="Nouveau modèle">Nouveau modèle</Fab>

		{#if modalOpen}
			<div
				class="modal-backdrop"
				role="presentation"
				onclick={closeModal}
				onkeydown={(e) => e.key === 'Escape' && closeModal()}
			></div>
			<div class="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title">
				<header class="modal-head">
					<div>
						<p class="modal-eyebrow">{isEditing ? 'Édition' : 'Nouveau modèle'}</p>
						<h2 id="modal-title">
							{isEditing ? 'Modifier le modèle' : 'Créer un modèle'}
						</h2>
					</div>
					<IconButton
						icon="close"
						variant="text"
						aria-label="Fermer"
						onclick={closeModal}
					/>
				</header>

				<form class="modal-body" onsubmit={onSubmit}>
					<label class="field">
						<span class="lbl">Nom</span>
						<input
							type="text"
							required
							bind:value={name}
							placeholder="Ex. EDF, SUEZ Eau, Taxe foncière…"
						/>
					</label>

					<div class="grid-2">
						<label class="field">
							<span class="lbl">Foyer payeur</span>
							<select bind:value={payerFoyerId} required>
								<option value="" disabled>—</option>
								{#each foyers as f (f.id)}
									<option value={f.id}>{f.name}</option>
								{/each}
							</select>
						</label>
						<label class="field">
							<span class="lbl">Catégorie</span>
							<select bind:value={categoryId} required>
								<option value="" disabled>—</option>
								{#each categories as c (c.id)}
									<option value={c.id}>{c.name}</option>
								{/each}
							</select>
							{#if inlineCatCreating}
								<div class="inline-cat">
									<input
										type="text"
										bind:value={inlineCatName}
										placeholder="Nom de la catégorie"
										onkeydown={(e) => {
											if (e.key === 'Enter') {
												e.preventDefault();
												confirmInlineCat();
											}
										}}
									/>
									<button type="button" onclick={confirmInlineCat} disabled={inlineCatSaving}>
										{inlineCatSaving ? '…' : 'Créer'}
									</button>
									<button type="button" class="inline-cat-cancel" onclick={closeInlineCat} disabled={inlineCatSaving}>
										Annuler
									</button>
								</div>
								{#if inlineCatError}
									<p class="inline-cat-err" role="alert">{inlineCatError}</p>
								{/if}
							{:else}
								<button type="button" class="inline-cat-trigger" onclick={openInlineCat}>
									+ Nouvelle catégorie
								</button>
							{/if}
						</label>
					</div>

					<fieldset class="field">
						<legend class="lbl">Répartition</legend>
						<div class="mode-row">
							<label class="mode-opt">
								<input type="radio" bind:group={mode} value="equal" />
								<span>50/50</span>
							</label>
							<label class="mode-opt">
								<input type="radio" bind:group={mode} value="tantiemes" />
								<span>Tantièmes</span>
							</label>
							<label class="mode-opt">
								<input type="radio" bind:group={mode} value="custom" />
								<span>Manuel</span>
							</label>
						</div>
						{#if mode === 'custom'}
							<p class="field-hint">
								Le mode manuel demandera de saisir les parts à chaque création depuis ce modèle.
							</p>
						{/if}
					</fieldset>

					<label class="field">
						<span class="lbl">
							Montant par défaut
							<span class="lbl-aside">— vide = « à compléter » à chaque création</span>
						</span>
						<div class="input-suffix">
							<input
								type="text"
								inputmode="decimal"
								bind:value={amountDefaultEuros}
								placeholder="laisser vide"
							/>
							<span class="suffix">€</span>
						</div>
					</label>

					<label class="field">
						<span class="lbl">Note (optionnel)</span>
						<input type="text" bind:value={note} placeholder="Référence, prestataire…" />
					</label>

					<fieldset class="schedule-group">
						<legend class="lbl">Planification</legend>
						<label class="schedule-toggle">
							<input type="checkbox" bind:checked={scheduleActive} />
							<span>Activer la création automatique</span>
						</label>
						{#if scheduleActive}
							<div class="grid-2">
								<label class="field">
									<span class="lbl">Cadence</span>
									<select bind:value={frequency}>
										<option value="monthly">Mensuel</option>
										<option value="quarterly">Trimestriel</option>
										<option value="yearly">Annuel</option>
									</select>
								</label>
								<label class="field">
									<span class="lbl">Jour du mois (1–31)</span>
									<input
										type="number"
										inputmode="numeric"
										min="1"
										max="31"
										bind:value={dayOfMonth}
									/>
									<span class="lbl-aside">
										Pour les mois plus courts (Février surtout), la dépense sera ramenée
										au dernier jour du mois.
									</span>
								</label>
							</div>
							<div class="grid-2">
								<label class="field">
									<span class="lbl">Date de début</span>
									<input type="date" bind:value={startDate} />
								</label>
								<label class="field">
									<span class="lbl">Date de fin (optionnel)</span>
									<input type="date" bind:value={endDate} />
								</label>
							</div>
							<p class="field-hint">
								Une dépense sera créée automatiquement chaque jour à 06:00 (Paris) à partir
								de la date de début. Si le montant par défaut est vide, la ligne est créée
								en attente — un membre du foyer payeur la complétera à réception de la facture.
							</p>
						{/if}
					</fieldset>

					{#if formError}
						<p class="form-error" role="alert">{formError}</p>
					{/if}

					<div class="modal-actions">
						<Button variant="ghost" onclick={closeModal} disabled={saving}>Annuler</Button>
						<Button type="submit" variant="primary" mark disabled={saving}>
							{#if saving}
								{isEditing ? 'Mise à jour…' : 'Création…'}
							{:else}
								{isEditing ? 'Mettre à jour' : 'Créer'}
							{/if}
						</Button>
					</div>
				</form>
			</div>
		{/if}
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
		--rdc-soft: #e6ede5;
		--clay: #9e6a4d;
		--clay-soft: #f1e3d8;
		--danger: #b73223;
		--ok: #4f6e5c;
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
		padding: 1.5rem 0 1.8rem;
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
		max-width: 38rem;
		margin: 0 0 1rem;
	}
	.cards {
		display: grid;
		gap: 0.85rem;
	}
	.card {
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.85rem;
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
	.card-name {
		font-family: var(--display);
		font-weight: 500;
		font-size: 1.15rem;
		margin: 0;
		color: var(--ink);
	}
	.card-schedule {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--accent-deep);
		background: var(--accent-soft);
		border: 1px solid var(--accent);
		border-radius: 999px;
		padding: 0.18rem 0.55rem;
	}
	.card-schedule-manual {
		color: var(--ink-3);
		background: var(--bg-warm);
		border-color: var(--hairline-2);
	}
	.card-meta {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
		gap: 0.5rem 0.9rem;
		margin: 0;
	}
	.card-meta dt {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--ink-4);
		font-weight: 600;
		margin-bottom: 0.1rem;
	}
	.card-meta dd {
		margin: 0;
		font-size: 0.9rem;
		color: var(--ink);
	}
	.card-pending {
		font-style: italic;
		color: var(--accent);
	}
	.card-note {
		font-size: 0.85rem;
		color: var(--ink-3);
		font-style: italic;
		margin: 0;
	}
	.card-actions {
		display: flex;
		gap: 0.4rem;
		justify-content: flex-end;
	}
	.foyer-tag {
		display: inline-block;
		padding: 0.16rem 0.5rem;
		border-radius: 999px;
		font-size: 0.78rem;
		font-weight: 600;
		letter-spacing: 0.005em;
	}
	.foyer-rdc {
		background: var(--rdc-soft);
		color: var(--rdc);
	}
	.foyer-1er {
		background: var(--clay-soft);
		color: var(--clay);
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
		max-width: 28rem;
		margin: 0 0 0.4rem;
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

	/* ─── Modal ─── */
	.modal-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(20, 16, 12, 0.4);
		backdrop-filter: blur(4px);
		z-index: 70;
	}
	.modal {
		position: fixed;
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
		width: min(560px, calc(100vw - 2rem));
		max-height: calc(100vh - 2rem);
		overflow: auto;
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 1.1rem;
		box-shadow: 0 24px 60px rgba(20, 16, 12, 0.2);
		z-index: 80;
		display: flex;
		flex-direction: column;
	}
	.modal-head {
		padding: 1.2rem 1.4rem 0.5rem;
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}
	.modal-eyebrow {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.18em;
		color: var(--ink-4);
		margin: 0 0 0.25rem;
	}
	.modal-head h2 {
		font-family: var(--display);
		font-weight: 400;
		font-size: 1.4rem;
		margin: 0;
	}
	.modal-body {
		padding: 0.6rem 1.4rem 1.2rem;
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.lbl {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--ink-4);
		font-weight: 600;
	}
	.inline-cat-trigger {
		align-self: flex-start;
		font-family: var(--ui);
		font-size: 0.74rem;
		font-weight: 600;
		color: var(--accent);
		background: transparent;
		border: 1px dashed var(--hairline-2);
		border-radius: 999px;
		padding: 0.28rem 0.7rem;
		cursor: pointer;
	}
	.inline-cat-trigger:hover {
		background: var(--accent-soft);
		border-color: var(--accent);
	}
	.inline-cat {
		display: flex;
		gap: 0.4rem;
		flex-wrap: wrap;
		margin-top: 0.3rem;
	}
	.inline-cat input {
		flex: 1;
		min-width: 0;
		font-family: var(--ui);
		font-size: 0.9rem;
		padding: 0.45rem 0.65rem;
		border: 1px solid var(--accent);
		border-radius: 0.45rem;
		background: var(--surface);
		color: var(--ink);
	}
	.inline-cat input:focus {
		outline: none;
		box-shadow: 0 0 0 3px var(--accent-soft);
	}
	.inline-cat button {
		font-family: var(--ui);
		font-size: 0.78rem;
		font-weight: 600;
		padding: 0.4rem 0.85rem;
		border-radius: 999px;
		cursor: pointer;
		border: 0;
		color: var(--bg);
		background: var(--ink);
	}
	.inline-cat button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.inline-cat .inline-cat-cancel {
		background: transparent;
		color: var(--ink-2);
		border: 1px solid var(--hairline-2);
	}
	.inline-cat-err {
		color: var(--danger);
		font-size: 0.78rem;
		margin: 0.2rem 0 0;
	}
	.lbl-aside {
		text-transform: none;
		letter-spacing: 0;
		font-size: 0.7rem;
		font-weight: 400;
		color: var(--ink-3);
		font-style: italic;
		margin-left: 0.4rem;
	}
	.field-hint {
		font-size: 0.78rem;
		color: var(--ink-3);
		font-style: italic;
		margin: 0;
	}
	.modal-body input,
	.modal-body select {
		font-family: var(--ui);
		font-size: 0.95rem;
		padding: 0.55rem 0.7rem;
		border: 1px solid var(--hairline-2);
		border-radius: 0.45rem;
		background: var(--surface);
		color: var(--ink);
	}
	.modal-body input:focus,
	.modal-body select:focus {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-soft);
	}
	.grid-2 {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.7rem;
	}
	.input-suffix {
		display: flex;
		align-items: stretch;
		border: 1px solid var(--hairline-2);
		border-radius: 0.45rem;
		background: var(--surface);
		overflow: hidden;
	}
	.input-suffix:focus-within {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-soft);
	}
	.input-suffix input {
		border: 0;
		flex: 1;
		min-width: 0;
		font-family: var(--ui);
	}
	.input-suffix input:focus {
		box-shadow: none;
	}
	.input-suffix .suffix {
		padding: 0 0.7rem;
		display: flex;
		align-items: center;
		font-size: 0.9rem;
		color: var(--ink-3);
		background: var(--bg-warm);
	}
	.mode-row {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.mode-opt {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		padding: 0.32rem 0.7rem;
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		cursor: pointer;
		font-size: 0.85rem;
	}
	.mode-opt input {
		accent-color: var(--accent);
	}
	.schedule-group {
		border: 1px dashed var(--hairline-2);
		border-radius: 0.7rem;
		padding: 0.85rem 1rem;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		background: var(--bg);
	}
	.schedule-group legend {
		padding: 0 0.4rem;
	}
	.schedule-toggle {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		font-size: 0.9rem;
		color: var(--ink-2);
		cursor: pointer;
	}
	.schedule-toggle input[type='checkbox'] {
		width: 1rem;
		height: 1rem;
		accent-color: var(--accent);
		margin: 0;
	}
	.form-error {
		color: var(--danger);
		font-size: 0.85rem;
		margin: 0;
	}
	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		padding-top: 0.4rem;
	}
	.center {
		text-align: center;
	}
	.muted {
		color: var(--ink-3);
	}
</style>
