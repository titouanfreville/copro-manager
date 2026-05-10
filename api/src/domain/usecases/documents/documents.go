// Package documents owns the document layer: standalone uploads,
// per-expense attachments (LinkedExpenseID set), and contract
// attestations (LinkedContractID set). The signed-URL upload dance
// has two legs:
//
//   1. RequestUploadURL — validate, mint a short-lived PUT URL.
//   2. Record           — verify the GCS object matches, persist metadata.
//
// Validation lives in adapters/validators/documents.go; entity
// construction lives in build.go. This file is pure orchestration so
// the upload + edit + delete flows stay legible top-to-bottom.
package documents

import (
	"context"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/core/authz"
	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// documentURLTTL is the lifetime of every signed PUT/GET URL — short
// on purpose: the browser uses each URL once, immediately.
const documentURLTTL = 10 * time.Minute

// RequestUploadInput is the route-layer DTO for the pre-upload leg.
// The actor UID is separate from the data fields so the validator
// only sees draft data.
type RequestUploadInput struct {
	ActorUserID string
	entities.DocumentDraft
}

// RequestUploadResult is what the route returns to the browser.
type RequestUploadResult struct {
	DocumentID  string
	ObjectName  string
	UploadURL   string
	ContentType string
	ExpiresAt   time.Time
}

// RecordDocumentInput is the post-upload confirmation: same draft
// shape as RequestUpload plus the document_id minted upstream.
type RecordDocumentInput struct {
	ActorUserID string
	DocumentID  string
	entities.DocumentDraft
}

// UpdateDocumentInput edits the metadata of an existing doc — file
// blob is immutable.
type UpdateDocumentInput struct {
	ActorUserID string
	entities.DocumentMetadataDraft
}

// Usecases is the documents domain contract.
type Usecases interface {
	List(ctx context.Context, actorUserID string) ([]entities.Document, error)
	RequestUploadURL(ctx context.Context, in RequestUploadInput) (*RequestUploadResult, error)
	Record(ctx context.Context, in RecordDocumentInput) (*entities.Document, error)
	Update(ctx context.Context, id string, in UpdateDocumentInput) (*entities.Document, error)
	Delete(ctx context.Context, id, actorUserID string) error
	GetDownloadURL(ctx context.Context, id, actorUserID string) (string, time.Time, error)
	// DeleteByLinkedExpense wipes every document attached to the given
	// expense — used by the expense-delete cascade. Best-effort: a
	// per-doc failure is logged and the loop continues.
	DeleteByLinkedExpense(ctx context.Context, expenseID string) error
}

type usecases struct {
	logger    *zap.Logger
	documents interfaces.DocumentsStore
	foyers    interfaces.FoyersStore
	storage   interfaces.StorageService
	validator interfaces.DocumentValidator
	builder   *builder
	now       func() time.Time
}

// New builds a documents usecase.
func New(
	logger *zap.Logger,
	documents interfaces.DocumentsStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	expenses interfaces.ExpensesStore,
	storage interfaces.StorageService,
	validator interfaces.DocumentValidator,
) Usecases {
	now := time.Now
	return &usecases{
		logger:    logger.Named("usecases.documents"),
		documents: documents,
		foyers:    foyers,
		storage:   storage,
		validator: validator,
		builder:   newBuilder(copros, expenses, now),
		now:       now,
	}
}

// List returns every document in the copro. Foyer-membership gated.
func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.Document, error) {
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	return uc.documents.List(ctx)
}

