// Real-time subscriptions to the Firestore collections that the foyer-facing
// UI cares about. Reads come straight from Firestore (auth-gated by the
// rules in infra/firebase/firestore.rules); writes still go through the Go
// API so the share-computation logic stays canonical.

import {
	collection,
	collectionGroup,
	onSnapshot,
	type QueryDocumentSnapshot,
	type Unsubscribe
} from 'firebase/firestore';

import type { Attachment, Category, ExpenseTemplate, Expense, Foyer, Frequency } from './api';
import { firebaseFirestore } from './firebase';

type SnapData = Record<string, unknown>;

/** Convert a Firestore Timestamp / Date / ISO string into an ISO string,
 *  matching the Go API's JSON shape so the UI doesn't care which path
 *  produced the data. */
function isoOf(v: unknown): string {
	if (!v) return '';
	if (typeof v === 'string') return v;
	if (v instanceof Date) return v.toISOString();
	if (typeof v === 'object' && v !== null && 'toDate' in v) {
		try {
			const d = (v as { toDate: () => Date }).toDate();
			return d.toISOString();
		} catch {
			return '';
		}
	}
	return '';
}

function isoOrUndef(v: unknown): string | undefined {
	const out = isoOf(v);
	return out || undefined;
}

function subscribe<T>(
	col: string,
	mapDoc: (snap: QueryDocumentSnapshot, d: SnapData) => T,
	postProcess: (out: T[]) => T[],
	onData: (rows: T[]) => void,
	onError?: (err: Error) => void
): Unsubscribe {
	return onSnapshot(
		collection(firebaseFirestore(), col),
		(snap) => {
			const rows = snap.docs.map((d) => mapDoc(d, d.data() as SnapData));
			onData(postProcess(rows));
		},
		(err) => onError?.(err)
	);
}

const KNOWN_FOYER_FLOORS: ReadonlyArray<Foyer['floor']> = ['rdc', '1er'];
function asFloor(v: unknown): Foyer['floor'] | undefined {
	return KNOWN_FOYER_FLOORS.find((f) => f === v);
}

// ─── Foyers ───────────────────────────────────────────────────────────
export function subscribeFoyers(
	onData: (foyers: Foyer[]) => void,
	onError?: (err: Error) => void
): Unsubscribe {
	return subscribe<Foyer | null>(
		'foyers',
		(snap, d) => {
			const floor = asFloor(d.floor);
			if (!floor) {
				console.warn('foyers: skipping doc with unknown floor', snap.id, d.floor);
				return null;
			}
			return {
				// Always derive the entity id from the Firestore doc key — the
				// `id` field inside the data isn't guaranteed to be present.
				id: snap.id,
				copro_id: String(d.copro_id ?? ''),
				floor,
				name: String(d.name ?? ''),
				parts: Number(d.parts ?? 0),
				member_ids: Array.isArray(d.member_ids) ? (d.member_ids as string[]) : []
			} satisfies Foyer;
		},
		(rows) => rows.filter((r): r is Foyer => r !== null).sort((a, b) => a.floor.localeCompare(b.floor)),
		(out) => onData(out as Foyer[]),
		onError
	);
}

// ─── Categories ──────────────────────────────────────────────────────
export function subscribeCategories(
	onData: (cats: Category[]) => void,
	onError?: (err: Error) => void
): Unsubscribe {
	return subscribe<Category>(
		'categories',
		(snap, d) =>
			({
				id: snap.id,
				name: String(d.name ?? ''),
				predefined: Boolean(d.predefined),
				hidden: Boolean(d.hidden),
				default_distribution_mode:
					d.default_distribution_mode as Category['default_distribution_mode']
			}) satisfies Category,
		// Hide system categories like the CSV-import triage bucket (`tbd`).
		// The API persists them so expenses can FK-reference them, but
		// they shouldn't surface in the user-facing list (PRD FR10 lists 6
		// visible categories).
		(rows) =>
			rows
				.filter((c) => !c.hidden)
				.sort((a, b) => a.name.localeCompare(b.name, 'fr')),
		onData,
		onError
	);
}

// ─── Expenses ────────────────────────────────────────────────────────
//
// Attachments live in the subcollection `expenses/{id}/attachments/{aid}`
// (not inline on the expense doc) since the v2 review. Use
// `subscribeAllAttachments` alongside `subscribeExpenses` and merge by
// expense_id in the page — that keeps the listener overhead at two
// (collection + collectionGroup) regardless of how many expenses exist.

