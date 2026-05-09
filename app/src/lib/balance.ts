// Net balance between the two foyers, computed from the live expense +
// settlement streams. Settled rows (CSV-imported "Paiement complet" or
// rows manually flipped to Réglée) are excluded — they're already off the
// books. Settlements reduce the net per their direction.

import type { Expense, Foyer, Settlement } from "./api";

export interface Balance {
  /** Net cents from RDC's perspective: positive → 1er owes RDC; negative → RDC owes 1er. */
  net: number;
  rdc: Foyer;
  premier: Foyer;
}

/**
 * Pure balance compute. Returns null when either foyer is missing — the
 * caller decides how to render that (placeholder, prompt to seed foyers).
 *
 * Math (RDC's perspective):
 *  - Non-settled expense paid by RDC → 1er owes RDC their share → +share_1er
 *  - Non-settled expense paid by 1er → RDC owes 1er their share → -share_rdc
 *  - Settlement from=1er,to=RDC → 1er paid RDC → debt to RDC decreases → -amount
 *  - Settlement from=RDC,to=1er → RDC paid 1er → 1er's claim decreases → +amount
 */
export function computeBalance(
  expenses: Expense[],
  settlements: Settlement[],
  foyers: Foyer[],
): Balance | null {
  const rdc = foyers.find((f) => f.floor === "rdc");
  const premier = foyers.find((f) => f.floor === "1er");
  if (!rdc || !premier) return null;
  let net = 0;
  for (const e of expenses) {
    // Settled rows are off the books (already balanced); pending rows
    // have no amount yet (waiting on a bill to arrive). Both excluded.
    if (e.settled || e.amount_pending) continue;
    if (e.payer_foyer_id === rdc.id) net += e.share_1er_cents;
    else if (e.payer_foyer_id === premier.id) net -= e.share_rdc_cents;
  }
  for (const s of settlements) {
    if (s.from_foyer_id === premier.id && s.to_foyer_id === rdc.id) {
      net -= s.amount_cents;
    } else if (s.from_foyer_id === rdc.id && s.to_foyer_id === premier.id) {
      net += s.amount_cents;
    }
  }
  return { net, rdc, premier };
}

export function formatBalanceEUR(cents: number): string {
  return (Math.abs(cents) / 100).toLocaleString("fr-FR", {
    style: "currency",
    currency: "EUR",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}
