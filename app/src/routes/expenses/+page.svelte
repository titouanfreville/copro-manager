<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError } from '$lib/api';
	import { authState } from '$lib/auth';
	import Button from '$lib/components/Button.svelte';
	import Fab from '$lib/components/Fab.svelte';
	import IconButton from '$lib/components/IconButton.svelte';
	import { computeBalance } from '$lib/balance';
	import { formatDate } from '$lib/format';
	import {
		ATTACHMENT_ACCEPT,
		ATTACHMENT_MAX_BYTES,
		ATTACHMENT_MAX_PER_EXPENSE,
		attachFile,
		createExpense,
		deleteAttachment,
		deleteExpense,
		getAttachmentDownloadUrl,
		isImageAttachment,
		materializeRecurring,
		updateExpense
	} from '$lib/expenses';
	import {
		subscribeAllAttachments,
		subscribeCategories,
		subscribeExpenses,
		subscribeFoyers,
		subscribeSettlements,
		subscribeTemplates,
		type ExpenseAttachment
	} from '$lib/live';
	import { createCategory } from '$lib/categories';
	import {
		createSettlement,
		deleteSettlement,
		updateSettlement
	} from '$lib/settlements';
	import type {
		Attachment,
		Category,
		CreateSettlementInput,
		DistributionMode,
		Expense,
		ExpenseTemplate,
		Foyer,
		Settlement
	} from '$lib/api';

	// ─────────────────────────────────────────────────────────────
	// State
	// ─────────────────────────────────────────────────────────────
	let foyers = $state<Foyer[]>([]);
	let categories = $state<Category[]>([]);
	let expenses = $state<Expense[]>([]);
	let templates = $state<ExpenseTemplate[]>([]);
	let settlements = $state<Settlement[]>([]);
	let liveError = $state('');
	let foyersReady = $state(false);
	let categoriesReady = $state(false);
	let expensesReady = $state(false);
	let live = $derived(foyersReady && categoriesReady && expensesReady);

	// Create-expense form (kept lifted; modal toggles its visibility)
	let modalOpen = $state(false);
	let name = $state('');
	let amountEuros = $state('');
	let date = $state(new Date().toISOString().slice(0, 10));
	let paymentDate = $state('');
	let settled = $state(false);
	let settledAt = $state('');
	let payerFoyerId = $state('');
	let categoryId = $state('');
	let mode = $state<DistributionMode>('equal');
	// Manuel sub-mode: 'percent' = proportional slider (default), 'exact' = literal € amounts.
	let customSubMode = $state<'percent' | 'exact'>('percent');
	// Stored as basis points (0–10000) so 2-decimal % is exact integer math.
	// e.g. 5000 = 50.00 %, 3333 = 33.33 %.
	let rdcPercentBp = $state(5000);
	const BP_TOTAL = 10000;
	let shareRDCEuros = $state('');
	let share1erEuros = $state('');
	let note = $state('');
	let creating = $state(false);
	let createError = $state('');
	// When set, the modal is in EDIT mode targeting this expense ID;
	// otherwise it's CREATE mode.
	let editingId = $state<string | null>(null);
	let isEditing = $derived(editingId !== null);
	// Per-row "deleting…" tracker so the UI can show a busy state on the
	// specific row being removed without freezing the whole page.
	let deletingId = $state<string | null>(null);

	// Chooser sheet (Vide / Depuis un modèle) and template picker sheet —
	// shown one at a time when the user taps the FAB.
	let chooserOpen = $state(false);
	let pickerOpen = $state(false);

	// ─── Filters + search ──────────────────────────────────────────
	type FilterType = 'all' | 'expenses' | 'settlements';
	type FilterState = 'all' | 'pending' | 'settled' | 'paired';
	type LedgerFilters = {
		from: string;
		to: string;
		categories: string[];
		payers: string[];
		modes: DistributionMode[];
		type: FilterType;
		state: FilterState;
	};
	const emptyFilters: LedgerFilters = {
		from: '',
		to: '',
		categories: [],
		payers: [],
		modes: [],
		type: 'all',
		state: 'all'
	};
	let filters = $state<LedgerFilters>({ ...emptyFilters });
	let searchQuery = $state('');
	let debouncedQuery = $state('');
	let filtersOpen = $state(false);

	// Debounce typing → filter pipeline so the user doesn't see flicker on
	// every keystroke.
	$effect(() => {
		const q = searchQuery;
		const t = setTimeout(() => (debouncedQuery = q), 250);
		return () => clearTimeout(t);
	});

	// One-shot hydration from the URL on the first browser render. We read
	// `window.location` (not the reactive $page store) so writing back via
	// `history.replaceState` doesn't trip the effect into a loop.
	$effect(() => {
		if (typeof window === 'undefined') return;
		const sp = new URL(window.location.href).searchParams;
		filters = {
			from: sp.get('from') ?? '',
			to: sp.get('to') ?? '',
			categories: splitParam(sp.get('cat')),
			payers: splitParam(sp.get('payer')),
			modes: splitParam(sp.get('mode')) as DistributionMode[],
			type: parseFilterType(sp.get('type')),
			state: parseFilterState(sp.get('state'))
		};
		searchQuery = sp.get('q') ?? '';
		// Run-once: deps are intentionally not touched, this fires only on
		// component mount because the body has no reactive reads.
	});

	// Mirror filter state back to the URL so the view is shareable. Writes
	// via raw `history.replaceState` to bypass SvelteKit navigation —
	// reading `window.location` keeps this effect independent of $page.
	$effect(() => {
		if (typeof window === 'undefined') return;
		const params = new URLSearchParams();
		if (filters.from) params.set('from', filters.from);
		if (filters.to) params.set('to', filters.to);
		if (filters.categories.length) params.set('cat', filters.categories.join(','));
		if (filters.payers.length) params.set('payer', filters.payers.join(','));
		if (filters.modes.length) params.set('mode', filters.modes.join(','));
		if (filters.type !== 'all') params.set('type', filters.type);
		if (filters.state !== 'all') params.set('state', filters.state);
		if (debouncedQuery) params.set('q', debouncedQuery);
		const url = new URL(window.location.href);
		const next = params.toString();
		if (url.search.replace(/^\?/, '') !== next) {
			url.search = next;
			window.history.replaceState(null, '', url);
		}
	});

	function splitParam(raw: string | null): string[] {
		if (!raw) return [];
		return raw.split(',').filter(Boolean);
	}
	function parseFilterType(raw: string | null): FilterType {
		return raw === 'expenses' || raw === 'settlements' ? raw : 'all';
	}
	function parseFilterState(raw: string | null): FilterState {
		return raw === 'pending' || raw === 'settled' || raw === 'paired' ? raw : 'all';
	}

	function toggleInArray<T>(arr: T[], value: T): T[] {
		return arr.includes(value) ? arr.filter((x) => x !== value) : [...arr, value];
	}
	function resetFilters() {
		filters = { ...emptyFilters };
		searchQuery = '';
	}
	let activeFilterCount = $derived.by(() => {
		let n = 0;
		if (filters.from) n++;
		if (filters.to) n++;
		n += filters.categories.length;
		n += filters.payers.length;
		n += filters.modes.length;
		if (filters.type !== 'all') n++;
		if (filters.state !== 'all') n++;
		if (debouncedQuery) n++;
		return n;
	});

	// Inline category creator (when none of the existing options matches).
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

	// ─── Settlement modal state ──────────────────────────────────
	let settlementModalOpen = $state(false);
	let editingSettlementId = $state<string | null>(null);
	let isEditingSettlement = $derived(editingSettlementId !== null);
	let savingSettlement = $state(false);
	let settlementError = $state('');
	let deletingSettlementId = $state<string | null>(null);
	let settleFromFoyerId = $state('');
	let settleToFoyerId = $state('');
	let settleAmountEuros = $state('');
	let settleDate = $state(new Date().toISOString().slice(0, 10));
	let settleNote = $state('');
	let settleLinkedExpenseIds = $state<string[]>([]);
	// Inline guard: from === to is a usage error; the API also rejects.
	let settleSameFoyer = $derived(
		settleFromFoyerId !== '' && settleFromFoyerId === settleToFoyerId
	);

	// ─── Attachments ──────────────────────────────────────────────
	// `pendingFiles`: queued File objects in the modal; uploaded after the
	// expense is saved so the create flow can include attachments in one
	// shot. `uploadingFile` is the index currently being uploaded.
	let pendingFiles = $state<File[]>([]);
	let uploadingFileIdx = $state<number | null>(null);
	let uploadError = $state('');
	// Per-row drawer expand state, keyed by expense ID.
	let expandedRows = $state<Record<string, boolean>>({});
	// Per-row "deleting attachment" tracker, keyed by attachment ID.
	let deletingAttachmentId = $state<string | null>(null);
	// Cache of resolved signed download URLs, keyed by attachment ID. The
	// API issues 10-min URLs; we keep a 30-second safety margin so we
	// don't hand out an about-to-expire URL.
	type CachedUrl = { url: string; expiresAtMs: number };
	let downloadUrlCache = $state<Record<string, CachedUrl>>({});
	// Attachments live in a Firestore subcollection now (one doc per file
	// under each expense). One collectionGroup listener delivers all of
	// them; we group by expense_id and merge into the rendered ledger rows
	// via $derived.
	let attachmentsByExpense = $state<Record<string, ExpenseAttachment[]>>({});
	// Effective row used for rendering: expense + its attachments.
	let renderedExpenses = $derived(
		expenses.map((e) => ({
			...e,
			attachments: attachmentsByExpense[e.id] ?? e.attachments ?? []
		}))
	);
	// Aborter shared by the in-flight upload loop so the user can bail.
	let uploadAborter = $state<AbortController | null>(null);

	function attachmentKey(expenseId: string, attId: string): string {
		return `${expenseId}/${attId}`;
	}

	async function resolveDownloadUrl(expenseId: string, attId: string): Promise<string> {
		const key = attachmentKey(expenseId, attId);
		const cached = downloadUrlCache[key];
		const safetyMs = 30_000;
		if (cached && cached.expiresAtMs - safetyMs > Date.now()) {
			return cached.url;
		}
		const { url, expiresAt } = await getAttachmentDownloadUrl(expenseId, attId);
		// Spread-style assignment so Svelte 5's fine-grained reactivity picks
		// up the cache update on every read, not just the first.
		downloadUrlCache = {
			...downloadUrlCache,
			[key]: { url, expiresAtMs: new Date(expiresAt).getTime() }
		};
		return url;
	}

	function toggleRowDrawer(expenseId: string) {
		expandedRows[expenseId] = !expandedRows[expenseId];
	}

	async function onViewAttachment(expense: Expense, att: Attachment) {
		// Open the popup synchronously inside the click handler — Safari and
		// most pop-up blockers lose the user-gesture context across `await`,
		// so a `window.open` after the resolveDownloadUrl Promise resolves
		// would be silently swallowed. The placeholder window navigates to
		// the signed URL once it resolves.
		const popup = window.open('about:blank', '_blank', 'noopener,noreferrer');
		try {
			const url = await resolveDownloadUrl(expense.id, att.id);
			if (popup) {
				popup.location.href = url;
			} else {
				// Pop-up blocked. Fall back to in-tab navigation so the user
				// still gets the file.
				window.location.href = url;
			}
		} catch (err) {
			popup?.close();
			liveError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		}
	}

	async function onDeleteAttachment(expense: Expense, att: Attachment) {
		if (deletingAttachmentId) return;
		const ok = window.confirm(
			`Supprimer la pièce jointe « ${att.original_filename || att.id} » ?`
		);
		if (!ok) return;
		deletingAttachmentId = att.id;
		try {
			await deleteAttachment(expense.id, att.id);
			const next = { ...downloadUrlCache };
			delete next[attachmentKey(expense.id, att.id)];
			downloadUrlCache = next;
		} catch (err) {
			liveError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			deletingAttachmentId = null;
		}
	}

	function onPickFiles(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const picked = input.files;
		if (!picked || picked.length === 0) return;
		uploadError = '';
		const accepted = ATTACHMENT_ACCEPT.split(',');
		const next: File[] = pendingFiles.slice();
		// When editing, count the attachments already saved on the expense
		// so we don't blow past the per-expense cap by adding too many
		// pendings.
		const existing = editingId ? (attachmentsByExpense[editingId]?.length ?? 0) : 0;
		const errors: string[] = [];
		for (const file of Array.from(picked)) {
			if (next.length + existing >= ATTACHMENT_MAX_PER_EXPENSE) {
				errors.push(`Maximum ${ATTACHMENT_MAX_PER_EXPENSE} pièces jointes par dépense.`);
				break;
			}
			// HEIC/HEIF files often arrive with `file.type === ''` on iOS —
			// fall back to the extension before rejecting outright.
			const lowered = file.name.toLowerCase();
			const heicByExt = lowered.endsWith('.heic') || lowered.endsWith('.heif');
			const typeOk = accepted.includes(file.type) || (file.type === '' && heicByExt);
			if (!typeOk) {
				errors.push(`Type non supporté : ${file.name} (${file.type || 'inconnu'}).`);
				continue;
			}
			if (file.size > ATTACHMENT_MAX_BYTES) {
				errors.push(`Trop volumineux : ${file.name} (max 10 Mo).`);
				continue;
			}
			next.push(file);
		}
		// Accumulate errors instead of overwriting — surface every reason
		// the user's pick was partially rejected.
		if (errors.length) uploadError = errors.join(' ');
		pendingFiles = next;
		// Reset the input so picking the same file twice fires the change event.
		input.value = '';
	}

	function removePendingFile(idx: number) {
		pendingFiles = pendingFiles.filter((_, i) => i !== idx);
	}

	function formatFileSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} o`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} Ko`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} Mo`;
	}

	async function uploadPendingFiles(expenseId: string) {
		uploadAborter = new AbortController();
		try {
			// Process sequentially with per-file isolation: if file N fails
			// we keep files 1..N-1 marked as uploaded (they really did
			// land), drop them from the queue, and re-throw with the
			// remaining queue still in `pendingFiles` so the user can
			// retry just the failures rather than re-uploading the
			// successes.
			while (pendingFiles.length > 0) {
				uploadingFileIdx = 0;
				const file = pendingFiles[0];
				try {
					await attachFile(expenseId, file, { signal: uploadAborter.signal });
				} catch (err) {
					// Leave the failing file at the head of the queue so
					// the user sees what didn't make it.
					throw err;
				}
				// Success — drop the head and continue.
				pendingFiles = pendingFiles.slice(1);
			}
		} finally {
			uploadingFileIdx = null;
			uploadAborter = null;
		}
	}

	function cancelUpload() {
		uploadAborter?.abort();
	}

	function setRdcPercentBp(v: number) {
		if (!Number.isFinite(v)) return;
		rdcPercentBp = Math.max(0, Math.min(BP_TOTAL, Math.round(v)));
	}
	/** Set RDC% from a string/number expressed in % (e.g. "33.33" or 33.33). */
	function setRdcPercentFromInput(v: unknown) {
		if (v === null || v === undefined || v === '') return;
		const n = typeof v === 'number' ? v : Number(String(v).replace(',', '.'));
		if (!Number.isFinite(n)) return;
		setRdcPercentBp(Math.round(n * 100));
	}
	function formatPct(bp: number): string {
		return (bp / 100).toFixed(2);
	}

	// ─────────────────────────────────────────────────────────────
	// Effects
	// ─────────────────────────────────────────────────────────────
	$effect(() => {
		if ($authState.status === 'signed-out') {
			goto('/login');
			return;
		}
		if ($authState.status !== 'signed-in') return;

		liveError = '';
		const onErr = (err: Error) => {
			liveError = err.message || String(err);
			// Mark every stream as "ready" on error so the UI escapes the
			// "Connexion…" placeholder and the user sees the diagnostic
			// banner instead of an infinite spinner. Permission-denied
			// (rules misconfigured, signed-out mid-flight, etc.) is the
			// canonical case.
			foyersReady = true;
			categoriesReady = true;
			expensesReady = true;
		};

		const unsubs = [
			subscribeFoyers((rows) => {
				foyers = rows;
				foyersReady = true;
				if (!payerFoyerId && rows[0]) payerFoyerId = rows[0].id;
			}, onErr),
			subscribeCategories((rows) => {
				categories = rows;
				categoriesReady = true;
				if (!categoryId && rows[0]) categoryId = rows[0].id;
			}, onErr),
			subscribeExpenses((rows) => {
				expenses = rows;
				expensesReady = true;
			}, onErr),
			subscribeTemplates((rows) => {
				templates = rows;
			}, onErr),
			subscribeAllAttachments((atts) => {
				const grouped: Record<string, ExpenseAttachment[]> = {};
				for (const a of atts) {
					(grouped[a.expense_id] ||= []).push(a);
				}
				attachmentsByExpense = grouped;
			}, onErr),
			subscribeSettlements((rows) => {
				settlements = rows;
			}, onErr)
		];

		// Lazy materialization: fire-and-forget. The daily Cloud Scheduler
		// cron is the primary path; this catches "I just created a template,
		// run it now" without waiting for tomorrow's job. Idempotent
		// server-side, so duplicate calls are no-ops.
		materializeRecurring().catch((err) => {
			// Don't surface to liveError — silent backstop. Log only.
			console.warn('materialize-recurring failed', err);
		});

		return () => {
			unsubs.forEach((u) => u());
			foyersReady = false;
			categoriesReady = false;
			expensesReady = false;
		};
	});

	// When the picked category has a default mode, auto-select it.
	$effect(() => {
		const c = categories.find((x) => x.id === categoryId);
		if (c?.default_distribution_mode) mode = c.default_distribution_mode;
	});

	// Lock body scroll while the modal sheet is open.
	$effect(() => {
		if (typeof document === 'undefined') return;
		if (modalOpen) {
			document.body.style.overflow = 'hidden';
			return () => {
				document.body.style.overflow = '';
			};
		}
	});

	// ─────────────────────────────────────────────────────────────
	// Loaders
	// ─────────────────────────────────────────────────────────────
	// ─────────────────────────────────────────────────────────────
	// Formatters & helpers
	// ─────────────────────────────────────────────────────────────
	const eurFormatter = new Intl.NumberFormat('fr-FR', {
		style: 'currency',
		currency: 'EUR',
		minimumFractionDigits: 2
	});
	const monthFormatter = new Intl.DateTimeFormat('fr-FR', {
		month: 'long',
		year: 'numeric',
		timeZone: 'UTC'
	});
	const monAbbrFormatter = new Intl.DateTimeFormat('fr-FR', {
		month: 'short',
		timeZone: 'UTC'
	});

	function formatEUR(cents: number): string {
		return eurFormatter.format(cents / 100);
	}
	// Accepts whatever bind:value coerces an `<input type="number">` to —
	// strings (typed manually), numbers (after coercion), null/undefined
	// (cleared), and the literal '' that some browsers emit.
	function eurosToCents(v: unknown): number {
		if (v === null || v === undefined || v === '') return NaN;
		const n = typeof v === 'number' ? v : Number(String(v).replace(',', '.'));
		if (!Number.isFinite(n) || n < 0) return NaN;
		return Math.round(n * 100);
	}
	function formatMonth(yyyymm: string): string {
		if (!yyyymm || yyyymm.length < 7) return '—';
		const [y, m] = yyyymm.split('-').map(Number);
		if (!Number.isFinite(y) || !Number.isFinite(m)) return '—';
		return monthFormatter.format(new Date(Date.UTC(y, m - 1, 1)));
	}
	function dayParts(iso: string) {
		// Pull the calendar date out of the ISO string itself (`YYYY-MM-DD…`)
		// rather than parsing into a Date and reading UTC components — a
		// Paris-evening expense saved as `…T23:00:00+02:00` is still the
		// SAME calendar day for the user, but `getUTCDate()` would shift
		// it back one. The first 10 chars are the canonical date the user
		// typed regardless of the surrounding TZ noise.
		const datePart = iso.slice(0, 10);
		const [, mm, dd] = datePart.split('-');
		const formatted = monAbbrFormatter
			.format(new Date(`${datePart}T00:00:00Z`))
			.replace(/\.$/, '');
		return {
			num: dd ?? '',
			mon: mm ? formatted : ''
		};
	}

	// Each category gets a stable monogram + tone pair so the eye learns
	// the rhythm. Unknown categories fall back to a neutral pair.
	type CatStyle = { mono: string; tone: string; tint: string };
	const CATEGORY_STYLES: Record<string, CatStyle> = {
		eau: { mono: 'EA', tone: '#3F6B82', tint: '#E1ECF2' },
		electricite: { mono: 'ÉL', tone: '#A37423', tint: '#F4EAD3' },
		'taxe-fonciere': { mono: 'TF', tone: '#7A5E87', tint: '#ECE3F1' },
		travaux: { mono: 'TR', tone: '#9E6A4D', tint: '#F1E3D8' },
		assurance: { mono: 'AS', tone: '#5A7461', tint: '#E6EDE5' },
		syndic: { mono: 'SY', tone: '#4A4744', tint: '#E8E4E0' }
	};
	function categoryStyle(id: string): CatStyle {
		return (
			CATEGORY_STYLES[id] ?? {
				mono: id.slice(0, 2).toUpperCase(),
				tone: '#7A7268',
				tint: '#ECE8E0'
			}
		);
	}

	function categoryName(id: string): string {
		return categories.find((c) => c.id === id)?.name ?? id;
	}

	function modeLabel(m: DistributionMode): string {
		return m === 'equal' ? 'Égalité' : m === 'tantiemes' ? 'Tantièmes' : 'Personnalisé';
	}
	function modeGlyph(m: DistributionMode): string {
		return m === 'equal' ? '½' : m === 'tantiemes' ? '‰' : '✱';
	}
	function foyerLabel(f: Foyer): string {
		return f.floor === 'rdc' ? 'RDC' : '1ᵉʳ';
	}

	// ─────────────────────────────────────────────────────────────
	// Derived
	// ─────────────────────────────────────────────────────────────
	let rdcFoyer = $derived(foyers.find((f) => f.floor === 'rdc'));
	let premierFoyer = $derived(foyers.find((f) => f.floor === '1er'));

	// Net balance from RDC's perspective. Positive → 1er owes RDC.
	// Shared with the global chrome via $lib/balance — settled rows are
	// excluded both places.
	let balance = $derived(computeBalance(expenses, settlements, foyers));

	// ─── Ledger merge: expenses + settlements ──────────────────
	// LedgerRow is a discriminated union so the row template can branch on
	// `kind`. Both row types carry a `date` and `created_at` for sort
	// stability across kinds.
	type LedgerRow =
		| {
				kind: 'expense';
				date: string;
				created_at: string;
				id: string;
				expense: Expense & { attachments: Attachment[] };
		  }
		| {
				kind: 'settlement';
				date: string;
				created_at: string;
				id: string;
				settlement: Settlement;
		  };

	let groupedExpenses = $derived.by(() => {
		const map = new Map<string, Expense[]>();
		// Iterate `renderedExpenses` so each row carries its merged
		// attachments (from the subcollection listener) for inline
		// thumbnails / counts.
		for (const e of renderedExpenses) {
			const key = e.date.slice(0, 7);
			const arr = map.get(key) ?? [];
			arr.push(e);
			map.set(key, arr);
		}
		return Array.from(map.entries());
	});

	// Flat merged rows BEFORE filtering — keeps filter logic decoupled
	// from grouping and makes the unfiltered total available for empty
	// states ("filtres actifs, aucune ligne").
	let mergedRows = $derived.by(() => {
		const rows: LedgerRow[] = [];
		for (const e of renderedExpenses) {
			rows.push({
				kind: 'expense',
				date: e.date,
				created_at: e.created_at,
				id: e.id,
				expense: e
			});
		}
		for (const s of settlements) {
			rows.push({
				kind: 'settlement',
				date: s.date,
				created_at: s.created_at,
				id: s.id,
				settlement: s
			});
		}
		rows.sort((a, b) => {
			if (a.date !== b.date) return b.date.localeCompare(a.date);
			return b.created_at.localeCompare(a.created_at);
		});
		return rows;
	});

	let filteredRows = $derived.by(() =>
		applyFilters(mergedRows, filters, debouncedQuery, linkedSettlementByExpense)
	);

	let groupedRows = $derived.by(() => {
		const map = new Map<string, LedgerRow[]>();
		for (const r of filteredRows) {
			const key = r.date.slice(0, 7);
			const arr = map.get(key) ?? [];
			arr.push(r);
			map.set(key, arr);
		}
		return Array.from(map.entries());
	});

	// Pure filter pipeline — extracted so the typing is explicit and so
	// any future test harness can hit it without booting the page.
	function applyFilters(
		rows: LedgerRow[],
		f: LedgerFilters,
		q: string,
		linkedMap: Record<string, Settlement>
	): LedgerRow[] {
		const numericQ = parseNumericQuery(q);
		const textQ = q.trim().toLocaleLowerCase('fr');
		const fromTs = f.from ? Date.parse(f.from + 'T00:00:00') : NaN;
		const toTs = f.to ? Date.parse(f.to + 'T23:59:59') : NaN;
		const out: LedgerRow[] = [];
		for (const r of rows) {
			// Type
			if (f.type === 'expenses' && r.kind !== 'expense') continue;
			if (f.type === 'settlements' && r.kind !== 'settlement') continue;
			// Date range (uses the row.date which is ISO YYYY-MM-DD…)
			const rowTs = Date.parse(r.date);
			if (!Number.isNaN(fromTs) && rowTs < fromTs) continue;
			if (!Number.isNaN(toTs) && rowTs > toTs) continue;
			// Filters that only apply to expense rows
			if (r.kind === 'expense') {
				const e = r.expense;
				if (f.categories.length && !f.categories.includes(e.category_id)) continue;
				if (f.payers.length && !f.payers.includes(e.payer_foyer_id)) continue;
				if (f.modes.length && !f.modes.includes(e.distribution_mode)) continue;
				if (f.state === 'pending' && !e.amount_pending) continue;
				if (f.state === 'settled' && !e.settled) continue;
				if (f.state === 'paired' && !linkedMap[e.id]) continue;
			} else {
				// State filter on settlement rows: only "all" makes sense;
				// pending/settled/paired are expense-only — drop settlements
				// when those are active so the listing stays coherent.
				if (f.state !== 'all') continue;
				if (f.categories.length || f.payers.length || f.modes.length) continue;
			}
			// Search (numeric ∪ text)
			if (q.trim()) {
				if (!matchesSearch(r, numericQ, textQ)) continue;
			}
			out.push(r);
		}
		return out;
	}

	function parseNumericQuery(q: string): number | null {
		const trimmed = q.trim().replace(/\s/g, '');
		if (!trimmed) return null;
		const n = Number(trimmed.replace(',', '.'));
		return Number.isFinite(n) ? n : null;
	}

	function matchesSearch(r: LedgerRow, numericQ: number | null, textQ: string): boolean {
		if (numericQ !== null) {
			const cents = Math.round(numericQ * 100);
			if (r.kind === 'expense') {
				const e = r.expense;
				if (e.amount_cents === cents) return true;
				if (e.share_rdc_cents === cents) return true;
				if (e.share_1er_cents === cents) return true;
			} else {
				if (r.settlement.amount_cents === cents) return true;
			}
		}
		if (textQ) {
			if (r.kind === 'expense') {
				const e = r.expense;
				const hay = (e.name + ' ' + (e.note ?? '')).toLocaleLowerCase('fr');
				if (hay.includes(textQ)) return true;
			} else {
				const hay = (r.settlement.note ?? '').toLocaleLowerCase('fr');
				if (hay.includes(textQ)) return true;
			}
		}
		return false;
	}

	// Reverse index: expense_id → linking Settlement (or null). Used by the
	// per-row pairing chip and to filter the linked-expenses picker.
	let linkedSettlementByExpense = $derived.by(() => {
		const map: Record<string, Settlement> = {};
		for (const s of settlements) {
			for (const eid of s.expense_ids ?? []) map[eid] = s;
		}
		return map;
	});

	let monthCount = $derived(groupedRows.length);
	let totalCount = $derived(expenses.length + settlements.length);
	let pendingCount = $derived(expenses.filter((e) => e.amount_pending).length);

	// Live € preview for the Manuel/Pourcentage panel.
	let amountCentsPreview = $derived.by(() => {
		const c = eurosToCents(amountEuros);
		return Number.isFinite(c) && c > 0 ? c : 0;
	});
	let percentRdcCents = $derived(Math.round((amountCentsPreview * rdcPercentBp) / BP_TOTAL));
	let percent1erCents = $derived(amountCentsPreview - percentRdcCents);

	// ─────────────────────────────────────────────────────────────
	// Actions
	// ─────────────────────────────────────────────────────────────
	function resetForm() {
		editingId = null;
		pendingTemplateId = null;
		name = '';
		amountEuros = '';
		date = new Date().toISOString().slice(0, 10);
		paymentDate = '';
		settled = false;
		settledAt = '';
		mode = 'equal';
		customSubMode = 'percent';
		rdcPercentBp = BP_TOTAL / 2;
		shareRDCEuros = '';
		share1erEuros = '';
		note = '';
		createError = '';
		pendingFiles = [];
		uploadingFileIdx = null;
		uploadError = '';
	}

	function openChooser() {
		chooserOpen = true;
	}
	function closeChooser() {
		chooserOpen = false;
	}
	function chooseBlank() {
		chooserOpen = false;
		openCreate();
	}
	function chooseFromTemplate() {
		chooserOpen = false;
		// If no templates yet, route to /templates so the user can add one.
		if (templates.length === 0) {
			goto('/templates');
			return;
		}
		pickerOpen = true;
	}
	function closePicker() {
		pickerOpen = false;
	}
	function applyTemplate(t: ExpenseTemplate) {
		pickerOpen = false;
		resetForm();
		name = t.name;
		amountEuros = t.amount_default_cents > 0 ? (t.amount_default_cents / 100).toFixed(2) : '';
		payerFoyerId = t.payer_foyer_id;
		categoryId = t.category_id;
		mode = t.distribution_mode;
		note = t.note ?? '';
		if (t.distribution_mode === 'custom') {
			customSubMode = 'exact';
			shareRDCEuros = t.share_rdc_cents
				? (t.share_rdc_cents / 100).toFixed(2)
				: '';
			share1erEuros = t.share_1er_cents
				? (t.share_1er_cents / 100).toFixed(2)
				: '';
		}
		// Stamp the lineage; the create payload carries it through.
		pendingTemplateId = t.id;
		modalOpen = true;
	}
	// Holds the template_id that the next createExpense call should carry.
	// Cleared on resetForm; consumed in onSubmit.
	let pendingTemplateId = $state<string | null>(null);

	function openCreate() {
		resetForm();
		modalOpen = true;
	}

	/** Pre-fill the form from an existing expense and open the modal in
	 *  edit mode. Cents are rendered to 2-decimal € strings; the sub-mode
	 *  defaults to "exact" because the operator is most likely fixing
	 *  literal amounts (CSV-imported rows, rounding artifacts, etc.). */
	function openEdit(exp: Expense) {
		editingId = exp.id;
		name = exp.name;
		// Pending rows have no amount yet — leave the field blank so the
		// user is prompted to type. Otherwise pre-fill from the existing
		// amount.
		amountEuros = exp.amount_pending ? '' : (exp.amount_cents / 100).toFixed(2);
		date = exp.date.slice(0, 10);
		paymentDate = exp.payment_date ? exp.payment_date.slice(0, 10) : '';
		settled = exp.settled;
		settledAt = exp.settled && exp.settled_at ? exp.settled_at.slice(0, 10) : '';
		payerFoyerId = exp.payer_foyer_id;
		categoryId = exp.category_id;
		mode = exp.distribution_mode;
		// For tantiemes / equal we don't show the share fields, but pre-fill
		// the percent slider so a switch to Manuel mid-edit lands on the
		// existing ratio rather than 50/50.
		if (exp.amount_cents > 0) {
			rdcPercentBp = Math.round((exp.share_rdc_cents / exp.amount_cents) * BP_TOTAL);
		} else {
			rdcPercentBp = BP_TOTAL / 2;
		}
		shareRDCEuros = (exp.share_rdc_cents / 100).toFixed(2);
		share1erEuros = (exp.share_1er_cents / 100).toFixed(2);
		// Default to "exact" when editing — the operator is most often
		// adjusting literal amounts on imported rows.
		customSubMode = 'exact';
		note = exp.note ?? '';
		createError = '';
		modalOpen = true;
	}

	function closeCreate() {
		if (creating) return;
		modalOpen = false;
		// Defer reset to next tick so the closing animation doesn't visibly
		// lose the form contents mid-flight.
		setTimeout(resetForm, 220);
	}

	async function onDelete(exp: Expense) {
		if (deletingId) return;
		const confirmed = window.confirm(
			`Supprimer définitivement « ${exp.name || 'cette dépense'} » ?\nCette action est irréversible.`
		);
		if (!confirmed) return;
		deletingId = exp.id;
		try {
			await deleteExpense(exp.id);
			// Live subscription pushes the removal — no manual refresh.
		} catch (err) {
			liveError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			deletingId = null;
		}
	}

	// ─── Settlement actions ─────────────────────────────────────
	function resetSettlementForm() {
		editingSettlementId = null;
		settleFromFoyerId = '';
		settleToFoyerId = '';
		settleAmountEuros = '';
		settleDate = new Date().toISOString().slice(0, 10);
		settleNote = '';
		settleLinkedExpenseIds = [];
		settlementError = '';
	}

	function openSettlement() {
		resetSettlementForm();
		// Pre-fill from the current balance direction. balance.net > 0 →
		// 1er owes RDC, so the natural settlement is from=1er, to=rdc.
		if (balance && balance.net !== 0) {
			if (balance.net > 0) {
				settleFromFoyerId = balance.premier.id;
				settleToFoyerId = balance.rdc.id;
			} else {
				settleFromFoyerId = balance.rdc.id;
				settleToFoyerId = balance.premier.id;
			}
			settleAmountEuros = (Math.abs(balance.net) / 100).toFixed(2);
		}
		settlementModalOpen = true;
	}

	function openSettlementEdit(s: Settlement) {
		resetSettlementForm();
		editingSettlementId = s.id;
		settleFromFoyerId = s.from_foyer_id;
		settleToFoyerId = s.to_foyer_id;
		settleAmountEuros = (s.amount_cents / 100).toFixed(2);
		settleDate = s.date.slice(0, 10);
		settleNote = s.note ?? '';
		settleLinkedExpenseIds = [...(s.expense_ids ?? [])];
		settlementModalOpen = true;
	}

	function closeSettlementModal() {
		if (savingSettlement) return;
		settlementModalOpen = false;
		setTimeout(resetSettlementForm, 220);
	}

	function toggleSettleLinked(expenseId: string) {
		if (settleLinkedExpenseIds.includes(expenseId)) {
			settleLinkedExpenseIds = settleLinkedExpenseIds.filter((id) => id !== expenseId);
		} else {
			settleLinkedExpenseIds = [...settleLinkedExpenseIds, expenseId];
		}
	}

	// Expenses available to link from the modal: not already settled
	// (off-the-books), not pending (no amount yet), and not already linked
	// to a DIFFERENT settlement (the current edit's own links stay
	// available — see `editingSettlementId` check).
	let availableLinkExpenses = $derived.by(() => {
		const out: Expense[] = [];
		for (const e of expenses) {
			if (e.settled || e.amount_pending) continue;
			const linked = linkedSettlementByExpense[e.id];
			if (linked && linked.id !== editingSettlementId) continue;
			out.push(e);
		}
		return out.sort((a, b) => b.date.localeCompare(a.date));
	});

	// Live counter: total debt (in the from→to direction) covered by the
	// currently-checked expenses. Helps the user gauge whether their amount
	// matches the linked-expense sum.
	let settleLinkedCoverage = $derived.by(() => {
		if (!balance) return 0;
		let total = 0;
		for (const id of settleLinkedExpenseIds) {
			const e = expenses.find((x) => x.id === id);
			if (!e) continue;
			// "From" foyer's debt contribution from this expense.
			if (e.payer_foyer_id === settleToFoyerId) {
				if (settleFromFoyerId === balance.rdc.id) total += e.share_rdc_cents;
				else if (settleFromFoyerId === balance.premier.id) total += e.share_1er_cents;
			}
		}
		return total;
	});

	async function onSubmitSettlement(e: SubmitEvent) {
		e.preventDefault();
		if (savingSettlement) return;
		settlementError = '';

		if (settleSameFoyer) {
			settlementError = 'Les foyers « De » et « Vers » doivent être différents.';
			return;
		}
		if (!settleFromFoyerId || !settleToFoyerId) {
			settlementError = 'Sélectionne les deux foyers.';
			return;
		}
		const amountCents = eurosToCents(settleAmountEuros);
		if (!Number.isFinite(amountCents) || amountCents <= 0) {
			settlementError = 'Montant invalide.';
			return;
		}
		if (!settleDate) {
			settlementError = 'Date requise.';
			return;
		}
		const body: CreateSettlementInput = {
			from_foyer_id: settleFromFoyerId,
			to_foyer_id: settleToFoyerId,
			amount_cents: amountCents,
			date: settleDate,
			note: settleNote.trim() || undefined,
			expense_ids: settleLinkedExpenseIds.length ? settleLinkedExpenseIds : undefined
		};
		savingSettlement = true;
		try {
			if (editingSettlementId) {
				await updateSettlement(editingSettlementId, body);
			} else {
				await createSettlement(body);
			}
			settlementModalOpen = false;
			setTimeout(resetSettlementForm, 220);
		} catch (err) {
			settlementError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			savingSettlement = false;
		}
	}

	async function onDeleteSettlement(s: Settlement) {
		if (deletingSettlementId) return;
		const ok = window.confirm(
			`Supprimer le règlement de ${(s.amount_cents / 100).toFixed(2)} € du ${formatDate(s.date)} ?\nCette action est irréversible.`
		);
		if (!ok) return;
		deletingSettlementId = s.id;
		try {
			await deleteSettlement(s.id);
		} catch (err) {
			liveError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			deletingSettlementId = null;
		}
	}

	function foyerNameFor(id: string): string {
		return foyers.find((f) => f.id === id)?.name ?? id;
	}
	function foyerFloorFor(id: string): 'rdc' | '1er' | undefined {
		return foyers.find((f) => f.id === id)?.floor;
	}

	async function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		// Idempotency guard: a fast double-tap on iOS Safari (or Enter-Enter
		// on a slow desktop) can fire `submit` twice before the disabled
		// attribute applies — without this guard we'd POST twice and create
		// duplicate expenses.
		if (creating) return;
		createError = '';
		const amountCents = eurosToCents(amountEuros);
		if (!Number.isFinite(amountCents) || amountCents <= 0) {
			createError = 'Montant invalide.';
			return;
		}
		const trimmedName = name.trim();
		if (!trimmedName) {
			createError = 'Donne un nom à la dépense.';
			return;
		}
		const body: import('$lib/api').CreateExpenseInput = {
			name: trimmedName,
			amount_cents: amountCents,
			date,
			payment_date: paymentDate || undefined,
			payer_foyer_id: payerFoyerId,
			category_id: categoryId,
			distribution_mode: mode,
			settled: settled || undefined,
			settled_at: settled && settledAt ? settledAt : undefined,
			note: note.trim() || undefined,
			template_id: pendingTemplateId ?? undefined
		};
		if (mode === 'custom') {
			if (customSubMode === 'percent') {
				body.share_rdc_cents = Math.round((amountCents * rdcPercentBp) / BP_TOTAL);
				body.share_1er_cents = amountCents - body.share_rdc_cents;
			} else {
				const shareRDC = eurosToCents(shareRDCEuros);
				const share1er = eurosToCents(share1erEuros);
				if (!Number.isFinite(shareRDC) || !Number.isFinite(share1er)) {
					createError = 'Renseigne les deux parts (RDC et 1er) en euros.';
					return;
				}
				if (shareRDC + share1er !== amountCents) {
					const total = formatEUR(amountCents);
					const sum = formatEUR(shareRDC + share1er);
					createError = `Les parts doivent totaliser ${total} (somme actuelle : ${sum}).`;
					return;
				}
				body.share_rdc_cents = shareRDC;
				body.share_1er_cents = share1er;
			}
		}
		creating = true;
		try {
			let savedId: string;
			if (editingId) {
				const updated = await updateExpense(editingId, body);
				savedId = updated.id;
			} else {
				const created = await createExpense(body);
				savedId = created.id;
				// Switch the modal to EDIT mode immediately so a retry of
				// "Enregistrer" after a failed upload doesn't create a
				// second expense — the user is now editing the row that
				// just landed.
				editingId = savedId;
			}
			// Files queued in the modal upload AFTER the expense saves so
			// edit and create share the same flow. Failures keep the modal
			// open so the user sees which file broke and can retry.
			if (pendingFiles.length > 0) {
				try {
					await uploadPendingFiles(savedId);
				} catch (err) {
					createError =
						err instanceof ApiError
							? `Pièce jointe: ${err.code}: ${err.message}`
							: `Pièce jointe: ${String(err)}`;
					return; // leave the modal open, don't reset
				}
			}
			modalOpen = false;
			// Same defer-reset trick used in closeCreate so we don't blank
			// the form before the closing animation finishes.
			setTimeout(resetForm, 220);
			// No manual refresh needed — Firestore onSnapshot pushes the new doc.
		} catch (err) {
			createError = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
		} finally {
			creating = false;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && modalOpen) closeCreate();
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
			<!-- ─── Hero balance ─────────────────────────── -->
			<section class="hero" aria-labelledby="hero-label">
				<p class="hero-label" id="hero-label">État du compte commun</p>

				{#if !balance || foyers.length < 2}
					<p class="hero-amount muted">—</p>
					<p class="hero-sub">
						Les deux foyers doivent être enregistrés avant le calcul de la balance.
					</p>
				{:else if balance.net === 0}
					<p class="hero-amount even">Équilibré</p>
					<p class="hero-sub">
						<span class="foyer-tag foyer-rdc">{balance.rdc.name}</span>
						et
						<span class="foyer-tag foyer-1er">{balance.premier.name}</span>
						sont à parts égales.
					</p>
				{:else if balance.net > 0}
					<p class="hero-amount">{formatEUR(balance.net)}</p>
					<p class="hero-sub">
						<span class="foyer-tag foyer-1er">{balance.premier.name}</span>
						doit à
						<span class="foyer-tag foyer-rdc">{balance.rdc.name}</span>
					</p>
				{:else}
					<p class="hero-amount">{formatEUR(-balance.net)}</p>
					<p class="hero-sub">
						<span class="foyer-tag foyer-rdc">{balance.rdc.name}</span>
						doit à
						<span class="foyer-tag foyer-1er">{balance.premier.name}</span>
					</p>
				{/if}

				{#if balance && balance.net !== 0}
					<button type="button" class="hero-settle-btn" onclick={openSettlement}>
						<span class="hero-settle-glyph" aria-hidden="true">↺</span>
						Régler
					</button>
				{/if}

				{#if totalCount > 0}
					<dl class="hero-stats">
						<div>
							<dt>Lignes</dt>
							<dd>{totalCount}</dd>
						</div>
						<div>
							<dt>Mois actifs</dt>
							<dd>{monthCount}</dd>
						</div>
					</dl>
				{/if}
			</section>

			<!-- ─── Ledger ─────────────────────────────── -->
			<section class="ledger" aria-labelledby="ledger-title">
				<header class="ledger-head">
					<h2 id="ledger-title">Carnet de dépenses</h2>
					<span
						class="live-pill"
						aria-live="polite"
						title={live ? 'Connecté en direct à Firestore' : 'Connexion en cours'}
					>
						<span class="live-dot" class:on={live}></span>
						{live ? 'En direct' : 'Connexion…'}
					</span>
				</header>

				<!-- ─── Filter bar ─── -->
				<div class="filter-bar">
					<input
						type="search"
						class="filter-search"
						placeholder="Rechercher (montant, note, intitulé)…"
						bind:value={searchQuery}
					/>
					<button
						type="button"
						class="filter-toggle"
						class:on={filtersOpen}
						onclick={() => (filtersOpen = !filtersOpen)}
						aria-expanded={filtersOpen}
					>
						Filtres
						{#if activeFilterCount > 0}
							<span class="filter-count">{activeFilterCount}</span>
						{/if}
					</button>
					{#if activeFilterCount > 0}
						<button type="button" class="filter-reset" onclick={resetFilters}>
							Réinitialiser
						</button>
					{/if}
				</div>

				{#if filtersOpen}
					<div class="filter-panel">
						<div class="filter-row">
							<span class="filter-label">Type</span>
							<div class="seg" role="tablist">
								<button
									type="button"
									class:active={filters.type === 'all'}
									onclick={() => (filters = { ...filters, type: 'all' })}>Tout</button
								>
								<button
									type="button"
									class:active={filters.type === 'expenses'}
									onclick={() => (filters = { ...filters, type: 'expenses' })}>Dépenses</button
								>
								<button
									type="button"
									class:active={filters.type === 'settlements'}
									onclick={() => (filters = { ...filters, type: 'settlements' })}
									>Règlements</button
								>
							</div>
						</div>

						<div class="filter-row">
							<span class="filter-label">État</span>
							<div class="seg" role="tablist">
								<button
									type="button"
									class:active={filters.state === 'all'}
									onclick={() => (filters = { ...filters, state: 'all' })}>Tout</button
								>
								<button
									type="button"
									class:active={filters.state === 'pending'}
									onclick={() => (filters = { ...filters, state: 'pending' })}
									>À&nbsp;compléter</button
								>
								<button
									type="button"
									class:active={filters.state === 'settled'}
									onclick={() => (filters = { ...filters, state: 'settled' })}>Réglée</button
								>
								<button
									type="button"
									class:active={filters.state === 'paired'}
									onclick={() => (filters = { ...filters, state: 'paired' })}>Liée à un règlement</button
								>
							</div>
						</div>

						<div class="filter-row filter-row-dates">
							<span class="filter-label">Période</span>
							<input
								type="date"
								class="filter-date"
								bind:value={filters.from}
								aria-label="Du"
							/>
							<span class="filter-date-sep">→</span>
							<input
								type="date"
								class="filter-date"
								bind:value={filters.to}
								aria-label="Au"
							/>
						</div>

						<div class="filter-row filter-row-chips">
							<span class="filter-label">Catégories</span>
							<div class="filter-chips">
								{#each categories as c (c.id)}
									{@const on = filters.categories.includes(c.id)}
									<button
										type="button"
										class="filter-chip"
										class:on
										onclick={() =>
											(filters = {
												...filters,
												categories: toggleInArray(filters.categories, c.id)
											})}
									>
										{c.name}
									</button>
								{/each}
							</div>
						</div>

						<div class="filter-row filter-row-chips">
							<span class="filter-label">Payeur</span>
							<div class="filter-chips">
								{#each foyers as f (f.id)}
									{@const on = filters.payers.includes(f.id)}
									<button
										type="button"
										class="filter-chip"
										class:on
										onclick={() =>
											(filters = { ...filters, payers: toggleInArray(filters.payers, f.id) })}
									>
										{f.name}
									</button>
								{/each}
							</div>
						</div>

						<div class="filter-row filter-row-chips">
							<span class="filter-label">Répartition</span>
							<div class="filter-chips">
								{#each ['equal', 'tantiemes', 'custom'] as m}
									{@const on = filters.modes.includes(m as DistributionMode)}
									<button
										type="button"
										class="filter-chip"
										class:on
										onclick={() =>
											(filters = {
												...filters,
												modes: toggleInArray(filters.modes, m as DistributionMode)
											})}
									>
										{m === 'equal' ? '50/50' : m === 'tantiemes' ? 'Tantièmes' : 'Manuel'}
									</button>
								{/each}
							</div>
						</div>
					</div>
				{/if}

				{#if pendingCount > 0}
					<div class="pending-banner" role="status">
						<span class="pending-banner-glyph" aria-hidden="true">⌇</span>
						<span class="pending-banner-text">
							<strong>{pendingCount}</strong>
							{pendingCount > 1 ? 'dépenses' : 'dépense'} à compléter — un membre du foyer
							payeur saisit le montant à réception de la facture.
						</span>
					</div>
				{/if}

				{#if liveError}
					<div class="error-card" role="alert">{liveError}</div>
				{:else if !live && expenses.length === 0}
					<div class="placeholder">
						<span class="placeholder-bar"></span>
						<span class="placeholder-bar short"></span>
						<span class="placeholder-bar"></span>
					</div>
				{:else if expenses.length === 0 && settlements.length === 0}
					<div class="empty">
						<div class="empty-mark" aria-hidden="true">❦</div>
						<h3>Le carnet est vierge.</h3>
						<p>
							La première facture ouvre le compte. Eau, taxe foncière, travaux —
							chaque dépense partagée trouve sa ligne ici.
						</p>
						<button type="button" class="empty-cta" onclick={openCreate}>
							Inscrire la première dépense
						</button>
					</div>
				{:else if filteredRows.length === 0}
					<div class="empty">
						<div class="empty-mark" aria-hidden="true">❦</div>
						<h3>Aucune ligne ne correspond aux filtres actifs.</h3>
						<p>
							Ajuste la recherche ou réinitialise les filtres pour voir tout le carnet.
						</p>
						<button type="button" class="empty-cta" onclick={resetFilters}>
							Réinitialiser les filtres
						</button>
					</div>
				{:else}
					{#each groupedRows as [yyyymm, group] (yyyymm)}
						<div class="month">
							<header class="month-head">
								<span class="month-label">{formatMonth(yyyymm)}</span>
								<span class="month-rule"></span>
								<span class="month-count">
									{group.length} ligne{group.length > 1 ? 's' : ''}
								</span>
							</header>
							<ul class="rows">
								{#each group as row, idx (row.id)}
								{#if row.kind === 'expense'}
								{@const exp = row.expense}
								{@const linkedSettlement = linkedSettlementByExpense[exp.id]}
									{@const cat = categoryStyle(exp.category_id)}
									{@const dp = dayParts(exp.date)}
									{@const payer = foyers.find((f) => f.id === exp.payer_foyer_id)}
									<li class="row" style:--idx={idx}>
										<div class="row-day">
											<span class="row-day-num">{dp.num}</span>
											<span class="row-day-mon">{dp.mon}</span>
										</div>

										<div class="row-mono"
											style:color={cat.tone}
											style:background={cat.tint}
											aria-hidden="true"
										>
											{cat.mono}
										</div>

										<div class="row-body">
											<p class="row-title">
												<span class="cat-name">
													{exp.name || categoryName(exp.category_id)}
												</span>
												{#if exp.name && exp.name.toLowerCase() !== categoryName(exp.category_id).toLowerCase()}
													<span class="row-cat">{categoryName(exp.category_id)}</span>
												{/if}
												{#if exp.settled}
													<span class="row-settled" title="Réglée — exclue de la balance">Réglée</span>
												{/if}
												{#if exp.amount_pending}
													<span class="row-pending" title="Montant à compléter">À compléter</span>
												{/if}
												{#if exp.template_id}
													<span class="row-template" title="Créée depuis un modèle">modèle</span>
												{/if}
												{#if linkedSettlement}
													<span
														class="row-paired"
														title="Réglée par le règlement du {formatDate(linkedSettlement.date)}"
													>
														réglé · {linkedSettlement.date.slice(5, 10)}
													</span>
												{/if}
												{#if exp.note}
													<span class="row-note">{exp.note}</span>
												{/if}
											</p>
											<p class="row-meta">
												{#if payer}
													<span class="meta-label">payé&nbsp;par</span>
													<span class="foyer-tag foyer-{payer.floor}">{payer.name}</span>
												{/if}
												<span class="row-mode" title={modeLabel(exp.distribution_mode)}>
													<span class="mode-glyph">{modeGlyph(exp.distribution_mode)}</span>
													<span class="mode-text">{modeLabel(exp.distribution_mode)}</span>
												</span>
											</p>
											{#if exp.payment_date || (exp.settled && exp.settled_at)}
												<p class="row-dates">
													{#if exp.payment_date}
														<span>
															<span class="meta-label">Payée&nbsp;le</span>
															{formatDate(exp.payment_date)}
														</span>
													{/if}
													{#if exp.settled && exp.settled_at}
														<span>
															<span class="meta-label">Réglée&nbsp;le</span>
															{formatDate(exp.settled_at)}
														</span>
													{/if}
												</p>
											{/if}
											<p class="row-shares">
												<span class="share">
													<span class="share-label">RDC</span>
													<span class="share-amt">{formatEUR(exp.share_rdc_cents)}</span>
												</span>
												<span class="share-sep">·</span>
												<span class="share">
													<span class="share-label">1ᵉʳ</span>
													<span class="share-amt">{formatEUR(exp.share_1er_cents)}</span>
												</span>
											</p>
										</div>

										<div class="row-right">
											{#if exp.amount_pending}
												<button
													type="button"
													class="row-amount-cta"
													onclick={() => openEdit(exp)}
													title="Cliquer pour saisir le montant"
												>
													Compléter
												</button>
											{:else}
												<div class="row-amount">{formatEUR(exp.amount_cents)}</div>
											{/if}
											{#if exp.attachments && exp.attachments.length > 0}
												<button
													type="button"
													class="row-attach-chip"
													onclick={() => toggleRowDrawer(exp.id)}
													aria-expanded={!!expandedRows[exp.id]}
													aria-label={`${exp.attachments.length} pièce${exp.attachments.length > 1 ? 's' : ''} jointe${exp.attachments.length > 1 ? 's' : ''}`}
													title="Pièces jointes"
												>
													<span class="paperclip" aria-hidden="true">⌇</span>
													<span class="row-attach-count">{exp.attachments.length}</span>
												</button>
											{/if}
											<div class="row-actions">
												<IconButton
													icon="edit"
													size="sm"
													aria-label="Modifier la dépense"
													onclick={() => openEdit(exp)}
													disabled={creating || deletingId !== null}
												/>
												<IconButton
													icon="delete"
													variant="danger"
													size="sm"
													aria-label="Supprimer la dépense"
													aria-busy={deletingId === exp.id}
													onclick={() => onDelete(exp)}
													disabled={creating || deletingId !== null}
												/>
											</div>
										</div>
										{#if expandedRows[exp.id] && exp.attachments && exp.attachments.length > 0}
											<div class="row-drawer">
												{#each exp.attachments as att (att.id)}
													<div class="att-card">
														{#if isImageAttachment(att)}
															{#await resolveDownloadUrl(exp.id, att.id)}
																<div class="att-thumb att-thumb-loading"></div>
															{:then thumbUrl}
																<button
																	type="button"
																	class="att-thumb"
																	onclick={() => onViewAttachment(exp, att)}
																	title={att.original_filename}
																	aria-label={`Voir ${att.original_filename}`}
																>
																	<img src={thumbUrl} alt={att.original_filename} loading="lazy" />
																</button>
															{:catch}
																<div class="att-thumb att-thumb-failed" title="Aperçu indisponible">
																	!
																</div>
															{/await}
														{:else}
															<div class="att-thumb att-thumb-pdf" aria-hidden="true">PDF</div>
														{/if}
														<div class="att-meta">
															<span class="att-meta-name" title={att.original_filename}>
																{att.original_filename || att.id}
															</span>
															<span class="att-meta-size">{formatFileSize(att.size_bytes)}</span>
														</div>
														<div class="att-actions">
															<IconButton
																icon="download"
																size="sm"
																aria-label="Voir {att.original_filename}"
																onclick={() => onViewAttachment(exp, att)}
															/>
															<IconButton
																icon="delete"
																variant="danger"
																size="sm"
																aria-label="Supprimer {att.original_filename}"
																aria-busy={deletingAttachmentId === att.id}
																onclick={() => onDeleteAttachment(exp, att)}
																disabled={deletingAttachmentId !== null}
															/>
														</div>
													</div>
												{/each}
											</div>
										{/if}
									</li>
								{:else}
									{@const s = row.settlement}
									<li class="row settlement-row" style:--idx={idx}>
										<div class="row-day">
											<span class="row-day-num">{dayParts(s.date).num}</span>
											<span class="row-day-mon">{dayParts(s.date).mon}</span>
										</div>
										<div class="row-mono settlement-glyph" aria-hidden="true">
											<span>↺</span>
										</div>
										<div class="row-body">
											<p class="row-title">
												<span class="cat-name">Règlement</span>
												<span class="settlement-direction">
													<span class="foyer-tag foyer-{foyerFloorFor(s.from_foyer_id) ?? 'rdc'}">
														{foyerNameFor(s.from_foyer_id)}
													</span>
													<span class="settlement-arrow" aria-hidden="true">→</span>
													<span class="foyer-tag foyer-{foyerFloorFor(s.to_foyer_id) ?? '1er'}">
														{foyerNameFor(s.to_foyer_id)}
													</span>
												</span>
												{#if s.note}
													<span class="row-note">{s.note}</span>
												{/if}
											</p>
											{#if (s.expense_ids?.length ?? 0) > 0}
												<p class="row-meta">
													<span class="meta-label">couvre</span>
													<span>{s.expense_ids?.length} dépense{(s.expense_ids?.length ?? 0) > 1 ? 's' : ''}</span>
												</p>
											{/if}
										</div>
										<div class="row-right">
											<div class="row-amount settlement-amount">{formatEUR(s.amount_cents)}</div>
											<div class="row-actions">
												<IconButton
													icon="edit"
													size="sm"
													aria-label="Modifier le règlement"
													onclick={() => openSettlementEdit(s)}
													disabled={savingSettlement || deletingSettlementId !== null}
												/>
												<IconButton
													icon="delete"
													variant="danger"
													size="sm"
													aria-label="Supprimer le règlement"
													aria-busy={deletingSettlementId === s.id}
													onclick={() => onDeleteSettlement(s)}
													disabled={savingSettlement || deletingSettlementId !== null}
												/>
											</div>
										</div>
									</li>
								{/if}
								{/each}
							</ul>
						</div>
					{/each}
				{/if}
			</section>
		</main>

		<!-- ─── FAB ─────────────────────────────────── -->
		<Fab onclick={openChooser} aria-label="Nouvelle dépense">Nouvelle dépense</Fab>

		<!-- ─── Chooser sheet (Vide / Depuis un modèle) ─── -->
		{#if chooserOpen}
			<div
				class="modal-backdrop"
				role="presentation"
				onclick={closeChooser}
				onkeydown={(e) => e.key === 'Escape' && closeChooser()}
			></div>
			<div class="chooser" role="dialog" aria-modal="true" aria-labelledby="chooser-title">
				<header class="chooser-head">
					<h2 id="chooser-title">Nouvelle dépense</h2>
					<IconButton
						icon="close"
						variant="text"
						aria-label="Fermer"
						onclick={closeChooser}
					/>
				</header>
				<div class="chooser-body">
					<button class="chooser-opt" type="button" onclick={chooseBlank}>
						<span class="chooser-opt-glyph">＋</span>
						<span class="chooser-opt-text">
							<strong>Saisie libre</strong>
							<small>Nouvelle ligne, formulaire vierge.</small>
						</span>
					</button>
					<button class="chooser-opt" type="button" onclick={chooseFromTemplate}>
						<span class="chooser-opt-glyph">⌇</span>
						<span class="chooser-opt-text">
							<strong>Depuis un modèle</strong>
							<small>
								{templates.length === 0
									? 'Aucun modèle — créer le premier.'
									: `${templates.length} modèle${templates.length > 1 ? 's' : ''} disponible${templates.length > 1 ? 's' : ''}.`}
							</small>
						</span>
					</button>
				</div>
			</div>
		{/if}

		<!-- ─── Template picker sheet ─── -->
		{#if pickerOpen}
			<div
				class="modal-backdrop"
				role="presentation"
				onclick={closePicker}
				onkeydown={(e) => e.key === 'Escape' && closePicker()}
			></div>
			<div class="picker" role="dialog" aria-modal="true" aria-labelledby="picker-title">
				<header class="picker-head">
					<div>
						<p class="modal-eyebrow">Modèles</p>
						<h2 id="picker-title">Choisir un modèle</h2>
					</div>
					<IconButton
						icon="close"
						variant="text"
						aria-label="Fermer"
						onclick={closePicker}
					/>
				</header>
				<ul class="picker-list">
					{#each templates as t (t.id)}
						{@const payer = foyers.find((f) => f.id === t.payer_foyer_id)}
						<li>
							<button type="button" class="picker-item" onclick={() => applyTemplate(t)}>
								<span class="picker-item-name">{t.name}</span>
								<span class="picker-item-meta">
									{#if payer}
										<span class="foyer-tag foyer-{payer.floor}">{payer.name}</span>
									{/if}
									{#if t.amount_default_cents > 0}
										<span class="picker-item-amt">{formatEUR(t.amount_default_cents)}</span>
									{:else}
										<span class="picker-item-pending">à compléter</span>
									{/if}
								</span>
							</button>
						</li>
					{/each}
				</ul>
				<footer class="picker-foot">
					<Button variant="text" href="/templates">Gérer les modèles →</Button>
				</footer>
			</div>
		{/if}

		<!-- ─── Settlement modal ─────────────────────── -->
		{#if settlementModalOpen}
			<div
				class="modal-backdrop"
				role="presentation"
				onclick={closeSettlementModal}
				onkeydown={(e) => e.key === 'Escape' && closeSettlementModal()}
			></div>
			<div
				class="modal"
				role="dialog"
				aria-modal="true"
				aria-labelledby="settle-modal-title"
			>
				<header class="modal-head">
					<div>
						<p class="modal-eyebrow">{isEditingSettlement ? 'Édition' : 'Règlement'}</p>
						<h2 id="settle-modal-title">
							{isEditingSettlement ? 'Modifier le règlement' : 'Enregistrer un règlement'}
						</h2>
					</div>
					<IconButton
						icon="close"
						variant="text"
						aria-label="Fermer"
						onclick={closeSettlementModal}
					/>
				</header>

				<form class="modal-body" onsubmit={onSubmitSettlement}>
					<div class="grid-2">
						<label class="field">
							<span class="lbl">De</span>
							<select bind:value={settleFromFoyerId} required>
								<option value="" disabled>—</option>
								{#each foyers as f (f.id)}
									<option value={f.id}>{f.name}</option>
								{/each}
							</select>
						</label>
						<label class="field">
							<span class="lbl">Vers</span>
							<select bind:value={settleToFoyerId} required>
								<option value="" disabled>—</option>
								{#each foyers as f (f.id)}
									<option value={f.id}>{f.name}</option>
								{/each}
							</select>
						</label>
					</div>
					{#if settleSameFoyer}
						<p class="form-error" role="alert">
							Les foyers « De » et « Vers » doivent être différents.
						</p>
					{/if}

					<div class="grid-2">
						<label class="field">
							<span class="lbl">Montant</span>
							<div class="input-suffix">
								<input
									type="text"
									inputmode="decimal"
									bind:value={settleAmountEuros}
									placeholder="0,00"
									required
								/>
								<span class="suffix">€</span>
							</div>
						</label>
						<label class="field">
							<span class="lbl">Date</span>
							<input type="date" bind:value={settleDate} required />
						</label>
					</div>

					<label class="field">
						<span class="lbl">Note (optionnel)</span>
						<input
							type="text"
							bind:value={settleNote}
							placeholder="Réf SEPA, virement…"
						/>
					</label>

					<fieldset class="link-group">
						<legend class="lbl">
							Dépenses couvertes
							<span class="lbl-aside">— optionnel</span>
						</legend>
						<p class="link-counter">
							Sélectionnées :
							<strong>{settleLinkedExpenseIds.length}</strong>
							· couvre {formatEUR(settleLinkedCoverage)} de dette
						</p>
						{#if availableLinkExpenses.length === 0}
							<p class="field-hint">Aucune dépense disponible à lier.</p>
						{:else}
							<ul class="link-list">
								{#each availableLinkExpenses as exp (exp.id)}
									{@const checked = settleLinkedExpenseIds.includes(exp.id)}
									{@const payer = foyers.find((f) => f.id === exp.payer_foyer_id)}
									<li class="link-item" class:link-item-on={checked}>
										<label>
											<input
												type="checkbox"
												{checked}
												onchange={() => toggleSettleLinked(exp.id)}
											/>
											<span class="link-item-body">
												<span class="link-item-name">
													{exp.name || categoryName(exp.category_id)}
												</span>
												<span class="link-item-meta">
													<span>{formatDate(exp.date)}</span>
													{#if payer}
														<span class="foyer-tag foyer-{payer.floor}">{payer.name}</span>
													{/if}
													<span class="link-item-amt">{formatEUR(exp.amount_cents)}</span>
												</span>
											</span>
										</label>
									</li>
								{/each}
							</ul>
						{/if}
					</fieldset>

					{#if settlementError}
						<p class="form-error" role="alert">{settlementError}</p>
					{/if}

					<div class="modal-actions">
						<Button
							variant="ghost"
							onclick={closeSettlementModal}
							disabled={savingSettlement}
						>
							Annuler
						</Button>
						<Button
							type="submit"
							variant="primary"
							mark
							disabled={savingSettlement || settleSameFoyer}
						>
							{#if savingSettlement}
								{isEditingSettlement ? 'Mise à jour…' : 'Enregistrement…'}
							{:else}
								{isEditingSettlement ? 'Mettre à jour' : 'Enregistrer le règlement'}
							{/if}
						</Button>
					</div>
				</form>
			</div>
		{/if}

		<!-- ─── Modal sheet ─────────────────────────── -->
		{#if modalOpen}
			<div
				class="modal-backdrop"
				role="presentation"
				onclick={closeCreate}
				onkeydown={(e) => e.key === 'Escape' && closeCreate()}
			></div>
			<div class="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title">
				<header class="modal-head">
					<div>
						<p class="modal-eyebrow">{isEditing ? 'Édition' : 'Nouvelle ligne'}</p>
						<h2 id="modal-title">
							{isEditing ? 'Modifier la dépense' : 'Inscrire une dépense'}
						</h2>
					</div>
					<IconButton
						icon="close"
						variant="text"
						aria-label="Fermer"
						onclick={closeCreate}
					/>
				</header>

				<form class="modal-body" onsubmit={onSubmit}>
					{#if liveError}
						<div class="error-card" role="alert">
							<strong>Lecture Firestore impossible :</strong>
							{liveError}
							<br />
							<span class="error-hint">
								Les listes de foyers/catégories ne pourront pas s'afficher tant que les
								règles de sécurité ne sont pas déployées
								(<code>./infra/firebase/deploy-rules.sh</code>).
							</span>
						</div>
					{/if}
					<label class="field">
						<span class="lbl">Intitulé</span>
						<input
							type="text"
							required
							bind:value={name}
							placeholder="Ex. Eau été, Taxe foncière, Travaux haie…"
						/>
					</label>
					<div class="grid-2">
						<label class="field">
							<span class="lbl">Montant</span>
							<div class="input-suffix">
								<input
									type="number"
									inputmode="decimal"
									step="0.01"
									min="0"
									required
									bind:value={amountEuros}
									placeholder="0,00"
								/>
								<span class="suffix">€</span>
							</div>
						</label>
						<label class="field">
							<span class="lbl">Date facture</span>
							<input type="date" required bind:value={date} />
						</label>
					</div>
					<label class="field">
						<span class="lbl">Date de paiement (fournisseur)</span>
						<input
							type="date"
							bind:value={paymentDate}
							placeholder="Optionnel — laisser vide si payé le jour même"
						/>
					</label>

					<label class="field">
						<span class="lbl">Payeur</span>
						<select bind:value={payerFoyerId} required>
							{#each foyers as f (f.id)}
								<option value={f.id}>{f.name} — {foyerLabel(f)}</option>
							{/each}
						</select>
					</label>

					<label class="field">
						<span class="lbl">Catégorie</span>
						<select bind:value={categoryId} required>
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

					<fieldset class="mode-group">
						<legend class="lbl">Répartition</legend>
						<div class="mode-tabs" role="tablist">
							<button
								type="button"
								role="tab"
								class:active={mode === 'equal'}
								onclick={() => (mode = 'equal')}
							>
								<span class="mt-glyph">½</span>
								<span class="mt-name">Égalité</span>
								<span class="mt-sub">50 / 50</span>
							</button>
							<button
								type="button"
								role="tab"
								class:active={mode === 'tantiemes'}
								onclick={() => (mode = 'tantiemes')}
							>
								<span class="mt-glyph">‰</span>
								<span class="mt-name">Tantièmes</span>
								<span class="mt-sub">selon parts</span>
							</button>
							<button
								type="button"
								role="tab"
								class:active={mode === 'custom'}
								onclick={() => (mode = 'custom')}
							>
								<span class="mt-glyph">✱</span>
								<span class="mt-name">Manuel</span>
								<span class="mt-sub">parts libres</span>
							</button>
						</div>
					</fieldset>

					{#if mode === 'custom'}
						<div class="custom-pane">
							<div class="custom-sub-tabs" role="tablist" aria-label="Mode de saisie manuelle">
								<button
									type="button"
									role="tab"
									aria-selected={customSubMode === 'percent'}
									class:active={customSubMode === 'percent'}
									onclick={() => (customSubMode = 'percent')}
								>
									Pourcentage
								</button>
								<button
									type="button"
									role="tab"
									aria-selected={customSubMode === 'exact'}
									class:active={customSubMode === 'exact'}
									onclick={() => (customSubMode = 'exact')}
								>
									Exact €
								</button>
							</div>

							{#if customSubMode === 'percent'}
								<div class="percent-pane">
									<div class="percent-row">
										<label class="percent-cell">
											<span class="percent-label">RDC</span>
											<div class="pct-field">
												<input
													type="number"
													min="0"
													max="100"
													step="0.01"
													value={formatPct(rdcPercentBp)}
													oninput={(e) =>
														setRdcPercentFromInput(e.currentTarget.value)}
													aria-label="Pourcentage RDC"
												/>
												<span class="pct-suffix">%</span>
											</div>
											<span class="percent-eur">{formatEUR(percentRdcCents)}</span>
										</label>

										<label class="percent-cell">
											<span class="percent-label">1ᵉʳ</span>
											<div class="pct-field">
												<input
													type="number"
													min="0"
													max="100"
													step="0.01"
													value={formatPct(BP_TOTAL - rdcPercentBp)}
													oninput={(e) => {
														const raw = e.currentTarget.value;
														if (raw === '' || raw === null) return;
														const n = Number(String(raw).replace(',', '.'));
														if (!Number.isFinite(n)) return;
														setRdcPercentBp(BP_TOTAL - Math.round(n * 100));
													}}
													aria-label="Pourcentage 1ᵉʳ"
												/>
												<span class="pct-suffix">%</span>
											</div>
											<span class="percent-eur">{formatEUR(percent1erCents)}</span>
										</label>
									</div>

									<input
										class="percent-slider"
										type="range"
										min="0"
										max={BP_TOTAL}
										step="1"
										bind:value={rdcPercentBp}
										style:--p={rdcPercentBp / 100}
										aria-label="Curseur de répartition RDC / 1ᵉʳ"
									/>

									<p class="percent-hint">
										Glisse le curseur (précision&nbsp;0,01&nbsp;%) ou tape une valeur précise.
										La somme reste à 100&nbsp;%.
									</p>
								</div>
							{:else}
								<div class="grid-2">
									<label class="field">
										<span class="lbl">Part RDC</span>
										<div class="input-suffix">
											<input
												type="number"
												step="0.01"
												min="0"
												required
												bind:value={shareRDCEuros}
												placeholder="0,00"
											/>
											<span class="suffix">€</span>
										</div>
									</label>
									<label class="field">
										<span class="lbl">Part 1ᵉʳ</span>
										<div class="input-suffix">
											<input
												type="number"
												step="0.01"
												min="0"
												required
												bind:value={share1erEuros}
												placeholder="0,00"
											/>
											<span class="suffix">€</span>
										</div>
									</label>
								</div>
							{/if}
						</div>
					{/if}

					<fieldset class="settled-group">
						<legend class="lbl">Règlement entre foyers</legend>
						<label class="settled-toggle">
							<input type="checkbox" bind:checked={settled} />
							<span>Déjà réglée — exclue de la balance</span>
						</label>
						{#if settled}
							<label class="field">
								<span class="lbl">Date de règlement</span>
								<input
									type="date"
									bind:value={settledAt}
									placeholder="Optionnel"
								/>
							</label>
						{/if}
					</fieldset>

					<label class="field">
						<span class="lbl">Note (optionnel)</span>
						<input type="text" bind:value={note} placeholder="Référence, prestataire…" />
					</label>

					<fieldset class="attach-group">
						<legend class="lbl">Pièces jointes</legend>
						{#if isEditing && editingId}
							{@const editingExp = renderedExpenses.find((e) => e.id === editingId)}
							{#if editingExp && editingExp.attachments && editingExp.attachments.length > 0}
								<ul class="attach-existing">
									{#each editingExp.attachments as att (att.id)}
										<li class="attach-existing-item">
											<span class="attach-name" title={att.original_filename}>
												{att.original_filename || att.id}
											</span>
											<span class="attach-size">{formatFileSize(att.size_bytes)}</span>
											<IconButton
												icon="download"
												size="sm"
												aria-label="Voir {att.original_filename}"
												onclick={() => editingExp && onViewAttachment(editingExp, att)}
											/>
											<IconButton
												icon="delete"
												variant="danger"
												size="sm"
												aria-label="Retirer {att.original_filename}"
												aria-busy={deletingAttachmentId === att.id}
												onclick={() => editingExp && onDeleteAttachment(editingExp, att)}
												disabled={deletingAttachmentId !== null}
											/>
										</li>
									{/each}
								</ul>
							{:else if !editingExp}
								<p class="attach-hint" style="color: #b91c1c;">
									Cette dépense a été supprimée ailleurs — les pièces jointes ne sont plus accessibles.
								</p>
							{/if}
						{/if}
						<label class="attach-picker">
							<input
								type="file"
								accept={ATTACHMENT_ACCEPT}
								capture="environment"
								multiple
								onchange={onPickFiles}
								disabled={creating}
							/>
							<span class="attach-hint">
								Ajouter une photo de reçu ou un PDF — 10 Mo max, jusqu'à {ATTACHMENT_MAX_PER_EXPENSE} fichiers.
							</span>
						</label>
						{#if pendingFiles.length > 0}
							<ul class="attach-queue">
								{#each pendingFiles as file, i}
									<li class="attach-queue-item" class:uploading={uploadingFileIdx === i}>
										<span class="attach-name" title={file.name}>{file.name}</span>
										<span class="attach-size">{formatFileSize(file.size)}</span>
										{#if uploadingFileIdx === i}
											<span class="attach-status">Téléversement…</span>
										{:else if uploadingFileIdx !== null && i < uploadingFileIdx}
											<span class="attach-status attach-status-ok">Envoyé</span>
										{:else}
											<IconButton
												icon="close"
												variant="danger"
												size="sm"
												aria-label="Retirer le fichier"
												onclick={() => removePendingFile(i)}
												disabled={creating}
											/>
										{/if}
									</li>
								{/each}
							</ul>
						{/if}
						{#if uploadError}
							<p class="form-error" role="alert">{uploadError}</p>
						{/if}
					</fieldset>

					{#if createError}
						<p class="form-error" role="alert">{createError}</p>
					{/if}

					<div class="modal-actions">
						<Button variant="ghost" onclick={closeCreate} disabled={creating}>Annuler</Button>
						<Button
							type="submit"
							variant="primary"
							mark
							disabled={creating || !payerFoyerId || !categoryId}
						>
							{#if creating}
								{isEditing ? 'Mise à jour…' : 'Enregistrement…'}
							{:else}
								{isEditing ? 'Mettre à jour' : 'Enregistrer'}
							{/if}
						</Button>
					</div>
				</form>
			</div>
		{/if}
	{/if}
</div>

<style>
	/* =========================================================
	   TOKENS
	   ========================================================= */
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
		--shadow-sm: 0 1px 2px rgba(20, 16, 12, 0.04), 0 1px 1px rgba(20, 16, 12, 0.02);
		--shadow-md: 0 6px 24px rgba(20, 16, 12, 0.06), 0 2px 6px rgba(20, 16, 12, 0.04);
		--shadow-lg: 0 24px 60px rgba(20, 16, 12, 0.18), 0 8px 24px rgba(20, 16, 12, 0.08);
		--display: 'Fraunces', 'Hoefler Text', 'Iowan Old Style', Georgia, serif;
		--ui:
			'Hanken Grotesk', -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;

		min-height: 100vh;
		font-family: var(--ui);
		color: var(--ink);
		background:
			radial-gradient(1100px 520px at 6% -10%, rgba(194, 78, 42, 0.07), transparent 70%),
			radial-gradient(900px 460px at 110% 110%, rgba(90, 116, 97, 0.06), transparent 70%),
			var(--bg);
		padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 8.5rem);
	}

	/* =========================================================
	   MAIN
	   ========================================================= */
	.main {
		max-width: 720px;
		margin: 0 auto;
		padding: 1rem 1.25rem 0;
	}
	.center {
		text-align: center;
	}
	.muted {
		color: var(--ink-3);
	}

	/* =========================================================
	   HERO
	   ========================================================= */
	.hero {
		position: relative;
		margin-top: 0.75rem;
		padding: 2.4rem 1.5rem 1.6rem;
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 1.25rem;
		box-shadow: var(--shadow-md);
		overflow: hidden;
		animation: fade-up 480ms cubic-bezier(0.2, 0.8, 0.2, 1) backwards;
	}
	.hero::before {
		content: '';
		position: absolute;
		top: -40px;
		right: -40px;
		width: 220px;
		height: 220px;
		background: radial-gradient(circle, rgba(194, 78, 42, 0.1), transparent 70%);
		pointer-events: none;
	}
	.hero::after {
		content: '';
		position: absolute;
		bottom: 0;
		left: 1.5rem;
		right: 1.5rem;
		height: 1px;
		background: linear-gradient(
			90deg,
			transparent,
			var(--hairline-2) 30%,
			var(--hairline-2) 70%,
			transparent
		);
	}

	.hero-label {
		margin: 0 0 0.7rem;
		font-size: 0.66rem;
		letter-spacing: 0.32em;
		text-transform: uppercase;
		color: var(--ink-3);
		font-weight: 600;
	}
	.hero-amount {
		margin: 0;
		font-family: var(--display);
		font-size: clamp(2.6rem, 9vw, 4rem);
		font-weight: 300;
		letter-spacing: -0.04em;
		line-height: 1;
		color: var(--ink);
		font-feature-settings: 'tnum' 1, 'lnum' 1;
		font-variant-numeric: tabular-nums lining-nums;
	}
	.hero-amount.muted {
		color: var(--ink-4);
	}
	.hero-amount.even {
		font-style: italic;
		color: var(--ok);
	}
	.hero-sub {
		margin: 1rem 0 0;
		font-size: 0.95rem;
		color: var(--ink-2);
		line-height: 1.5;
	}

	.hero-stats {
		margin: 1.5rem 0 0;
		padding: 1rem 0 0;
		display: flex;
		gap: 2rem;
	}
	.hero-stats div {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}
	.hero-stats dt {
		font-size: 0.62rem;
		letter-spacing: 0.28em;
		text-transform: uppercase;
		color: var(--ink-3);
		font-weight: 600;
	}
	.hero-stats dd {
		margin: 0;
		font-family: var(--display);
		font-size: 1.4rem;
		font-weight: 400;
		color: var(--ink);
		font-feature-settings: 'tnum' 1, 'lnum' 1;
	}

	/* =========================================================
	   FOYER TAG (used in hero + rows)
	   ========================================================= */
	.foyer-tag {
		display: inline-block;
		font-size: 0.78rem;
		font-weight: 600;
		padding: 0.18rem 0.55rem;
		border-radius: 999px;
		line-height: 1.5;
		vertical-align: baseline;
		border: 1px solid;
		white-space: nowrap;
	}
	.foyer-tag.foyer-rdc {
		color: var(--rdc);
		background: var(--rdc-soft);
		border-color: rgba(90, 116, 97, 0.3);
	}
	.foyer-tag.foyer-1er {
		color: var(--clay);
		background: var(--clay-soft);
		border-color: rgba(158, 106, 77, 0.3);
	}

	/* =========================================================
	   LEDGER
	   ========================================================= */
	.ledger {
		margin-top: 2rem;
	}
	.ledger-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		padding: 0 0.25rem 1rem;
	}
	.ledger-head h2 {
		margin: 0;
		font-family: var(--display);
		font-weight: 400;
		font-style: italic;
		font-size: 1.65rem;
		letter-spacing: -0.005em;
	}

	.live-pill {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0.35rem 0.75rem 0.35rem 0.55rem;
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		background: var(--surface);
		font-family: var(--ui);
		font-size: 0.7rem;
		font-weight: 600;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--ink-3);
	}
	.live-dot {
		width: 7px;
		height: 7px;
		border-radius: 999px;
		background: var(--ink-4);
		box-shadow: 0 0 0 0 rgba(79, 110, 92, 0);
	}
	.live-dot.on {
		background: var(--ok);
		animation: pulse 2.4s ease-out infinite;
	}
	@keyframes pulse {
		0% {
			box-shadow: 0 0 0 0 rgba(79, 110, 92, 0.4);
		}
		70% {
			box-shadow: 0 0 0 6px rgba(79, 110, 92, 0);
		}
		100% {
			box-shadow: 0 0 0 0 rgba(79, 110, 92, 0);
		}
	}

	.month + .month {
		margin-top: 1.75rem;
	}
	.month-head {
		display: flex;
		align-items: center;
		gap: 0.85rem;
		margin: 0 0.25rem 0.65rem;
	}
	.month-label {
		font-family: var(--display);
		font-style: italic;
		font-weight: 400;
		font-size: 0.95rem;
		color: var(--ink-2);
		text-transform: capitalize;
	}
	.month-rule {
		flex: 1;
		height: 1px;
		background: var(--hairline);
	}
	.month-count {
		font-size: 0.62rem;
		letter-spacing: 0.22em;
		text-transform: uppercase;
		color: var(--ink-4);
		font-weight: 600;
	}

	.rows {
		list-style: none;
		margin: 0;
		padding: 0;
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 1rem;
		box-shadow: var(--shadow-sm);
		overflow: hidden;
	}
	.row {
		display: grid;
		grid-template-columns: auto auto 1fr auto;
		align-items: flex-start;
		gap: 0.9rem;
		padding: 1rem 1.1rem;
		border-bottom: 1px solid var(--hairline);
		transition: background 140ms ease;
		animation: row-in 320ms cubic-bezier(0.2, 0.8, 0.2, 1) backwards;
		animation-delay: calc(var(--idx, 0) * 30ms);
	}
	.row:last-child {
		border-bottom: 0;
	}
	.row:hover {
		background: rgba(194, 78, 42, 0.025);
	}

	.row-day {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-width: 2.5rem;
		padding: 0.35rem 0.4rem;
		border: 1px solid var(--hairline);
		border-radius: 0.55rem;
		background: var(--bg);
	}
	.row-day-num {
		font-family: var(--display);
		font-size: 1.2rem;
		line-height: 1;
		font-weight: 500;
		font-feature-settings: 'tnum' 1, 'lnum' 1;
	}
	.row-day-mon {
		font-size: 0.6rem;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--ink-3);
		margin-top: 0.18rem;
	}

	.row-mono {
		width: 2.1rem;
		height: 2.1rem;
		flex-shrink: 0;
		border-radius: 999px;
		display: grid;
		place-items: center;
		font-family: var(--display);
		font-style: italic;
		font-weight: 500;
		font-size: 0.78rem;
		letter-spacing: -0.02em;
		align-self: center;
	}

	.row-body {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.row-title {
		margin: 0;
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.5rem;
	}
	.cat-name {
		font-size: 1rem;
		font-weight: 500;
		color: var(--ink);
	}
	.row-cat {
		font-size: 0.62rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.18em;
		color: var(--accent);
		background: var(--accent-soft);
		padding: 0.18rem 0.5rem;
		border-radius: 0.3rem;
	}
	.row-settled {
		font-size: 0.62rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.18em;
		color: var(--ok);
		background: rgba(79, 110, 92, 0.1);
		padding: 0.18rem 0.5rem;
		border-radius: 0.3rem;
		border: 1px solid rgba(79, 110, 92, 0.2);
	}
	.row-pending {
		font-size: 0.62rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.18em;
		color: var(--accent-deep);
		background: var(--accent-soft);
		padding: 0.18rem 0.5rem;
		border-radius: 0.3rem;
		border: 1px solid var(--accent);
	}
	.row-template {
		font-size: 0.6rem;
		font-weight: 600;
		text-transform: lowercase;
		font-style: italic;
		color: var(--ink-3);
		background: var(--bg-warm);
		padding: 0.16rem 0.45rem;
		border-radius: 0.3rem;
		border: 1px solid var(--hairline-2);
	}
	.row-paired {
		font-size: 0.62rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.16em;
		color: var(--rdc);
		background: var(--rdc-soft);
		padding: 0.18rem 0.5rem;
		border-radius: 0.3rem;
		border: 1px solid rgba(90, 116, 97, 0.25);
	}
	/* ─── Settlement row variant ─── */
	.settlement-row {
		background: var(--bg-warm);
	}
	.settlement-row .row-mono.settlement-glyph {
		background: var(--accent-soft);
		color: var(--accent-deep);
		font-family: var(--display);
		font-size: 1.4rem;
		font-style: italic;
		border-radius: 999px;
	}
	.settlement-direction {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.85rem;
		color: var(--ink-2);
		margin-left: 0.4rem;
	}
	.settlement-arrow {
		color: var(--accent);
		font-family: var(--display);
		font-size: 1rem;
	}
	.settlement-amount {
		color: var(--accent-deep);
		font-feature-settings: 'tnum';
	}
	/* ─── Hero "Régler" button ─── */
	.hero-settle-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		margin-top: 1rem;
		padding: 0.55rem 1.1rem;
		font-family: var(--ui);
		font-size: 0.85rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--bg);
		background: var(--accent);
		border: 0;
		border-radius: 999px;
		cursor: pointer;
		transition:
			transform 160ms,
			background 160ms;
	}
	.hero-settle-btn:hover {
		transform: translateY(-1px);
		background: var(--accent-deep);
	}
	.hero-settle-glyph {
		font-family: var(--display);
		font-size: 1.1rem;
		font-style: italic;
		line-height: 1;
	}
	/* ─── Linked-expenses picker (settlement modal) ─── */
	.link-group {
		border: 1px dashed var(--hairline-2);
		border-radius: 0.7rem;
		padding: 0.85rem 1rem;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		background: var(--bg);
	}
	.link-group legend {
		padding: 0 0.4rem;
	}
	.link-counter {
		font-size: 0.78rem;
		color: var(--ink-3);
		margin: 0;
	}
	.link-counter strong {
		color: var(--ink);
		font-feature-settings: 'tnum';
	}
	.link-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		max-height: 240px;
		overflow-y: auto;
	}
	.link-item {
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.5rem;
		padding: 0.45rem 0.65rem;
		transition:
			background 160ms,
			border-color 160ms;
	}
	.link-item-on {
		background: var(--accent-soft);
		border-color: var(--accent);
	}
	.link-item label {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		cursor: pointer;
	}
	.link-item input[type='checkbox'] {
		accent-color: var(--accent);
		width: 1rem;
		height: 1rem;
		flex-shrink: 0;
	}
	.link-item-body {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		min-width: 0;
		flex: 1;
	}
	.link-item-name {
		font-size: 0.88rem;
		color: var(--ink);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.link-item-meta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		font-size: 0.74rem;
		color: var(--ink-3);
	}
	.link-item-amt {
		margin-left: auto;
		font-feature-settings: 'tnum';
		color: var(--ink-2);
	}
	.row-amount-cta {
		font-family: var(--display);
		font-size: 0.95rem;
		font-style: italic;
		font-weight: 500;
		color: var(--accent-deep);
		background: var(--accent-soft);
		border: 1px dashed var(--accent);
		border-radius: 0.55rem;
		padding: 0.4rem 0.85rem;
		cursor: pointer;
		transition:
			background 160ms,
			transform 160ms;
		white-space: nowrap;
	}
	.row-amount-cta:hover {
		background: var(--accent);
		color: var(--bg);
		transform: translateY(-1px);
	}
	.pending-banner {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		padding: 0.7rem 1rem;
		margin: 0 0 1rem;
		background: var(--accent-soft);
		border: 1px solid var(--accent);
		border-radius: 0.65rem;
		font-size: 0.85rem;
		color: var(--accent-deep);
	}

	/* ─── Filter bar + panel ─── */
	.filter-bar {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		align-items: center;
		margin: 0 0 0.6rem;
	}
	.filter-search {
		flex: 1;
		min-width: 200px;
		padding: 0.55rem 0.85rem;
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		font-family: var(--ui);
		font-size: 0.9rem;
		background: var(--surface);
		color: var(--ink);
	}
	.filter-search:focus {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-soft);
	}
	.filter-toggle {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.45rem 0.95rem;
		font-family: var(--ui);
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--ink-2);
		background: var(--surface);
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		cursor: pointer;
	}
	.filter-toggle.on,
	.filter-toggle:hover {
		background: var(--accent-soft);
		color: var(--accent-deep);
		border-color: var(--accent);
	}
	.filter-count {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.2rem;
		height: 1.2rem;
		padding: 0 0.35rem;
		background: var(--accent);
		color: var(--bg);
		border-radius: 999px;
		font-size: 0.7rem;
		font-feature-settings: 'tnum';
	}
	.filter-reset {
		font-family: var(--ui);
		font-size: 0.78rem;
		color: var(--accent);
		background: transparent;
		border: 0;
		padding: 0.4rem 0.6rem;
		cursor: pointer;
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.filter-panel {
		display: flex;
		flex-direction: column;
		gap: 0.65rem;
		padding: 0.85rem 1rem 1rem;
		background: var(--bg-warm);
		border: 1px solid var(--hairline);
		border-radius: 0.7rem;
		margin: 0 0 1rem;
	}
	.filter-row {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
	.filter-row-chips {
		align-items: flex-start;
	}
	.filter-row-dates {
		gap: 0.4rem;
	}
	.filter-label {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.16em;
		color: var(--ink-3);
		font-weight: 600;
		min-width: 5.5rem;
	}
	.filter-date {
		font-family: var(--ui);
		font-size: 0.85rem;
		padding: 0.35rem 0.55rem;
		border: 1px solid var(--hairline-2);
		border-radius: 0.45rem;
		background: var(--surface);
		color: var(--ink);
	}
	.filter-date-sep {
		color: var(--ink-3);
		font-family: var(--display);
	}
	.filter-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		flex: 1;
	}
	.filter-chip {
		font-family: var(--ui);
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--ink-2);
		background: var(--surface);
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		padding: 0.22rem 0.7rem;
		cursor: pointer;
	}
	.filter-chip:hover {
		border-color: var(--accent);
	}
	.filter-chip.on {
		background: var(--accent);
		color: var(--bg);
		border-color: var(--accent);
	}
	.seg {
		display: inline-flex;
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		overflow: hidden;
		flex-wrap: wrap;
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
	.pending-banner-glyph {
		font-family: var(--display);
		font-style: italic;
		font-size: 1.15rem;
		color: var(--accent);
		flex-shrink: 0;
	}
	.pending-banner-text strong {
		font-feature-settings: 'tnum';
	}
	.row-dates {
		margin: 0.25rem 0 0;
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
		font-size: 0.78rem;
		color: var(--ink-3);
	}
	.row-dates .meta-label {
		margin-right: 0.25rem;
	}

	.settled-group {
		border: 1px dashed var(--hairline-2);
		border-radius: 0.7rem;
		padding: 0.85rem 1rem;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		background: var(--bg);
	}
	.settled-group legend {
		padding: 0 0.4rem;
	}
	.settled-toggle {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		font-size: 0.9rem;
		color: var(--ink-2);
		cursor: pointer;
	}
	.settled-toggle input[type='checkbox'] {
		width: 1rem;
		height: 1rem;
		accent-color: var(--ok);
		margin: 0;
	}

	/* ─── Attach group (modal) ─── */
	.attach-group {
		border: 1px dashed var(--hairline-2);
		border-radius: 0.7rem;
		padding: 0.85rem 1rem;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		background: var(--bg);
	}
	.attach-group legend {
		padding: 0 0.4rem;
	}
	.attach-existing,
	.attach-queue {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.attach-existing-item,
	.attach-queue-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.35rem 0.55rem;
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.45rem;
		font-size: 0.85rem;
	}
	.attach-queue-item.uploading {
		border-color: var(--accent);
		background: var(--accent-soft);
	}
	.attach-name {
		flex: 1 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--ink);
	}
	.attach-size {
		font-size: 0.72rem;
		color: var(--ink-3);
		font-feature-settings: 'tnum';
		flex-shrink: 0;
	}
	.attach-status {
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--accent-deep);
	}
	.attach-status-ok {
		color: var(--ok);
	}
	.attach-picker {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}
	.attach-picker input[type='file'] {
		font-family: var(--ui);
		font-size: 0.85rem;
		color: var(--ink-2);
	}
	.attach-picker input[type='file']::file-selector-button {
		font-family: var(--ui);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--ink-2);
		background: var(--bg-warm);
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		padding: 0.32rem 0.85rem;
		margin-right: 0.6rem;
		cursor: pointer;
	}
	.attach-picker input[type='file']::file-selector-button:hover {
		background: var(--accent-soft);
		color: var(--accent-deep);
		border-color: var(--accent);
	}
	.attach-hint {
		font-size: 0.72rem;
		color: var(--ink-3);
	}
	.row-note {
		font-size: 0.82rem;
		color: var(--ink-3);
		font-style: italic;
	}
	.row-meta {
		margin: 0;
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.45rem;
		font-size: 0.82rem;
		color: var(--ink-3);
	}
	.meta-label {
		font-size: 0.7rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--ink-4);
		font-weight: 600;
	}
	.row-mode {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.15em;
		color: var(--ink-3);
		font-weight: 600;
		padding: 0.18rem 0.55rem;
		border: 1px solid var(--hairline);
		border-radius: 999px;
		background: var(--bg);
	}
	.row-mode .mode-glyph {
		font-family: var(--display);
		font-size: 0.95rem;
		font-style: normal;
		color: var(--accent);
		letter-spacing: 0;
	}
	.row-shares {
		margin: 0.2rem 0 0;
		display: flex;
		gap: 0.6rem;
		align-items: baseline;
		font-size: 0.82rem;
		color: var(--ink-3);
	}
	.share {
		display: inline-flex;
		align-items: baseline;
		gap: 0.4rem;
	}
	.share-label {
		font-size: 0.65rem;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--ink-4);
		font-weight: 600;
	}
	.share-amt {
		color: var(--ink);
		font-family: var(--display);
		font-weight: 400;
		font-size: 0.92rem;
		font-feature-settings: 'tnum' 1, 'lnum' 1;
	}
	.share-sep {
		color: var(--hairline-2);
	}

	.row-amount {
		font-family: var(--display);
		font-size: 1.45rem;
		font-weight: 500;
		letter-spacing: -0.02em;
		text-align: right;
		font-feature-settings: 'tnum' 1, 'lnum' 1;
		font-variant-numeric: tabular-nums lining-nums;
		color: var(--ink);
	}
	.row-right {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 0.4rem;
		align-self: center;
	}
	.row-actions {
		display: flex;
		gap: 0.3rem;
		opacity: 0;
		transition: opacity 160ms ease;
	}
	.row:hover .row-actions,
	.row:focus-within .row-actions {
		opacity: 1;
	}
	@media (hover: none) {
		/* Touch devices have no hover — keep the actions visible. */
		.row-actions {
			opacity: 1;
		}
	}
	/* ─── Attachments: chip + drawer + thumbnails ─── */
	.row-attach-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--ink-3);
		background: transparent;
		border: 1px solid var(--hairline-2);
		border-radius: 999px;
		padding: 0.18rem 0.55rem;
		cursor: pointer;
		transition:
			background 160ms,
			color 160ms,
			border-color 160ms;
	}
	.row-attach-chip:hover,
	.row-attach-chip[aria-expanded='true'] {
		background: var(--accent-soft);
		color: var(--accent-deep);
		border-color: var(--accent);
	}
	.paperclip {
		display: inline-block;
		font-family: var(--display);
		font-size: 0.95rem;
		line-height: 1;
		transform: rotate(-30deg);
		font-style: italic;
	}
	.row-attach-count {
		font-feature-settings: 'tnum';
	}
	.row-drawer {
		grid-column: 1 / -1;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 0.7rem;
		margin: 0.4rem 0 0.2rem;
		padding: 0.7rem 0.9rem;
		background: var(--bg-warm);
		border: 1px solid var(--hairline);
		border-radius: 0.7rem;
	}
	.att-card {
		display: grid;
		grid-template-columns: 80px 1fr;
		grid-template-rows: auto auto;
		gap: 0.45rem 0.7rem;
		align-items: center;
	}
	.att-thumb {
		width: 80px;
		height: 80px;
		border-radius: 0.5rem;
		border: 1px solid var(--hairline);
		background: var(--surface);
		overflow: hidden;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0;
		grid-row: span 2;
		cursor: pointer;
	}
	.att-thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}
	.att-thumb-loading {
		background: linear-gradient(90deg, var(--bg-warm) 25%, var(--surface) 50%, var(--bg-warm) 75%);
		background-size: 200% 100%;
		animation: att-shimmer 1.1s linear infinite;
	}
	@keyframes att-shimmer {
		0% { background-position: 200% 0; }
		100% { background-position: -200% 0; }
	}
	.att-thumb-failed {
		color: var(--danger);
		font-weight: 700;
		font-size: 1.5rem;
	}
	.att-thumb-pdf {
		color: var(--clay);
		background: var(--clay-soft);
		font-family: var(--display);
		font-weight: 600;
		font-size: 0.95rem;
		letter-spacing: 0.12em;
		cursor: default;
	}
	.att-meta {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		min-width: 0;
	}
	.att-meta-name {
		font-size: 0.85rem;
		color: var(--ink);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.att-meta-size {
		font-size: 0.7rem;
		color: var(--ink-3);
		font-feature-settings: 'tnum';
	}
	.att-actions {
		display: flex;
		gap: 0.3rem;
		align-self: end;
	}

	/* =========================================================
	   EMPTY / PLACEHOLDER / ERROR
	   ========================================================= */
	.empty {
		background: var(--surface);
		border: 1px dashed var(--hairline-2);
		border-radius: 1rem;
		padding: 3rem 1.5rem 2.4rem;
		text-align: center;
	}
	.empty-mark {
		font-family: var(--display);
		font-size: 2.2rem;
		color: var(--accent);
		line-height: 1;
		margin-bottom: 0.85rem;
	}
	.empty h3 {
		margin: 0 0 0.5rem;
		font-family: var(--display);
		font-style: italic;
		font-weight: 400;
		font-size: 1.45rem;
		color: var(--ink);
		letter-spacing: -0.005em;
	}
	.empty p {
		margin: 0 auto 1.4rem;
		max-width: 26rem;
		color: var(--ink-3);
		font-size: 0.92rem;
		line-height: 1.55;
	}
	.empty-cta {
		background: var(--ink);
		color: var(--surface);
		border: 0;
		padding: 0.7rem 1.2rem;
		border-radius: 999px;
		font-family: var(--ui);
		font-weight: 600;
		font-size: 0.85rem;
		cursor: pointer;
		transition:
			background 200ms,
			transform 200ms;
	}
	.empty-cta:hover {
		background: var(--accent);
		transform: translateY(-1px);
	}

	.placeholder {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		padding: 1rem;
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 1rem;
	}
	.placeholder-bar {
		height: 14px;
		border-radius: 4px;
		background: linear-gradient(
			90deg,
			var(--bg-warm),
			var(--bg) 40%,
			var(--bg-warm) 80%
		);
		background-size: 200% 100%;
		animation: shimmer 1.4s infinite linear;
	}
	.placeholder-bar.short {
		width: 60%;
	}

	.error-card {
		background: rgba(183, 50, 35, 0.06);
		color: var(--danger);
		border: 1px solid rgba(183, 50, 35, 0.2);
		border-radius: 0.85rem;
		padding: 0.9rem 1rem;
		font-size: 0.88rem;
	}
	.error-card .error-hint {
		display: block;
		margin-top: 0.4rem;
		color: var(--ink-2);
		font-size: 0.78rem;
	}
	.error-card code {
		font-family:
			'SFMono-Regular', 'Menlo', monospace;
		font-size: 0.78rem;
		background: rgba(20, 16, 12, 0.05);
		padding: 0.05rem 0.3rem;
		border-radius: 0.25rem;
		color: var(--ink);
	}

	/* ─── Chooser sheet (Vide / Depuis un modèle) ─── */
	.chooser {
		position: fixed;
		left: 50%;
		bottom: 1.4rem;
		transform: translateX(-50%);
		width: min(420px, calc(100vw - 2rem));
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 1rem;
		box-shadow: 0 24px 60px rgba(20, 16, 12, 0.2);
		z-index: 60;
		display: flex;
		flex-direction: column;
		animation: slide-up 220ms cubic-bezier(0.2, 0.8, 0.2, 1);
	}
	@keyframes slide-up {
		from { transform: translate(-50%, 16px); opacity: 0; }
		to   { transform: translate(-50%, 0);    opacity: 1; }
	}
	.chooser-head {
		padding: 1rem 1.2rem 0.4rem;
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.chooser-head h2 {
		font-family: var(--display);
		font-weight: 400;
		font-size: 1.15rem;
		margin: 0;
	}
	.chooser-body {
		padding: 0.5rem 0.85rem 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.chooser-opt {
		display: flex;
		align-items: center;
		gap: 0.85rem;
		padding: 0.85rem 1rem;
		background: var(--bg);
		border: 1px solid var(--hairline-2);
		border-radius: 0.75rem;
		text-align: left;
		cursor: pointer;
		transition:
			background 160ms,
			border-color 160ms,
			transform 160ms;
		font-family: var(--ui);
		color: var(--ink);
	}
	.chooser-opt:hover {
		background: var(--accent-soft);
		border-color: var(--accent);
		transform: translateY(-1px);
	}
	.chooser-opt-glyph {
		font-family: var(--display);
		font-size: 1.5rem;
		line-height: 1;
		color: var(--accent);
		font-style: italic;
		flex-shrink: 0;
		width: 1.6rem;
		text-align: center;
	}
	.chooser-opt-text {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		min-width: 0;
	}
	.chooser-opt-text strong {
		font-weight: 600;
		font-size: 0.95rem;
	}
	.chooser-opt-text small {
		font-size: 0.78rem;
		color: var(--ink-3);
	}

	/* ─── Template picker sheet ─── */
	.picker {
		position: fixed;
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
		width: min(480px, calc(100vw - 2rem));
		max-height: calc(100vh - 2rem);
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 1rem;
		box-shadow: 0 24px 60px rgba(20, 16, 12, 0.2);
		z-index: 60;
		display: flex;
		flex-direction: column;
		animation: fade-in 220ms ease;
	}
	.picker-head {
		padding: 1rem 1.2rem 0.4rem;
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}
	.picker-head h2 {
		font-family: var(--display);
		font-weight: 400;
		font-size: 1.3rem;
		margin: 0;
	}
	.picker-list {
		list-style: none;
		margin: 0;
		padding: 0.4rem 0.85rem;
		overflow: auto;
		max-height: 60vh;
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}
	.picker-item {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.3rem;
		padding: 0.7rem 0.85rem;
		background: var(--bg);
		border: 1px solid var(--hairline-2);
		border-radius: 0.65rem;
		cursor: pointer;
		font-family: var(--ui);
		color: var(--ink);
		text-align: left;
		width: 100%;
	}
	.picker-item:hover {
		background: var(--accent-soft);
		border-color: var(--accent);
	}
	.picker-item-name {
		font-weight: 600;
		font-size: 0.95rem;
	}
	.picker-item-meta {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		flex-wrap: wrap;
		font-size: 0.78rem;
		color: var(--ink-3);
	}
	.picker-item-amt {
		font-feature-settings: 'tnum';
		color: var(--ink-2);
	}
	.picker-item-pending {
		font-style: italic;
		color: var(--accent);
	}
	.picker-foot {
		padding: 0.7rem 1.2rem 1rem;
		text-align: right;
	}

	/* =========================================================
	   MODAL
	   ========================================================= */
	.modal-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(20, 16, 12, 0.45);
		backdrop-filter: blur(6px);
		-webkit-backdrop-filter: blur(6px);
		z-index: 40;
		animation: fade-in 200ms ease;
	}
	.modal {
		position: fixed;
		inset: auto 0 0 0;
		z-index: 50;
		background: var(--surface);
		border-top: 1px solid var(--hairline);
		border-radius: 1.4rem 1.4rem 0 0;
		box-shadow: var(--shadow-lg);
		max-height: 92vh;
		overflow-y: auto;
		animation: slide-up 280ms cubic-bezier(0.2, 0.8, 0.2, 1);
		padding-bottom: env(safe-area-inset-bottom, 0px);
	}
	@media (min-width: 720px) {
		.modal {
			inset: auto auto 50% 50%;
			transform: translate(-50%, 50%);
			width: min(560px, calc(100vw - 2rem));
			max-height: 88vh;
			border-radius: 1.2rem;
			border: 1px solid var(--hairline);
			animation: pop 260ms cubic-bezier(0.2, 0.8, 0.2, 1);
		}
	}

	.modal-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		padding: 1.4rem 1.5rem 0.6rem;
	}
	.modal-eyebrow {
		margin: 0 0 0.3rem;
		font-size: 0.62rem;
		letter-spacing: 0.3em;
		text-transform: uppercase;
		color: var(--ink-3);
		font-weight: 600;
	}
	.modal-head h2 {
		margin: 0;
		font-family: var(--display);
		font-style: italic;
		font-weight: 400;
		font-size: 1.6rem;
		letter-spacing: -0.01em;
	}
	.modal-body {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 0.8rem 1.5rem 1.6rem;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		color: var(--ink-2);
	}
	.lbl {
		font-size: 0.62rem;
		text-transform: uppercase;
		letter-spacing: 0.22em;
		color: var(--ink-3);
		font-weight: 700;
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
		transition:
			background 160ms,
			border-color 160ms;
	}
	.inline-cat-trigger:hover {
		background: var(--accent-soft);
		border-color: var(--accent);
	}
	.inline-cat {
		display: flex;
		gap: 0.4rem;
		align-items: stretch;
		flex-wrap: wrap;
	}
	.inline-cat input {
		flex: 1;
		min-width: 0;
		font-family: var(--ui);
		font-size: 0.9rem;
		padding: 0.5rem 0.7rem;
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
		margin: 0;
	}
	.modal-body input,
	.modal-body select {
		font-family: var(--ui);
		font-size: 1rem;
		color: var(--ink);
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.6rem;
		padding: 0.7rem 0.85rem;
		transition:
			border-color 150ms,
			box-shadow 150ms;
		appearance: none;
	}
	.modal-body select {
		background-image:
			linear-gradient(45deg, transparent 50%, var(--ink-3) 50%),
			linear-gradient(135deg, var(--ink-3) 50%, transparent 50%);
		background-position:
			calc(100% - 18px) 50%,
			calc(100% - 13px) 50%;
		background-size:
			5px 5px,
			5px 5px;
		background-repeat: no-repeat;
		padding-right: 2rem;
	}
	.modal-body input:focus,
	.modal-body select:focus {
		outline: none;
		border-color: var(--ink);
		box-shadow: 0 0 0 3px rgba(20, 16, 12, 0.06);
	}
	.input-suffix {
		position: relative;
	}
	.input-suffix input {
		width: 100%;
		padding-right: 2rem;
	}
	.input-suffix .suffix {
		position: absolute;
		right: 0.85rem;
		top: 50%;
		transform: translateY(-50%);
		color: var(--ink-3);
		font-family: var(--display);
		font-size: 0.95rem;
		pointer-events: none;
	}
	.grid-2 {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1rem;
	}

	.mode-group {
		border: 0;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.mode-group legend {
		padding: 0;
	}
	.mode-tabs {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.4rem;
		background: var(--bg);
		border: 1px solid var(--hairline);
		border-radius: 0.7rem;
		padding: 0.3rem;
	}
	.mode-tabs button {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.15rem;
		padding: 0.7rem 0.4rem;
		background: transparent;
		border: 0;
		border-radius: 0.5rem;
		cursor: pointer;
		font-family: var(--ui);
		color: var(--ink-2);
		transition:
			background 180ms,
			color 180ms,
			box-shadow 180ms;
	}
	.mode-tabs button.active {
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--shadow-sm);
	}
	.mode-tabs .mt-glyph {
		font-family: var(--display);
		font-size: 1.25rem;
		line-height: 1;
		color: var(--accent);
	}
	.mode-tabs .mt-name {
		font-size: 0.85rem;
		font-weight: 600;
	}
	.mode-tabs .mt-sub {
		font-size: 0.65rem;
		letter-spacing: 0.05em;
		color: var(--ink-3);
	}
	.mode-tabs button.active .mt-sub {
		color: var(--ink-2);
	}

	.custom-pane {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 0.9rem 1rem 1.05rem;
		background: var(--bg);
		border: 1px dashed var(--hairline-2);
		border-radius: 0.7rem;
	}

	.custom-sub-tabs {
		align-self: flex-start;
		display: inline-flex;
		gap: 0.2rem;
		padding: 0.22rem;
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 999px;
		box-shadow: var(--shadow-sm);
	}
	.custom-sub-tabs button {
		border: 0;
		background: transparent;
		padding: 0.42rem 0.95rem;
		border-radius: 999px;
		font-family: var(--ui);
		font-size: 0.78rem;
		font-weight: 600;
		letter-spacing: 0.01em;
		color: var(--ink-3);
		cursor: pointer;
		transition: background 180ms, color 180ms;
	}
	.custom-sub-tabs button.active {
		background: var(--ink);
		color: var(--surface);
	}

	/* Pourcentage pane */
	.percent-pane {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding-top: 0.25rem;
	}
	.percent-row {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1rem;
	}
	.percent-cell {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.45rem;
		padding: 0.4rem 0.2rem 0;
	}
	.percent-label {
		font-size: 0.65rem;
		letter-spacing: 0.22em;
		text-transform: uppercase;
		color: var(--ink-3);
		font-weight: 700;
	}
	.pct-field {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 0.55rem;
		padding: 0.5rem 0.75rem;
		transition: border-color 150ms, box-shadow 150ms;
	}
	.pct-field:focus-within {
		border-color: var(--ink);
		box-shadow: 0 0 0 3px rgba(20, 16, 12, 0.06);
	}
	.pct-field input {
		width: 2.6rem;
		border: 0;
		background: transparent;
		padding: 0;
		text-align: right;
		font-family: var(--display);
		font-size: 1.15rem;
		font-weight: 500;
		color: var(--ink);
		font-feature-settings: 'tnum' 1, 'lnum' 1;
		font-variant-numeric: tabular-nums lining-nums;
		outline: none;
		appearance: textfield;
		-moz-appearance: textfield;
	}
	.pct-field input::-webkit-outer-spin-button,
	.pct-field input::-webkit-inner-spin-button {
		-webkit-appearance: none;
		margin: 0;
	}
	.pct-suffix {
		color: var(--ink-3);
		font-family: var(--display);
		font-size: 1rem;
	}
	.percent-eur {
		font-family: var(--display);
		font-size: 0.9rem;
		color: var(--ink-2);
		font-feature-settings: 'tnum' 1, 'lnum' 1;
		font-variant-numeric: tabular-nums lining-nums;
	}

	/* Slider — single thumb whose position represents RDC%.
	   Track is two-tone (sage left = RDC, clay right = 1ᵉʳ) so the
	   colour split mirrors the foyer tags above. */
	.percent-slider {
		-webkit-appearance: none;
		appearance: none;
		width: 100%;
		height: 22px;
		background: transparent;
		cursor: pointer;
		margin: 0.4rem 0 0;
	}
	.percent-slider:focus {
		outline: none;
	}
	.percent-slider::-webkit-slider-runnable-track {
		height: 6px;
		border-radius: 999px;
		background: linear-gradient(
			to right,
			var(--rdc) 0%,
			var(--rdc) calc(var(--p, 50) * 1%),
			var(--clay) calc(var(--p, 50) * 1%),
			var(--clay) 100%
		);
	}
	.percent-slider::-webkit-slider-thumb {
		-webkit-appearance: none;
		appearance: none;
		width: 22px;
		height: 22px;
		margin-top: -8px;
		background: var(--surface);
		border: 2px solid var(--ink);
		border-radius: 999px;
		cursor: grab;
		box-shadow: var(--shadow-sm);
		transition: transform 150ms;
	}
	.percent-slider::-webkit-slider-thumb:hover {
		transform: scale(1.08);
	}
	.percent-slider::-webkit-slider-thumb:active {
		cursor: grabbing;
		transform: scale(1.04);
	}
	.percent-slider:focus::-webkit-slider-thumb {
		box-shadow: 0 0 0 4px rgba(20, 16, 12, 0.1);
	}

	.percent-slider::-moz-range-track {
		height: 6px;
		border-radius: 999px;
		background: linear-gradient(
			to right,
			var(--rdc) 0%,
			var(--rdc) calc(var(--p, 50) * 1%),
			var(--clay) calc(var(--p, 50) * 1%),
			var(--clay) 100%
		);
		border: 0;
	}
	.percent-slider::-moz-range-thumb {
		width: 18px;
		height: 18px;
		background: var(--surface);
		border: 2px solid var(--ink);
		border-radius: 999px;
		cursor: grab;
		box-shadow: var(--shadow-sm);
	}

	.percent-hint {
		margin: 0;
		font-size: 0.78rem;
		color: var(--ink-3);
		text-align: center;
		font-style: italic;
	}

	.modal-actions {
		display: flex;
		gap: 0.6rem;
		justify-content: flex-end;
		margin-top: 0.4rem;
	}
	.form-error {
		margin: 0;
		color: var(--danger);
		font-size: 0.85rem;
		background: rgba(183, 50, 35, 0.06);
		border: 1px solid rgba(183, 50, 35, 0.2);
		padding: 0.55rem 0.75rem;
		border-radius: 0.5rem;
	}

	/* =========================================================
	   ANIMATIONS
	   ========================================================= */
	@keyframes fade-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
	@keyframes fade-up {
		from {
			opacity: 0;
			transform: translateY(8px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}
	@keyframes row-in {
		from {
			opacity: 0;
			transform: translateY(4px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}
	@keyframes slide-up {
		from {
			transform: translateY(100%);
		}
		to {
			transform: translateY(0);
		}
	}
	@keyframes pop {
		from {
			transform: translate(-50%, 56%);
			opacity: 0;
		}
		to {
			transform: translate(-50%, 50%);
			opacity: 1;
		}
	}
	@keyframes shimmer {
		from {
			background-position: 200% 0;
		}
		to {
			background-position: -200% 0;
		}
	}

	/* =========================================================
	   RESPONSIVE
	   ========================================================= */
	@media (max-width: 540px) {
		.row {
			grid-template-columns: auto auto 1fr;
			grid-template-rows: auto auto;
			gap: 0.7rem 0.85rem;
			padding: 0.95rem 0.95rem;
		}
		.row-amount {
			grid-column: 3;
			grid-row: 1;
			font-size: 1.2rem;
			align-self: start;
			padding-top: 0.05rem;
		}
		.row-body {
			grid-column: 1 / -1;
			grid-row: 2;
		}
		.row-mono,
		.row-day {
			grid-row: 1;
		}
		.hero {
			padding: 2rem 1.25rem 1.4rem;
		}
		.hero-stats {
			gap: 1.5rem;
		}
	}

	/* Hide scrollbar style on the modal sheet for cleanliness */
	.modal::-webkit-scrollbar {
		width: 8px;
	}
	.modal::-webkit-scrollbar-thumb {
		background: var(--hairline-2);
		border-radius: 4px;
	}
</style>
