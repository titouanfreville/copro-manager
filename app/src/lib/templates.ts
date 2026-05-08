// Foyer-facing template helpers. Mutations only — listings come live from
// Firestore via subscribeTemplates in $lib/live.

import { api, type CreateTemplateInput, type ExpenseTemplate } from './api';

export function createTemplate(input: CreateTemplateInput): Promise<ExpenseTemplate> {
	return api<ExpenseTemplate>('/templates', { method: 'POST', body: input });
}

export function updateTemplate(id: string, input: CreateTemplateInput): Promise<ExpenseTemplate> {
	return api<ExpenseTemplate>(`/templates/${encodeURIComponent(id)}`, {
		method: 'PATCH',
		body: input
	});
}

export function deleteTemplate(id: string): Promise<void> {
	return api<void>(`/templates/${encodeURIComponent(id)}`, { method: 'DELETE' });
}
