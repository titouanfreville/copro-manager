package entities

import "time"

// AttachmentMaxSizeBytes caps a single uploaded file at 10MB. Enforced both
// at signed-URL issuance (via x-goog-content-length-range) and at metadata
// record time (HEAD on the uploaded object).
const AttachmentMaxSizeBytes int64 = 10 * 1024 * 1024

// AttachmentMaxPerExpense limits how many files can hang off a single
// expense. Small N keeps the inline Firestore array cheap to round-trip
// through onSnapshot.
const AttachmentMaxPerExpense = 10

// AllowedAttachmentMimeTypes is the canonical MIME whitelist. Anything else
// is rejected before a signed URL is issued. Map values are the file
// extension we use when minting the GCS object key. HEIC/HEIF are
// intentionally excluded: the SvelteKit client converts iPhone HEIC
// uploads to JPEG via prepareForUpload before issuing the signed URL,
// and the chrome-desktop bitmap fallback path now fails fast rather
// than upload mismatched bytes.
var AllowedAttachmentMimeTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"application/pdf": ".pdf",
}

// IsAllowedAttachmentMime reports whether the supplied content-type is in
// the whitelist.
func IsAllowedAttachmentMime(mime string) bool {
	_, ok := AllowedAttachmentMimeTypes[mime]
	return ok
}

// AttachmentExtension returns the canonical extension for a whitelisted
// MIME type, or "" if unknown.
func AttachmentExtension(mime string) string {
	return AllowedAttachmentMimeTypes[mime]
}

// Attachment is a single uploaded document tied to an expense. The blob
// itself lives in GCS at ObjectName; this struct is metadata stored inline
// on the expense doc (Firestore array). Reads are auth-gated by the same
// Firestore rules as the parent expense doc; writes go through the API.
type Attachment struct {
	ID               string    `json:"id"`
	ObjectName       string    `json:"object_name"`
	ContentType      string    `json:"content_type"`
	SizeBytes        int64     `json:"size_bytes"`
	OriginalFilename string    `json:"original_filename"`
	UploadedAt       time.Time `json:"uploaded_at"`
	UploadedBy       string    `json:"uploaded_by"`
}
