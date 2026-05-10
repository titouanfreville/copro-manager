package entities

import "time"

// DocumentMaxSizeBytes caps an upload at 10 MB (PRD FR34, mirroring the
// per-expense attachment cap so users have a consistent limit).
const DocumentMaxSizeBytes int64 = 10 * 1024 * 1024

// AllowedDocumentMimeTypes is the same whitelist as expense attachments.
// Keeping the two lists identical avoids "this works as an attachment but
// not as a standalone" UX surprises.
var AllowedDocumentMimeTypes = AllowedAttachmentMimeTypes

// IsAllowedDocumentMime reports whether the supplied content-type is in
// the document whitelist.
func IsAllowedDocumentMime(mime string) bool {
	_, ok := AllowedDocumentMimeTypes[mime]
	return ok
}

// DocumentExtension returns the canonical extension for a whitelisted
// MIME type, or "" if unknown.
func DocumentExtension(mime string) string {
	return AllowedDocumentMimeTypes[mime]
}

// Document is a standalone uploaded artifact (insurance contract, syndic
// statement, AGE minutes, plumber estimate, …) that may or may not be
// linked to a specific Expense. Per-expense attachments live in the
// expense subcollection — this entity is for the cases where a doc
// stands on its own.
//
// Group is a user-typed tag (devis / facture / contrat / attestation /
// etc.) used to fold similar docs together in the archive view. Free
// text — the catalog is derived from existing values rather than
// maintained as a separate entity. The server lowercases + trims on
// write so display variants merge into a single section client-side.
type Document struct {
	ID               string    `json:"id"`
	CoproID          string    `json:"copro_id"`
	CategoryID       string    `json:"category_id"`
	Group            string    `json:"group,omitempty"`
	Title            string    `json:"title"`
	Description      string    `json:"description,omitempty"`
	ObjectName       string    `json:"object_name"`
	ContentType      string    `json:"content_type"`
	SizeBytes        int64     `json:"size_bytes"`
	OriginalFilename string    `json:"original_filename"`
	UploadedAt       time.Time `json:"uploaded_at"`
	UploadedBy       string    `json:"uploaded_by"`
	// LinkedExpenseID is empty for v1 standalone documents. The field is
	// the hook for a future "link existing document to expense" flow
	// without a schema migration.
	LinkedExpenseID string `json:"linked_expense_id,omitempty"`
	// LinkedContractID pins the document to a Contract (the contract PDF,
	// addenda, attestations). Mutually orthogonal with LinkedExpenseID —
	// a single doc can be linked to both an expense (e.g. a renewal
	// invoice) and the contract that produced it.
	LinkedContractID string `json:"linked_contract_id,omitempty"`
}
