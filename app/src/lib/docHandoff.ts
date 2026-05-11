// One-shot sessionStorage handoff from /documents → /expenses or /contracts.
//
// When the user clicks "Créer une dépense" / "Créer un contrat" on a
// document with a Gemini analysis, we stash the extraction here and
// navigate to the target page. That page peeks the payload, validates
// the kind, applies the pre-fill, and only then calls clearDocHandoff()
// — that way a stray navigation to the wrong page (or a refresh) does
// NOT silently drop the user's intent.
//
// sessionStorage (not localStorage) — the handoff is per-tab and should
// not survive a tab close, since the user might pick a different action.

import type { ContractExtraction, ExpenseExtraction } from "./api";

const STORAGE_KEY = "doc-handoff:v1";

export type DocHandoff =
  | {
      kind: "expense";
      doc_id: string;
      doc_title: string;
      extraction: ExpenseExtraction;
    }
  | {
      kind: "contract";
      doc_id: string;
      doc_title: string;
      extraction: ContractExtraction;
    };

/** Stash a handoff. Safe in non-browser contexts (no-op during SSR). */
export function setDocHandoff(payload: DocHandoff): void {
  if (typeof sessionStorage === "undefined") return;
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
  } catch {
    /* quota / disabled storage — best-effort */
  }
}

/**
 * Peek at the pending handoff without removing it. Returns undefined
 * when none is pending OR when the stored payload fails shape
 * validation (corrupted entry, stale schema version, devtools
 * tampering). Failed validation also clears the slot to prevent the
 * same bad payload from being returned forever.
 *
 * Callers MUST call `clearDocHandoff()` once they've successfully
 * applied the pre-fill, otherwise navigating away and back would
 * re-trigger.
 */
export function peekDocHandoff(): DocHandoff | undefined {
  if (typeof sessionStorage === "undefined") return undefined;
  const raw = sessionStorage.getItem(STORAGE_KEY);
  if (!raw) return undefined;
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!isValidHandoff(parsed)) {
      sessionStorage.removeItem(STORAGE_KEY);
      return undefined;
    }
    return parsed;
  } catch {
    sessionStorage.removeItem(STORAGE_KEY);
    return undefined;
  }
}

/** Remove the pending handoff. Safe to call when none is pending. */
export function clearDocHandoff(): void {
  if (typeof sessionStorage === "undefined") return;
  try {
    sessionStorage.removeItem(STORAGE_KEY);
  } catch {
    /* best-effort */
  }
}

/**
 * Type-narrow + shape-check a parsed JSON blob into a DocHandoff. The
 * top-level shape AND every nested extraction field is validated, so
 * a tampered sessionStorage entry can't flow garbage (wrong types,
 * unexpected nulls) into the form bindings via the apply path.
 */
function isValidHandoff(v: unknown): v is DocHandoff {
  if (!v || typeof v !== "object") return false;
  const o = v as Record<string, unknown>;
  if (o.kind !== "expense" && o.kind !== "contract") return false;
  if (typeof o.doc_id !== "string" || o.doc_id.length === 0) return false;
  if (typeof o.doc_title !== "string") return false;
  if (!o.extraction || typeof o.extraction !== "object") return false;

  const ext = o.extraction as Record<string, unknown>;
  if (o.kind === "expense") {
    return isValidExpenseExtraction(ext);
  }
  return isValidContractExtraction(ext);
}

function isOptionalString(v: unknown): boolean {
  return v === undefined || typeof v === "string";
}

function isOptionalFiniteNumber(v: unknown): boolean {
  return v === undefined || (typeof v === "number" && Number.isFinite(v));
}

function isValidExpenseExtraction(ext: Record<string, unknown>): boolean {
  return (
    isOptionalFiniteNumber(ext.amount_eur) &&
    isOptionalString(ext.date) &&
    isOptionalString(ext.vendor) &&
    isOptionalString(ext.category_hint) &&
    isOptionalString(ext.description)
  );
}

function isValidContractExtraction(ext: Record<string, unknown>): boolean {
  return (
    isOptionalString(ext.provider) &&
    isOptionalString(ext.contract_type) &&
    isOptionalString(ext.start_date) &&
    isOptionalString(ext.end_date) &&
    isOptionalFiniteNumber(ext.monthly_amount_eur) &&
    isOptionalString(ext.contract_number)
  );
}
