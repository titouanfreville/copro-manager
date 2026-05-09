// Foyer-facing settlement helpers. Reads come live from Firestore via
// $lib/live; only mutations stay here so the API keeps its monopoly on
// link-collision validation and balance-affecting writes.

import { api, type CreateSettlementInput, type Settlement } from "./api";

export function createSettlement(
  input: CreateSettlementInput,
): Promise<Settlement> {
  return api<Settlement>("/settlements", { method: "POST", body: input });
}

export function updateSettlement(
  id: string,
  input: CreateSettlementInput,
): Promise<Settlement> {
  return api<Settlement>(`/settlements/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: input,
  });
}

export function deleteSettlement(id: string): Promise<void> {
  return api<void>(`/settlements/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}
