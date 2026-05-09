<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError } from '$lib/api';
	import { authState } from '$lib/auth';
	import { createCategory } from '$lib/categories';
	import Button from '$lib/components/Button.svelte';
	import Fab from '$lib/components/Fab.svelte';
	import IconButton from '$lib/components/IconButton.svelte';
	import {
		deleteDocument,
		getDocumentDownloadUrl,
		isImageDocument,
		updateDocument,
		uploadDocument
	} from '$lib/documents';
	import {
		subscribeAllAttachments,
		subscribeCategories,
		subscribeDocuments,
		subscribeExpenses,
		subscribeFoyers,
		type ExpenseAttachment
	} from '$lib/live';
	import type { Category, Document, Expense, Foyer } from '$lib/api';

	// ─── Live data ─────────────────────────────────────────────
	let foyers = $state<Foyer[]>([]);
	let categories = $state<Category[]>([]);
	let documents = $state<Document[]>([]);
	let attachments = $state<ExpenseAttachment[]>([]);
	let expenses = $state<Expense[]>([]);
	let liveError = $state('');

	$effect(() => {
		if ($authState.status === 'signed-out') goto('/login');
		if ($authState.status !== 'signed-in') return;
		const onErr = (err: Error) => (liveError = err.message);
		const unsubs = [
			subscribeFoyers((rows) => (foyers = rows), onErr),
			subscribeCategories((rows) => (categories = rows), onErr),
			subscribeDocuments((rows) => (documents = rows), onErr),
			subscribeAllAttachments((rows) => (attachments = rows), onErr),
			subscribeExpenses((rows) => (expenses = rows), onErr)
		];
		return () => unsubs.forEach((u) => u());
	});

	// ─── Unified ArchiveDoc ────────────────────────────────────
	// Merges standalone documents (`kind: 'standalone'`) with per-expense
	// attachments (`kind: 'attachment'`). One row shape, two storage paths.
	type ArchiveDoc = {
		kind: 'standalone' | 'attachment';
		id: string;
		category_id: string;
		group: string; // empty = "sans groupe"
		title: string;
		description: string;
		size_bytes: number;
		content_type: string;
		uploaded_at: string;
		linked_expense_id?: string;
		linked_expense_name?: string;
	};

	let archive = $derived.by(() => {
		const out: ArchiveDoc[] = [];
		for (const d of documents) {
			out.push({
				kind: 'standalone',
				id: d.id,
				category_id: d.category_id,
				group: (d.group ?? '').trim(),
				title: d.title,
				description: d.description ?? '',
				size_bytes: d.size_bytes,
				content_type: d.content_type,
				uploaded_at: d.uploaded_at,
				linked_expense_id: d.linked_expense_id
			});
		}
		for (const a of attachments) {
			const exp = expenses.find((e) => e.id === a.expense_id);
			out.push({
				kind: 'attachment',
				id: `att:${a.expense_id}:${a.id}`,
				category_id: exp?.category_id ?? '',
				group: '', // attachments have no group; they get bucketed into "Sans groupe"
				title: a.original_filename || '(pièce jointe sans nom)',
				description: '',
				size_bytes: a.size_bytes,
				content_type: a.content_type,
				uploaded_at: a.uploaded_at,
				linked_expense_id: a.expense_id,
				linked_expense_name: exp?.name
			});
		}
		return out;
	});

	// ─── Filters + search ──────────────────────────────────────
	let linkageFilter = $state<'all' | 'standalone' | 'linked'>('all');
	let categoryFilter = $state<string>(''); // empty = all
	let searchQuery = $state('');
	let debouncedQuery = $state('');
	$effect(() => {
		const q = searchQuery;
		const t = setTimeout(() => (debouncedQuery = q), 250);
		return () => clearTimeout(t);
	});

	let filteredArchive = $derived.by(() => {
		const q = debouncedQuery.trim().toLocaleLowerCase('fr');
		return archive.filter((d) => {
			if (linkageFilter === 'standalone' && d.kind !== 'standalone') return false;
			if (linkageFilter === 'linked' && d.kind !== 'attachment' && !d.linked_expense_id) return false;
			if (categoryFilter && d.category_id !== categoryFilter) return false;
			if (q) {
				const hay = (d.title + ' ' + d.description).toLocaleLowerCase('fr');
				if (!hay.includes(q)) return false;
			}
			return true;
		});
	});

	// ─── Group folding (sessionStorage) ────────────────────────
	const FOLD_KEY = 'documents.foldedGroups';
	let foldedGroups = $state<Record<string, boolean>>({});

	$effect(() => {
		if (typeof sessionStorage === 'undefined') return;
		try {
			const raw = sessionStorage.getItem(FOLD_KEY);
			if (raw) foldedGroups = JSON.parse(raw);
		} catch {
			/* corrupt — start fresh */
		}
	});

	function persistFold(state: Record<string, boolean>) {
		if (typeof sessionStorage === 'undefined') return;
		try {
			sessionStorage.setItem(FOLD_KEY, JSON.stringify(state));
		} catch {
			/* quota etc. — best-effort */
		}
	}

	function toggleFold(groupKey: string) {
		const next = { ...foldedGroups, [groupKey]: !foldedGroups[groupKey] };
		foldedGroups = next;
		persistFold(next);
	}

	// ─── Group catalog (autocomplete) ──────────────────────────
	let knownGroups = $derived.by(() => {
		const set = new Set<string>();
		for (const d of documents) {
			const g = (d.group ?? '').trim();
			if (g) set.add(g);
		}
		return Array.from(set).sort();
	});

	// ─── Foldable sections ─────────────────────────────────────
	type Section = { key: string; label: string; rows: ArchiveDoc[] };
	let sections = $derived.by(() => {
		const map = new Map<string, ArchiveDoc[]>();
		for (const d of filteredArchive) {
			const key = d.group || '__no_group__';
			const arr = map.get(key) ?? [];
			arr.push(d);
			map.set(key, arr);
		}
		const out: Section[] = [];
		const keys = Array.from(map.keys()).sort((a, b) => {
			if (a === '__no_group__') return 1;
			if (b === '__no_group__') return -1;
			return a.localeCompare(b, 'fr');
		});
		for (const key of keys) {
			out.push({
				key,
				label: key === '__no_group__' ? 'Sans groupe' : capitalize(key),
				rows: map.get(key)!.sort((a, b) => b.uploaded_at.localeCompare(a.uploaded_at))
			});
		}
		return out;
	});

	function capitalize(s: string): string {
		return s.charAt(0).toLocaleUpperCase('fr') + s.slice(1);
	}

	function isImageKind(ct: string): boolean {
		return ct.startsWith('image/');
	}
	function kindLabel(ct: string): string {
		if (ct === 'application/pdf') return 'PDF';
		if (ct.startsWith('image/')) return 'IMG';
		return 'DOC';
	}

	function categoryName(id: string): string {
		return categories.find((c) => c.id === id)?.name ?? id;
	}

	function formatDate(iso: string): string {
		if (!iso) return '';
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString('fr-FR');
	}

	function formatSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} o`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} Ko`;
		return `${(bytes / 1024 / 1024).toFixed(1)} Mo`;
	}

	// ─── Download URL cache ────────────────────────────────────
	type CachedUrl = { url: string; expiresAtMs: number };
	let downloadUrlCache = $state<Record<string, CachedUrl>>({});

	async function resolveStandaloneUrl(id: string): Promise<string> {
		const cached = downloadUrlCache[id];
		if (cached && cached.expiresAtMs - 30_000 > Date.now()) return cached.url;
		const { url, expiresAt } = await getDocumentDownloadUrl(id);
		downloadUrlCache[id] = { url, expiresAtMs: new Date(expiresAt).getTime() };
		return url;
	}

	async function onView(d: ArchiveDoc) {
		// Open the popup synchronously to keep Safari's user-gesture
		// gate happy — popup blockers eat `window.open` after `await`.
		const popup = window.open('about:blank', '_blank', 'noopener,noreferrer');
		try {
			let url: string;
			if (d.kind === 'standalone') {
				url = await resolveStandaloneUrl(d.id);
			} else {
				// Attachment: use the existing per-expense download endpoint.
				const expId = d.linked_expense_id!;
				const attId = d.id.split(':')[2];
				const { getAttachmentDownloadUrl } = await import('$lib/expenses');
				const dl = await getAttachmentDownloadUrl(expId, attId);
				url = dl.url;
			}
			if (popup) {
				popup.location.href = url;
			} else {
				window.location.href = url;
			}
		} catch (err) {
			popup?.close();
			liveError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		}
	}

	// ─── Modal state (add / edit) ──────────────────────────────
	let modalOpen = $state(false);
	let editingId = $state<string | null>(null);
	let isEditing = $derived(editingId !== null);
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
			mCategoryId = c.id;
			closeInlineCat();
		} catch (err) {
			inlineCatError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			inlineCatSaving = false;
		}
	}
	let mTitle = $state('');
	let mDescription = $state('');
	let mCategoryId = $state('');
	let mGroup = $state('');
	let mFile = $state<File | null>(null);
	let deletingId = $state<string | null>(null);

	function resetForm() {
		editingId = null;
		mTitle = '';
		mDescription = '';
		mCategoryId = categories[0]?.id ?? '';
		mGroup = '';
		mFile = null;
		formError = '';
	}

	function openCreate() {
		resetForm();
		modalOpen = true;
	}

	function openEdit(d: Document) {
		resetForm();
		editingId = d.id;
		mTitle = d.title;
		mDescription = d.description ?? '';
		mCategoryId = d.category_id;
		mGroup = d.group ?? '';
		modalOpen = true;
	}

	function closeModal() {
		if (saving) return;
		modalOpen = false;
		setTimeout(resetForm, 220);
	}

	function onPickFile(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		mFile = input.files?.[0] ?? null;
	}

	async function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (saving) return;
		formError = '';
		if (!mTitle.trim()) {
			formError = 'Donne un titre au document.';
			return;
		}
		if (!mCategoryId) {
			formError = 'Choisis une catégorie.';
			return;
		}
		if (!isEditing && !mFile) {
			formError = 'Sélectionne un fichier (image ou PDF).';
			return;
		}
		saving = true;
		try {
			if (isEditing && editingId) {
				await updateDocument(editingId, {
					title: mTitle.trim(),
					description: mDescription.trim() || undefined,
					category_id: mCategoryId,
					group: mGroup.trim() || undefined
				});
			} else if (mFile) {
				await uploadDocument(mFile, {
					title: mTitle.trim(),
					description: mDescription.trim() || undefined,
					category_id: mCategoryId,
					group: mGroup.trim() || undefined
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

	async function onDelete(d: ArchiveDoc) {
		if (d.kind === 'attachment') {
			liveError = 'Supprimer cette pièce jointe depuis la dépense correspondante.';
			return;
		}
		if (deletingId) return;
		const ok = window.confirm(`Supprimer définitivement « ${d.title} » ?`);
		if (!ok) return;
		deletingId = d.id;
		try {
			await deleteDocument(d.id);
		} catch (err) {
			liveError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			deletingId = null;
		}
	}

	function onEditClick(d: ArchiveDoc) {
		if (d.kind === 'attachment') {
			liveError = 'Modifier cette pièce jointe depuis la dépense correspondante.';
			return;
		}
		const standalone = documents.find((doc) => doc.id === d.id);
		if (standalone) openEdit(standalone);
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && modalOpen) closeModal();
	}

	// Stats for hero
	let totalDocs = $derived(archive.length);
	let groupCount = $derived(sections.filter((s) => s.key !== '__no_group__').length);
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
				<p class="hero-eyebrow">Archives</p>
				<h1 class="hero-title">Documents</h1>
				<p class="hero-sub">
					{totalDocs} document{totalDocs > 1 ? 's' : ''} · {groupCount} groupe{groupCount > 1
						? 's'
						: ''}
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

			<section class="controls">
				<div class="search-row">
					<input
						type="search"
						class="search-input"
						placeholder="Rechercher dans les titres et descriptions…"
						bind:value={searchQuery}
					/>
				</div>
				<div class="filter-row">
					<div class="seg" role="tablist">
						<button
							type="button"
							class:active={linkageFilter === 'all'}
							onclick={() => (linkageFilter = 'all')}>Tous</button
						>
						<button
							type="button"
							class:active={linkageFilter === 'standalone'}
							onclick={() => (linkageFilter = 'standalone')}>Indépendants</button
						>
						<button
							type="button"
							class:active={linkageFilter === 'linked'}
							onclick={() => (linkageFilter = 'linked')}>Liés à une dépense</button
						>
					</div>
					<select bind:value={categoryFilter} class="cat-filter">
						<option value="">Toutes catégories</option>
						{#each categories as c (c.id)}
							<option value={c.id}>{c.name}</option>
						{/each}
					</select>
				</div>
			</section>

			{#if sections.length === 0}
				<section class="empty">
					<p class="empty-title">Archive vide</p>
					<p class="empty-sub">
						Aucun document ne correspond aux filtres actifs. Téléverse un contrat, un devis ou une
						attestation pour le retrouver ici.
					</p>
					<Button variant="primary" mark onclick={openCreate}>Nouveau document</Button>
				</section>
			{:else}
				{#each sections as section (section.key)}
					{@const folded = !!foldedGroups[section.key]}
					<section class="group-section" class:folded>
						<button
							type="button"
							class="group-head"
							aria-expanded={!folded}
							onclick={() => toggleFold(section.key)}
						>
							<span class="group-chevron" aria-hidden="true">{folded ? '▸' : '▾'}</span>
							<span class="group-name">{section.label}</span>
							<span class="group-count">{section.rows.length}</span>
						</button>
						{#if !folded}
							<div class="card-grid">
								{#each section.rows as d (d.id)}
									<article class="card">
										<div class="card-thumb" class:card-thumb-pdf={!isImageKind(d.content_type)}>
											{#if isImageKind(d.content_type) && d.kind === 'standalone'}
												{#await resolveStandaloneUrl(d.id)}
													<div class="card-thumb-loading"></div>
												{:then thumbUrl}
													<img src={thumbUrl} alt={d.title} loading="lazy" />
												{:catch}
													<span class="card-thumb-err">!</span>
												{/await}
											{:else}
												<span>{kindLabel(d.content_type)}</span>
											{/if}
										</div>
										<div class="card-body">
											<p class="card-title" title={d.title}>{d.title}</p>
											<p class="card-meta">
												<span class="card-cat">{categoryName(d.category_id)}</span>
												{#if d.kind === 'attachment'}
													<span class="card-link">lié · {d.linked_expense_name ?? ''}</span>
												{:else}
													<span class="card-standalone">indépendant</span>
												{/if}
											</p>
											<p class="card-foot">
												{formatDate(d.uploaded_at)} · {formatSize(d.size_bytes)}
											</p>
										</div>
										<div class="card-actions">
											<IconButton
												icon="download"
												aria-label="Voir {d.title}"
												onclick={() => onView(d)}
											/>
											{#if d.kind === 'standalone'}
												<IconButton
													icon="edit"
													aria-label="Modifier {d.title}"
													onclick={() => onEditClick(d)}
												/>
												<IconButton
													icon="delete"
													variant="danger"
													aria-label="Supprimer {d.title}"
													aria-busy={deletingId === d.id}
													onclick={() => onDelete(d)}
													disabled={deletingId !== null}
												/>
											{/if}
										</div>
									</article>
								{/each}
							</div>
						{/if}
					</section>
				{/each}
			{/if}
		</main>

		<Fab onclick={openCreate} aria-label="Nouveau document">Nouveau document</Fab>

		{#if modalOpen}
			<div
				class="modal-backdrop"
				role="presentation"
				onclick={closeModal}
				onkeydown={(e) => e.key === 'Escape' && closeModal()}
			></div>
			<div class="modal" role="dialog" aria-modal="true" aria-labelledby="doc-modal-title">
				<header class="modal-head">
					<div>
						<p class="modal-eyebrow">{isEditing ? 'Édition' : 'Nouveau'}</p>
						<h2 id="doc-modal-title">
							{isEditing ? 'Modifier le document' : 'Téléverser un document'}
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
						<span class="lbl">Titre</span>
						<input
							type="text"
							required
							bind:value={mTitle}
							placeholder="Ex. MAIF — contrat 2026"
						/>
					</label>

					<label class="field">
						<span class="lbl">Description (optionnel)</span>
						<input type="text" bind:value={mDescription} placeholder="Note, référence, etc." />
					</label>

					<div class="grid-2">
						<label class="field">
							<span class="lbl">Catégorie</span>
							<select bind:value={mCategoryId} required>
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
									<button
										type="button"
										class="inline-cat-cancel"
										onclick={closeInlineCat}
										disabled={inlineCatSaving}
									>
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
						<label class="field">
							<span class="lbl">
								Groupe
								<span class="lbl-aside">— optionnel</span>
							</span>
							<input
								type="text"
								list="known-groups"
								bind:value={mGroup}
								placeholder="devis, facture, contrat…"
							/>
							<datalist id="known-groups">
								{#each knownGroups as g}
									<option value={g}></option>
								{/each}
							</datalist>
						</label>
					</div>

					{#if !isEditing}
						<label class="field">
							<span class="lbl">Fichier</span>
							<input
								type="file"
								accept="image/jpeg,image/png,image/heic,image/heif,application/pdf"
								capture="environment"
								onchange={onPickFile}
							/>
							{#if mFile}
								<span class="field-hint">{mFile.name} · {formatSize(mFile.size)}</span>
							{/if}
						</label>
					{/if}

					{#if formError}
						<p class="form-error" role="alert">{formError}</p>
					{/if}

					<div class="modal-actions">
						<Button variant="ghost" onclick={closeModal} disabled={saving}>Annuler</Button>
						<Button type="submit" variant="primary" mark disabled={saving}>
							{#if saving}
								{isEditing ? 'Mise à jour…' : 'Téléversement…'}
							{:else}
								{isEditing ? 'Mettre à jour' : 'Téléverser'}
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
		max-width: 960px;
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
	.controls {
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
		margin-bottom: 1rem;
	}
	.search-row {
		display: flex;
	}
	.search-input {
		flex: 1;
		padding: 0.55rem 0.85rem;
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		font-family: var(--ui);
		font-size: 0.95rem;
		background: var(--surface);
		color: var(--ink);
	}
	.search-input:focus {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-soft);
	}
	.filter-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.6rem;
		align-items: center;
	}
	.seg {
		display: inline-flex;
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		overflow: hidden;
	}
	.seg button {
		background: transparent;
		border: 0;
		padding: 0.32rem 0.85rem;
		font-family: var(--ui);
		font-size: 0.78rem;
		color: var(--ink-3);
		cursor: pointer;
	}
	.seg button.active {
		background: var(--ink);
		color: var(--bg);
	}
	.cat-filter {
		font-family: var(--ui);
		font-size: 0.85rem;
		padding: 0.35rem 0.6rem;
		border: 1px solid var(--hairline-2);
		border-radius: 0.5rem;
		background: var(--surface);
		color: var(--ink);
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

	.group-section {
		margin-bottom: 1.4rem;
	}
	.group-head {
		display: flex;
		align-items: baseline;
		gap: 0.65rem;
		width: 100%;
		background: transparent;
		border: 0;
		padding: 0.4rem 0.2rem;
		cursor: pointer;
		text-align: left;
		border-bottom: 1px solid var(--hairline);
	}
	.group-chevron {
		color: var(--accent);
		font-size: 0.8rem;
	}
	.group-name {
		font-family: var(--display);
		font-weight: 500;
		font-size: 1.2rem;
		color: var(--ink);
		flex: 1;
	}
	.group-count {
		font-size: 0.7rem;
		color: var(--ink-3);
		background: var(--bg-warm);
		border-radius: 999px;
		padding: 0.16rem 0.55rem;
		font-feature-settings: 'tnum';
	}

	.card-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 0.7rem;
		padding-top: 0.7rem;
	}
	.card {
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.75rem;
		overflow: hidden;
		display: flex;
		flex-direction: column;
	}
	.card-thumb {
		width: 100%;
		aspect-ratio: 4 / 3;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-warm);
		color: var(--ink-3);
		font-family: var(--display);
		font-size: 1.5rem;
		font-weight: 500;
		letter-spacing: 0.1em;
	}
	.card-thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.card-thumb-loading {
		width: 100%;
		height: 100%;
		background: linear-gradient(90deg, var(--bg-warm) 25%, var(--surface) 50%, var(--bg-warm) 75%);
		background-size: 200% 100%;
		animation: shimmer 1.1s linear infinite;
	}
	@keyframes shimmer {
		0% {
			background-position: 200% 0;
		}
		100% {
			background-position: -200% 0;
		}
	}
	.card-thumb-err {
		color: var(--danger);
		font-size: 1.5rem;
	}
	.card-thumb-pdf {
		background: var(--clay-soft);
		color: var(--clay);
	}
	.card-body {
		padding: 0.55rem 0.7rem 0.3rem;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		flex: 1;
	}
	.card-title {
		font-weight: 600;
		font-size: 0.92rem;
		margin: 0;
		color: var(--ink);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.card-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin: 0;
		font-size: 0.7rem;
	}
	.card-cat {
		color: var(--ink-3);
		text-transform: lowercase;
	}
	.card-link {
		color: var(--clay);
		font-style: italic;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 100%;
	}
	.card-standalone {
		color: var(--rdc);
		font-style: italic;
	}
	.card-foot {
		font-size: 0.7rem;
		color: var(--ink-4);
		margin: 0;
		font-feature-settings: 'tnum';
	}
	.card-actions {
		display: flex;
		gap: 0.3rem;
		padding: 0.4rem 0.7rem 0.7rem;
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
	.lbl-aside {
		text-transform: none;
		letter-spacing: 0;
		font-size: 0.7rem;
		font-weight: 400;
		color: var(--ink-3);
		font-style: italic;
		margin-left: 0.4rem;
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
	.field-hint {
		font-size: 0.78rem;
		color: var(--ink-3);
		font-style: italic;
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
