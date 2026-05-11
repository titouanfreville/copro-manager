// Real-time subscriptions to the Firestore collections that the foyer-facing
// UI cares about. Reads come straight from Firestore (auth-gated by the
// rules in infra/firebase/firestore.rules); writes still go through the Go
// API so the share-computation logic stays canonical.

import {
  collection,
  onSnapshot,
  type QueryDocumentSnapshot,
  type Unsubscribe,
} from "firebase/firestore";

import { limit, orderBy, query, where } from "firebase/firestore";

import type {
  Alert,
  Attachment,
  Category,
  Contract,
  ContractStatus,
  ContractExtraction,
  Document,
  DocumentAnalysis,
  DocumentAnalysisKind,
  ExpenseExtraction,
  ExpenseTemplate,
  Expense,
  Foyer,
  Frequency,
  MeterReading,
  Settlement,
} from "./api";
import { firebaseFirestore } from "./firebase";

type SnapData = Record<string, unknown>;

/** Convert a Firestore Timestamp / Date / ISO string into an ISO string,
 *  matching the Go API's JSON shape so the UI doesn't care which path
 *  produced the data. */
function isoOf(v: unknown): string {
  if (!v) return "";
  if (typeof v === "string") return v;
  if (v instanceof Date) return v.toISOString();
  if (typeof v === "object" && v !== null && "toDate" in v) {
    try {
      const d = (v as { toDate: () => Date }).toDate();
      return d.toISOString();
    } catch {
      return "";
    }
  }
  return "";
}

function isoOrUndef(v: unknown): string | undefined {
  const out = isoOf(v);
  return out || undefined;
}

const KNOWN_DOC_ANALYSIS_KINDS: ReadonlyArray<DocumentAnalysisKind> = [
  "expense",
  "contract",
  "other",
];

function asAnalysisKind(v: unknown): DocumentAnalysisKind | undefined {
  return typeof v === "string" &&
    (KNOWN_DOC_ANALYSIS_KINDS as ReadonlyArray<string>).includes(v)
    ? (v as DocumentAnalysisKind)
    : undefined;
}

function asNonBlankStr(v: unknown): string | undefined {
  return typeof v === "string" && v.length > 0 ? v : undefined;
}

function asFiniteNumber(v: unknown): number | undefined {
  return typeof v === "number" && Number.isFinite(v) ? v : undefined;
}

function mapExpenseExtraction(raw: unknown): ExpenseExtraction | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const d = raw as SnapData;
  const out: ExpenseExtraction = {};
  const amt = asFiniteNumber(d.amount_eur);
  if (amt !== undefined) out.amount_eur = amt;
  const date = asNonBlankStr(d.date);
  if (date) out.date = date;
  const vendor = asNonBlankStr(d.vendor);
  if (vendor) out.vendor = vendor;
  const hint = asNonBlankStr(d.category_hint);
  if (hint) out.category_hint = hint;
  const desc = asNonBlankStr(d.description);
  if (desc) out.description = desc;
  return Object.keys(out).length > 0 ? out : undefined;
}

function mapContractExtraction(raw: unknown): ContractExtraction | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const d = raw as SnapData;
  const out: ContractExtraction = {};
  const provider = asNonBlankStr(d.provider);
  if (provider) out.provider = provider;
  const ctype = asNonBlankStr(d.contract_type);
  if (ctype) out.contract_type = ctype;
  const start = asNonBlankStr(d.start_date);
  if (start) out.start_date = start;
  const end = asNonBlankStr(d.end_date);
  if (end) out.end_date = end;
  const monthly = asFiniteNumber(d.monthly_amount_eur);
  if (monthly !== undefined) out.monthly_amount_eur = monthly;
  const num = asNonBlankStr(d.contract_number);
  if (num) out.contract_number = num;
  return Object.keys(out).length > 0 ? out : undefined;
}

function mapAnalysis(raw: unknown): DocumentAnalysis | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const d = raw as SnapData;
  const kind = asAnalysisKind(d.kind);
  if (!kind) return undefined;
  return {
    kind,
    confidence:
      asFiniteNumber(d.confidence) ?? 0,
    analyzed_at: isoOf(d.analyzed_at),
    model: typeof d.model === "string" ? d.model : "",
    reason: asNonBlankStr(d.reason),
    expense: kind === "expense" ? mapExpenseExtraction(d.expense) : undefined,
    contract:
      kind === "contract" ? mapContractExtraction(d.contract) : undefined,
  };
}

