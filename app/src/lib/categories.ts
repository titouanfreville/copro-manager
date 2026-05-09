// Foyer-facing category mutations. Reads come live from Firestore via
// $lib/live; only Create/Update/Delete go through the API.

import { api, type Category, type DistributionMode } from "./api";

export interface CreateCategoryInput {
  name: string;
  default_distribution_mode?: DistributionMode;
}

export interface UpdateCategoryInput {
  name?: string; // ignored server-side for predefined categories
  default_distribution_mode?: DistributionMode;
}

export function createCategory(input: CreateCategoryInput): Promise<Category> {
  return api<Category>("/categories", { method: "POST", body: input });
}

export function updateCategory(
  id: string,
  input: UpdateCategoryInput,
): Promise<Category> {
  return api<Category>(`/categories/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: input,
  });
}

export function deleteCategory(id: string): Promise<void> {
  return api<void>(`/categories/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}
