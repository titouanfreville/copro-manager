package routes

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/documents"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type uploadDocURLRequest struct {
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	CategoryID       string `json:"category_id"`
	Group            string `json:"group,omitempty"`
	OriginalFilename string `json:"original_filename"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	LinkedContractID string `json:"linked_contract_id,omitempty"`
}

type uploadDocURLResponse struct {
	DocumentID  string    `json:"document_id"`
	ObjectName  string    `json:"object_name"`
	UploadURL   string    `json:"upload_url"`
	ContentType string    `json:"content_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type recordDocRequest struct {
	DocumentID       string `json:"document_id"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	CategoryID       string `json:"category_id"`
	Group            string `json:"group,omitempty"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	OriginalFilename string `json:"original_filename"`
	LinkedContractID string `json:"linked_contract_id,omitempty"`
}

type updateDocRequest struct {
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	CategoryID       string `json:"category_id"`
	Group            string `json:"group,omitempty"`
	LinkedContractID string `json:"linked_contract_id,omitempty"`
}

type docDownloadURLResponse struct {
	DownloadURL string    `json:"download_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// ListDocuments handles GET /documents.
func (e *Endpoints) ListDocuments(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	out, err := e.usecases.Documents.List(r.Context(), actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	if out == nil {
		out = []entities.Document{}
	}
	rest.Render().JSON(http.StatusOK, w, r, out)
}

// RequestDocumentUploadURL handles POST /documents/upload-url.
func (e *Endpoints) RequestDocumentUploadURL(w http.ResponseWriter, r *http.Request) {
	var req uploadDocURLRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)

	result, err := e.usecases.Documents.RequestUploadURL(r.Context(), documents.RequestUploadInput{
		ActorUserID: actorUID,
		DocumentDraft: entities.DocumentDraft{
			Title:            req.Title,
			Description:      req.Description,
			CategoryID:       req.CategoryID,
			Group:            req.Group,
			OriginalFilename: req.OriginalFilename,
			ContentType:      req.ContentType,
			SizeBytes:        req.SizeBytes,
			LinkedContractID: req.LinkedContractID,
		},
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, uploadDocURLResponse{
		DocumentID:  result.DocumentID,
		ObjectName:  result.ObjectName,
		UploadURL:   result.UploadURL,
		ContentType: result.ContentType,
		ExpiresAt:   result.ExpiresAt,
	})
}

// RecordDocument handles POST /documents — the post-upload confirmation.
func (e *Endpoints) RecordDocument(w http.ResponseWriter, r *http.Request) {
	var req recordDocRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)

	d, err := e.usecases.Documents.Record(r.Context(), documents.RecordDocumentInput{
		ActorUserID: actorUID,
		DocumentID:  req.DocumentID,
		DocumentDraft: entities.DocumentDraft{
			Title:            req.Title,
			Description:      req.Description,
			CategoryID:       req.CategoryID,
			Group:            req.Group,
			ContentType:      req.ContentType,
			SizeBytes:        req.SizeBytes,
			OriginalFilename: req.OriginalFilename,
			LinkedContractID: req.LinkedContractID,
		},
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusCreated, w, r, d)
}

// UpdateDocument handles PATCH /documents/{id}.
func (e *Endpoints) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing document id"))
		return
	}
	var req updateDocRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)

	d, err := e.usecases.Documents.Update(r.Context(), id, documents.UpdateDocumentInput{
		ActorUserID: actorUID,
		DocumentMetadataDraft: entities.DocumentMetadataDraft{
			Title:            req.Title,
			Description:      req.Description,
			CategoryID:       req.CategoryID,
			Group:            req.Group,
			LinkedContractID: req.LinkedContractID,
		},
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, d)
}

// DeleteDocument handles DELETE /documents/{id}.
func (e *Endpoints) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing document id"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Documents.Delete(r.Context(), id, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}

// GetDocumentDownloadURL handles GET /documents/{id}/download-url.
func (e *Endpoints) GetDocumentDownloadURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing document id"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	url, expiresAt, err := e.usecases.Documents.GetDownloadURL(r.Context(), id, actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, docDownloadURLResponse{
		DownloadURL: url,
		ExpiresAt:   expiresAt,
	})
}

// AnalyzeDocument handles POST /documents/{id}/analyze. Runs Gemini
// classification + extraction (cached on the document; `?force=true`
// bypasses the cache). Returns the full Document with Analysis set.
func (e *Endpoints) AnalyzeDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing document id"))
		return
	}
	// strconv.ParseBool accepts the standard truthy set (1/t/T/TRUE/
	// true/True) instead of the strict "true" match — friendlier to
	// shell users and matches how Go's standard library treats bools.
	force, _ := strconv.ParseBool(r.URL.Query().Get("force"))
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	doc, err := e.usecases.Documents.Analyze(r.Context(), id, actorUID, force)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, doc)
}
