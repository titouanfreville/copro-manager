package expenses

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
)

// attachmentURLTTL is the lifetime of a signed PUT/GET URL. Short on
// purpose — the browser uses each URL once, immediately.
const attachmentURLTTL = 10 * time.Minute

// originalFilenameMaxLen caps the per-attachment filename to keep the
// Firestore doc small and to discourage adversarial 64KB payloads.
const originalFilenameMaxLen = 256

// RequestUploadInput captures the client-side declaration of a file the
// user wants to upload. The server validates type + size, mints an
// attachment ID, and returns a signed URL the browser will PUT to.
type RequestUploadInput struct {
	ActorUserID      string
	OriginalFilename string
	ContentType      string
	SizeBytes        int64
}

// RequestUploadResult is what the route returns to the browser. The
// `ContentType` echoes the value the client must send as the `Content-Type`
// HTTP header on the PUT — it's signed into the URL so a mismatch returns
// 403 SignatureDoesNotMatch.
type RequestUploadResult struct {
	AttachmentID string
	ObjectName   string
	UploadURL    string
	ContentType  string
	ExpiresAt    time.Time
}

// RecordAttachmentInput is the second leg of the upload dance: the client
// confirms it has uploaded the file (matching the previously-issued
// attachment ID), and the server verifies via HEAD + records.
type RecordAttachmentInput struct {
	ActorUserID      string
	AttachmentID     string
	ContentType      string
	SizeBytes        int64
	OriginalFilename string
}

// RequestUploadURL validates the upload request and returns a short-lived
// signed PUT URL. It does NOT yet write metadata — the client must come
// back with RecordAttachment after the PUT completes.
func (uc *usecases) RequestUploadURL(ctx context.Context, expenseID string, in RequestUploadInput) (*RequestUploadResult, error) {
	log := uc.logger.With(
		zap.String("method", "RequestUploadURL"),
		zap.String("expense_id", expenseID),
		zap.String("content_type", in.ContentType),
		zap.Int64("size_bytes", in.SizeBytes),
	)

	if uc.storage == nil || uc.attachments == nil {
		return nil, fmt.Errorf("attachments: storage or store not configured")
	}
	if !isSafePathComponent(expenseID) {
		return nil, entities.ValidationError{Key: "expense_id", Message: "invalid id"}
	}
	contentType, err := normalizeContentType(in.ContentType)
	if err != nil {
		log.Warn("declaration rejected", zap.Error(err))
		return nil, err
	}
	if err := validateAttachmentSize(in.SizeBytes); err != nil {
		log.Warn("declaration rejected", zap.Error(err))
		return nil, err
	}

	// Authorize before resource lookup — same reasoning as Update/Delete.
	if err := uc.authorizeFoyerActor(ctx, in.ActorUserID); err != nil {
		log.Warn("actor unauthorized", zap.Error(err))
		return nil, err
	}

	exp, err := uc.expenses.FindByID(ctx, expenseID)
	if err != nil {
		log.Error("expense lookup failed", zap.Error(err))
		return nil, fmt.Errorf("find expense: %w", err)
	}
	if exp == nil {
		return nil, fmt.Errorf("%w: expense %q", domainerrors.ErrNotFound, expenseID)
	}

	// Best-effort cap pre-check at issuance time so the user gets a fast
	// 4xx instead of a useless signed URL. The authoritative cap is
	// re-enforced atomically in RecordAttachment via CreateIfUnderCap.
	count, err := uc.attachments.Count(ctx, expenseID)
	if err != nil {
		log.Error("attachments count failed", zap.Error(err))
		return nil, fmt.Errorf("count attachments: %w", err)
	}
	if count >= entities.AttachmentMaxPerExpense {
		return nil, entities.ValidationError{
			Key:     "attachments",
			Message: fmt.Sprintf("max %d attachments per expense", entities.AttachmentMaxPerExpense),
		}
	}

	attachmentID := uuid.NewString()
	objectName := buildObjectName(expenseID, attachmentID, contentType)

	url, err := uc.storage.SignedPutURL(ctx, objectName, contentType, in.SizeBytes, attachmentURLTTL)
	if err != nil {
		log.Error("signed put url failed", zap.Error(err))
		return nil, fmt.Errorf("signed put url: %w", err)
	}

	log.Info("Success", zap.String("attachment_id", attachmentID))
	return &RequestUploadResult{
		AttachmentID: attachmentID,
		ObjectName:   objectName,
		UploadURL:    url,
		ContentType:  contentType,
		ExpiresAt:    uc.now().Add(attachmentURLTTL),
	}, nil
}