// RequestUploadURL validates the declaration and returns a signed
// PUT URL. Metadata is NOT written until Record is called.
func (uc *usecases) RequestUploadURL(ctx context.Context, in RequestUploadInput) (*RequestUploadResult, error) {
	log := uc.logger.With(
		zap.String("method", "RequestUploadURL"),
		zap.String("content_type", in.ContentType),
		zap.Int64("size_bytes", in.SizeBytes),
		zap.String("linked_expense_id", in.LinkedExpenseID),
	)

	if uc.storage == nil {
		return nil, fmt.Errorf("documents: storage not configured")
	}
	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	draft, err := uc.builder.applyLinkedDefaults(ctx, in.DocumentDraft)
	if err != nil {
		return nil, fmt.Errorf("apply linked defaults: %w", err)
	}
	if err := uc.validator.ValidateUpload(ctx, draft); err != nil {
		return nil, err
	}

	contentType, _ := rest.NormalizeUploadMime(draft.ContentType)
	docID := uuid.NewString()
	objectName := buildObjectName(docID, contentType)

	url, err := uc.storage.SignedPutURL(ctx, objectName, contentType, draft.SizeBytes, documentURLTTL)
	if err != nil {
		log.Error("signed put url failed", zap.Error(err))
		return nil, fmt.Errorf("signed put url: %w", err)
	}
	log.Info("Success", zap.String("document_id", docID))
	return &RequestUploadResult{
		DocumentID:  docID,
		ObjectName:  objectName,
		UploadURL:   url,
		ContentType: contentType,
		ExpiresAt:   uc.now().Add(documentURLTTL),
	}, nil
}

// Record verifies the uploaded GCS object and persists the metadata.
func (uc *usecases) Record(ctx context.Context, in RecordDocumentInput) (*entities.Document, error) {
	log := uc.logger.With(zap.String("method", "Record"), zap.String("document_id", in.DocumentID))

	if uc.storage == nil {
		return nil, fmt.Errorf("documents: storage not configured")
	}
	if !isSafeID(in.DocumentID) {
		return nil, entities.ValidationError{Key: "document_id", Message: "invalid id"}
	}
	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	draft, err := uc.builder.applyLinkedDefaults(ctx, in.DocumentDraft)
	if err != nil {
		return nil, fmt.Errorf("apply linked defaults: %w", err)
	}
	if err := uc.validator.ValidateUpload(ctx, draft); err != nil {
		return nil, err
	}

	contentType, _ := rest.NormalizeUploadMime(draft.ContentType)
	objectName := buildObjectName(in.DocumentID, contentType)
	if err := uc.verifyUpload(ctx, log, objectName, contentType, draft.SizeBytes); err != nil {
		return nil, err
	}
	d, err := uc.builder.build(ctx, in.DocumentID, objectName, contentType, draft, in.ActorUserID)
	if err != nil {
		return nil, fmt.Errorf("build document: %w", err)
	}
	if err := uc.documents.Create(ctx, d); err != nil {
		log.Error("store create failed", zap.Error(err))
		return nil, fmt.Errorf("create document: %w", err)
	}
	log.Info("Success")
	return &d, nil
}

// Update applies a validated metadata draft. The file blob is
// immutable in v1 — delete + re-upload to replace.
func (uc *usecases) Update(ctx context.Context, id string, in UpdateDocumentInput) (*entities.Document, error) {
	log := uc.logger.With(zap.String("method", "Update"), zap.String("document_id", id))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	existing, err := uc.documents.FindByID(ctx, id)
	if err != nil {
		log.Error("lookup failed", zap.Error(err))
		return nil, fmt.Errorf("find document: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: document %q", domainerrors.ErrNotFound, id)
	}
	if err := uc.validator.ValidateUpdate(ctx, in.DocumentMetadataDraft); err != nil {
		return nil, err
	}
	updated := uc.builder.applyMetadata(*existing, in.DocumentMetadataDraft)
	if err := uc.documents.Update(ctx, updated); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update document: %w", err)
	}
	log.Info("Success")
	return &updated, nil
}

