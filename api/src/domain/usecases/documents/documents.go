// Package documents owns standalone-document CRUD and the signed-URL
// upload dance (mirrors the per-expense attachment flow but keyed by
// document ID rather than expense ID). Group is normalized to lowercase +
// trimmed on write so display variants ("Devis" vs "devis") merge into a
// single foldable section in the archive view.
package documents

import (
	"context"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const (
	// documentURLTTL is the lifetime of a signed PUT/GET URL. Short on
	// purpose — the browser uses each URL once, immediately.
	documentURLTTL = 10 * time.Minute
	// titleMaxLen caps the title at a sane length for the Firestore doc.
	titleMaxLen = 200
	// descriptionMaxLen — same idea, longer ceiling.
	descriptionMaxLen = 2000
	// groupMaxLen — group is a tag, not free prose.
	groupMaxLen = 64
	// originalFilenameMaxLen mirrors the attachments cap.
	originalFilenameMaxLen = 256
)

// RequestUploadInput captures the client's pre-upload declaration.
type RequestUploadInput struct {
	ActorUserID      string
	Title            string
	Description      string
	CategoryID       string
	Group            string
	OriginalFilename string
	ContentType      string
	SizeBytes        int64
}

// RequestUploadResult is what the route returns to the browser.
type RequestUploadResult struct {
	DocumentID  string
	ObjectName  string
	UploadURL   string
	ContentType string
	ExpiresAt   time.Time
}

// RecordDocumentInput is the second leg of the upload dance.
type RecordDocumentInput struct {
	ActorUserID      string
	DocumentID       string
	Title            string
	Description      string
	CategoryID       string
	Group            string
	ContentType      string
	SizeBytes        int64
	OriginalFilename string
}

// UpdateDocumentInput is for editing the metadata of an existing doc.
// File replacement is out of scope for v1.
type UpdateDocumentInput struct {
	ActorUserID string
	Title       string
	Description string
	CategoryID  string
	Group       string
}

// Usecases is the documents domain contract.
type Usecases interface {
	List(ctx context.Context, actorUserID string) ([]entities.Document, error)
	RequestUploadURL(ctx context.Context, in RequestUploadInput) (*RequestUploadResult, error)
	Record(ctx context.Context, in RecordDocumentInput) (*entities.Document, error)
	Update(ctx context.Context, id string, in UpdateDocumentInput) (*entities.Document, error)
	Delete(ctx context.Context, id, actorUserID string) error
	GetDownloadURL(ctx context.Context, id, actorUserID string) (string, time.Time, error)
}

type usecases struct {
	logger     *zap.Logger
	documents  interfaces.DocumentsStore
	categories interfaces.CategoriesStore
	foyers     interfaces.FoyersStore
	copros     interfaces.CoprosStore
	storage    interfaces.StorageService
	now        func() time.Time
}

// New builds a documents usecase.
func New(
	logger *zap.Logger,
	documents interfaces.DocumentsStore,
	categories interfaces.CategoriesStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	storage interfaces.StorageService,
) Usecases {
	return &usecases{
		logger:     logger.Named("usecases.documents"),
		documents:  documents,
		categories: categories,
		foyers:     foyers,
		copros:     copros,
		storage:    storage,
		now:        time.Now,
	}
}

func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.Document, error) {
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	return uc.documents.List(ctx)
}

// RequestUploadURL validates the declaration and returns a signed PUT URL.
// Metadata is NOT written until Record is called post-upload.
func (uc *usecases) RequestUploadURL(ctx context.Context, in RequestUploadInput) (*RequestUploadResult, error) {
	log := uc.logger.With(
		zap.String("method", "RequestUploadURL"),
		zap.String("content_type", in.ContentType),
		zap.Int64("size_bytes", in.SizeBytes),
	)

	if uc.storage == nil {
		return nil, fmt.Errorf("documents: storage not configured")
	}
	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		log.Warn("actor unauthorized", zap.Error(err))
		return nil, err
	}
	contentType, err := normalizeContentType(in.ContentType)
	if err != nil {
		return nil, err
	}
	if err := validateSize(in.SizeBytes); err != nil {
		return nil, err
	}
	if err := uc.validateTitle(in.Title); err != nil {
		return nil, err
	}
	if err := uc.checkCategory(ctx, in.CategoryID); err != nil {
		return nil, err
	}

	docID := uuid.NewString()
	objectName := buildObjectName(docID, contentType)

	url, err := uc.storage.SignedPutURL(ctx, objectName, contentType, in.SizeBytes, documentURLTTL)
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

