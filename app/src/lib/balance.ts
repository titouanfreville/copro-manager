// Net balance between the two foyers, computed from the live expense
// stream. Settled rows (CSV-imported "Paiement complet" or rows manually
// flipped to Réglée) are excluded — they're already off the books.

import type { Expense, Foyer } from './api';

export interface Balance {
	/** Net cents from RDC's perspective: positive → 1er owes RDC; negative → RDC owes 1er. */
	net: number;
	rdc: Foyer;
	premier: Foyer;
}

/**
 * Pure balance compute. Returns null when either foyer is missing — the
 * caller decides how to render that (placeholder, prompt to seed foyers).
 */
export function computeBalance(expenses: Expense[], foyers: Foyer[]): Balance | null {
	const rdc = foyers.find((f) => f.floor === 'rdc');
	const premier = foyers.find((f) => f.floor === '1er');
	if (!rdc || !premier) return null;
	let net = 0;
	for (const e of expenses) {
		// Settled rows are off the books (already balanced); pending rows
		// have no amount yet (waiting on a bill to arrive). Both excluded.
		if (e.settled || e.amount_pending) continue;
		if (e.payer_foyer_id === rdc.id) net += e.share_1er_cents;
		else if (e.payer_foyer_id === premier.id) net -= e.share_rdc_cents;
	}
	return { net, rdc, premier };
}

export function formatBalanceEUR(cents: number): string {
	return (Math.abs(cents) / 100).toLocaleString('fr-FR', {
		style: 'currency',
		currency: 'EUR',
		minimumFractionDigits: 2,
		maximumFractionDigits: 2
	});
}
