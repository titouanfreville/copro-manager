<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, type Category, type DistributionMode } from '$lib/api';
	import { authState } from '$lib/auth';
	import { createCategory, deleteCategory, updateCategory } from '$lib/categories';
	import Button from '$lib/components/Button.svelte';
	import Fab from '$lib/components/Fab.svelte';
	import IconButton from '$lib/components/IconButton.svelte';
	import { subscribeCategories } from '$lib/live';

	let categories = $state<Category[]>([]);
	let liveError = $state('');

	$effect(() => {
		if ($authState.status === 'signed-out') goto('/login');
		if ($authState.status !== 'signed-in') return;
		const unsub = subscribeCategories(
			(rows) => (categories = rows),
			(err) => (liveError = err.message)
		);
		return () => unsub();
	});

	let modalOpen = $state(false);
	let editingId = $state<string | null>(null);
	let editingPredefined = $state(false);
	let isEditing = $derived(editingId !== null);
	let mName = $state('');
	let mMode = $state<DistributionMode | ''>('');
	let mIcon = $state('');
	let mColor = $state('');
	let saving = $state(false);
	let formError = $state('');
	let deletingId = $state<string | null>(null);

	// Curated palette: each chip shows the emoji over its accent so the
	// preview reads as a single object. Free-text icon entry stays
	// available for users who want something off-list.
	const ICON_PRESETS = ['💧', '⚡', '🏛️', '🔧', '🛡️', '🏢', '🔥', '🌳', '🚿', '🚪', '🅿️', '🧹'];
	const COLOR_PRESETS = [
		'#3F6B82', // bleu eau
		'#A37423', // ocre électricité
		'#7A5E87', // mauve taxe
		'#9E6A4D', // terre cuite
		'#5A7461', // vert assurance
		'#4A4744', // ardoise syndic
		'#C24E2A', // accent
		'#7A7268' // neutre
	];

	function resetForm() {
		editingId = null;
		editingPredefined = false;
		mName = '';
		mMode = '';
		mIcon = '';
		mColor = '';
		formError = '';
	}

	function openCreate() {
		resetForm();
		modalOpen = true;
	}

	function openEdit(c: Category) {
		resetForm();
		editingId = c.id;
		editingPredefined = c.predefined;
		mName = c.name;
		mMode = c.default_distribution_mode ?? '';
		mIcon = c.icon ?? '';
		mColor = c.color ?? '';
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
		if (!editingPredefined && mName.trim().length < 2) {
			formError = 'Le nom doit faire au moins 2 caractères.';
			return;
		}
		const trimmedColor = mColor.trim();
		if (trimmedColor && !/^#[0-9a-fA-F]{6}$/.test(trimmedColor)) {
			formError = 'Couleur invalide (attendu : #RRGGBB).';
			return;
		}
		saving = true;
		try {
			if (isEditing && editingId) {
				await updateCategory(editingId, {
					name: editingPredefined ? undefined : mName.trim(),
					default_distribution_mode: (mMode || undefined) as DistributionMode | undefined,
					icon: mIcon.trim(),
					color: trimmedColor.toLowerCase()
				});
			} else {
				await createCategory({
					name: mName.trim(),
					default_distribution_mode: (mMode || undefined) as DistributionMode | undefined,
					icon: mIcon.trim() || undefined,
					color: trimmedColor.toLowerCase() || undefined
				});
			}
			modalOpen = false;
			setTimeout(resetForm, 220);
		} catch (err) {
			formError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			saving = false;
		}
	}

	async function onDelete(c: Category) {
		if (deletingId) return;
		const ok = window.confirm(`Supprimer la catégorie « ${c.name} » ?`);
		if (!ok) return;
		deletingId = c.id;
		try {
			await deleteCategory(c.id);
		} catch (err) {
			liveError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			deletingId = null;
		}
	}

	function modeLabel(m: DistributionMode | '' | undefined): string {
		switch (m) {
			case 'equal':
				return '50/50';
			case 'tantiemes':
				return 'Tantièmes';
			case 'custom':
				return 'Manuel';
			case 'water_3_meters':
				return 'Eau (3 sous-compteurs)';
			default:
				return 'Aucun défaut';
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && modalOpen) closeModal();
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
				<h1 class="hero-title">Catégories</h1>
				<p class="hero-sub">
					{categories.length} catégorie{categories.length > 1 ? 's' : ''} · les catégories
					prédéfinies sont en lecture seule (sauf le mode par défaut).
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

			{#if categories.length === 0}
				<section class="empty">
					<p class="empty-title">Aucune catégorie</p>
					<p class="empty-sub">Crée ta première catégorie personnalisée.</p>
					<Button variant="primary" mark onclick={openCreate}>Nouvelle catégorie</Button>
				</section>
			{:else}
				<section class="cards">
					{#each categories as c (c.id)}
						<article class="card">
							<header class="card-head">
								<span
									class="card-chip"
									aria-hidden="true"
									style="background:{(c.color || '#7A7268') + '22'};color:{c.color ||
										'#44403a'};border-color:{(c.color || '#7A7268') + '55'}"
								>
									{c.icon || c.name.slice(0, 2).toUpperCase()}
								</span>
								<h2 class="card-name">{c.name}</h2>
								{#if c.predefined}
									<span class="card-badge card-badge-pre">prédéfinie</span>
								{:else}
									<span class="card-badge">personnalisée</span>
								{/if}
							</header>
							<p class="card-meta">
								<span class="meta-label">Mode par défaut</span>
								<span>{modeLabel(c.default_distribution_mode)}</span>
							</p>
							<div class="card-actions">
								<IconButton
									icon="edit"
									aria-label="Modifier {c.name}"
									onclick={() => openEdit(c)}
								/>
								{#if !c.predefined}
									<IconButton
										icon="delete"
										variant="danger"
										aria-label="Supprimer {c.name}"
										aria-busy={deletingId === c.id}
										onclick={() => onDelete(c)}
										disabled={deletingId !== null}
									/>
								{/if}
							</div>
						</article>
					{/each}
				</section>
			{/if}
		</main>

		<Fab onclick={openCreate} aria-label="Nouvelle catégorie">Nouvelle catégorie</Fab>

		{#if modalOpen}
			<div
				class="modal-backdrop"
				role="presentation"
				onclick={closeModal}
				onkeydown={(e) => e.key === 'Escape' && closeModal()}
			></div>
			<div class="modal" role="dialog" aria-modal="true" aria-labelledby="cat-modal-title">
				<header class="modal-head">
					<div>
						<p class="modal-eyebrow">{isEditing ? 'Édition' : 'Nouvelle'}</p>
						<h2 id="cat-modal-title">
							{isEditing ? 'Modifier la catégorie' : 'Créer une catégorie'}
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
						<span class="lbl">
							Nom
							{#if editingPredefined}
								<span class="lbl-aside">— en lecture seule</span>
							{/if}
						</span>
						<input
							type="text"
							required={!editingPredefined}
							bind:value={mName}
							readonly={editingPredefined}
							placeholder="Garage, Ascenseur…"
						/>
					</label>

					<label class="field">
						<span class="lbl">Mode de répartition par défaut</span>
						<select bind:value={mMode}>
							<option value="">Aucun (défaut sur 50/50)</option>
							<option value="equal">50/50</option>
							<option value="tantiemes">Tantièmes</option>
							<option value="custom">Manuel</option>
							<option value="water_3_meters">Eau (3 sous-compteurs)</option>
						</select>
					</label>

					<div class="field">
						<span class="lbl">Icône</span>
						<div class="picker-row">
							<input
								type="text"
								class="icon-input"
								maxlength="8"
								bind:value={mIcon}
								placeholder="💧"
								aria-label="Icône (emoji)"
							/>
							<div class="picker-presets" role="listbox" aria-label="Icônes suggérées">
								{#each ICON_PRESETS as preset (preset)}
									<button
										type="button"
										class="preset-chip"
										class:selected={mIcon === preset}
										onclick={() => (mIcon = preset)}
										aria-label="Choisir {preset}"
									>
										{preset}
									</button>
								{/each}
							</div>
						</div>
					</div>

					<div class="field">
						<span class="lbl">Couleur d'accent</span>
						<div class="picker-row">
							<input
								type="text"
								class="color-input"
								maxlength="7"
								bind:value={mColor}
								placeholder="#3F6B82"
								aria-label="Couleur (hex)"
							/>
							<div class="picker-presets" role="listbox" aria-label="Couleurs suggérées">
								{#each COLOR_PRESETS as preset (preset)}
									<button
										type="button"
										class="color-chip"
										class:selected={mColor.toLowerCase() === preset.toLowerCase()}
										style="background:{preset}"
										onclick={() => (mColor = preset)}
										aria-label="Choisir {preset}"
									></button>
								{/each}
							</div>
						</div>
					</div>

					{#if mIcon || mColor || mName.trim()}
						<div class="preview-row">
							<span class="preview-lbl">Aperçu</span>
							<span
								class="preview-chip"
								style="background:{(mColor || '#7A7268') + '22'};color:{mColor ||
									'#44403a'};border-color:{(mColor || '#7A7268') + '55'}"
							>
								<span class="preview-icon">{mIcon || (mName.trim().slice(0, 2).toUpperCase() || '··')}</span>
								<span class="preview-name">{mName.trim() || 'Catégorie'}</span>
							</span>
						</div>
					{/if}

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
		max-width: 28rem;
		margin: 0 0 0.4rem;
	}
	.cards {
		display: grid;
		gap: 0.7rem;
	}
	.card {
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.85rem;
		padding: 0.85rem 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.card-head {
		display: flex;
		align-items: center;
		justify-content: flex-start;
		gap: 0.6rem;
	}
	.card-chip {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 2rem;
		height: 2rem;
		border-radius: 0.55rem;
		border: 1px solid;
		font-size: 1.05rem;
		font-weight: 600;
		font-family: var(--ui);
		flex-shrink: 0;
	}
	.card-head .card-name {
		flex: 1;
	}
	.card-name {
		font-family: var(--display);
		font-weight: 500;
		font-size: 1.15rem;
		margin: 0;
		color: var(--ink);
	}
	.card-badge {
		font-size: 0.62rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.16em;
		color: var(--ink-3);
		background: var(--bg-warm);
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		padding: 0.16rem 0.5rem;
	}
	.card-badge-pre {
		color: var(--accent-deep);
		background: var(--accent-soft);
		border-color: var(--accent);
	}
	.card-meta {
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
		font-size: 0.85rem;
		color: var(--ink);
		margin: 0;
	}
	.meta-label {
		font-size: 0.7rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--ink-4);
		font-weight: 600;
	}
	.card-actions {
		display: flex;
		gap: 0.4rem;
		justify-content: flex-end;
	}

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
		width: min(480px, calc(100vw - 2rem));
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
	.lbl-aside {
		text-transform: none;
		letter-spacing: 0;
		font-size: 0.7rem;
		font-weight: 400;
		color: var(--ink-3);
		font-style: italic;
		margin-left: 0.4rem;
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
	.modal-body input[readonly] {
		background: var(--bg-warm);
		color: var(--ink-3);
	}
	.modal-body input:focus,
	.modal-body select:focus {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-soft);
	}
	.picker-row {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.icon-input,
	.color-input {
		font-family: var(--ui);
		max-width: 8rem;
	}
	.picker-presets {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
	}
	.preset-chip {
		width: 2.1rem;
		height: 2.1rem;
		border-radius: 0.5rem;
		border: 1px solid var(--hairline-2);
		background: var(--surface);
		font-size: 1.1rem;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0;
	}
	.preset-chip.selected {
		border-color: var(--accent);
		box-shadow: 0 0 0 2px var(--accent-soft);
	}
	.color-chip {
		width: 1.8rem;
		height: 1.8rem;
		border-radius: 0.45rem;
		border: 1px solid var(--hairline-2);
		cursor: pointer;
		padding: 0;
	}
	.color-chip.selected {
		border-color: var(--ink);
		box-shadow: 0 0 0 2px var(--accent-soft);
	}
	.preview-row {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding-top: 0.2rem;
	}
	.preview-lbl {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--ink-4);
		font-weight: 600;
	}
	.preview-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0.3rem 0.65rem;
		border-radius: 999px;
		border: 1px solid;
		font-size: 0.85rem;
		font-weight: 500;
	}
	.preview-icon {
		font-size: 1rem;
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
