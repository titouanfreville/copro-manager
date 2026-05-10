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

// Attachment is a single uploaded document tied to an expense. The blob
// itself lives in GCS at ObjectName; this struct is metadata stored inline
// on the expense doc (Firestore array). Reads are auth-gated by the same
// Firestore rules as the parent expense doc; writes go through the API.
//
// The MIME whitelist for uploads lives in core/rest — content-type is
// part of the HTTP boundary's vocabulary, not a business invariant of
// the Attachment entity.
type Attachment struct {
	ID               string    `json:"id"`
	ObjectName       string    `json:"object_name"`
	ContentType      string    `json:"content_type"`
	SizeBytes        int64     `json:"size_bytes"`
	OriginalFilename string    `json:"original_filename"`
	UploadedAt       time.Time `json:"uploaded_at"`
	UploadedBy       string    `json:"uploaded_by"`
}
