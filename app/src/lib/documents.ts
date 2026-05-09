// Foyer-facing document helpers. Reads come live from Firestore via
// $lib/live; only mutations stay here. The upload flow mirrors the
// per-expense attachment pipeline (signed PUT URL → direct browser→GCS
// upload → POST /documents to record metadata) but at the documents/
// prefix.

import { ApiError, api, type CreateDocumentInput, type Document } from "./api";
import { prepareForUpload } from "./expenses";

interface UploadDocURLResponse {
  document_id: string;
  object_name: string;
  upload_url: string;
  content_type: string;
  expires_at: string;
}

interface DocDownloadURLResponse {
  download_url: string;
  expires_at: string;
}

export interface UploadDocumentOptions {
  signal?: AbortSignal;
  onProgress?: (loaded: number, total: number) => void;
}

/**
 * One-shot standalone-document upload with rollback semantics:
 *  1. prepareForUpload (HEIC→JPEG + ~400 KB compression — same pipeline
 *     as expense attachments)
 *  2. POST /documents/upload-url with the FINAL content-type
 *  3. PUT to GCS using the SAME content-type the server signed in
 *  4. POST /documents to record metadata
 *
 * If step 4 fails after step 3 succeeded, we attempt a best-effort
 * server-side cleanup via DELETE /documents/{id} (idempotent — if the
 * metadata didn't write, the storage delete still drops the orphan).
 */
export async function uploadDocument(
  file: File,
  meta: CreateDocumentInput,
  opts: UploadDocumentOptions = {},
): Promise<Document> {
  const { blob, contentType, originalFilename } = await prepareForUpload(file);

  const issued = await api<UploadDocURLResponse>("/documents/upload-url", {
    method: "POST",
    body: {
      title: meta.title,
      description: meta.description,
      category_id: meta.category_id,
      group: meta.group,
      original_filename: originalFilename,
      content_type: contentType,
      size_bytes: blob.size,
    },
    signal: opts.signal,
  });

  await putToGCS(issued.upload_url, blob, issued.content_type, blob.size, opts);

  try {
    return await api<Document>("/documents", {
      method: "POST",
      body: {
        document_id: issued.document_id,
        title: meta.title,
        description: meta.description,
        category_id: meta.category_id,
        group: meta.group,
        content_type: issued.content_type,
        size_bytes: blob.size,
        original_filename: originalFilename,
      },
      signal: opts.signal,
    });
  } catch (err) {
    void deleteDocument(issued.document_id).catch(() => {
      /* swallow — best-effort orphan cleanup */
    });
    throw err;
  }
}

export function updateDocument(
  id: string,
  input: {
    title: string;
    description?: string;
    category_id: string;
    group?: string;
  },
): Promise<Document> {
  return api<Document>(`/documents/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: input,
  });
}

export function deleteDocument(id: string): Promise<void> {
  return api<void>(`/documents/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function getDocumentDownloadUrl(
  id: string,
): Promise<{ url: string; expiresAt: string }> {
  const res = await api<DocDownloadURLResponse>(
    `/documents/${encodeURIComponent(id)}/download-url`,
    { method: "GET" },
  );
  return { url: res.download_url, expiresAt: res.expires_at };
}

// PUT a Blob to a signed GCS URL with optional progress reporting + abort
// support. Mirrors the helper in $lib/expenses but kept local to avoid a
// cross-module import for what is effectively the same primitive.
async function putToGCS(
  url: string,
  blob: Blob,
  contentType: string,
  sizeBytes: number,
  opts: UploadDocumentOptions,
): Promise<void> {
  // V4 signed both Content-Type and x-goog-content-length-range; the
  // client must echo them exactly or GCS responds 400.
  const signedHeaders = {
    "Content-Type": contentType,
    "x-goog-content-length-range": `0,${sizeBytes}`,
  };

  if (typeof XMLHttpRequest === "undefined" || !opts.onProgress) {
    const res = await fetch(url, {
      method: "PUT",
      headers: signedHeaders,
      body: blob,
      signal: opts.signal,
    });
    if (!res.ok) {
      throw new ApiError(
        res.status,
        "GCS_UPLOAD_FAILED",
        `upload failed (${res.status})`,
      );
    }
    return;
  }
  await new Promise<void>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url);
    for (const [name, value] of Object.entries(signedHeaders)) {
      xhr.setRequestHeader(name, value);
    }
    xhr.upload.addEventListener("progress", (e) => {
      if (e.lengthComputable) opts.onProgress?.(e.loaded, e.total);
    });
    xhr.addEventListener("load", () => {
      if (xhr.status >= 200 && xhr.status < 300) resolve();
      else
        reject(
          new ApiError(
            xhr.status,
            "GCS_UPLOAD_FAILED",
            `upload failed (${xhr.status})`,
          ),
        );
    });
    xhr.addEventListener("error", () =>
      reject(new ApiError(0, "GCS_UPLOAD_FAILED", "network error")),
    );
    xhr.addEventListener("abort", () =>
      reject(new ApiError(0, "UPLOAD_ABORTED", "upload cancelled")),
    );
    opts.signal?.addEventListener("abort", () => xhr.abort());
    xhr.send(blob);
  });
}

export function isImageDocument(d: Document): boolean {
  return d.content_type.startsWith("image/");
}