export function subscribeExpenses(
	onData: (exps: Expense[]) => void,
	onError?: (err: Error) => void
): Unsubscribe {
	return subscribe<Expense>(
		'expenses',
		(snap, d) =>
			({
				id: snap.id,
				copro_id: String(d.copro_id ?? ''),
				name: String(d.name ?? ''),
				amount_cents: Number(d.amount_cents ?? 0),
				currency: String(d.currency ?? 'EUR'),
				date: isoOf(d.date),
				payment_date: isoOrUndef(d.payment_date),
				payer_foyer_id: String(d.payer_foyer_id ?? ''),
				category_id: String(d.category_id ?? ''),
				distribution_mode: d.distribution_mode as Expense['distribution_mode'],
				share_rdc_cents: Number(d.share_rdc_cents ?? 0),
				share_1er_cents: Number(d.share_1er_cents ?? 0),
				settled: Boolean(d.settled),
				settled_at: isoOrUndef(d.settled_at),
				note: typeof d.note === 'string' ? d.note : undefined,
				template_id: typeof d.template_id === 'string' && d.template_id ? d.template_id : undefined,
				amount_pending: Boolean(d.amount_pending),
				created_at: isoOf(d.created_at),
				updated_at: isoOf(d.updated_at)
			}) satisfies Expense,
		(rows) =>
			rows.sort((a, b) => {
				if (a.date !== b.date) return b.date.localeCompare(a.date);
				return b.created_at.localeCompare(a.created_at);
			}),
		onData,
		onError
	);
}

// ─── Attachments (subcollection across all expenses) ─────────────────
//
// One collectionGroup listener delivers every attachment doc across every
// expense in the copro. Callers index by `expense_id` to merge into their
// expense rows. The `expense_id` is derived from the doc's parent ref —
// not a stored field — so it's always trustworthy.

export interface ExpenseAttachment extends Attachment {
	expense_id: string;
}

export function subscribeAllAttachments(
	onData: (atts: ExpenseAttachment[]) => void,
	onError?: (err: Error) => void
): Unsubscribe {
	return onSnapshot(
		collectionGroup(firebaseFirestore(), 'attachments'),
		(snap) => {
			const out: ExpenseAttachment[] = [];
			for (const d of snap.docs) {
				const data = d.data() as SnapData;
				// `parent.parent` is the expense doc reference; its id is
				// the expense_id. Defensive: skip orphan docs that somehow
				// don't have a parent expense (shouldn't happen).
				const expenseRef = d.ref.parent.parent;
				if (!expenseRef) continue;
				out.push({
					expense_id: expenseRef.id,
					id: d.id,
					object_name: String(data.object_name ?? ''),
					content_type: String(data.content_type ?? ''),
					size_bytes: Number(data.size_bytes ?? 0),
					original_filename: String(data.original_filename ?? ''),
					uploaded_at: isoOf(data.uploaded_at),
					uploaded_by: String(data.uploaded_by ?? '')
				});
			}
			out.sort((a, b) => a.uploaded_at.localeCompare(b.uploaded_at));
			onData(out);
		},
		(err) => onError?.(err)
	);
}

// ─── Templates ───────────────────────────────────────────────────────
const KNOWN_FREQUENCIES: ReadonlyArray<Frequency> = ['monthly', 'quarterly', 'yearly'];
function asFrequency(v: unknown): Frequency | undefined {
	return KNOWN_FREQUENCIES.find((f) => f === v);
}

const KNOWN_DIST_MODES: ReadonlyArray<ExpenseTemplate['distribution_mode']> = [
	'equal',
	'tantiemes',
	'custom'
];
function asDistMode(v: unknown): ExpenseTemplate['distribution_mode'] {
	const found = KNOWN_DIST_MODES.find((m) => m === v);
	// Garbage Firestore data shouldn't crash the UI — default to `equal`
	// and warn so the bad doc is visible in DevTools.
	if (!found) {
		console.warn('subscribeTemplates: unknown distribution_mode, defaulting to equal', v);
		return 'equal';
	}
	return found;
}

export function subscribeTemplates(
	onData: (tpls: ExpenseTemplate[]) => void,
	onError?: (err: Error) => void
): Unsubscribe {
	return subscribe<ExpenseTemplate>(
		'expense_templates',
		(snap, d) =>
			({
				id: snap.id,
				copro_id: String(d.copro_id ?? ''),
				name: String(d.name ?? ''),
				amount_default_cents: Number(d.amount_default_cents ?? 0),
				currency: String(d.currency ?? 'EUR'),
				category_id: String(d.category_id ?? ''),
				payer_foyer_id: String(d.payer_foyer_id ?? ''),
				distribution_mode: asDistMode(d.distribution_mode),
				share_rdc_cents:
					typeof d.share_rdc_cents === 'number' ? Number(d.share_rdc_cents) : undefined,
				share_1er_cents:
					typeof d.share_1er_cents === 'number' ? Number(d.share_1er_cents) : undefined,
				note: typeof d.note === 'string' ? d.note : undefined,
				schedule_active: Boolean(d.schedule_active),
				frequency: asFrequency(d.frequency),
				day_of_month: typeof d.day_of_month === 'number' ? Number(d.day_of_month) : undefined,
				next_occurrence_at: isoOrUndef(d.next_occurrence_at),
				end_date: isoOrUndef(d.end_date),
				created_at: isoOf(d.created_at),
				updated_at: isoOf(d.updated_at)
			}) satisfies ExpenseTemplate,
		(rows) => rows.sort((a, b) => a.name.localeCompare(b.name, 'fr')),
		onData,
		onError
	);
}