// RecordAttachment verifies the uploaded blob exists with the expected
// type + size, then stores the metadata via the AttachmentsStore (subcoll).
// The persisted record uses the values from GCS HEAD, NOT the client's
// declaration — a client that lied about its upload can't store one set of
// metadata while serving another.
//
// The cap is re-enforced inside the Firestore transaction so concurrent
// uploaders can't both pass.
func (uc *usecases) RecordAttachment(ctx context.Context, expenseID string, in RecordAttachmentInput) (*entities.Attachment, error) {
	log := uc.logger.With(
		zap.String("method", "RecordAttachment"),
		zap.String("expense_id", expenseID),
		zap.String("attachment_id", in.AttachmentID),
	)

	if uc.storage == nil || uc.attachments == nil {
		return nil, fmt.Errorf("attachments: storage or store not configured")
	}
	if !isSafePathComponent(expenseID) {
		return nil, entities.ValidationError{Key: "expense_id", Message: "invalid id"}
	}
	if !isSafePathComponent(in.AttachmentID) {
		return nil, entities.ValidationError{Key: "attachment_id", Message: "invalid id"}
	}
	contentType, err := normalizeContentType(in.ContentType)
	if err != nil {
		return nil, err
	}
	if err := validateAttachmentSize(in.SizeBytes); err != nil {
		return nil, err
	}

	if err := uc.authorizeFoyerActor(ctx, in.ActorUserID); err != nil {
		return nil, err
	}

	exp, err := uc.expenses.FindByID(ctx, expenseID)
	if err != nil {
		log.Error("expense lookup failed", zap.Error(err))
		return nil, fmt.Errorf("find expense: %w", err)
	}
	if exp == nil {
		return nil, fmt.Errorf("%w: expense %q", domainerrors.ErrNotFound, expenseID)
	}

	objectName := buildObjectName(expenseID, in.AttachmentID, contentType)
	stat, found, err := uc.storage.Head(ctx, objectName)
	if err != nil {
		log.Error("head failed", zap.Error(err))
		return nil, fmt.Errorf("head object: %w", err)
	}
	if !found {
		return nil, entities.ValidationError{Key: "object", Message: "uploaded object not found — upload may not have completed"}
	}

	// GCS must echo a content-type for us to verify it. Empty stat.ContentType
	// would silently bypass the type check; reject so a client that omits
	// the Content-Type header on PUT can't sneak through.
	if stat.ContentType == "" {
		_ = uc.storage.Delete(ctx, objectName)
		return nil, entities.ValidationError{Key: "object", Message: "uploaded object missing content-type"}
	}
	statCT, _, _ := mime.ParseMediaType(stat.ContentType)
	if statCT == "" {
		statCT = stat.ContentType
	}
	if statCT != contentType || stat.SizeBytes != in.SizeBytes {
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

	att := entities.Attachment{
		ID:         in.AttachmentID,
		ObjectName: objectName,
		// Persist the values verified against GCS, not the client declaration.
		ContentType:      statCT,
		SizeBytes:        stat.SizeBytes,
		OriginalFilename: truncateFilename(in.OriginalFilename),
		UploadedAt:       uc.now(),
		UploadedBy:       in.ActorUserID,
	}

	if err := uc.attachments.CreateIfUnderCap(ctx, expenseID, att, entities.AttachmentMaxPerExpense); err != nil {
		// The store maps cap-reached and duplicate-id to ErrAlreadyExists.
		// Either way the blob we just verified is now an orphan; clean up
		// best-effort.
		if delErr := uc.storage.Delete(ctx, objectName); delErr != nil {
			log.Warn("rollback delete failed", zap.Error(delErr))
		}
		log.Warn("attachment record rejected", zap.Error(err))
		return nil, err
	}

	// missing_receipt is now moot for this expense — clear every stage.
	// Best-effort; failures don't block the attachment record.
	if uc.alerts != nil {
		if err := uc.alerts.ResolveMissingReceipt(ctx, expenseID); err != nil {
			log.Warn("resolve missing-receipt alerts failed", zap.Error(err))
		}
	}

	log.Info("Success")
	return &att, nil
}

// GetDownloadURL returns a fresh signed GET URL for the given attachment.
// The actor must be a foyer member and the attachment must exist.
func (uc *usecases) GetDownloadURL(ctx context.Context, expenseID, attachmentID, actorUserID string) (string, time.Time, error) {
	log := uc.logger.With(
		zap.String("method", "GetDownloadURL"),
		zap.String("expense_id", expenseID),
		zap.String("attachment_id", attachmentID),
	)

	if uc.storage == nil || uc.attachments == nil {
		return "", time.Time{}, fmt.Errorf("attachments: storage or store not configured")
	}
	if !isSafePathComponent(expenseID) || !isSafePathComponent(attachmentID) {
		return "", time.Time{}, entities.ValidationError{Key: "id", Message: "invalid id"}
	}

	if err := uc.authorizeFoyerActor(ctx, actorUserID); err != nil {
		return "", time.Time{}, err
	}

	att, err := uc.attachments.FindByID(ctx, expenseID, attachmentID)
	if err != nil {
		log.Error("attachment lookup failed", zap.Error(err))
		return "", time.Time{}, fmt.Errorf("find attachment: %w", err)
	}
	if att == nil {
		return "", time.Time{}, fmt.Errorf("%w: attachment %q", domainerrors.ErrNotFound, attachmentID)
	}

	url, err := uc.storage.SignedGetURL(ctx, att.ObjectName, attachmentURLTTL)
	if err != nil {
		log.Error("signed get url failed", zap.Error(err))
		return "", time.Time{}, fmt.Errorf("signed get url: %w", err)
	}
	log.Info("Success")
	return url, uc.now().Add(attachmentURLTTL), nil
}

// DeleteAttachment removes the GCS object then strips the metadata.
// Storage delete is idempotent; metadata removal is a no-op when the
// attachment is already absent.
func (uc *usecases) DeleteAttachment(ctx context.Context, expenseID, attachmentID, actorUserID string) error {
	log := uc.logger.With(
		zap.String("method", "DeleteAttachment"),
		zap.String("expense_id", expenseID),
		zap.String("attachment_id", attachmentID),
	)

	if uc.storage == nil || uc.attachments == nil {
		return fmt.Errorf("attachments: storage or store not configured")
	}
	if !isSafePathComponent(expenseID) || !isSafePathComponent(attachmentID) {
		return entities.ValidationError{Key: "id", Message: "invalid id"}
	}

	if err := uc.authorizeFoyerActor(ctx, actorUserID); err != nil {
		return err
	}

	att, err := uc.attachments.FindByID(ctx, expenseID, attachmentID)
	if err != nil {
		log.Error("attachment lookup failed", zap.Error(err))
		return fmt.Errorf("find attachment: %w", err)
	}
	if att == nil {
		return fmt.Errorf("%w: attachment %q", domainerrors.ErrNotFound, attachmentID)
	}

	if err := uc.storage.Delete(ctx, att.ObjectName); err != nil {
		log.Warn("storage delete failed (will still drop metadata)", zap.Error(err))
	}
	if err := uc.attachments.Delete(ctx, expenseID, attachmentID); err != nil {
		log.Error("metadata remove failed", zap.Error(err))
		return fmt.Errorf("remove attachment: %w", err)
	}

	log.Info("Success")
	return nil
}

// authorizeFoyerActor mirrors the gate used by Create/Update/Delete:
// loaded foyers, then membership check. An empty actor short-circuits to
// allow admin/CSV-import callers (the AdminKey gate covers them at
// transport).
func (uc *usecases) authorizeFoyerActor(ctx context.Context, actorUserID string) error {
	if actorUserID == "" {
		return nil
	}
	rdc, premier, err := uc.loadFoyers(ctx)
	if err != nil {
		return err
	}
	if !isFoyerMember(actorUserID, rdc, premier) {
		return entities.AuthorizationError{Code: "not_foyer_member"}
	}
	return nil
}

// normalizeContentType lowercases and parses the client-supplied media
// type, dropping parameters (`; charset=…`). Rejects values that aren't in
// the whitelist.
func normalizeContentType(raw string) (string, error) {
	parsed, _, err := mime.ParseMediaType(raw)
	if err != nil {
		parsed = strings.ToLower(strings.TrimSpace(raw))
	} else {
		parsed = strings.ToLower(parsed)
	}
	if !entities.IsAllowedAttachmentMime(parsed) {
		return "", entities.ValidationError{Key: "content_type", Message: "unsupported (allowed: jpeg, png, heic, heif, pdf)"}
	}
	return parsed, nil
}

func validateAttachmentSize(sizeBytes int64) error {
	if sizeBytes <= 0 {
		return entities.ValidationError{Key: "size_bytes", Message: "must be > 0"}
	}
	if sizeBytes > entities.AttachmentMaxSizeBytes {
		return entities.ValidationError{Key: "size_bytes", Message: fmt.Sprintf("exceeds %d bytes (10MB)", entities.AttachmentMaxSizeBytes)}
	}
	return nil
}

func truncateFilename(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > originalFilenameMaxLen {
		// Truncate by bytes; safe for our content (UTF-8 filenames) since
		// we only store this for display and never feed it back into a path.
		return name[:originalFilenameMaxLen]
	}
	return name
}

// isSafePathComponent rejects strings that would let a malicious caller
// escape an expense's GCS prefix or Firestore subcollection. We reject
// anything containing slashes, backslashes, or `..`, plus empty inputs and
// excessively long values. Stricter validation (e.g. UUID v4) lives at the
// route layer for newly-issued IDs.
func isSafePathComponent(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	if strings.ContainsAny(s, "/\\") {
		return false
	}
	if strings.Contains(s, "..") {
		return false
	}
	return true
}

// attachmentPrefix is the GCS object-name prefix for an expense's
// attachments. Used by both upload and cascade-delete.
func attachmentPrefix(expenseID string) string {
	return "expenses/" + expenseID + "/"
}

// buildObjectName composes the canonical object name from expense ID,
// attachment ID, and the canonical extension for the content type. Server
// authoritative: clients never get to choose the path.
func buildObjectName(expenseID, attachmentID, contentType string) string {
	return attachmentPrefix(expenseID) + attachmentID + entities.AttachmentExtension(contentType)
}
