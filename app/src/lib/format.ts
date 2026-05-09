// Display-side date helpers. Persistence stays ISO-8601 everywhere
// (Firestore + Go API + <input type="date"> all keep YYYY-MM-DD); these
// helpers only affect what the user reads. PRD FR67 mandates DD/MM/YYYY
// for the rendered output.

/**
 * Render an ISO 8601 string (or empty / undefined) as `DD/MM/YYYY` in
 * `fr-FR`. Returns an empty string for missing or unparseable input so
 * templates can safely interpolate without conditional guards.
 */
export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleDateString("fr-FR");
}

/**
 * Long form, e.g. "8 mai 2026". Used sparingly — month headers, hero
 * subtitles, dates that need to read warmly.
 */
export function formatDateLong(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString("fr-FR", {
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}

const eurFormatter = new Intl.NumberFormat("fr-FR", {
  style: "currency",
  currency: "EUR",
});

/**
 * Render integer cents as a fr-FR EUR amount (e.g. `1234` → `12,34 €`).
 * Single source of truth — every page that shows money should call this
 * so decimal separators and currency placement stay consistent.
 */
export function formatEUR(cents: number): string {
  if (!Number.isFinite(cents)) return "0,00 €";
  return eurFormatter.format(cents / 100);
}
