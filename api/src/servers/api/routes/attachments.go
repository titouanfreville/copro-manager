// Package routes exposes the per-expense attachment endpoints. The
// underlying storage is the unified `documents` collection — a per-expense
// attachment is a Document with `linked_expense_id` set. The routes keep
// the legacy `attachment_id` wire-field for client compatibility.
package routes

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/documents"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type uploadURLRequest struct {
	OriginalFilename string `json:"original_filename"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
}

type uploadURLResponse struct {
	AttachmentID string `json:"attachment_id"`
	ObjectName   string `json:"object_name"`
	UploadURL    string `json:"upload_url"`
	// ContentType echoes the value the client must send as the
	// `Content-Type` HTTP header on the PUT — it's signed into the URL so
	// any mismatch returns 403 SignatureDoesNotMatch from GCS.
	ContentType string    `json:"content_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type recordAttachmentRequest struct {
	AttachmentID     string `json:"attachment_id"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	OriginalFilename string `json:"original_filename"`
}

type attachmentResponse struct {
	ID               string    `json:"id"`
	ObjectName       string    `json:"object_name"`
	ContentType      string    `json:"content_type"`
	SizeBytes        int64     `json:"size_bytes"`
	OriginalFilename string    `json:"original_filename"`
	UploadedAt       time.Time `json:"uploaded_at"`
	UploadedBy       string    `json:"uploaded_by"`
}

type downloadURLResponse struct {
	DownloadURL string    `json:"download_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func documentToAttachment(d *entities.Document) attachmentResponse {
	return attachmentResponse{
		ID:               d.ID,
		ObjectName:       d.ObjectName,
		ContentType:      d.ContentType,
		SizeBytes:        d.SizeBytes,
		OriginalFilename: d.OriginalFilename,
		UploadedAt:       d.UploadedAt,
		UploadedBy:       d.UploadedBy,
	}
}

// RequestAttachmentUploadURL handles POST /expenses/{id}/attachments/upload-url.
// Delegates to the Documents usecase with linked_expense_id set; title and
// category default from the parent expense.
func (e *Endpoints) RequestAttachmentUploadURL(w http.ResponseWriter, r *http.Request) {
	expenseID := chi.URLParam(r, "id")
	if expenseID == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing expense id"))
		return
	}

	var req uploadURLRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}

	actorUID, _ := r.Context().Value(shared.UserID).(string)

	result, err := e.usecases.Documents.RequestUploadURL(r.Context(), documents.RequestUploadInput{
		ActorUserID: actorUID,
		DocumentDraft: entities.DocumentDraft{
			OriginalFilename: req.OriginalFilename,
			ContentType:      req.ContentType,
			SizeBytes:        req.SizeBytes,
			LinkedExpenseID:  expenseID,
		},
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusOK, w, r, uploadURLResponse{
		AttachmentID: result.DocumentID,
		ObjectName:   result.ObjectName,
		UploadURL:    result.UploadURL,
		ContentType:  result.ContentType,
		ExpiresAt:    result.ExpiresAt,
	})
}

// RecordAttachment handles POST /expenses/{id}/attachments — the
// post-upload confirmation step. Persists a Document with
// linked_expense_id pointing at this expense.
func (e *Endpoints) RecordAttachment(w http.ResponseWriter, r *http.Request) {
	expenseID := chi.URLParam(r, "id")
	if expenseID == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing expense id"))
		return
	}

	var req recordAttachmentRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}

	actorUID, _ := r.Context().Value(shared.UserID).(string)

	d, err := e.usecases.Documents.Record(r.Context(), documents.RecordDocumentInput{
		ActorUserID: actorUID,
		DocumentID:  req.AttachmentID,
		DocumentDraft: entities.DocumentDraft{
			ContentType:      req.ContentType,
			SizeBytes:        req.SizeBytes,
			OriginalFilename: req.OriginalFilename,
			LinkedExpenseID:  expenseID,
		},
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusCreated, w, r, documentToAttachment(d))
}

// GetAttachmentDownloadURL handles GET /expenses/{id}/attachments/{attID}/download-url.
func (e *Endpoints) GetAttachmentDownloadURL(w http.ResponseWriter, r *http.Request) {
	expenseID := chi.URLParam(r, "id")
	attID := chi.URLParam(r, "attID")
	if expenseID == "" || attID == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing expense or attachment id"))
		return
	}

	actorUID, _ := r.Context().Value(shared.UserID).(string)

	url, expiresAt, err := e.usecases.Documents.GetDownloadURL(r.Context(), attID, actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusOK, w, r, downloadURLResponse{
		DownloadURL: url,
		ExpiresAt:   expiresAt,
	})
}

// DeleteAttachment handles DELETE /expenses/{id}/attachments/{attID}.
func (e *Endpoints) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	expenseID := chi.URLParam(r, "id")
	attID := chi.URLParam(r, "attID")
	if expenseID == "" || attID == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing expense or attachment id"))
		return
	}

	actorUID, _ := r.Context().Value(shared.UserID).(string)

	if err := e.usecases.Documents.Delete(r.Context(), attID, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().NoContent(http.StatusNoContent, w)
}