function subscribe<T>(
  col: string,
  mapDoc: (snap: QueryDocumentSnapshot, d: SnapData) => T,
  postProcess: (out: T[]) => T[],
  onData: (rows: T[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  return onSnapshot(
    collection(firebaseFirestore(), col),
    (snap) => {
      const rows = snap.docs.map((d) => mapDoc(d, d.data() as SnapData));
      onData(postProcess(rows));
    },
    (err) => onError?.(err),
  );
}

const KNOWN_FOYER_FLOORS: ReadonlyArray<Foyer["floor"]> = ["rdc", "1er"];
function asFloor(v: unknown): Foyer["floor"] | undefined {
  return KNOWN_FOYER_FLOORS.find((f) => f === v);
}

// ─── Foyers ───────────────────────────────────────────────────────────
export function subscribeFoyers(
  onData: (foyers: Foyer[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  return subscribe<Foyer | null>(
    "foyers",
    (snap, d) => {
      const floor = asFloor(d.floor);
      if (!floor) {
        console.warn(
          "foyers: skipping doc with unknown floor",
          snap.id,
          d.floor,
        );
        return null;
      }
      return {
        // Always derive the entity id from the Firestore doc key — the
        // `id` field inside the data isn't guaranteed to be present.
        id: snap.id,
        copro_id: String(d.copro_id ?? ""),
        floor,
        name: String(d.name ?? ""),
        parts: Number(d.parts ?? 0),
        member_ids: Array.isArray(d.member_ids)
          ? (d.member_ids as string[])
          : [],
      } satisfies Foyer;
    },
    (rows) =>
      rows
        .filter((r): r is Foyer => r !== null)
        .sort((a, b) => a.floor.localeCompare(b.floor)),
    (out) => onData(out as Foyer[]),
    onError,
  );
}

// ─── Categories ──────────────────────────────────────────────────────
export function subscribeCategories(
  onData: (cats: Category[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  return subscribe<Category>(
    "categories",
    (snap, d) =>
      ({
        id: snap.id,
        name: String(d.name ?? ""),
        predefined: Boolean(d.predefined),
        hidden: Boolean(d.hidden),
        default_distribution_mode:
          d.default_distribution_mode as Category["default_distribution_mode"],
        icon: typeof d.icon === "string" && d.icon ? d.icon : undefined,
        color: typeof d.color === "string" && d.color ? d.color : undefined,
      }) satisfies Category,
    // Hide system categories like the CSV-import triage bucket (`tbd`).
    // The API persists them so expenses can FK-reference them, but
    // they shouldn't surface in the user-facing list (PRD FR10 lists 6
    // visible categories).
    (rows) =>
      rows
        .filter((c) => !c.hidden)
        .sort((a, b) => a.name.localeCompare(b.name, "fr")),
    onData,
    onError,
  );
}

// ─── Expenses ────────────────────────────────────────────────────────
//
// Per-expense attachments are now Documents with `linked_expense_id`
// set (the legacy `expenses/{id}/attachments` subcollection was
// migrated). Pages that need attachments call `subscribeDocuments` and
// filter by `linked_expense_id` — one listener covers both standalone
// and per-expense documents.

export function subscribeExpenses(
  onData: (exps: Expense[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  return subscribe<Expense>(
    "expenses",
    (snap, d) =>
      ({
        id: snap.id,
        copro_id: String(d.copro_id ?? ""),
        name: String(d.name ?? ""),
        amount_cents: Number(d.amount_cents ?? 0),
        currency: String(d.currency ?? "EUR"),
        date: isoOf(d.date),
        payment_date: isoOrUndef(d.payment_date),
        payer_foyer_id: String(d.payer_foyer_id ?? ""),
        category_id: String(d.category_id ?? ""),
        distribution_mode: d.distribution_mode as Expense["distribution_mode"],
        share_rdc_cents: Number(d.share_rdc_cents ?? 0),
        share_1er_cents: Number(d.share_1er_cents ?? 0),
        settled: Boolean(d.settled),
        settled_at: isoOrUndef(d.settled_at),
        note: typeof d.note === "string" ? d.note : undefined,
        template_id:
          typeof d.template_id === "string" && d.template_id
            ? d.template_id
            : undefined,
        amount_pending: Boolean(d.amount_pending),
        meter_reading_period:
          typeof d.meter_reading_period === "string" && d.meter_reading_period
            ? d.meter_reading_period
            : undefined,
        created_at: isoOf(d.created_at),
        updated_at: isoOf(d.updated_at),
      }) satisfies Expense,
    (rows) =>
      rows.sort((a, b) => {
        if (a.date !== b.date) return b.date.localeCompare(a.date);
        return b.created_at.localeCompare(a.created_at);
      }),
    onData,
    onError,
  );
}

// ─── Per-expense attachments (view shape) ────────────────────────────
//
// `ExpenseAttachment` is the shape the expenses page renders for inline
// thumbnails — it's an `Attachment` with the parent expense id attached
// as a flat field. Build it by filtering the unified documents stream
// for docs with `linked_expense_id` set; see /expenses/+page.svelte.
export interface ExpenseAttachment extends Attachment {
  expense_id: string;
}

// ─── Templates ───────────────────────────────────────────────────────
const KNOWN_FREQUENCIES: ReadonlyArray<Frequency> = [
  "monthly",
  "quarterly",
  "yearly",
];
function asFrequency(v: unknown): Frequency | undefined {
  return KNOWN_FREQUENCIES.find((f) => f === v);
}

const KNOWN_DIST_MODES: ReadonlyArray<ExpenseTemplate["distribution_mode"]> = [
  "equal",
  "tantiemes",
  "custom",
  "water_3_meters",
];
function asDistMode(v: unknown): ExpenseTemplate["distribution_mode"] {
  const found = KNOWN_DIST_MODES.find((m) => m === v);
  // Garbage Firestore data shouldn't crash the UI — default to `equal`
  // and warn so the bad doc is visible in DevTools.
  if (!found) {
    console.warn(
      "subscribeTemplates: unknown distribution_mode, defaulting to equal",
      v,
    );
    return "equal";
  }
  return found;
}

export function subscribeTemplates(
  onData: (tpls: ExpenseTemplate[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  return subscribe<ExpenseTemplate>(
    "expense_templates",
    (snap, d) =>
      ({
        id: snap.id,
        copro_id: String(d.copro_id ?? ""),
        name: String(d.name ?? ""),
        amount_default_cents: Number(d.amount_default_cents ?? 0),
        currency: String(d.currency ?? "EUR"),
        category_id: String(d.category_id ?? ""),
        payer_foyer_id: String(d.payer_foyer_id ?? ""),
        distribution_mode: asDistMode(d.distribution_mode),
        share_rdc_cents:
          typeof d.share_rdc_cents === "number"
            ? Number(d.share_rdc_cents)
            : undefined,
        share_1er_cents:
          typeof d.share_1er_cents === "number"
            ? Number(d.share_1er_cents)
            : undefined,
        note: typeof d.note === "string" ? d.note : undefined,
        schedule_active: Boolean(d.schedule_active),
        frequency: asFrequency(d.frequency),
        day_of_month:
          typeof d.day_of_month === "number"
            ? Number(d.day_of_month)
            : undefined,
        next_occurrence_at: isoOrUndef(d.next_occurrence_at),
        end_date: isoOrUndef(d.end_date),
        created_at: isoOf(d.created_at),
        updated_at: isoOf(d.updated_at),
      }) satisfies ExpenseTemplate,
    (rows) => rows.sort((a, b) => a.name.localeCompare(b.name, "fr")),
    onData,
    onError,
  );
}

// ─── Documents (standalone) ─────────────────────────────────────────
export function subscribeDocuments(
  onData: (rows: Document[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  return subscribe<Document>(
    "documents",
    (snap, d) =>
      ({
        id: snap.id,
        copro_id: String(d.copro_id ?? ""),
        category_id: String(d.category_id ?? ""),
        group: typeof d.group === "string" && d.group ? d.group : undefined,
        title: String(d.title ?? ""),
        description:
          typeof d.description === "string" ? d.description : undefined,
        object_name: String(d.object_name ?? ""),
        content_type: String(d.content_type ?? ""),
        size_bytes: Number(d.size_bytes ?? 0),
        original_filename: String(d.original_filename ?? ""),
        uploaded_at: isoOf(d.uploaded_at),
        uploaded_by: String(d.uploaded_by ?? ""),
        linked_expense_id:
          typeof d.linked_expense_id === "string" && d.linked_expense_id
            ? d.linked_expense_id
            : undefined,
        linked_contract_id:
          typeof d.linked_contract_id === "string" && d.linked_contract_id
            ? d.linked_contract_id
            : undefined,
        analysis: mapAnalysis(d.analysis),
      }) satisfies Document,
    (rows) => rows.sort((a, b) => b.uploaded_at.localeCompare(a.uploaded_at)),
    onData,
    onError,
  );
}

// ─── Contracts ──────────────────────────────────────────────────────
//
// Service agreements bound to the copro (insurance, syndic, energy,
// maintenance, …). Both foyers see the full list; mutations still flow
// through the API so server-side validation is the single source of
// truth.

const KNOWN_CONTRACT_STATUSES: ReadonlyArray<ContractStatus> = [
  "active",
  "expired",
  "cancelled",
];
function asContractStatus(v: unknown): ContractStatus {
  return KNOWN_CONTRACT_STATUSES.find((s) => s === v) ?? "active";
}

export function subscribeContracts(
  onData: (rows: Contract[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  return subscribe<Contract>(
    "contracts",
    (snap, d) => {
      const society = (d.society ?? {}) as SnapData;
      const contact = (d.contact ?? {}) as SnapData;
      return {
        id: snap.id,
        copro_id: String(d.copro_id ?? ""),
        name: String(d.name ?? ""),
        category_id: String(d.category_id ?? ""),
        society: {
          name: String(society.name ?? ""),
          phone:
            typeof society.phone === "string" && society.phone
              ? society.phone
              : undefined,
          email:
            typeof society.email === "string" && society.email
              ? society.email
              : undefined,
          website:
            typeof society.website === "string" && society.website
              ? society.website
              : undefined,
          address:
            typeof society.address === "string" && society.address
              ? society.address
              : undefined,
        },
        contact:
          contact.name || contact.role || contact.phone || contact.email
            ? {
                name:
                  typeof contact.name === "string" && contact.name
                    ? contact.name
                    : undefined,
                role:
                  typeof contact.role === "string" && contact.role
                    ? contact.role
                    : undefined,
                phone:
                  typeof contact.phone === "string" && contact.phone
                    ? contact.phone
                    : undefined,
                email:
                  typeof contact.email === "string" && contact.email
                    ? contact.email
                    : undefined,
              }
            : undefined,
        start_date: isoOrUndef(d.start_date),
        end_date: isoOrUndef(d.end_date),
        amount_cents:
          typeof d.amount_cents === "number" ? Number(d.amount_cents) : undefined,
        billing_frequency: asFrequency(d.billing_frequency),
        template_id:
          typeof d.template_id === "string" && d.template_id
            ? d.template_id
            : undefined,
        status: asContractStatus(d.status),
        note: typeof d.note === "string" && d.note ? d.note : undefined,
        created_at: isoOf(d.created_at),
        updated_at: isoOf(d.updated_at),
      } satisfies Contract;
    },
    (rows) => rows.sort((a, b) => a.name.localeCompare(b.name, "fr")),
    onData,
    onError,
  );
}

// ─── Alerts ─────────────────────────────────────────────────────────
//
// Scoped to a foyer rather than fetching the whole copro: the backend
// addresses alerts to a specific foyer, and surfacing a member's own
// foyer's alerts is the only intended UX. We use a Firestore query
// (`recipient_foyer_id == X`) to avoid pulling the other foyer's feed
// down the wire.

export function subscribeAlertsForFoyer(
  foyerID: string,
  onData: (rows: Alert[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  if (!foyerID) {
    // Defensive: callers should pass a non-empty foyer. Return a
    // no-op unsubscribe so the consumer's $effect cleanup doesn't
    // crash in the loading state.
    onData([]);
    return () => {};
  }
  // Bound the live query — most-recent first, capped at 200 rows. The
  // bell badge counts unread out of this window; older alerts roll off
  // organically as the cron resolves them.
  const q = query(
    collection(firebaseFirestore(), "alerts"),
    where("recipient_foyer_id", "==", foyerID),
    orderBy("fired_at", "desc"),
    limit(200),
  );
  return onSnapshot(
    q,
    (snap) => onData(decodeAlertSnapshot(snap)),
    (err) => onError?.(err),
  );
}

// subscribeAlerts is the unfiltered fallback used by AlertsBell when
// the signed-in UID can't be matched to any foyer's member_ids — a
// data-link state where the foyer-scoped query would always return
// empty. Same 200-row cap, same filtering. Firestore rules already gate
// by auth, so this is safe at our 2-foyer scale.
export function subscribeAlerts(
  onData: (rows: Alert[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  const q = query(
    collection(firebaseFirestore(), "alerts"),
    orderBy("fired_at", "desc"),
    limit(200),
  );
  return onSnapshot(
    q,
    (snap) => onData(decodeAlertSnapshot(snap)),
    (err) => onError?.(err),
  );
}

function decodeAlertSnapshot(snap: {
  docs: ReadonlyArray<QueryDocumentSnapshot>;
}): Alert[] {
  const rows: Alert[] = [];
  for (const d of snap.docs) {
    const data = d.data() as SnapData;
    const kind = data.kind;
    if (
      kind !== "pending_completion" &&
      kind !== "missing_receipt" &&
      kind !== "peer_expense_added" &&
      kind !== "balance_seasonal" &&
      kind !== "monthly_meter_reading" &&
      kind !== "contract_expiring"
    ) {
      continue;
    }
    rows.push({
      id: d.id,
      copro_id: String(data.copro_id ?? ""),
      kind,
      recipient_foyer_id: String(data.recipient_foyer_id ?? ""),
      dedupe_key: String(data.dedupe_key ?? ""),
      payload:
        data.payload && typeof data.payload === "object"
          ? (data.payload as Record<string, unknown>)
          : undefined,
      deep_link:
        typeof data.deep_link === "string" ? data.deep_link : undefined,
      fired_at: isoOf(data.fired_at),
      read_at: isoOrUndef(data.read_at),
      resolved_at: isoOrUndef(data.resolved_at),
      dismissed_at: isoOrUndef(data.dismissed_at),
    });
  }
  rows.sort((a, b) => b.fired_at.localeCompare(a.fired_at));
  return rows;
}

// ─── Meter readings ──────────────────────────────────────────────────
//
// Period (YYYY-MM) is the doc id, so the rows arrive already keyed.
// Sorting descending by Period puts the most recent at the top, matching
// the /meters list view's default order.

export function subscribeMeters(
  onData: (rows: MeterReading[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  return subscribe<MeterReading>(
    "meter_readings",
    (snap, d) =>
      ({
        id: String(d.id ?? snap.id),
        copro_id: String(d.copro_id ?? ""),
        period: String(d.period ?? snap.id),
        global_m3: Number(d.global_m3 ?? 0),
        common_m3: Number(d.common_m3 ?? 0),
        rdc_m3: Number(d.rdc_m3 ?? 0),
        premier_m3: Number(d.premier_m3 ?? 0),
        global_photo_object:
          typeof d.global_photo_object === "string" && d.global_photo_object
            ? d.global_photo_object
            : undefined,
        global_photo_content_type:
          typeof d.global_photo_content_type === "string" &&
          d.global_photo_content_type
            ? d.global_photo_content_type
            : undefined,
        global_photo_size_bytes:
          typeof d.global_photo_size_bytes === "number"
            ? Number(d.global_photo_size_bytes)
            : undefined,
        detail_photo_object:
          typeof d.detail_photo_object === "string" && d.detail_photo_object
            ? d.detail_photo_object
            : undefined,
        detail_photo_content_type:
          typeof d.detail_photo_content_type === "string" &&
          d.detail_photo_content_type
            ? d.detail_photo_content_type
            : undefined,
        detail_photo_size_bytes:
          typeof d.detail_photo_size_bytes === "number"
            ? Number(d.detail_photo_size_bytes)
            : undefined,
        captured_at: isoOf(d.captured_at),
        captured_by_uid: String(d.captured_by_uid ?? ""),
        created_at: isoOf(d.created_at),
        updated_at: isoOf(d.updated_at),
      }) satisfies MeterReading,
    (rows) => rows.sort((a, b) => b.period.localeCompare(a.period)),
    onData,
    onError,
  );
}

// ─── Settlements ─────────────────────────────────────────────────────
export function subscribeSettlements(
  onData: (rows: Settlement[]) => void,
  onError?: (err: Error) => void,
): Unsubscribe {
  return subscribe<Settlement>(
    "settlements",
    (snap, d) =>
      ({
        id: snap.id,
        copro_id: String(d.copro_id ?? ""),
        from_foyer_id: String(d.from_foyer_id ?? ""),
        to_foyer_id: String(d.to_foyer_id ?? ""),
        amount_cents: Number(d.amount_cents ?? 0),
        currency: String(d.currency ?? "EUR"),
        date: isoOf(d.date),
        note: typeof d.note === "string" ? d.note : undefined,
        expense_ids: Array.isArray(d.expense_ids)
          ? (d.expense_ids as unknown[]).filter(
              (x): x is string => typeof x === "string",
            )
          : undefined,
        created_at: isoOf(d.created_at),
        updated_at: isoOf(d.updated_at),
      }) satisfies Settlement,
    (rows) =>
      rows.sort((a, b) => {
        if (a.date !== b.date) return b.date.localeCompare(a.date);
        return b.created_at.localeCompare(a.created_at);
      }),
    onData,
    onError,
  );
}
