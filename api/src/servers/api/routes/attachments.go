package routes

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/expenses"
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
	// any mismatch returns 403 SignatureDoesNotMatch from GCS. This is
	// critical for HEIC files where the browser-supplied `file.type` may
	// be empty after a HEIC→JPEG conversion in the client.
	ContentType string    `json:"content_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type recordAttachmentRequest struct {
	AttachmentID     string `json:"attachment_id"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	OriginalFilename string `json:"original_filename"`
}

type downloadURLResponse struct {
	DownloadURL string    `json:"download_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// RequestAttachmentUploadURL handles POST /expenses/{id}/attachments/upload-url.
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

	result, err := e.usecases.Expenses.RequestUploadURL(r.Context(), expenseID, expenses.RequestUploadInput{
		ActorUserID:      actorUID,
		OriginalFilename: req.OriginalFilename,
		ContentType:      req.ContentType,
		SizeBytes:        req.SizeBytes,
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusOK, w, r, uploadURLResponse{
		AttachmentID: result.AttachmentID,
		ObjectName:   result.ObjectName,
		UploadURL:    result.UploadURL,
		ContentType:  result.ContentType,
		ExpiresAt:    result.ExpiresAt,
	})
}

// RecordAttachment handles POST /expenses/{id}/attachments — the
// post-upload confirmation step.
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

	att, err := e.usecases.Expenses.RecordAttachment(r.Context(), expenseID, expenses.RecordAttachmentInput{
		ActorUserID:      actorUID,
		AttachmentID:     req.AttachmentID,
		ContentType:      req.ContentType,
		SizeBytes:        req.SizeBytes,
		OriginalFilename: req.OriginalFilename,
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusCreated, w, r, att)
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

	url, expiresAt, err := e.usecases.Expenses.GetDownloadURL(r.Context(), expenseID, attID, actorUID)
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

	if err := e.usecases.Expenses.DeleteAttachment(r.Context(), expenseID, attID, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().NoContent(http.StatusNoContent, w)
}
