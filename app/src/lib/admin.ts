import { PUBLIC_API_BASE_URL } from "$env/static/public";
import { ApiError } from "./api";

// Re-exported for the import page; admin uses fetch directly for the
// multipart upload (the JSON helper above isn't a fit).

const ADMIN_KEY_STORAGE = "admin_api_key";

export function getAdminKey(): string {
  if (typeof sessionStorage === "undefined") return "";
  return sessionStorage.getItem(ADMIN_KEY_STORAGE) ?? "";
}

export function setAdminKey(key: string): void {
  if (typeof sessionStorage === "undefined") return;
  if (key) sessionStorage.setItem(ADMIN_KEY_STORAGE, key);
  else sessionStorage.removeItem(ADMIN_KEY_STORAGE);
}

interface AdminOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
}

export async function adminApi<T>(
  path: string,
  opts: AdminOptions = {},
): Promise<T> {
  const key = getAdminKey();
  if (!key) throw new ApiError(401, "NO_ADMIN_KEY", "Admin key non renseignée");

  const headers: Record<string, string> = {
    Accept: "application/json",
    Authorization: `AdminKey ${key}`,
  };
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";

  const res = await fetch(`${PUBLIC_API_BASE_URL}${path}`, {
    method: opts.method ?? "GET",
    headers,
    body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
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

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export interface User {
  id: string;
  email: string;
  display_name: string;
}

export interface Foyer {
  id: string;
  copro_id: string;
  floor: "rdc" | "1er";
  name: string;
  parts: number;
  member_ids: string[];
}

export interface ListedFoyer extends Foyer {
  members: User[];
}

export interface MemberInput {
  user_id?: string;
  email?: string;
  display_name?: string;
}

export interface CreateFoyerInput {
  floor: "rdc" | "1er";
  name: string;
  parts: number;
  member: MemberInput;
}

export interface CreateFoyerResponse {
  foyer: Foyer;
  /**
   * One-shot Firebase password-reset URL for the freshly provisioned member.
   * Set only when this call minted a brand-new Firebase user. Forward via
   * any channel; the user clicks it to set their own password. The API
   * intentionally never returns the auto-generated initial password.
   */
  reset_link?: string;
}

export interface AddMemberResponse {
  foyer: Foyer;
  /** See CreateFoyerResponse.reset_link. */
  reset_link?: string;
}

export interface ListFoyersResponse {
  foyers: ListedFoyer[];
}

export interface ResetPasswordResponse {
  reset_link: string;
}

export function createFoyer(
  input: CreateFoyerInput,
): Promise<CreateFoyerResponse> {
  return adminApi<CreateFoyerResponse>("/admin/foyers", {
    method: "POST",
    body: input,
  });
}

export function listFoyers(): Promise<ListFoyersResponse> {
  return adminApi<ListFoyersResponse>("/admin/foyers");
}

export function addFoyerMember(
  foyerId: string,
  member: MemberInput,
): Promise<AddMemberResponse> {
  return adminApi<AddMemberResponse>(
    `/admin/foyers/${encodeURIComponent(foyerId)}/members`,
    {
      method: "POST",
      body: member,
    },
  );
}

export function updateFoyerParts(
  foyerId: string,
  parts: number,
): Promise<void> {
  return adminApi<void>(`/admin/foyers/${encodeURIComponent(foyerId)}`, {
    method: "PATCH",
    body: { parts },
  });
}

export function resetPassword(userId: string): Promise<ResetPasswordResponse> {
  return adminApi<ResetPasswordResponse>(
    `/admin/users/${encodeURIComponent(userId)}/reset-password`,
    { method: "POST" },
  );
}

export interface ImportSummary {
  processed: number;
  created: number;
  updated: number;
  skipped: number;
  skip_reasons: Record<string, number>;
  errors: { line: number; item?: string; message: string }[];
}

export async function importExpensesCSV(
  file: File,
  payerFoyerId: string,
): Promise<ImportSummary> {
  const key = getAdminKey();
  if (!key) throw new ApiError(401, "NO_ADMIN_KEY", "Admin key non renseignée");

  const form = new FormData();
  form.append("file", file);
  form.append("payer_foyer_id", payerFoyerId);

  const res = await fetch(`${PUBLIC_API_BASE_URL}/admin/expenses/import`, {
    method: "POST",
    headers: { Authorization: `AdminKey ${key}` },
    body: form,
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
      // non-JSON
    }
    throw new ApiError(res.status, code, message);
  }
  const data = (await res.json()) as Partial<ImportSummary>;
  // Defensive — historically Go marshalled nil slices/maps as `null`,
  // which broke the page render before we were doing this on the API
  // side. Keep the fallback so a stale API binary can't poison the UI.
  return {
    processed: data.processed ?? 0,
    created: data.created ?? 0,
    updated: data.updated ?? 0,
    skipped: data.skipped ?? 0,
    skip_reasons: data.skip_reasons ?? {},
    errors: data.errors ?? [],
  };
}