// Record verifies the GCS object matches the declaration, then persists
// the metadata. Mirrors RecordAttachment's verify-then-write pattern.
func (uc *usecases) Record(ctx context.Context, in RecordDocumentInput) (*entities.Document, error) {
	log := uc.logger.With(
		zap.String("method", "Record"),
		zap.String("document_id", in.DocumentID),
	)

	if uc.storage == nil {
		return nil, fmt.Errorf("documents: storage not configured")
	}
	if !isSafeID(in.DocumentID) {
		return nil, entities.ValidationError{Key: "document_id", Message: "invalid id"}
	}
	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	contentType, err := normalizeContentType(in.ContentType)
	if err != nil {
		return nil, err
	}
	if err := validateSize(in.SizeBytes); err != nil {
		return nil, err
	}
	if err := uc.validateTitle(in.Title); err != nil {
		return nil, err
	}
	if err := uc.checkCategory(ctx, in.CategoryID); err != nil {
		return nil, err
	}

	objectName := buildObjectName(in.DocumentID, contentType)
	stat, found, err := uc.storage.Head(ctx, objectName)
	if err != nil {
		log.Error("head failed", zap.Error(err))
		return nil, fmt.Errorf("head object: %w", err)
	}
	if !found {
		return nil, entities.ValidationError{Key: "object", Message: "uploaded object not found — upload may not have completed"}
	}
	statCT, _, _ := mime.ParseMediaType(stat.ContentType)
	if statCT == "" {
		statCT = stat.ContentType
	}
	if stat.ContentType == "" || statCT != contentType || stat.SizeBytes != in.SizeBytes {
		// Orphan cleanup: client lied about the upload (size or type
		// mismatch). Drop the bad blob and surface a validation error.
		if delErr := uc.storage.Delete(ctx, objectName); delErr != nil {
			log.Warn("orphan cleanup failed", zap.Error(delErr))
		}
		return nil, entities.ValidationError{
			Key:     "object",
			Message: fmt.Sprintf("uploaded object metadata mismatch (size=%d, type=%q)", stat.SizeBytes, stat.ContentType),
		}
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return nil, fmt.Errorf("copro lookup: %w", err)
	}

	now := uc.now()
	d := entities.Document{
		ID:               in.DocumentID,
		CoproID:          copro.ID,
		CategoryID:       in.CategoryID,
		Group:            normalizeGroup(in.Group),
		Title:            strings.TrimSpace(in.Title),
		Description:      truncate(strings.TrimSpace(in.Description), descriptionMaxLen),
		ObjectName:       objectName,
		ContentType:      contentType,
		SizeBytes:        in.SizeBytes,
		OriginalFilename: truncate(strings.TrimSpace(in.OriginalFilename), originalFilenameMaxLen),
		UploadedAt:       now,
		UploadedBy:       in.ActorUserID,
	}
	if err := uc.documents.Create(ctx, d); err != nil {
		log.Error("store create failed", zap.Error(err))
		return nil, fmt.Errorf("create document: %w", err)
	}
	log.Info("Success")
	return &d, nil
}

