// Foyer-facing contract mutations. Reads come live from Firestore via
// $lib/live.subscribeContracts; only Create/Update/Delete go through
// the API.

import { api, type Contract, type ContractInput } from "./api";

export function createContract(input: ContractInput): Promise<Contract> {
  return api<Contract>("/contracts", { method: "POST", body: input });
}

export function updateContract(
  id: string,
  input: ContractInput,
): Promise<Contract> {
  return api<Contract>(`/contracts/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: input,
  });
}

export function deleteContract(id: string): Promise<void> {
  return api<void>(`/contracts/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}
