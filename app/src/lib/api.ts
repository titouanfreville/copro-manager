import { PUBLIC_API_BASE_URL } from "$env/static/public";
import { idToken } from "./auth";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

interface ApiOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  headers?: Record<string, string>;
  /** Optional AbortSignal so callers can cancel an in-flight request. */
  signal?: AbortSignal;
}

export interface Foyer {
  id: string;
  copro_id: string;
  floor: "rdc" | "1er";
  name: string;
  parts: number;
  member_ids: string[];
}

export interface Category {
  id: string;
  name: string;
  predefined: boolean;
  /**
   * System categories (e.g. the CSV-import triage bucket `tbd`) are flagged
   * Hidden by the API and filtered out of the user-facing list — they're
   * persisted only so expenses can keep an FK reference.
   */
  hidden?: boolean;
  default_distribution_mode?: DistributionMode;
  /** Optional emoji or short string rendered next to the name. */
  icon?: string;
  /** Optional `#RRGGBB` accent color. */
  color?: string;
}

export type DistributionMode =
  | "equal"
  | "tantiemes"
  | "custom"
  | "water_3_meters";

export type Frequency = "monthly" | "quarterly" | "yearly";

export interface ExpenseTemplate {
  id: string;
  copro_id: string;
  name: string;
  amount_default_cents: number;
  currency: string;
  category_id: string;
  payer_foyer_id: string;
  distribution_mode: DistributionMode;
  share_rdc_cents?: number;
  share_1er_cents?: number;
  note?: string;
  schedule_active: boolean;
  frequency?: Frequency;
  day_of_month?: number;
  next_occurrence_at?: string;
  end_date?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateTemplateInput {
  name: string;
  amount_default_cents: number;
  currency?: string;
  category_id: string;
  payer_foyer_id: string;
  distribution_mode: DistributionMode;
  share_rdc_cents?: number;
  share_1er_cents?: number;
  note?: string;
  schedule_active?: boolean;
  frequency?: Frequency;
  day_of_month?: number;
  start_date?: string;
  end_date?: string;
}

export interface Attachment {
  id: string;
  object_name: string;
  content_type: string;
  size_bytes: number;
  original_filename: string;
  uploaded_at: string;
  uploaded_by: string;
}

export interface Expense {
  id: string;
  copro_id: string;
  name: string;
  amount_cents: number;
  currency: string;
  date: string;
  payment_date?: string;
  payer_foyer_id: string;
  category_id: string;
  distribution_mode: DistributionMode;
  share_rdc_cents: number;
  share_1er_cents: number;
  settled: boolean;
  settled_at?: string;
  note?: string;
  attachments?: Attachment[];
  template_id?: string;
  amount_pending?: boolean;
  meter_reading_period?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateExpenseInput {
  name: string;
  amount_cents: number;
  currency?: string;
  date: string;
  payment_date?: string;
  payer_foyer_id: string;
  category_id: string;
  distribution_mode: DistributionMode;
  share_rdc_cents?: number;
  share_1er_cents?: number;
  settled?: boolean;
  settled_at?: string;
  note?: string;
  template_id?: string;
  amount_pending?: boolean;
  meter_reading_period?: string;
}

// ─── Alerts ─────────────────────────────────────────────────────────

export type AlertKind =
  | "pending_completion"
  | "missing_receipt"
  | "peer_expense_added"
  | "balance_seasonal"
  | "monthly_meter_reading"
  | "contract_expiring";

export interface Alert {
  id: string;
  copro_id: string;
  kind: AlertKind;
  recipient_foyer_id: string;
  dedupe_key: string;
  payload?: Record<string, unknown>;
  deep_link?: string;
  fired_at: string;
  read_at?: string;
  resolved_at?: string;
  dismissed_at?: string;
}

/**
 * A standalone uploaded document — insurance contract, syndic statement,
 * AGE minutes, plumber estimate, etc. Per-expense attachments live in the
 * expense subcollection (see ExpenseAttachment in $lib/live); this type
 * is for documents that stand on their own.
 *
 * `group` is a free-text tag (devis / facture / contrat / attestation /
 * etc.) used to fold similar docs together in the archive view. Server
 * normalizes to lowercase + trimmed on write so display variants merge.
 */
export interface Document {
  id: string;
  copro_id: string;
  category_id: string;
  group?: string;
  title: string;
  description?: string;
  object_name: string;
  content_type: string;
  size_bytes: number;
  original_filename: string;
  uploaded_at: string;
  uploaded_by: string;
  linked_expense_id?: string;
  linked_contract_id?: string;
  /**
   * Cached Gemini verdict — produced lazily by POST /documents/{id}/analyze.
   * Absent until the user (or a future eager hook) triggers analysis.
   */
  analysis?: DocumentAnalysis;
}

export type DocumentAnalysisKind = "expense" | "contract" | "other";

export interface DocumentAnalysis {
  kind: DocumentAnalysisKind;
  /** Self-reported model confidence ∈ [0, 1]. */
  confidence: number;
  /**
   * ISO timestamp when the analysis was produced. May be an empty
   * string when the Firestore document is missing the field (legacy
   * rows, partial migrations). Consumers should treat as optional.
   */
  analyzed_at: string;
  model: string;
  /** Free-text justification — mainly useful for kind=other. */
  reason?: string;
  /** Populated when kind=expense. */
  expense?: ExpenseExtraction;
  /** Populated when kind=contract. */
  contract?: ContractExtraction;
}

export interface ExpenseExtraction {
  amount_eur?: number;
  /** ISO YYYY-MM-DD. */
  date?: string;
  vendor?: string;
  /** Free-text suggestion (e.g. "électricité"). */
  category_hint?: string;
  description?: string;
}

export interface ContractExtraction {
  provider?: string;
  contract_type?: string;
  /** ISO YYYY-MM-DD. */
  start_date?: string;
  /** ISO YYYY-MM-DD. */
  end_date?: string;
  monthly_amount_eur?: number;
  contract_number?: string;
}

export interface CreateDocumentInput {
  title: string;
  description?: string;
  category_id: string;
  group?: string;
  linked_contract_id?: string;
}

// ─── Contracts ──────────────────────────────────────────────────────

export type ContractStatus = "active" | "expired" | "cancelled";

export interface Society {
  name: string;
  phone?: string;
  email?: string;
  website?: string;
  address?: string;
}

export interface Contact {
  name?: string;
  role?: string;
  phone?: string;
  email?: string;
}

/**
 * A long-lived service agreement bound to the copro: insurance, syndic,
 * energy, maintenance. Two contracts with the same provider duplicate
 * the inline society fields — acceptable at our scale.
 *
 * Linked Documents back-reference via Document.linked_contract_id; the
 * optional ExpenseTemplate link drives recurring billing.
 */
export interface Contract {
  id: string;
  copro_id: string;
  name: string;
  category_id: string;
  society: Society;
  contact?: Contact;
  start_date?: string;
  end_date?: string;
  amount_cents?: number;
  billing_frequency?: Frequency;
  template_id?: string;
  status: ContractStatus;
  note?: string;
  created_at: string;
  updated_at: string;
}

export interface ContractInput {
  name: string;
  category_id: string;
  society: Society;
  contact?: Contact;
  start_date?: string;
  end_date?: string;
  amount_cents?: number;
  billing_frequency?: Frequency;
  template_id?: string;
  status?: ContractStatus;
  note?: string;
}

/**
 * A balance-reducing transfer between the two foyers, recorded as a
 * distinct ledger row (PRD FR40 — settlements never mutate expenses).
 * `expense_ids` audit-link the expenses considered covered by this
 * transfer; the link is informational only and does NOT toggle
 * Expense.settled. Balance math is straight subtraction of `amount_cents`.
 */
export interface Settlement {
  id: string;
  copro_id: string;
  from_foyer_id: string;
  to_foyer_id: string;
  amount_cents: number;
  currency: string;
  date: string;
  note?: string;
  expense_ids?: string[];
  created_at: string;
  updated_at: string;
}

export interface CreateSettlementInput {
  from_foyer_id: string;
  to_foyer_id: string;
  amount_cents: number;
  currency?: string;
  date: string;
  note?: string;
  expense_ids?: string[];
}

// ─── Meter readings ─────────────────────────────────────────────────

/**
 * One calendar month's worth of meter snapshots — global building meter
 * plus the three detail submeters (common / RDC / 1er) used by the
 * `water_3_meters` distribution mode. The two photos document the
 * reading session: one of the global meter, one of the panel showing
 * all three sub-meters in a single frame.
 */
export interface MeterReading {
  id: string;
  copro_id: string;
  period: string; // "YYYY-MM"
  global_m3: number;
  common_m3: number;
  rdc_m3: number;
  premier_m3: number;
  global_photo_object?: string;
  global_photo_content_type?: string;
  global_photo_size_bytes?: number;
  detail_photo_object?: string;
  detail_photo_content_type?: string;
  detail_photo_size_bytes?: number;
  captured_at: string;
  captured_by_uid: string;
  created_at: string;
  updated_at: string;
}

export interface SaveMeterInput {
  period: string;
  global_m3: number;
  common_m3: number;
  rdc_m3: number;
  premier_m3: number;
}

export type MeterPhotoKind = "global" | "detail";

export async function api<T>(path: string, opts: ApiOptions = {}): Promise<T> {
  const token = await idToken();

  const headers: Record<string, string> = {
    Accept: "application/json",
    ...opts.headers,
  };

  if (opts.body !== undefined && !(opts.body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
  }

  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${PUBLIC_API_BASE_URL}${path}`, {
    method: opts.method ?? "GET",
    headers,
    signal: opts.signal,
    body:
      opts.body === undefined
        ? undefined
        : opts.body instanceof FormData
          ? opts.body
          : JSON.stringify(opts.body),
  });

  if (!res.ok) {
    let code = "UNKNOWN";
    let message = res.statusText;
    try {
      const data = (await res.json()) as {
        errors?: { code: string; message: string }[];
      };
      if (data.errors?.length) {
        code = data.errors[0].code;
        message = data.errors[0].message;
      }
    } catch {
      // non-JSON error
    }
    throw new ApiError(res.status, code, message);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return (await res.json()) as T;
}
