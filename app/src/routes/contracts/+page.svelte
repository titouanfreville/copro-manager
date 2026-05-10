<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import {
		ApiError,
		type Category,
		type Contact,
		type Contract,
		type ContractStatus,
		type Document,
		type ExpenseTemplate,
		type Frequency,
		type Society
	} from '$lib/api';
	import { authState } from '$lib/auth';
	import Button from '$lib/components/Button.svelte';
	import Fab from '$lib/components/Fab.svelte';
	import IconButton from '$lib/components/IconButton.svelte';
	import { createContract, deleteContract, updateContract } from '$lib/contracts';
	import {
		subscribeCategories,
		subscribeContracts,
		subscribeDocuments,
		subscribeTemplates
	} from '$lib/live';
	import { formatEUR } from '$lib/format';

	// ─── Live data ────────────────────────────────────────────────────
	let contracts = $state<Contract[]>([]);
	let categories = $state<Category[]>([]);
	let documents = $state<Document[]>([]);
	let templates = $state<ExpenseTemplate[]>([]);
	let liveError = $state('');

	$effect(() => {
		if ($authState.status === 'signed-out') goto('/login');
		if ($authState.status !== 'signed-in') return;
		const onErr = (err: Error) => (liveError = err.message);
		const unsubs = [
			subscribeContracts((rows) => (contracts = rows), onErr),
			subscribeCategories((rows) => (categories = rows), onErr),
			subscribeDocuments((rows) => (documents = rows), onErr),
			subscribeTemplates((rows) => (templates = rows), onErr)
		];
		return () => unsubs.forEach((u) => u());
	});

	// ─── Derived helpers ──────────────────────────────────────────────
	// `Date.now()` inside the function so a long-lived tab past midnight
	// still gets fresh deltas. A `$derived(new Date())` would freeze on
	// first render — its dependency graph is empty, so it never recomputes.
	function daysUntil(iso?: string): number | null {
		if (!iso) return null;
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return null;
		const ms = d.getTime() - Date.now();
		return Math.ceil(ms / 86_400_000);
	}

	function effectiveStatus(c: Contract): ContractStatus {
		if (c.status === 'cancelled') return 'cancelled';
		const days = daysUntil(c.end_date);
		if (days !== null && days < 0) return 'expired';
		return c.status;
	}

	// "expiring" = active AND end_date within 30 days. Pinned at the top.
	let expiringContracts = $derived(
		contracts.filter((c) => {
			if (effectiveStatus(c) !== 'active') return false;
			const days = daysUntil(c.end_date);
			return days !== null && days >= 0 && days <= 30;
		})
	);
	let expiredContracts = $derived(
		contracts.filter((c) => effectiveStatus(c) === 'expired')
	);
	// Categories grouping for the active set (excludes already-expiring,
	// expired, cancelled to avoid double-listing). The expiring/expired
	// banners pin those at the top.
	let groupedActive = $derived.by(() => {
		const expiringSet = new Set(expiringContracts.map((c) => c.id));
		const map = new Map<string, Contract[]>();
		for (const c of contracts) {
			if (expiringSet.has(c.id)) continue;
			if (effectiveStatus(c) !== 'active') continue;
			const key = c.category_id || '__no_cat__';
			(map.get(key) ?? map.set(key, []).get(key)!)!.push(c);
		}
		return Array.from(map.entries())
			.map(([category_id, rows]) => ({
				category_id,
				rows: rows.sort((a, b) => a.name.localeCompare(b.name, 'fr'))
			}))
			.sort((a, b) =>
				categoryName(a.category_id).localeCompare(categoryName(b.category_id), 'fr')
			);
	});
	let cancelledContracts = $derived(
		contracts.filter((c) => c.status === 'cancelled')
	);

	function categoryById(id: string): Category | undefined {
		return categories.find((c) => c.id === id);
	}
	function categoryName(id: string): string {
		return categoryById(id)?.name ?? id;
	}
	function categoryIcon(id: string): string {
		return categoryById(id)?.icon ?? '';
	}
	function categoryColor(id: string): string {
		return categoryById(id)?.color ?? '#7A7268';
	}
	function frequencyLabel(f?: Frequency): string {
		switch (f) {
			case 'monthly':
				return '/ mois';
			case 'quarterly':
				return '/ trimestre';
			case 'yearly':
				return '/ an';
			default:
				return '';
		}
	}
	function statusLabel(s: ContractStatus): string {
		switch (s) {
			case 'active':
				return 'Actif';
			case 'expired':
				return 'Expiré';
			case 'cancelled':
				return 'Résilié';
		}
	}
	function templateName(id: string | undefined): string | null {
		if (!id) return null;
		return templates.find((t) => t.id === id)?.name ?? null;
	}
	function docCount(contractId: string): number {
		return documents.filter((d) => d.linked_contract_id === contractId).length;
	}
	function formatDateFr(iso?: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString('fr-FR');
	}
	function expiringChip(c: Contract): string {
		const days = daysUntil(c.end_date);
		if (days === null) return '';
		if (days < 0) return `Expiré depuis ${-days} j`;
		if (days === 0) return "Expire aujourd'hui";
		if (days === 1) return 'Expire demain';
		return `Expire dans ${days} j`;
	}

	// ─── Modal state ──────────────────────────────────────────────────
	let modalOpen = $state(false);
	let editingId = $state<string | null>(null);
	let isEditing = $derived(editingId !== null);
	let saving = $state(false);
	let formError = $state('');
	let deletingId = $state<string | null>(null);

	let mName = $state('');
	let mCategoryId = $state('');
	let mSocietyName = $state('');
	let mSocietyPhone = $state('');
	let mSocietyEmail = $state('');
	let mSocietyWebsite = $state('');
	let mSocietyAddress = $state('');
	let mContactName = $state('');
	let mContactRole = $state('');
	let mContactPhone = $state('');
	let mContactEmail = $state('');
	let mStartDate = $state('');
	let mEndDate = $state('');
	let mAmountEUR = $state(''); // text-bound, parsed on save
	let mFrequency = $state<Frequency | ''>('');
	let mTemplateId = $state('');
	let mStatus = $state<ContractStatus>('active');
	let mNote = $state('');

	function resetForm() {
		editingId = null;
		mName = '';
		mCategoryId = categories[0]?.id ?? '';
		mSocietyName = '';
		mSocietyPhone = '';
		mSocietyEmail = '';
		mSocietyWebsite = '';
		mSocietyAddress = '';
		mContactName = '';
		mContactRole = '';
		mContactPhone = '';
		mContactEmail = '';
		mStartDate = '';
		mEndDate = '';
		mAmountEUR = '';
		mFrequency = '';
		mTemplateId = '';
		mStatus = 'active';
		mNote = '';
		formError = '';
	}

	function openCreate() {
		resetForm();
		modalOpen = true;
	}

	function openEdit(c: Contract) {
		resetForm();
		editingId = c.id;
		mName = c.name;
		mCategoryId = c.category_id;
		mSocietyName = c.society.name;
		mSocietyPhone = c.society.phone ?? '';
		mSocietyEmail = c.society.email ?? '';
		mSocietyWebsite = c.society.website ?? '';
		mSocietyAddress = c.society.address ?? '';
		mContactName = c.contact?.name ?? '';
		mContactRole = c.contact?.role ?? '';
		mContactPhone = c.contact?.phone ?? '';
		mContactEmail = c.contact?.email ?? '';
		mStartDate = c.start_date?.slice(0, 10) ?? '';
		mEndDate = c.end_date?.slice(0, 10) ?? '';
		mAmountEUR =
			c.amount_cents !== undefined && c.amount_cents > 0
				? (c.amount_cents / 100).toFixed(2).replace('.', ',')
				: '';
		mFrequency = c.billing_frequency ?? '';
		mTemplateId = c.template_id ?? '';
		mStatus = c.status;
		mNote = c.note ?? '';
		modalOpen = true;
	}

	function closeModal() {
		if (saving) return;
		modalOpen = false;
		setTimeout(resetForm, 220);
	}

	function parseAmountToCents(raw: string): number | null {
		const trimmed = raw.trim();
		if (!trimmed) return 0;
		// Strict shape: integer part + optional decimal separator (`,` or
		// `.`) + 1–2 fractional digits. Rejects scientific notation
		// ("12e5"), thousands separators, leading sign — `Number()` alone
		// would silently accept those and produce surprises.
		if (!/^\d+([.,]\d{1,2})?$/.test(trimmed)) return null;
		const v = Number(trimmed.replace(',', '.'));
		if (!Number.isFinite(v) || v < 0) return null;
		return Math.round(v * 100);
	}

	async function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (saving) return;
		formError = '';
		if (mName.trim().length < 2) {
			formError = 'Donne un nom au contrat (min 2 caractères).';
			return;
		}
		if (!mCategoryId) {
			formError = 'Choisis une catégorie.';
			return;
		}
		if (!mSocietyName.trim()) {
			formError = 'Renseigne au moins le nom de la société.';
			return;
		}
		const cents = parseAmountToCents(mAmountEUR);
		if (cents === null) {
			formError = 'Montant invalide.';
			return;
		}
		if (mStartDate && mEndDate && mEndDate < mStartDate) {
			formError = 'La date de fin doit être après la date de début.';
			return;
		}

		const society: Society = {
			name: mSocietyName.trim(),
			phone: mSocietyPhone.trim() || undefined,
			email: mSocietyEmail.trim() || undefined,
			website: mSocietyWebsite.trim() || undefined,
			address: mSocietyAddress.trim() || undefined
		};
		const contact: Contact | undefined =
			mContactName || mContactRole || mContactPhone || mContactEmail
				? {
						name: mContactName.trim() || undefined,
						role: mContactRole.trim() || undefined,
						phone: mContactPhone.trim() || undefined,
						email: mContactEmail.trim() || undefined
					}
				: undefined;

		const payload = {
			name: mName.trim(),
			category_id: mCategoryId,
			society,
			contact,
			start_date: mStartDate || undefined,
			end_date: mEndDate || undefined,
			amount_cents: cents > 0 ? cents : undefined,
			billing_frequency: (mFrequency || undefined) as Frequency | undefined,
			template_id: mTemplateId || undefined,
			status: mStatus,
			note: mNote.trim() || undefined
		};

		saving = true;
		try {
			if (isEditing && editingId) {
				await updateContract(editingId, payload);
			} else {
				await createContract(payload);
			}
			modalOpen = false;
			setTimeout(resetForm, 220);
		} catch (err) {
			formError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			saving = false;
		}
	}

	async function onDelete(c: Contract) {
		if (deletingId) return;
		const ok = window.confirm(`Supprimer définitivement « ${c.name} » ?`);
		if (!ok) return;
		deletingId = c.id;
		try {
			await deleteContract(c.id);
		} catch (err) {
			liveError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			deletingId = null;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && modalOpen) closeModal();
	}

	// Deep-link from contract_expiring alert: /contracts?focus=ID auto-opens
	// the matching contract's edit modal once contracts have loaded. The
	// `consumed` flag below stops the effect from re-firing on every
	// Firestore snapshot — without it, closing the modal without
	// stripping the URL param would let the next live tick reopen it.
	let focusConsumed = $state(false);
	$effect(() => {
		const focusId = $page.url.searchParams.get('focus');
		if (!focusId || focusConsumed || contracts.length === 0) return;
		const target = contracts.find((c) => c.id === focusId);
		if (!target) return;
		focusConsumed = true;
		openEdit(target);
		// Strip the param so refreshes / shares don't keep auto-opening.
		const url = new URL(window.location.href);
		url.searchParams.delete('focus');
		goto(url.pathname + url.search, { replaceState: true, noScroll: true });
	});
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
				<p class="hero-eyebrow">Engagements</p>
				<h1 class="hero-title">Contrats</h1>
				<p class="hero-sub">
					{contracts.length} contrat{contracts.length > 1 ? 's' : ''} suivi{contracts.length > 1 ? 's' : ''}.
					Société, contact, échéance et documents au même endroit.
				</p>
			</section>

			{#if liveError}
				<div class="error-card" role="alert">{liveError}</div>
			{/if}

			{#if contracts.length === 0}
				<section class="empty">
					<p class="empty-title">Aucun contrat enregistré.</p>
					<p class="empty-sub">
						Démarre avec ton assurance habitation, ton syndic, ou ton contrat d'entretien :
						tu retrouveras leurs coordonnées et documents en un coup d'œil.
					</p>
					<Button variant="primary" mark onclick={openCreate}>Nouveau contrat</Button>
				</section>
			{:else}
				{#if expiringContracts.length > 0}
					<section class="alert-zone alert-zone-warn">
						<header class="zone-head">
							<span class="zone-label">À renouveler bientôt</span>
							<span class="zone-count">{expiringContracts.length}</span>
						</header>
						<div class="cards">
							{#each expiringContracts as c (c.id)}
								{@const cat = categoryById(c.category_id)}
								<article class="card card-warn">
									<header class="card-head">
										<span
											class="card-chip"
											aria-hidden="true"
											style="background:{categoryColor(c.category_id) +
												'22'};color:{categoryColor(
												c.category_id
											)};border-color:{categoryColor(c.category_id) + '55'}"
										>
											{categoryIcon(c.category_id) || c.name.slice(0, 2).toUpperCase()}
										</span>
										<div class="card-title-block">
											<h2 class="card-name">{c.name}</h2>
											<p class="card-society">{c.society.name}</p>
										</div>
										<span class="card-chip-warn">{expiringChip(c)}</span>
									</header>
									<dl class="card-meta">
										<div><dt>Catégorie</dt><dd>{cat?.name ?? c.category_id}</dd></div>
										<div><dt>Fin</dt><dd>{formatDateFr(c.end_date)}</dd></div>
									</dl>
									<div class="card-actions">
										<IconButton icon="edit" aria-label="Modifier {c.name}" onclick={() => openEdit(c)} />
									</div>
								</article>
							{/each}
						</div>
					</section>
				{/if}

				{#if expiredContracts.length > 0}
					<section class="alert-zone alert-zone-expired">
						<header class="zone-head">
							<span class="zone-label">Expirés</span>
							<span class="zone-count">{expiredContracts.length}</span>
						</header>
						<div class="cards">
							{#each expiredContracts as c (c.id)}
								<article class="card card-muted">
									<header class="card-head">
										<span
											class="card-chip"
											aria-hidden="true"
											style="background:{categoryColor(c.category_id) +
												'22'};color:{categoryColor(
												c.category_id
											)};border-color:{categoryColor(c.category_id) + '55'}"
										>
											{categoryIcon(c.category_id) || c.name.slice(0, 2).toUpperCase()}
										</span>
										<div class="card-title-block">
											<h2 class="card-name">{c.name}</h2>
											<p class="card-society">{c.society.name}</p>
										</div>
										<span class="card-chip-expired">{expiringChip(c)}</span>
									</header>
									<dl class="card-meta">
										<div><dt>Fin</dt><dd>{formatDateFr(c.end_date)}</dd></div>
									</dl>
									<div class="card-actions">
										<IconButton icon="edit" aria-label="Modifier {c.name}" onclick={() => openEdit(c)} />
										<IconButton
											icon="delete"
											variant="danger"
											aria-label="Supprimer {c.name}"
											aria-busy={deletingId === c.id}
											onclick={() => onDelete(c)}
											disabled={deletingId !== null}
										/>
									</div>
								</article>
							{/each}
						</div>
					</section>
				{/if}

				{#each groupedActive as group (group.category_id)}
					<section class="group-section">
						<header class="zone-head">
							<span class="zone-label">{categoryName(group.category_id)}</span>
							<span class="zone-count">{group.rows.length}</span>
						</header>
						<div class="cards">
							{#each group.rows as c (c.id)}
								<article class="card">
									<header class="card-head">
										<span
											class="card-chip"
											aria-hidden="true"
											style="background:{categoryColor(c.category_id) +
												'22'};color:{categoryColor(
												c.category_id
											)};border-color:{categoryColor(c.category_id) + '55'}"
										>
											{categoryIcon(c.category_id) || c.name.slice(0, 2).toUpperCase()}
										</span>
										<div class="card-title-block">
											<h2 class="card-name">{c.name}</h2>
											<p class="card-society">{c.society.name}</p>
										</div>
										{#if c.end_date}
											<span class="card-chip-info">{formatDateFr(c.end_date)}</span>
										{/if}
									</header>
									<dl class="card-meta">
										{#if c.contact?.name || c.contact?.phone}
											<div>
												<dt>Contact</dt>
												<dd>
													{c.contact.name ?? ''}
													{#if c.contact.role}<span class="meta-role"> · {c.contact.role}</span>{/if}
												</dd>
											</div>
										{/if}
										{#if c.society.phone}
											<div>
												<dt>Tél</dt>
												<dd>
													<a href={`tel:${c.society.phone}`}>{c.society.phone}</a>
												</dd>
											</div>
										{/if}
										{#if c.society.email}
											<div>
												<dt>Email</dt>
												<dd>
													<a href={`mailto:${c.society.email}`}>{c.society.email}</a>
												</dd>
											</div>
										{/if}
										{#if c.amount_cents}
											<div>
												<dt>Montant</dt>
												<dd>
													{formatEUR(c.amount_cents)}<span class="meta-role">
														{frequencyLabel(c.billing_frequency)}</span
													>
												</dd>
											</div>
										{/if}
										{#if templateName(c.template_id)}
											<div>
												<dt>Modèle</dt>
												<dd>
													<a href={`/templates`}>{templateName(c.template_id)}</a>
												</dd>
											</div>
										{/if}
										{#if docCount(c.id) > 0}
											<div>
												<dt>Docs</dt>
												<dd>
													<a href={`/documents?linked_contract_id=${c.id}`}>
														{docCount(c.id)} document{docCount(c.id) > 1 ? 's' : ''}
													</a>
												</dd>
											</div>
										{/if}
									</dl>
									<div class="card-actions">
										<IconButton icon="edit" aria-label="Modifier {c.name}" onclick={() => openEdit(c)} />
										<IconButton
											icon="delete"
											variant="danger"
											aria-label="Supprimer {c.name}"
											aria-busy={deletingId === c.id}
											onclick={() => onDelete(c)}
											disabled={deletingId !== null}
										/>
									</div>
								</article>
							{/each}
						</div>
					</section>
				{/each}

				{#if cancelledContracts.length > 0}
					<section class="group-section group-section-muted">
						<header class="zone-head">
							<span class="zone-label">Résiliés</span>
							<span class="zone-count">{cancelledContracts.length}</span>
						</header>
						<div class="cards">
							{#each cancelledContracts as c (c.id)}
								<article class="card card-muted">
									<header class="card-head">
										<div class="card-title-block">
											<h2 class="card-name">{c.name}</h2>
											<p class="card-society">{c.society.name}</p>
										</div>
										<span class="card-chip-info">{statusLabel(c.status)}</span>
									</header>
									<div class="card-actions">
										<IconButton icon="edit" aria-label="Modifier {c.name}" onclick={() => openEdit(c)} />
										<IconButton
											icon="delete"
											variant="danger"
											aria-label="Supprimer {c.name}"
											aria-busy={deletingId === c.id}
											onclick={() => onDelete(c)}
											disabled={deletingId !== null}
										/>
									</div>
								</article>
							{/each}
						</div>
					</section>
				{/if}
			{/if}
		</main>

		<Fab onclick={openCreate} aria-label="Nouveau contrat">Nouveau contrat</Fab>

		{#if modalOpen}
			<div
				class="modal-backdrop"
				role="presentation"
				onclick={closeModal}
				onkeydown={(e) => e.key === 'Escape' && closeModal()}
			></div>
			<div
				class="modal"
				role="dialog"
				aria-modal="true"
				aria-labelledby="contract-modal-title"
			>
				<header class="modal-head">
					<div>
						<p class="modal-eyebrow">{isEditing ? 'Édition' : 'Nouveau'}</p>
						<h2 id="contract-modal-title">
							{isEditing ? 'Modifier le contrat' : 'Nouveau contrat'}
						</h2>
					</div>
					<IconButton icon="close" variant="text" aria-label="Fermer" onclick={closeModal} />
				</header>

				<form class="modal-body" onsubmit={onSubmit}>
					<label class="field">
						<span class="lbl">Nom</span>
						<input type="text" required bind:value={mName} placeholder="Assurance habitation" />
					</label>

					<label class="field">
						<span class="lbl">Catégorie</span>
						<select bind:value={mCategoryId}>
							{#each categories as c (c.id)}
								<option value={c.id}>{c.name}</option>
							{/each}
						</select>
					</label>

					<fieldset class="fieldset">
						<legend>Société</legend>
						<label class="field">
							<span class="lbl">Nom</span>
							<input type="text" required bind:value={mSocietyName} placeholder="Maaf" />
						</label>
						<div class="field-grid">
							<label class="field">
								<span class="lbl">Téléphone</span>
								<input type="tel" inputmode="tel" bind:value={mSocietyPhone} />
							</label>
							<label class="field">
								<span class="lbl">Email</span>
								<input type="email" inputmode="email" bind:value={mSocietyEmail} />
							</label>
						</div>
						<label class="field">
							<span class="lbl">Site web</span>
							<input type="url" inputmode="url" bind:value={mSocietyWebsite} placeholder="https://" />
						</label>
						<label class="field">
							<span class="lbl">Adresse</span>
							<input type="text" bind:value={mSocietyAddress} />
						</label>
					</fieldset>

					<fieldset class="fieldset">
						<legend>Contact (optionnel)</legend>
						<div class="field-grid">
							<label class="field">
								<span class="lbl">Nom</span>
								<input type="text" bind:value={mContactName} />
							</label>
							<label class="field">
								<span class="lbl">Rôle</span>
								<input type="text" bind:value={mContactRole} placeholder="Conseiller" />
							</label>
						</div>
						<div class="field-grid">
							<label class="field">
								<span class="lbl">Téléphone</span>
								<input type="tel" inputmode="tel" bind:value={mContactPhone} />
							</label>
							<label class="field">
								<span class="lbl">Email</span>
								<input type="email" inputmode="email" bind:value={mContactEmail} />
							</label>
						</div>
					</fieldset>

					<div class="field-grid">
						<label class="field">
							<span class="lbl">Début</span>
							<input type="date" bind:value={mStartDate} />
						</label>
						<label class="field">
							<span class="lbl">Fin</span>
							<input type="date" bind:value={mEndDate} />
						</label>
					</div>

					<div class="field-grid">
						<label class="field">
							<span class="lbl">Montant (€)</span>
							<input
								type="text"
								inputmode="decimal"
								bind:value={mAmountEUR}
								placeholder="42,50"
							/>
						</label>
						<label class="field">
							<span class="lbl">Fréquence</span>
							<select bind:value={mFrequency}>
								<option value="">—</option>
								<option value="monthly">Mensuelle</option>
								<option value="quarterly">Trimestrielle</option>
								<option value="yearly">Annuelle</option>
							</select>
						</label>
					</div>

					{#if templates.length > 0}
						<label class="field">
							<span class="lbl">Modèle de dépense lié (optionnel)</span>
							<select bind:value={mTemplateId}>
								<option value="">—</option>
								{#each templates as t (t.id)}
									<option value={t.id}>{t.name}</option>
								{/each}
							</select>
						</label>
					{/if}

					<label class="field">
						<span class="lbl">Statut</span>
						<select bind:value={mStatus}>
							<option value="active">Actif</option>
							<option value="cancelled">Résilié</option>
							<!-- `expired` is rarely user-set (the FE auto-derives it
							     from end_date) but keeping the option prevents the
							     modal from silently coercing an expired contract back
							     to active when the bound value can't match an option. -->
							<option value="expired">Expiré</option>
						</select>
					</label>

					<label class="field">
						<span class="lbl">Note</span>
						<textarea rows="3" bind:value={mNote}></textarea>
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
		--clay: #9e6a4d;
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
		max-width: 920px;
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
		max-width: 36rem;
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

	.alert-zone {
		margin: 0 0 1.4rem;
		border-radius: 1rem;
		padding: 1rem 1.1rem;
		border: 1px solid;
	}
	.alert-zone-warn {
		border-color: rgba(194, 78, 42, 0.28);
		background: rgba(244, 226, 216, 0.55);
	}
	.alert-zone-expired {
		border-color: var(--hairline-2);
		background: var(--bg-warm);
	}

	.group-section {
		margin: 0 0 1.4rem;
	}
	.group-section-muted {
		opacity: 0.7;
	}
	.zone-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.6rem;
		padding: 0.2rem 0 0.7rem;
	}
	.zone-label {
		font-family: var(--display);
		font-weight: 500;
		font-size: 1.05rem;
	}
	.zone-count {
		font-size: 0.75rem;
		color: var(--ink-3);
	}

	.cards {
		display: grid;
		gap: 0.7rem;
	}
	@media (min-width: 720px) {
		.cards {
			grid-template-columns: repeat(2, 1fr);
		}
	}

	.card {
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.85rem;
		padding: 0.95rem 1.05rem;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.card-warn {
		border-color: rgba(194, 78, 42, 0.4);
	}
	.card-muted {
		opacity: 0.85;
	}
	.card-head {
		display: flex;
		align-items: center;
		gap: 0.7rem;
	}
	.card-chip {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 2.2rem;
		height: 2.2rem;
		border-radius: 0.55rem;
		border: 1px solid;
		font-size: 1.1rem;
		font-weight: 600;
		flex-shrink: 0;
	}
	.card-title-block {
		flex: 1;
		min-width: 0;
	}
	.card-name {
		font-family: var(--display);
		font-weight: 500;
		font-size: 1.1rem;
		margin: 0;
		line-height: 1.2;
	}
	.card-society {
		font-size: 0.78rem;
		color: var(--ink-3);
		margin: 0.15rem 0 0;
	}
	.card-chip-warn,
	.card-chip-expired,
	.card-chip-info {
		font-size: 0.7rem;
		padding: 0.2rem 0.55rem;
		border-radius: 999px;
		border: 1px solid;
		font-weight: 600;
		white-space: nowrap;
	}
	.card-chip-warn {
		color: var(--accent-deep);
		background: var(--accent-soft);
		border-color: rgba(194, 78, 42, 0.4);
	}
	.card-chip-expired {
		color: var(--ink-3);
		background: var(--bg-warm);
		border-color: var(--hairline-2);
	}
	.card-chip-info {
		color: var(--ink-2);
		background: transparent;
		border-color: var(--hairline-2);
	}

	.card-meta {
		display: grid;
		gap: 0.3rem;
		margin: 0;
		font-size: 0.85rem;
	}
	.card-meta div {
		display: flex;
		gap: 0.55rem;
	}
	.card-meta dt {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--ink-4);
		font-weight: 600;
		min-width: 4.5rem;
	}
	.card-meta dd {
		margin: 0;
		color: var(--ink-2);
		min-width: 0;
		flex: 1;
		overflow-wrap: anywhere;
	}
	.card-meta a {
		color: var(--accent-deep);
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.meta-role {
		color: var(--ink-3);
		font-size: 0.8rem;
	}

	.card-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.3rem;
	}

	.muted {
		color: var(--ink-3);
	}
	.center {
		text-align: center;
	}

	/* ─── Modal ───────────────────────────────────────────────────── */
	.modal-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(20, 16, 12, 0.32);
		backdrop-filter: blur(4px);
		-webkit-backdrop-filter: blur(4px);
		z-index: 110;
	}
	.modal {
		position: fixed;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		z-index: 120;
		width: min(640px, calc(100vw - 2rem));
		max-height: min(90vh, 800px);
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 1rem;
		box-shadow: 0 24px 48px rgba(20, 16, 12, 0.18);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.modal-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		padding: 1rem 1.4rem 0.6rem;
		border-bottom: 1px solid var(--hairline);
	}
	.modal-eyebrow {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.18em;
		color: var(--ink-4);
		margin: 0 0 0.25rem;
		font-weight: 600;
	}
	.modal-head h2 {
		font-family: var(--display);
		font-weight: 400;
		font-size: 1.4rem;
		margin: 0;
	}
	.modal-body {
		padding: 0.7rem 1.4rem 1.2rem;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
	}
	.fieldset {
		border: 1px solid var(--hairline);
		border-radius: 0.7rem;
		padding: 0.7rem 0.95rem 0.95rem;
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}
	.fieldset legend {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--ink-4);
		font-weight: 600;
		padding: 0 0.4rem;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.field-grid {
		display: grid;
		gap: 0.7rem;
		grid-template-columns: repeat(2, 1fr);
	}
	.lbl {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--ink-4);
		font-weight: 600;
	}
	.modal-body input,
	.modal-body select,
	.modal-body textarea {
		font-family: var(--ui);
		font-size: 0.95rem;
		padding: 0.55rem 0.7rem;
		border: 1px solid var(--hairline-2);
		border-radius: 0.45rem;
		background: var(--surface);
		color: var(--ink);
	}
	.modal-body textarea {
		resize: vertical;
	}
	.modal-body input:focus,
	.modal-body select:focus,
	.modal-body textarea:focus {
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
</style>
