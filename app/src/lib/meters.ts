// Foyer-facing API helpers for water-meter readings. Reads come from
// Firestore via $lib/live (subscribeMeters); only mutations live here.
//
// Photo uploads reuse the same signed-URL dance as expense attachments:
// 1. POST /upload-url declares the file's type + size, returns a signed URL.
// 2. Browser PUTs the blob directly to GCS.
// 3. POST /photos/{kind} confirms; server HEADs the object then patches the
//    meter reading doc.

import {
  ApiError,
  api,
  type MeterPhotoKind,
  type MeterReading,
  type SaveMeterInput,
} from "./api";
import { prepareForUpload } from "./expenses";

export function listMeters(): Promise<MeterReading[]> {
  return api<MeterReading[]>("/meters");
}

export function getMeter(period: string): Promise<MeterReading> {
  return api<MeterReading>(`/meters/${encodeURIComponent(period)}`);
}

export function createMeter(input: SaveMeterInput): Promise<MeterReading> {
  return api<MeterReading>("/meters", { method: "POST", body: input });
}

export function updateMeter(
  period: string,
  input: SaveMeterInput,
): Promise<MeterReading> {
  return api<MeterReading>(`/meters/${encodeURIComponent(period)}`, {
    method: "PATCH",
    body: input,
  });
}

export function deleteMeter(period: string): Promise<void> {
  return api<void>(`/meters/${encodeURIComponent(period)}`, {
    method: "DELETE",
  });
}

interface PhotoUploadURLResponse {
  object_name: string;
  upload_url: string;
  content_type: string;
  expires_at: string;
}

interface PhotoDownloadURLResponse {
  download_url: string;
  expires_at: string;
}

/**
 * Upload one of the two photos that document a reading session. Reuses
 * `prepareForUpload` so HEIC normalization and the ~400 KB compression
 * target match the rest of the app — meter photos go through the same
 * pipeline as expense attachments.
 */
export async function attachMeterPhoto(
  period: string,
  kind: MeterPhotoKind,
  file: File,
  opts: { signal?: AbortSignal } = {},
): Promise<MeterReading> {
  const { blob, contentType } = await prepareForUpload(file);

  const issued = await api<PhotoUploadURLResponse>(
    `/meters/${encodeURIComponent(period)}/photos/${encodeURIComponent(kind)}/upload-url`,
    {
      method: "POST",
      body: { content_type: contentType, size_bytes: blob.size },
      signal: opts.signal,
    },
  );

  await putToGCS(issued.upload_url, blob, issued.content_type, blob.size, opts);

  return await api<MeterReading>(
    `/meters/${encodeURIComponent(period)}/photos/${encodeURIComponent(kind)}`,
    {
      method: "POST",
      body: { content_type: issued.content_type, size_bytes: blob.size },
      signal: opts.signal,
    },
  );
}

export async function getMeterPhotoDownloadUrl(
  period: string,
  kind: MeterPhotoKind,
): Promise<{ url: string; expiresAt: string }> {
  const res = await api<PhotoDownloadURLResponse>(
    `/meters/${encodeURIComponent(period)}/photos/${encodeURIComponent(kind)}/download-url`,
    { method: "GET" },
  );
  return { url: res.download_url, expiresAt: res.expires_at };
}

export function deleteMeterPhoto(
  period: string,
  kind: MeterPhotoKind,
): Promise<MeterReading> {
  return api<MeterReading>(
    `/meters/${encodeURIComponent(period)}/photos/${encodeURIComponent(kind)}`,
    { method: "DELETE" },
  );
}

/**
 * Server-side OCR pass against an already-recorded photo. Returns up
 * to 1 (global) or 3 (detail) detected values plus per-value
 * confidence so the UI can surface low-confidence reads. Empty
 * arrays = OCR unavailable or no number detected; the user types
 * manually.
 */
export interface MeterOCRResult {
  values: number[];
  confidence: number[];
}

export function suggestMeterPhotoValues(
  period: string,
  kind: MeterPhotoKind,
): Promise<MeterOCRResult> {
  return api<MeterOCRResult>(
    `/meters/${encodeURIComponent(period)}/photos/${encodeURIComponent(kind)}/ocr`,
    { method: "POST" },
  );
}

/**
 * Stateless OCR for the capture flow: send the picked file directly,
 * before any meter doc or GCS object exists. Reuses `prepareForUpload`
 * so HEIC photos go through the same JPEG normalization pipeline as
 * the persisted version — the model sees the same bytes either way.
 */