// Update edits the metadata of an existing doc — title, description,
// category, group. The file itself is immutable in v1 (delete + re-upload
// to replace).
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
	if err := uc.validateTitle(in.Title); err != nil {
		return nil, err
	}
	if err := uc.checkCategory(ctx, in.CategoryID); err != nil {
		return nil, err
	}

	existing.Title = strings.TrimSpace(in.Title)
	existing.Description = truncate(strings.TrimSpace(in.Description), descriptionMaxLen)
	existing.CategoryID = in.CategoryID
	existing.Group = normalizeGroup(in.Group)

	if err := uc.documents.Update(ctx, *existing); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update document: %w", err)
	}
	log.Info("Success")
	return existing, nil
}

// Delete removes the metadata and the GCS object. Storage delete is
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

// GetDownloadURL issues a fresh signed GET URL for an existing document.
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

// authorize replicates the foyer-member gate used by every other domain
// in the project. Empty actor short-circuits — there are no admin/cron
// callers for documents today, but keeping the empty-bypass keeps the
// pattern consistent.
func (uc *usecases) authorize(ctx context.Context, actorUserID string) error {
	if actorUserID == "" {
		return nil
	}
	rdc, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloorRDC)
	if err != nil {
		return fmt.Errorf("find rdc: %w", err)
	}
	premier, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloor1er)
	if err != nil {
		return fmt.Errorf("find 1er: %w", err)
	}
	if rdc == nil || premier == nil {
		return fmt.Errorf("%w: both foyers must exist", domainerrors.ErrNotFound)
	}
	for _, mid := range rdc.MemberIDs {
		if mid == actorUserID {
			return nil
		}
	}
	for _, mid := range premier.MemberIDs {
		if mid == actorUserID {
			return nil
		}
	}
	return entities.AuthorizationError{Code: "not_foyer_member"}
}

func (uc *usecases) validateTitle(title string) error {
	t := strings.TrimSpace(title)
	if t == "" {
		return entities.ValidationError{Key: "title", Message: "required"}
	}
	if len(t) > titleMaxLen {
		return entities.ValidationError{Key: "title", Message: fmt.Sprintf("max %d chars", titleMaxLen)}
	}
	return nil
}

func (uc *usecases) checkCategory(ctx context.Context, categoryID string) error {
	id := strings.TrimSpace(categoryID)
	if id == "" {
		return entities.ValidationError{Key: "category_id", Message: "required"}
	}
	cat, err := uc.categories.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("category lookup: %w", err)
	}
	if cat == nil {
		return entities.ValidationError{Key: "category_id", Message: "not found"}
	}
	return nil
}

func normalizeContentType(ct string) (string, error) {
	parsed, _, err := mime.ParseMediaType(ct)
	if err != nil || parsed == "" {
		return "", entities.ValidationError{Key: "content_type", Message: "invalid"}
	}
	if !entities.IsAllowedDocumentMime(parsed) {
		return "", entities.ValidationError{Key: "content_type", Message: "unsupported (allowed: jpeg, png, heic, heif, pdf)"}
	}
	return parsed, nil
}

func validateSize(sizeBytes int64) error {
	if sizeBytes <= 0 {
		return entities.ValidationError{Key: "size_bytes", Message: "must be > 0"}
	}
	if sizeBytes > entities.DocumentMaxSizeBytes {
		return entities.ValidationError{Key: "size_bytes", Message: fmt.Sprintf("exceeds %d bytes (10MB)", entities.DocumentMaxSizeBytes)}
	}
	return nil
}

// normalizeGroup lowercases + trims so display variants merge. Empty
// stays empty (rendered as "Sans groupe" client-side).
func normalizeGroup(g string) string {
	cleaned := strings.ToLower(strings.TrimSpace(g))
	if cleaned == "" {
		return ""
	}
	return truncate(cleaned, groupMaxLen)
}

// buildObjectName composes the canonical GCS key. Server authoritative —
// clients never get to choose the path.
func buildObjectName(documentID, contentType string) string {
	return "documents/" + documentID + entities.DocumentExtension(contentType)
}

// isSafeID rejects anything that could escape the documents/ prefix or
// upset Firestore (slashes, control chars, leading dots).
func isSafeID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if r == '/' || r == '.' || r < 0x20 {
			return false
		}
	}
	return true
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
