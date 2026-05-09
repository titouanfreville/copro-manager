// Foyer-facing API helpers. Reads come from Firestore via $lib/live; only
// mutations stay here so the API keeps its monopoly on share-computation.

import {
  ApiError,
  api,
  type Attachment,
  type CreateExpenseInput,
  type Expense,
} from "./api";

export function createExpense(input: CreateExpenseInput): Promise<Expense> {
  return api<Expense>("/expenses", { method: "POST", body: input });
}

export function updateExpense(
  id: string,
  input: CreateExpenseInput,
): Promise<Expense> {
  return api<Expense>(`/expenses/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: input,
  });
}

export function deleteExpense(id: string): Promise<void> {
  return api<void>(`/expenses/${encodeURIComponent(id)}`, { method: "DELETE" });
}

// ─── Attachments ────────────────────────────────────────────────────

interface UploadURLResponse {
  attachment_id: string;
  object_name: string;
  upload_url: string;
  content_type: string;
  expires_at: string;
}

interface DownloadURLResponse {
  download_url: string;
  expires_at: string;
}

/**
 * Whitelist of MIME types the backend accepts. Used by the file-input
 * `accept` attribute and for client-side guards before the round-trip.
 */
export const ATTACHMENT_ACCEPT =
  "image/jpeg,image/png,image/heic,image/heif,application/pdf";
export const ATTACHMENT_MAX_BYTES = 10 * 1024 * 1024;
export const ATTACHMENT_MAX_PER_EXPENSE = 10;

// Target size budget for re-encoded images. PRD FR33 says "approximately
// 400 KB"; we treat 400 KB as the upper-bound and progressively lower the
// JPEG quality if the first pass overshoots.
const ATTACHMENT_IMAGE_TARGET_BYTES = 400 * 1024;
const ATTACHMENT_IMAGE_MAX_DIMENSION = 2000;
const ATTACHMENT_IMAGE_QUALITY_STEPS = [0.85, 0.7, 0.6, 0.5];

const HEIC_MIMES = new Set(["image/heic", "image/heif"]);
const HEIC_EXTS = [".heic", ".heif"];

function looksLikeHeic(file: File): boolean {
  if (HEIC_MIMES.has(file.type)) return true;
  const lowered = file.name.toLowerCase();
  return HEIC_EXTS.some((ext) => lowered.endsWith(ext));
}

function isImageFile(file: File): boolean {
  if (file.type.startsWith("image/")) return true;
  // HEIC/HEIF often arrive with empty file.type on iOS — fall back to ext.
  return looksLikeHeic(file);
}

/**
 * Re-encode an image to JPEG, scaling down if either dimension exceeds the
 * cap, and stepping the quality down until the result fits the target byte
 * budget. Used to normalize HEIC → JPEG (FR35) AND to satisfy the ~400 KB
 * compression target (FR33). PDFs and other non-image types short-circuit.
 *
 * Returns the original blob unchanged when:
 *  - the file is not an image (PDF, etc.)
 *  - canvas re-encode is unavailable (no DOM, ancient browser)
 *  - the result would be larger than the input
 */
async function compressImage(file: File): Promise<Blob> {
  if (!isImageFile(file)) return file;
  if (
    typeof document === "undefined" ||
    typeof createImageBitmap === "undefined"
  ) {
    return file;
  }

  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch (err) {
    // HEIC on Chrome desktop, corrupt files, etc. — fall back to upload
    // the raw file. The receiver can deal.
    console.warn("createImageBitmap failed; uploading original", err);
    return file;
  }

  try {
    const scale = Math.min(
      1,
      ATTACHMENT_IMAGE_MAX_DIMENSION / Math.max(bitmap.width, bitmap.height),
    );
    const w = Math.round(bitmap.width * scale);
    const h = Math.round(bitmap.height * scale);
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d");
    if (!ctx) return file;
    ctx.drawImage(bitmap, 0, 0, w, h);

    // Step the quality down progressively until the blob fits.
    for (const q of ATTACHMENT_IMAGE_QUALITY_STEPS) {
      const blob: Blob | null = await new Promise((resolve) => {
        canvas.toBlob((b) => resolve(b), "image/jpeg", q);
      });
      if (!blob) continue;
      // Skip the result if we somehow produced a larger blob than the
      // input — re-encoding a tiny PNG/WebP at q=0.85 can bloat past the
      // source. On the last quality step we fall through to `return file`
      // below so the caller sees the original.
      if (blob.size >= file.size) continue;
      if (
        blob.size <= ATTACHMENT_IMAGE_TARGET_BYTES ||
        q === ATTACHMENT_IMAGE_QUALITY_STEPS.at(-1)
      ) {
        return blob;
      }
    }
    return file;
  } finally {
    bitmap.close?.();
  }
}

export interface PreparedAttachment {
  blob: Blob;
  contentType: string;
  originalFilename: string;
}

/**
 * Normalize a user-picked file into something the API accepts:
 *  - HEIC/HEIF → JPEG (FR35)
 *  - large images → compressed JPEG ≤ ~400 KB (FR33)
 *  - PDFs pass through unchanged
 *
 * Throws an ApiError when the file would exceed the 10 MB cap even after
 * compression, or when the type isn't on the whitelist.
 */
export async function prepareForUpload(
  file: File,
): Promise<PreparedAttachment> {
  const isImage = isImageFile(file);
  let blob: Blob = file;
  let contentType = file.type;
  let filename = file.name;

  if (isImage) {
    const compressed = await compressImage(file);
    if (compressed === file) {
      // compressImage returned the original — either bitmap conversion
      // failed (HEIC on Chrome desktop) or every quality step produced a
      // larger blob than the input. The server only accepts JPEG/PNG, so
      // we can't safely re-label HEIC bytes as image/jpeg.
      const isJpegOrPng =
        file.type === "image/jpeg" || file.type === "image/png";
      if (!isJpegOrPng) {
        throw new ApiError(
          400,
          "UNSUPPORTED_TYPE",
          "Format non pris en charge sur ce navigateur. Convertis l'image en JPEG ou PNG avant l'envoi.",
        );
      }
      blob = file;
      contentType = file.type;
    } else {
      blob = compressed;
      contentType = "image/jpeg";
      // Force `.jpg` so the server-built object name matches the canonical
      // extension for image/jpeg.
      filename = swapExtension(filename, ".jpg");
    }
  } else if (file.type === "application/pdf") {
    contentType = "application/pdf";
  } else {
    throw new ApiError(
      400,
      "UNSUPPORTED_TYPE",
      `Type non supporté: ${file.type || "inconnu"}`,
    );
  }

  if (blob.size > ATTACHMENT_MAX_BYTES) {
    throw new ApiError(
      400,
      "FILE_TOO_LARGE",
      `Fichier trop lourd (${(blob.size / 1024 / 1024).toFixed(1)} MB > 10 MB)`,
    );
  }
  return { blob, contentType, originalFilename: filename };
}

function swapExtension(name: string, newExt: string): string {
  const dot = name.lastIndexOf(".");
  if (dot <= 0) return name + newExt;
  return name.slice(0, dot) + newExt;
}

export interface AttachOptions {
  /** Optional AbortSignal so callers can cancel the in-flight upload. */
  signal?: AbortSignal;
  /** Fired with bytes uploaded vs total. Wire to a progress UI. */
  onProgress?: (loaded: number, total: number) => void;
}

/**
 * One-shot upload with rollback semantics:
 *  1. prepareForUpload (HEIC→JPEG + compress)
 *  2. POST /upload-url with the FINAL content-type
 *  3. PUT to GCS using the SAME content-type the server signed in
 *  4. POST /attachments to record metadata
 *
 * If step 4 fails after step 3 succeeded, we attempt a best-effort
 * server-side cleanup via DELETE /attachments/{id} (idempotent — the
 * metadata might not exist yet, but the storage delete still drops the
 * orphan blob). Throws on any leg.
 */
export async function attachFile(
  expenseId: string,
  file: File,
  opts: AttachOptions = {},
): Promise<Attachment> {
  const { blob, contentType, originalFilename } = await prepareForUpload(file);

  const issued = await api<UploadURLResponse>(
    `/expenses/${encodeURIComponent(expenseId)}/attachments/upload-url`,
    {
      method: "POST",
      body: {
        original_filename: originalFilename,
        content_type: contentType,
        size_bytes: blob.size,
      },
      signal: opts.signal,
    },
  );

  // Direct browser→GCS PUT. The signed URL pins Content-Type AND the
  // content-length range — we MUST send the same content-type the server
  // returned (which may differ from our local guess if the server
  // normalized e.g. parameters), AND the same x-goog-content-length-range
  // header the server signed in (V4 rejects on header mismatch).
  await putToGCSWithProgress(
    issued.upload_url,
    blob,
    issued.content_type,
    blob.size,
    opts,
  );

  try {
    return await api<Attachment>(
      `/expenses/${encodeURIComponent(expenseId)}/attachments`,
      {
        method: "POST",
        body: {
          attachment_id: issued.attachment_id,
          content_type: issued.content_type,
          size_bytes: blob.size,
          original_filename: originalFilename,
        },
        signal: opts.signal,
      },
    );
  } catch (err) {
    // PUT succeeded but recording the metadata failed (network blip,
    // server 5xx, validation regression). Best-effort orphan cleanup so
    // the bucket doesn't grow indefinitely.
    void deleteAttachment(expenseId, issued.attachment_id).catch(() => {
      /* swallow — this is a best-effort cleanup */
    });
    throw err;
  }
}

/**
 * PUT a Blob to a signed GCS URL with progress reporting and abort
 * support. We use XMLHttpRequest because `fetch` doesn't expose an upload
 * progress event in any browser today.
 */
function putToGCSWithProgress(
  url: string,
  blob: Blob,
  contentType: string,
  sizeBytes: number,
  opts: AttachOptions,
): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url, true);
    xhr.setRequestHeader("Content-Type", contentType);
    // V4 signs the exact value we hand back here; sending a different
    // (or missing) value flips GCS to a 400 SignatureDoesNotMatch.
    xhr.setRequestHeader("x-goog-content-length-range", `0,${sizeBytes}`);
    if (opts.onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) opts.onProgress?.(e.loaded, e.total);
      };
    }
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

export async function getAttachmentDownloadUrl(
  expenseId: string,
  attachmentId: string,
): Promise<{ url: string; expiresAt: string }> {
  const res = await api<DownloadURLResponse>(
    `/expenses/${encodeURIComponent(expenseId)}/attachments/${encodeURIComponent(attachmentId)}/download-url`,
    { method: "GET" },
  );
  return { url: res.download_url, expiresAt: res.expires_at };
}

export function deleteAttachment(
  expenseId: string,
  attachmentId: string,
): Promise<void> {
  return api<void>(
    `/expenses/${encodeURIComponent(expenseId)}/attachments/${encodeURIComponent(attachmentId)}`,
    { method: "DELETE" },
  );
}

export function isImageAttachment(att: Attachment): boolean {
  return att.content_type.startsWith("image/");
}

interface MaterializeSummary {
  templates_processed: number;
  expenses_created: number;
  errors?: { template_id: string; message: string }[];
}

/**
 * Fire the lazy materialization endpoint. Idempotent — the endpoint walks
 * due templates and creates expenses, advancing next_occurrence_at. Called
 * on /expenses mount as a backstop to the daily Cloud Scheduler cron.
 *
 * Debounced via sessionStorage so a tab navigating /expenses ↔ /templates
 * doesn't hammer the API on every mount: at most once per minute per tab.
 */
const MATERIALIZE_DEBOUNCE_KEY = "copro:materialize-recent";
const MATERIALIZE_DEBOUNCE_MS = 60 * 1000;

export async function materializeRecurring(): Promise<MaterializeSummary | null> {
  if (typeof sessionStorage !== "undefined") {
    const last = Number(sessionStorage.getItem(MATERIALIZE_DEBOUNCE_KEY) ?? 0);
    if (Date.now() - last < MATERIALIZE_DEBOUNCE_MS) {
      return null;
    }
    sessionStorage.setItem(MATERIALIZE_DEBOUNCE_KEY, String(Date.now()));
  }
  return api<MaterializeSummary>("/expenses/materialize-recurring", {
    method: "POST",
  });
}
