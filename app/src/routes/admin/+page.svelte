<script lang="ts">
	import { ApiError } from '$lib/api';
	import {
		addFoyerMember,
		createFoyer,
		getAdminKey,
		importExpensesCSV,
		listFoyers,
		resetPassword,
		setAdminKey,
		setUserPassword,
		updateFoyerParts,
		type AddMemberResponse,
		type CreateFoyerResponse,
		type ImportSummary,
		type ListedFoyer
	} from '$lib/admin';

	let adminKey = $state<string>('');
	let keySaved = $state<boolean>(false);

	// Foyer create form
	let createFloor = $state<'rdc' | '1er'>('rdc');
	let createName = $state<string>('');
	let createParts = $state<number>(500);
	let createMemberEmail = $state<string>('');
	let createMemberName = $state<string>('');
	let createSubmitting = $state<boolean>(false);
	let createError = $state<string>('');
	let createResult = $state<CreateFoyerResponse | null>(null);

	// List
	let foyersList = $state<ListedFoyer[]>([]);
	let listError = $state<string>('');
	let listLoading = $state<boolean>(false);

	// Per-foyer state for add-member, parts edit, password-reset feedback
	let addMemberForm = $state<Record<string, { email: string; name: string; submitting: boolean }>>({});
	let addMemberResult = $state<Record<string, AddMemberResponse | null>>({});
	let addMemberError = $state<Record<string, string>>({});
	let partsDraft = $state<Record<string, number>>({});
	let partsSubmitting = $state<Record<string, boolean>>({});
	let partsError = $state<Record<string, string>>({});
	let resetLink = $state<string>('');
	let resetTarget = $state<string>('');
	let resetError = $state<string>('');
	let resetSubmitting = $state<boolean>(false);

	// Per-member state for the direct set-password escape hatch.
	let setPwdDraft = $state<Record<string, string>>({});
	let setPwdSubmitting = $state<Record<string, boolean>>({});
	let setPwdError = $state<Record<string, string>>({});
	let setPwdSuccess = $state<Record<string, string>>({});

	// CSV import
	let importPayerFoyerId = $state<string>('');
	let importFile = $state<File | null>(null);
	let importSummary = $state<ImportSummary | null>(null);
	let importError = $state<string>('');
	let importSubmitting = $state<boolean>(false);

	$effect(() => {
		const existing = getAdminKey();
		if (existing) {
			adminKey = existing;
			keySaved = true;
		}
	});

	$effect(() => {
		if (keySaved && foyersList.length === 0 && !listError && !listLoading) {
			void refreshList();
		}
	});

	async function refreshList() {
		listError = '';
		listLoading = true;
		try {
			const data = await listFoyers();
			foyersList = data.foyers ?? [];
			// reset per-foyer drafts to current parts, ensure add-member form slots exist
			partsDraft = Object.fromEntries(foyersList.map((f) => [f.id, f.parts]));
			const formNext: typeof addMemberForm = {};
			for (const f of foyersList) {
				formNext[f.id] = addMemberForm[f.id] ?? { email: '', name: '', submitting: false };
			}
			addMemberForm = formNext;
		} catch (err) {
			listError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			listLoading = false;
		}
	}

	function saveKey(e: SubmitEvent) {
		e.preventDefault();
		setAdminKey(adminKey.trim());
		keySaved = !!adminKey.trim();
	}

	function clearKey() {
		setAdminKey('');
		adminKey = '';
		keySaved = false;
		createResult = null;
		createError = '';
		foyersList = [];
		resetLink = '';
		resetTarget = '';
	}

	async function onCreateFoyer(e: SubmitEvent) {
		e.preventDefault();
		createError = '';
		createResult = null;
		createSubmitting = true;
		try {
			createResult = await createFoyer({
				floor: createFloor,
				name: createName.trim(),
				parts: createParts,
				member: {
					email: createMemberEmail.trim(),
					display_name: createMemberName.trim()
				}
			});
			createName = '';
			createMemberEmail = '';
			createMemberName = '';
			void refreshList();
		} catch (err) {
			createError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			createSubmitting = false;
		}
	}

	async function copy(value: string) {
		if (!value) return;
		// `navigator.clipboard.writeText` rejects in non-HTTPS contexts and on
		// some embedded WebViews. Catch so the rejection doesn't surface as
		// an unhandled promise; the value stays visible on screen so the
		// operator can fall back to manual selection.
		try {
			await navigator.clipboard.writeText(value);
		} catch (err) {
			// eslint-disable-next-line no-alert
			alert('Copie impossible — sélectionne le texte affiché manuellement.');
			console.warn('clipboard.writeText failed', err);
		}
	}

	async function onAddMember(foyerId: string, e: SubmitEvent) {
		e.preventDefault();
		addMemberError[foyerId] = '';
		addMemberResult[foyerId] = null;
		addMemberForm[foyerId].submitting = true;
		try {
			const res = await addFoyerMember(foyerId, {
				email: addMemberForm[foyerId].email.trim(),
				display_name: addMemberForm[foyerId].name.trim()
			});
			addMemberResult[foyerId] = res;
			addMemberForm[foyerId].email = '';
			addMemberForm[foyerId].name = '';
			void refreshList();
		} catch (err) {
			addMemberError[foyerId] = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			addMemberForm[foyerId].submitting = false;
		}
	}

	async function onUpdateParts(foyerId: string) {
		const value = partsDraft[foyerId];
		if (value === undefined) return;
		partsError[foyerId] = '';
		partsSubmitting[foyerId] = true;
		try {
			await updateFoyerParts(foyerId, value);
			void refreshList();
		} catch (err) {
			partsError[foyerId] = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			partsSubmitting[foyerId] = false;
		}
	}

	async function onResetPassword(userId: string, label: string) {
		resetError = '';
		resetLink = '';
		resetTarget = label;
		resetSubmitting = true;
		try {
			const data = await resetPassword(userId);
			resetLink = data.reset_link;
		} catch (err) {
			resetError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			resetSubmitting = false;
		}
	}

	async function onSetPassword(userId: string, label: string) {
		const pwd = (setPwdDraft[userId] ?? '').trim();
		setPwdError[userId] = '';
		setPwdSuccess[userId] = '';
		if (pwd.length < 8) {
			setPwdError[userId] = 'Mot de passe trop court (8 caractères minimum).';
			return;
		}
		setPwdSubmitting[userId] = true;
		try {
			await setUserPassword(userId, pwd);
			setPwdSuccess[userId] = `Mot de passe défini pour ${label}.`;
			setPwdDraft[userId] = '';
		} catch (err) {
			setPwdError[userId] = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			setPwdSubmitting[userId] = false;
		}
	}

	function onImportFileChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		importFile = input.files?.[0] ?? null;
		importSummary = null;
		importError = '';
	}

	function skipReasonLabel(reason: string): string {
		switch (reason) {
			case 'payment_not_complete':
				return 'Paiement non finalisé (Paiement complet ≠ TRUE)';
			case 'missing_required_field':
				return 'Champ obligatoire manquant (Item ou Total vide)';
			case 'missing_date':
				return 'Date manquante dans la colonne Date';
			default:
				return reason;
		}
	}

	async function onImportSubmit(e: SubmitEvent) {
		e.preventDefault();
		importError = '';
		importSummary = null;
		if (!importFile) {
			importError = 'Aucun fichier sélectionné.';
			return;
		}
		if (!importPayerFoyerId) {
			importError = 'Choisis le foyer payeur par défaut.';
			return;
		}
		importSubmitting = true;
		try {
			importSummary = await importExpensesCSV(importFile, importPayerFoyerId);
		} catch (err) {
			importError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			importSubmitting = false;
		}
	}

	$effect(() => {
		// Default the import payer to the first foyer once the list loads.
		if (!importPayerFoyerId && foyersList[0]) {
			importPayerFoyerId = foyersList[0].id;
		}
	});