// Delete removes the metadata + GCS object. Storage delete is
// idempotent.
func (uc *usecases) Delete(ctx context.Context, id, actorUserID string) error {
	log := uc.logger.With(zap.String("method", "Delete"), zap.String("document_id", id))

	if err := uc.authorize(ctx, actorUserID); err != nil {
		return err
	}
	existing, err := uc.documents.FindByID(ctx, id)
	if err != nil {
		log.Error("lookup failed", zap.Error(err))
		return fmt.Errorf("find document: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%w: document %q", domainerrors.ErrNotFound, id)
	}
	if uc.storage != nil {
		if err := uc.storage.Delete(ctx, existing.ObjectName); err != nil {
			log.Warn("storage delete failed (will still drop metadata)", zap.Error(err))
		}
	}
	if err := uc.documents.Delete(ctx, id); err != nil {
		log.Error("metadata delete failed", zap.Error(err))
		return fmt.Errorf("delete document: %w", err)
	}
	log.Info("Success")
	return nil
}

// DeleteByLinkedExpense wipes every document attached to the given
// expense. Used by the expense-delete cascade — bypasses the foyer
// gate because the caller has already authorized.
func (uc *usecases) DeleteByLinkedExpense(ctx context.Context, expenseID string) error {
	log := uc.logger.With(zap.String("method", "DeleteByLinkedExpense"), zap.String("expense_id", expenseID))

	id := strings.TrimSpace(expenseID)
	if id == "" {
		return nil
	}
	docs, err := uc.documents.ListByLinkedExpense(ctx, id)
	if err != nil {
		return fmt.Errorf("list by linked: %w", err)
	}
	for _, d := range docs {
		if uc.storage != nil {
			if err := uc.storage.Delete(ctx, d.ObjectName); err != nil {
				log.Warn("storage delete failed (continuing)", zap.String("document_id", d.ID), zap.Error(err))
			}
		}
		if err := uc.documents.Delete(ctx, d.ID); err != nil {
			log.Warn("metadata delete failed (continuing)", zap.String("document_id", d.ID), zap.Error(err))
		}
	}
	log.Info("Success", zap.Int("documents_deleted", len(docs)))
	return nil
}

// GetDownloadURL issues a fresh signed GET URL for an existing doc.
func (uc *usecases) GetDownloadURL(ctx context.Context, id, actorUserID string) (string, time.Time, error) {
	if uc.storage == nil {
		return "", time.Time{}, fmt.Errorf("documents: storage not configured")
	}
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return "", time.Time{}, err
	}
	existing, err := uc.documents.FindByID(ctx, id)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("find document: %w", err)
	}
	if existing == nil {
		return "", time.Time{}, fmt.Errorf("%w: document %q", domainerrors.ErrNotFound, id)
	}
	url, err := uc.storage.SignedGetURL(ctx, existing.ObjectName, documentURLTTL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("signed get url: %w", err)
	}
	return url, uc.now().Add(documentURLTTL), nil
}

func (uc *usecases) authorize(ctx context.Context, actorUserID string) error {
	return authz.RequireFoyerMember(ctx, uc.foyers, actorUserID)
}

// verifyUpload HEADs the uploaded blob and confirms it matches the
// declared content-type + size. Mismatch (a client lying about the
// upload) yields a validation error and a best-effort orphan cleanup.
func (uc *usecases) verifyUpload(ctx context.Context, log *zap.Logger, objectName, contentType string, sizeBytes int64) error {
	stat, found, err := uc.storage.Head(ctx, objectName)
	if err != nil {
		log.Error("head failed", zap.Error(err))
		return fmt.Errorf("head object: %w", err)
	}
	if !found {
		return entities.ValidationError{Key: "object", Message: "uploaded object not found — upload may not have completed"}
	}
	statCT, _, _ := mime.ParseMediaType(stat.ContentType)
	if statCT == "" {
		statCT = stat.ContentType
	}
	if stat.ContentType == "" || statCT != contentType || stat.SizeBytes != sizeBytes {
		if delErr := uc.storage.Delete(ctx, objectName); delErr != nil {
			log.Warn("orphan cleanup failed", zap.Error(delErr))
		}
		return entities.ValidationError{
			Key:     "object",
			Message: fmt.Sprintf("uploaded object metadata mismatch (size=%d, type=%q)", stat.SizeBytes, stat.ContentType),
		}
	}
	return nil
}
