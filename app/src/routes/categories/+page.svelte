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
	let saving = $state(false);
	let formError = $state('');
	let deletingId = $state<string | null>(null);

	function resetForm() {
		editingId = null;
		editingPredefined = false;
		mName = '';
		mMode = '';
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
		saving = true;
		try {
			if (isEditing && editingId) {
				await updateCategory(editingId, {
					name: editingPredefined ? undefined : mName.trim(),
					default_distribution_mode: (mMode || undefined) as DistributionMode | undefined
				});
			} else {
				await createCategory({
					name: mName.trim(),
					default_distribution_mode: (mMode || undefined) as DistributionMode | undefined
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