</script>

<main class="mx-auto max-w-2xl p-6">
	<header class="mb-8">
		<h1 class="text-2xl font-semibold">Admin</h1>
		<p class="text-sm text-slate-500">
			Outils protégés par la clé admin globale. Aucune session Firebase requise.
		</p>
	</header>

	<section class="mb-8 rounded-lg border border-slate-200 bg-white p-4">
		<h2 class="mb-3 text-sm font-medium uppercase tracking-wide text-slate-500">Clé admin</h2>

		{#if keySaved}
			<p class="mb-3 text-sm text-emerald-700">Clé enregistrée pour cette session.</p>
			<button
				class="rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50"
				onclick={clearKey}
			>
				Oublier la clé
			</button>
		{:else}
			<form class="space-y-3" onsubmit={saveKey}>
				<input
					type="password"
					required
					placeholder="Coller la clé admin"
					bind:value={adminKey}
					class="w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				/>
				<button
					type="submit"
					class="rounded-md bg-slate-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800"
				>
					Enregistrer
				</button>
			</form>
		{/if}
	</section>

	<section
		class="mb-8 rounded-lg border border-slate-200 bg-white p-4"
		class:opacity-50={!keySaved}
	>
		<div class="mb-3 flex items-center justify-between">
			<h2 class="text-sm font-medium uppercase tracking-wide text-slate-500">Foyers</h2>
			<button
				class="rounded-md border border-slate-300 px-2 py-1 text-xs hover:bg-slate-50 disabled:opacity-50"
				disabled={!keySaved || listLoading}
				onclick={() => refreshList()}
			>
				{listLoading ? 'Chargement…' : 'Rafraîchir'}
			</button>
		</div>

		{#if listError}
			<p class="text-sm text-red-600">{listError}</p>
		{:else if foyersList.length === 0}
			<p class="text-sm text-slate-500">{listLoading ? 'Chargement…' : 'Aucun foyer.'}</p>
		{:else}
			<ul class="divide-y divide-slate-200">
				{#each foyersList as f (f.id)}
					<li class="py-4">
						<div class="mb-2">
							<p class="font-medium">{f.name} <span class="text-slate-500">({f.floor})</span></p>
							<p class="text-xs text-slate-500">id : {f.id}</p>
						</div>

						<!-- Parts editor -->
						<div class="mb-3 flex items-center gap-2 text-sm">
							<label class="flex items-center gap-2">
								<span class="font-medium">Tantièmes :</span>
								<input
									type="number"
									min="0"
									step="1"
									bind:value={partsDraft[f.id]}
									class="w-24 rounded border border-slate-300 px-2 py-1"
								/>
							</label>
							<button
								class="rounded border border-slate-300 px-2 py-1 text-xs hover:bg-slate-50 disabled:opacity-50"
								disabled={partsSubmitting[f.id] || partsDraft[f.id] === f.parts}
								onclick={() => onUpdateParts(f.id)}
							>
								{partsSubmitting[f.id] ? '…' : 'Enregistrer'}
							</button>
							{#if partsError[f.id]}
								<span class="text-xs text-red-600">{partsError[f.id]}</span>
							{/if}
						</div>

						<!-- Members -->
						{#if f.members?.length}
							<ul class="mb-3 space-y-2">
								{#each f.members as m (m.id)}
									<li class="rounded bg-slate-50 px-2 py-1 text-sm">
										<div class="flex items-center justify-between">
											<span>
												<span class="font-medium">{m.display_name || '—'}</span>
												<span class="ml-2 text-slate-500">{m.email}</span>
											</span>
											<button
												class="rounded border border-slate-300 px-2 py-0.5 text-xs hover:bg-white disabled:opacity-50"
												disabled={resetSubmitting}
												onclick={() => onResetPassword(m.id, m.email)}
											>
												Réinitialiser MDP
											</button>
										</div>
										<details class="mt-1">
											<summary class="cursor-pointer text-xs text-slate-500">
												Définir un mot de passe directement
											</summary>
											<form
												class="mt-2 flex flex-wrap items-center gap-2"
												onsubmit={(e) => {
													e.preventDefault();
													onSetPassword(m.id, m.email);
												}}
											>
												<input
													type="text"
													autocomplete="off"
													placeholder="Mot de passe (8 car. min.)"
													bind:value={setPwdDraft[m.id]}
													class="flex-1 min-w-0 rounded border border-slate-300 px-2 py-1 text-xs"
												/>
												<button
													type="submit"
													class="rounded bg-slate-900 px-2 py-1 text-xs font-medium text-white hover:bg-slate-800 disabled:opacity-50"
													disabled={setPwdSubmitting[m.id]}
												>
													{setPwdSubmitting[m.id] ? '…' : 'Définir'}
												</button>
												{#if setPwdError[m.id]}
													<p class="basis-full text-xs text-red-600">{setPwdError[m.id]}</p>
												{/if}
												{#if setPwdSuccess[m.id]}
													<p class="basis-full text-xs text-emerald-700">{setPwdSuccess[m.id]}</p>
												{/if}
												<p class="basis-full text-[11px] text-slate-500">
													Le mot de passe est appliqué immédiatement et n'est pas conservé.
													Communique-le au membre via un canal sûr.
												</p>
											</form>
										</details>
									</li>
								{/each}
							</ul>
						{:else}
							<p class="mb-3 text-xs text-slate-500">Aucun membre lié.</p>
						{/if}

						<!-- Add member -->
						<details class="rounded border border-slate-200 p-2 text-sm">
							<summary class="cursor-pointer text-slate-700">Ajouter un membre</summary>
							<form class="mt-2 space-y-2" onsubmit={(e) => onAddMember(f.id, e)}>
								<input
									type="email"
									required
									placeholder="email@exemple.com"
									bind:value={addMemberForm[f.id].email}
									class="w-full rounded border border-slate-300 px-2 py-1"
								/>
								<input
									type="text"
									required
									placeholder="Nom affiché"
									bind:value={addMemberForm[f.id].name}
									class="w-full rounded border border-slate-300 px-2 py-1"
								/>
								<button
									type="submit"
									class="rounded bg-slate-900 px-3 py-1 text-xs font-medium text-white hover:bg-slate-800 disabled:opacity-50"
									disabled={addMemberForm[f.id].submitting}
								>
									{addMemberForm[f.id].submitting ? '…' : 'Ajouter'}
								</button>
								{#if addMemberError[f.id]}
									<p class="text-xs text-red-600">{addMemberError[f.id]}</p>
								{/if}
								{#if addMemberResult[f.id]?.reset_link}
									<div class="rounded bg-emerald-50 p-2 text-xs">
										<p class="text-emerald-800">
											Lien de mot de passe à transmettre au membre :
										</p>
										<div class="mt-1 flex items-center gap-2">
											<code class="flex-1 break-all rounded bg-slate-900 px-2 py-1 text-white">
												{addMemberResult[f.id]?.reset_link}
											</code>
											<button
												type="button"
												class="rounded border border-slate-300 px-2 py-1 hover:bg-white"
												onclick={() => copy(addMemberResult[f.id]?.reset_link ?? '')}
											>
												Copier
											</button>
										</div>
									</div>
								{:else if addMemberResult[f.id]}
									<p class="text-xs text-emerald-700">
										Compte Firebase déjà existant — pas de lien à générer.
									</p>
								{/if}
							</form>
						</details>
					</li>
				{/each}
			</ul>
		{/if}

		{#if resetError}
			<p class="mt-3 text-sm text-red-600">{resetError}</p>
		{/if}

		{#if resetLink}
			<div class="mt-4 rounded-md border border-emerald-300 bg-emerald-50 p-3 text-sm">
				<p class="text-emerald-800">
					Lien de réinitialisation pour <span class="font-medium">{resetTarget}</span> — copie-le et envoie-le par le canal de ton choix :
				</p>
				<div class="mt-2 flex items-center gap-2">
					<code class="flex-1 break-all rounded bg-slate-900 px-2 py-1 text-white">{resetLink}</code>
					<button
						class="rounded-md border border-slate-300 px-2 py-1 text-xs hover:bg-white"
						onclick={() => copy(resetLink)}
					>
						Copier
					</button>
				</div>
			</div>
		{/if}
	</section>

	<section class="rounded-lg border border-slate-200 bg-white p-4" class:opacity-50={!keySaved}>
		<h2 class="mb-3 text-sm font-medium uppercase tracking-wide text-slate-500">
			Créer un foyer
		</h2>

		<form class="space-y-4" onsubmit={onCreateFoyer}>
			<label class="block">
				<span class="text-sm font-medium">Étage</span>
				<select
					bind:value={createFloor}
					disabled={!keySaved}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				>
					<option value="rdc">Rez-de-chaussée</option>
					<option value="1er">1er étage</option>
				</select>
			</label>

			<label class="block">
				<span class="text-sm font-medium">Nom du foyer</span>
				<input
					type="text"
					required
					placeholder="Famille Dupont"
					bind:value={createName}
					disabled={!keySaved}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				/>
			</label>

			<label class="block">
				<span class="text-sm font-medium">Tantièmes</span>
				<input
					type="number"
					min="0"
					step="1"
					required
					bind:value={createParts}
					disabled={!keySaved}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				/>
			</label>

			<fieldset class="rounded border border-slate-200 p-3">
				<legend class="px-1 text-xs uppercase tracking-wide text-slate-500">Membre initial</legend>
				<p class="mb-2 text-xs text-slate-500">
					Si un compte Firebase existe pour cet email, il est réutilisé. Sinon il est créé.
				</p>
				<label class="block">
					<span class="text-sm font-medium">Email</span>
					<input
						type="email"
						required
						autocomplete="off"
						bind:value={createMemberEmail}
						disabled={!keySaved}
						class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
					/>
				</label>
				<label class="mt-3 block">
					<span class="text-sm font-medium">Nom affiché</span>
					<input
						type="text"
						required
						bind:value={createMemberName}
						disabled={!keySaved}
						class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
					/>
				</label>
			</fieldset>

			{#if createError}
				<p role="alert" aria-live="assertive" class="text-sm text-red-600">{createError}</p>
			{/if}

			<button
				type="submit"
				disabled={createSubmitting || !keySaved}
				class="w-full rounded-md bg-slate-900 py-2 font-medium text-white hover:bg-slate-800 disabled:opacity-50"
			>
				{createSubmitting ? 'Création…' : 'Créer le foyer'}
			</button>
		</form>

		{#if createResult}
			<div class="mt-6 rounded-md border border-emerald-300 bg-emerald-50 p-3 text-sm">
				<p class="font-medium text-emerald-800">
					Foyer créé : {createResult.foyer.name} ({createResult.foyer.floor})
				</p>
				<p class="text-emerald-700">id : {createResult.foyer.id}</p>
				{#if createResult.reset_link}
					<div class="mt-2 rounded bg-white p-2">
						<p class="text-xs uppercase tracking-wide text-slate-500">
							Lien de mot de passe à transmettre au membre — usage unique :
						</p>
						<div class="mt-1 flex items-center gap-2">
							<code class="flex-1 break-all rounded bg-slate-900 px-2 py-1 text-white">
								{createResult.reset_link}
							</code>
							<button
								class="rounded-md border border-slate-300 px-2 py-1 text-xs hover:bg-slate-50"
								onclick={() => copy(createResult?.reset_link ?? '')}
							>
								Copier
							</button>
						</div>
					</div>
				{:else}
					<p class="mt-1 text-emerald-700">
						Compte Firebase déjà existant — pas de lien à générer.
					</p>
				{/if}
			</div>
		{/if}
	</section>

	<section class="mt-8 rounded-lg border border-slate-200 bg-white p-4" class:opacity-50={!keySaved}>
		<h2 class="mb-3 text-sm font-medium uppercase tracking-wide text-slate-500">
			Importer un CSV
		</h2>
		<p class="mb-3 text-xs text-slate-500">
			Reprends ton tableur historique. Chaque ligne avec
			<em>Item, Date, Total, Charge RDC, Charge 1er, Paiement complet=TRUE</em> est
			créée ou mise à jour (clé&nbsp;: nom + date). La répartition originale ("50/50",
			"prorata", "tantieme") est préservée dans la note&nbsp;; les parts sont stockées
			telles quelles dans la base.
		</p>

		<form class="space-y-3" onsubmit={onImportSubmit}>
			<label class="block">
				<span class="text-sm font-medium">Fichier CSV</span>
				<input
					type="file"
					accept=".csv,text/csv"
					onchange={onImportFileChange}
					disabled={!keySaved}
					class="mt-1 block w-full text-sm file:mr-3 file:rounded-md file:border-0 file:bg-slate-900 file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-white hover:file:bg-slate-800"
				/>
			</label>

			<label class="block">
				<span class="text-sm font-medium">Foyer payeur (par défaut, appliqué à toutes les lignes)</span>
				<select
					bind:value={importPayerFoyerId}
					disabled={!keySaved}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				>
					<option value="" disabled>— Choisir —</option>
					{#each foyersList as f (f.id)}
						<option value={f.id}>{f.name} ({f.floor})</option>
					{/each}
				</select>
				<span class="mt-1 block text-xs text-slate-500">
					Le format historique ne tracke pas le payeur ; tu pourras corriger
					ligne à ligne après l'import.
				</span>
			</label>

			{#if importError}
				<p role="alert" aria-live="assertive" class="text-sm text-red-600">{importError}</p>
			{/if}

			<button
				type="submit"
				disabled={importSubmitting || !keySaved || !importFile || !importPayerFoyerId}
				class="rounded-md bg-slate-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-50"
			>
				{importSubmitting ? 'Import…' : 'Importer'}
			</button>
		</form>

		{#if importSummary}
			<div class="mt-4 rounded-md border border-emerald-300 bg-emerald-50 p-3 text-sm">
				<p class="font-medium text-emerald-800">Import terminé</p>
				<dl class="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-xs text-emerald-900">
					<dt>Lignes lues</dt>
					<dd class="text-right font-mono">{importSummary.processed}</dd>
					<dt>Créées</dt>
					<dd class="text-right font-mono">{importSummary.created}</dd>
					<dt>Mises à jour</dt>
					<dd class="text-right font-mono">{importSummary.updated}</dd>
					<dt>Ignorées</dt>
					<dd class="text-right font-mono">{importSummary.skipped}</dd>
					{#if importSummary.errors.length > 0}
						<dt>Erreurs</dt>
						<dd class="text-right font-mono text-red-700">{importSummary.errors.length}</dd>
					{/if}
				</dl>

				{#if Object.keys(importSummary.skip_reasons).length > 0}
					<details class="mt-3 rounded bg-white p-2 text-xs text-slate-700" open>
						<summary class="cursor-pointer font-medium">
							Détail des lignes ignorées
						</summary>
						<ul class="mt-2 space-y-1">
							{#each Object.entries(importSummary.skip_reasons) as [reason, count] (reason)}
								<li class="flex items-baseline justify-between gap-3">
									<span>{skipReasonLabel(reason)}</span>
									<span class="font-mono text-slate-500">{count}</span>
								</li>
							{/each}
						</ul>
					</details>
				{/if}

				{#if importSummary.errors.length > 0}
					<details class="mt-3 rounded bg-white p-2 text-xs text-slate-700">
						<summary class="cursor-pointer">Détail des erreurs</summary>
						<ul class="mt-2 space-y-1">
							{#each importSummary.errors as err (err.line)}
								<li>
									<span class="font-mono">L{err.line}</span>
									{#if err.item}— <em>{err.item}</em>{/if}
									: {err.message}
								</li>
							{/each}
						</ul>
					</details>
				{/if}
			</div>
		{/if}
	</section>
</main>