export async function suggestRawMeterPhotoValues(
  kind: MeterPhotoKind,
  file: File,
): Promise<MeterOCRResult> {
  const { blob, originalFilename } = await prepareForUpload(file);
  const form = new FormData();
  form.append("file", blob, originalFilename);
  return api<MeterOCRResult>(`/meters/ocr/${encodeURIComponent(kind)}`, {
    method: "POST",
    body: form,
  });
}

function putToGCS(
  url: string,
  blob: Blob,
  contentType: string,
  sizeBytes: number,
  opts: { signal?: AbortSignal },
): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url, true);
    xhr.setRequestHeader("Content-Type", contentType);
    xhr.setRequestHeader("x-goog-content-length-range", `0,${sizeBytes}`);
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) resolve();
      else
        reject(
          new ApiError(
            xhr.status,
            "GCS_UPLOAD_FAILED",
            `upload failed (${xhr.status})`,
          ),
        );
    };
    xhr.onerror = () =>
      reject(
        new ApiError(0, "GCS_UPLOAD_FAILED", "network error during upload"),
      );
    xhr.onabort = () =>
      reject(new ApiError(0, "UPLOAD_ABORTED", "upload was cancelled"));
    if (opts.signal) {
      if (opts.signal.aborted) {
        xhr.abort();
        return;
      }
      opts.signal.addEventListener("abort", () => xhr.abort(), { once: true });
    }
    xhr.send(blob);
  });
}

// ─── Computation helpers (mirror the API formula for live previews) ───

export interface MeterDeltas {
  dCommon: number;
  dRDC: number;
  d1er: number;
  dGlobal: number;
  totalDetail: number; // dCommon + dRDC + d1er
}

/**
 * Returns the delta cubic meters between two readings — current minus
 * prior. `null` when no prior reading exists; the consumer renders "—"
 * for the first-ever period.
 */
export function computeDeltas(
  curr: MeterReading,
  prev: MeterReading | null,
): MeterDeltas | null {
  if (!prev) return null;
  return {
    dCommon: roundM3(curr.common_m3 - prev.common_m3),
    dRDC: roundM3(curr.rdc_m3 - prev.rdc_m3),
    d1er: roundM3(curr.premier_m3 - prev.premier_m3),
    dGlobal: roundM3(curr.global_m3 - prev.global_m3),
    totalDetail: roundM3(
      curr.common_m3 -
        prev.common_m3 +
        (curr.rdc_m3 - prev.rdc_m3) +
        (curr.premier_m3 - prev.premier_m3),
    ),
  };
}

function roundM3(v: number): number {
  // Display in m³ at 3 decimals (1 L resolution). Float math picks up
  // FP noise; round to the displayable resolution.
  return Math.round(v * 1000) / 1000;
}

/**
 * SANITY_CHECK_THRESHOLD: an advisory banner fires when |Δglobal − Σdetails|
 * exceeds 5% of |Δglobal|. The detail sub-meters were installed after the
 * global meter, so absolute values drift by design — the deltas are the
 * useful signal. Threshold is intentionally lenient (real plumbing leaks).
 */
export const SANITY_CHECK_THRESHOLD = 0.05;

export function driftPct(deltas: MeterDeltas | null): number | null {
  if (!deltas) return null;
  if (deltas.dGlobal === 0) return null;
  return (
    Math.abs(deltas.dGlobal - deltas.totalDetail) / Math.abs(deltas.dGlobal)
  );
}

/**
 * Computes the per-foyer share split for a `water_3_meters` expense
 * given its period's deltas. Mirrors the server formula so the form's
 * live breakdown panel can render the split before the user submits.
 *
 * `null` when the formula isn't computable (no prior period, or zero
 * total consumption — fixed fees only); the consumer should fall back
 * to manual entry per the PRD.
 */
export function computeWaterShare(
  amountCents: number,
  deltas: MeterDeltas | null,
): { shareRDCCents: number; share1erCents: number } | null {
  if (!deltas) return null;
  const total = deltas.totalDetail;
  if (total <= 0) return null;
  const rdcShare = (deltas.dRDC + deltas.dCommon / 2) / total;
  const shareRDC = Math.max(
    0,
    Math.min(amountCents, Math.round(rdcShare * amountCents)),
  );
  return {
    shareRDCCents: shareRDC,
    share1erCents: amountCents - shareRDC,
  };
}
