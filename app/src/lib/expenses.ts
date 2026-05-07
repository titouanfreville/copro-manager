// Foyer-facing API helpers. Reads come from Firestore via $lib/live; only
// mutations stay here so the API keeps its monopoly on share-computation.

import { api, type CreateExpenseInput, type Expense } from './api';

export function createExpense(input: CreateExpenseInput): Promise<Expense> {
	return api<Expense>('/expenses', { method: 'POST', body: input });
}
