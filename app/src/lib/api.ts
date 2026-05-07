import { PUBLIC_API_BASE_URL } from '$env/static/public';
import { idToken } from './auth';

export class ApiError extends Error {
	constructor(
		readonly status: number,
		readonly code: string,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

interface ApiOptions {
	method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
	body?: unknown;
	headers?: Record<string, string>;
}

export interface Foyer {
	id: string;
	copro_id: string;
	floor: 'rdc' | '1er';
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
}

export type DistributionMode = 'equal' | 'tantiemes' | 'custom';

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
}

export async function api<T>(path: string, opts: ApiOptions = {}): Promise<T> {
	const token = await idToken();

	const headers: Record<string, string> = {
		Accept: 'application/json',
		...opts.headers
	};

	if (opts.body !== undefined && !(opts.body instanceof FormData)) {
		headers['Content-Type'] = 'application/json';
	}

	if (token) {
		headers['Authorization'] = `Bearer ${token}`;
	}

	const res = await fetch(`${PUBLIC_API_BASE_URL}${path}`, {
		method: opts.method ?? 'GET',
		headers,
		body:
			opts.body === undefined
				? undefined
				: opts.body instanceof FormData
					? opts.body
					: JSON.stringify(opts.body)
	});

	if (!res.ok) {
		let code = 'UNKNOWN';
		let message = res.statusText;
		try {
			const data = (await res.json()) as { errors?: { code: string; message: string }[] };
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
