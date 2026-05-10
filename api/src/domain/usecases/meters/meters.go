// Package meters owns the water-meter-tracking domain: monthly readings
// (global + 3 detail submeters), the two photos that document each
// session, and the cascade rules that protect water_3_meters expenses
// from losing their reference.
//
// The Period (YYYY-MM) is the upsert key — at most one MeterReading per
// month, and re-capturing edits the existing row in place. Submeters
// were installed AFTER the global meter, so absolute readings drift by
// design; the only sanity check the UI surfaces is on month-over-month
// deltas (advisory, never blocking).
package meters

import (
	"context"
	"fmt"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/core/authz"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// meterPhotoURLTTL is the lifetime of a signed PUT/GET URL for a meter
// photo. Mirrors the attachments pipeline.
const meterPhotoURLTTL = 10 * time.Minute

// AlertsHook is the narrow contract this package needs from the alerts
// usecase: when a reading lands for the current period, the
// monthly_meter_reading alert auto-resolves. The alert is fired with a
// `:foyerID` suffix per recipient for idempotency, so we resolve by
// prefix to clear both rows in one call.
type AlertsHook interface {
	ResolveByPrefix(ctx context.Context, prefix string) error
}

// SaveInput is the user-facing CRUD shape — same struct for Create and
// Update. Period is the upsert key.
type SaveInput struct {
	ActorUserID string
	Period      string
	GlobalM3    float64
	CommonM3    float64
	RDCM3       float64
	PremierM3   float64
}

// RequestPhotoUploadInput is the client's pre-upload declaration for one
// of the two photos in a reading session.
type RequestPhotoUploadInput struct {
	ActorUserID string
	Period      string
	Kind        entities.MeterPhotoKind
	ContentType string
	SizeBytes   int64
}

// RequestPhotoUploadResult is what the route returns to the browser.
type RequestPhotoUploadResult struct {
	ObjectName  string
	UploadURL   string
	ContentType string
	ExpiresAt   time.Time
}

// RecordPhotoInput is the second leg of the upload dance: the client
// confirms the PUT completed and the server verifies via HEAD.
type RecordPhotoInput struct {
	ActorUserID string
	Period      string
	Kind        entities.MeterPhotoKind
	ContentType string
	SizeBytes   int64
}

// Usecases is the meters domain contract.
type Usecases interface {
	List(ctx context.Context, actorUserID string) ([]entities.MeterReading, error)
	FindByPeriod(ctx context.Context, period, actorUserID string) (*entities.MeterReading, error)
	Create(ctx context.Context, in SaveInput) (*entities.MeterReading, error)
	Update(ctx context.Context, in SaveInput) (*entities.MeterReading, error)
	Delete(ctx context.Context, period, actorUserID string) error

	RequestPhotoUploadURL(ctx context.Context, in RequestPhotoUploadInput) (*RequestPhotoUploadResult, error)
	RecordPhoto(ctx context.Context, in RecordPhotoInput) (*entities.MeterReading, error)
	GetPhotoDownloadURL(ctx context.Context, period string, kind entities.MeterPhotoKind, actorUserID string) (string, time.Time, error)
	DeletePhoto(ctx context.Context, period string, kind entities.MeterPhotoKind, actorUserID string) (*entities.MeterReading, error)

	// SuggestPhotoValues asks Gemini to read an already-recorded meter
	// photo. For `global` kind: 1 value (the main building dial). For
	// `detail` kind: 3 values [common, rdc, premier] in fixed order.
	// Empty slice when the reader is disabled / capped or the photo
	// isn't legible — the UI falls back to manual entry.
	SuggestPhotoValues(ctx context.Context, period string, kind entities.MeterPhotoKind, actorUserID string) (*OCRSuggestion, error)

	// SuggestRawPhotoValues is the stateless companion for the capture
	// flow's "Auto-lire" button BEFORE the photo is persisted: doesn't
	// touch Firestore or GCS, just sends the inline bytes to Gemini.
	SuggestRawPhotoValues(ctx context.Context, kind entities.MeterPhotoKind, image []byte, actorUserID string) (*OCRSuggestion, error)
}

// OCRSuggestion is the user-facing payload of the OCR endpoint. The
// per-index confidence slice is preserved as the wire format the
// frontend already consumes (`res.confidence?.[i]`); Gemini returns a
// single scalar that we replicate across each value's slot.
type OCRSuggestion struct {
	// Values per kind: `global` → 1 entry; `detail` → 3 entries
	// [common, rdc, premier].
	Values []float64
	// Confidence ∈ [0, 1] at the matching index. Currently identical
	// across slots (Gemini's overall confidence in the reading), kept
	// as a slice so the JSON contract with the SvelteKit UI is stable.
	Confidence []float64
}

type usecases struct {
	logger   *zap.Logger
	meters   interfaces.MetersStore
	expenses interfaces.ExpensesStore
	foyers   interfaces.FoyersStore
	copros   interfaces.CoprosStore
	storage  interfaces.StorageService
	reader   interfaces.MeterReader
	alerts   AlertsHook
	now      func() time.Time
}

// New builds a meters usecase. `alerts` and `reader` may be nil — the
// resolve hook and the OCR endpoint both degrade gracefully (empty
// suggestion when the reader is missing).
func New(
	logger *zap.Logger,
	meters interfaces.MetersStore,
	expenses interfaces.ExpensesStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	storage interfaces.StorageService,
	reader interfaces.MeterReader,
	alerts AlertsHook,
) Usecases {
	return &usecases{
		logger:   logger.Named("usecases.meters"),
		meters:   meters,
		expenses: expenses,
		foyers:   foyers,
		copros:   copros,
		storage:  storage,
		reader:   reader,
		alerts:   alerts,
		now:      time.Now,
	}
}

func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.MeterReading, error) {
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	return uc.meters.List(ctx)
}

func (uc *usecases) FindByPeriod(ctx context.Context, period, actorUserID string) (*entities.MeterReading, error) {
	if !entities.IsValidMeterPeriod(period) {
		return nil, entities.ValidationError{Key: "period", Message: "must match YYYY-MM"}
	}
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	m, err := uc.meters.FindByPeriod(ctx, period)
	if err != nil {
		return nil, fmt.Errorf("find meter: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("%w: meter reading %q", domainerrors.ErrNotFound, period)
	}
	return m, nil
}

// Create validates the inputs (well-formed period, non-negative values,
// no rollback vs. prior period) and writes a fresh reading. Fires the
// alert auto-resolve hook for the current calendar month.
func (uc *usecases) Create(ctx context.Context, in SaveInput) (*entities.MeterReading, error) {
	log := uc.logger.With(zap.String("method", "Create"), zap.String("period", in.Period))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		log.Warn("actor unauthorized", zap.Error(err))
		return nil, err
	}
	if err := validateSave(in); err != nil {
		return nil, err
	}

	prior, err := uc.meters.FindPriorPeriod(ctx, in.Period)
	if err != nil {
		return nil, fmt.Errorf("prior period lookup: %w", err)
	}
	if err := validateNoRollback(in, prior); err != nil {
		return nil, err
	}

	if existing, err := uc.meters.FindByPeriod(ctx, in.Period); err != nil {
		return nil, fmt.Errorf("find existing: %w", err)
	} else if existing != nil {
		return nil, entities.ValidationError{Key: "period", Message: "a reading already exists for this period — edit it instead"}
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return nil, fmt.Errorf("copro lookup: %w", err)
	}

	now := uc.now()
	m := entities.MeterReading{
		ID:            uuid.NewString(),
		CoproID:       copro.ID,
		Period:        in.Period,
		GlobalM3:      in.GlobalM3,
		CommonM3:      in.CommonM3,
		RDCM3:         in.RDCM3,
		PremierM3:     in.PremierM3,
		CapturedAt:    now,
		CapturedByUID: in.ActorUserID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := uc.meters.Create(ctx, m); err != nil {
		log.Error("store create failed", zap.Error(err))
		return nil, fmt.Errorf("create meter: %w", err)
	}

	// Auto-resolve the monthly_meter_reading alert ONLY when the user is
	// recording the current month — back-filling an old period shouldn't
	// silently dismiss the active reminder. Use the bare dedupe key as
	// prefix; the firing path appends `:foyerID` so a single call sweeps
	// both recipients.
	if uc.alerts != nil {
		currentPeriod := fmt.Sprintf("%04d-%02d", now.Year(), int(now.Month()))
		if in.Period == currentPeriod {
			if err := uc.alerts.ResolveByPrefix(ctx, entities.DedupeKeyMonthlyMeterReading(in.Period)+":"); err != nil {
				log.Warn("monthly meter reading auto-resolve failed", zap.Error(err))
			}
		}
	}

	log.Info("Success", zap.String("meter_id", m.ID))
	return &m, nil
}

// Update edits the four numeric values of an existing reading. Photos
// are managed via the dedicated photo endpoints, not here.
//
// Update REFUSES the edit when at least one water_3_meters expense
// still references this period — silently mutating the readings would
// drift the saved share-split away from the new deltas. The user must
// detach or delete those expenses first.
func (uc *usecases) Update(ctx context.Context, in SaveInput) (*entities.MeterReading, error) {
	log := uc.logger.With(zap.String("method", "Update"), zap.String("period", in.Period))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	if err := validateSave(in); err != nil {
		return nil, err
	}

	existing, err := uc.meters.FindByPeriod(ctx, in.Period)
	if err != nil {
		return nil, fmt.Errorf("find meter: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: meter reading %q", domainerrors.ErrNotFound, in.Period)
	}

	count, err := uc.expenses.CountByMeterReadingPeriod(ctx, in.Period)
	if err != nil {
		return nil, fmt.Errorf("count expense refs: %w", err)
	}
	if count > 0 {
		return nil, entities.ValidationError{
			Key:     "period",
			Message: fmt.Sprintf("%d expense(s) reference this period — supprime ou modifie-les avant d'éditer la lecture", count),
		}
	}

	prior, err := uc.meters.FindPriorPeriod(ctx, in.Period)
	if err != nil {
		return nil, fmt.Errorf("prior period lookup: %w", err)
	}
	if err := validateNoRollback(in, prior); err != nil {
		return nil, err
	}

	// Forward monotonicity: editing an old reading upward past the next
	// recorded period would silently break the rollback invariant for
	// every future delta. Reject symmetrically with the prior-rollback
	// guard.
	next, err := uc.meters.FindNextPeriod(ctx, in.Period)
	if err != nil {
		return nil, fmt.Errorf("next period lookup: %w", err)
	}
	if next != nil {
		details := []entities.Detail{}
		checkFwd := func(key string, edit, after float64) {
			if edit > after {
				details = append(details, entities.Detail{
					Key:     key,
					Message: fmt.Sprintf("ne peut pas dépasser la lecture suivante (%s : %.3f)", next.Period, after),
				})
			}
		}
		checkFwd("global_m3", in.GlobalM3, next.GlobalM3)
		checkFwd("common_m3", in.CommonM3, next.CommonM3)
		checkFwd("rdc_m3", in.RDCM3, next.RDCM3)
		checkFwd("premier_m3", in.PremierM3, next.PremierM3)
		if len(details) > 0 {
			return nil, entities.ValidationError{
				Key:     "forward_rollback",
				Message: "lecture supérieure à la période suivante",
				Details: details,
			}
		}
	}

	now := uc.now()
	existing.GlobalM3 = in.GlobalM3
	existing.CommonM3 = in.CommonM3
	existing.RDCM3 = in.RDCM3
	existing.PremierM3 = in.PremierM3
	existing.UpdatedAt = now

	if err := uc.meters.Update(ctx, *existing); err != nil {
		log.Error("store update failed", zap.Error(err))
		return nil, fmt.Errorf("update meter: %w", err)
	}
	log.Info("Success")
	return existing, nil
}

// Delete removes a reading (and its photos) — but only if no expense
// still references the period via meter_reading_period.
func (uc *usecases) Delete(ctx context.Context, period, actorUserID string) error {
	log := uc.logger.With(zap.String("method", "Delete"), zap.String("period", period))

	if !entities.IsValidMeterPeriod(period) {
		return entities.ValidationError{Key: "period", Message: "must match YYYY-MM"}
	}
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return err
	}

	existing, err := uc.meters.FindByPeriod(ctx, period)
	if err != nil {
		return fmt.Errorf("find meter: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%w: meter reading %q", domainerrors.ErrNotFound, period)
	}

	count, err := uc.expenses.CountByMeterReadingPeriod(ctx, period)
	if err != nil {
		return fmt.Errorf("count expense refs: %w", err)
	}
	if count > 0 {
		return entities.ValidationError{
			Key:     "period",
			Message: fmt.Sprintf("%d expense(s) reference this period — supprime ou modifie-les avant de retirer la lecture", count),
		}
	}

	if uc.storage != nil {
		if err := uc.storage.DeletePrefix(ctx, entities.MeterPhotoPrefix(period)); err != nil {
			log.Warn("photo prefix cleanup failed (orphan blobs may remain)", zap.Error(err))
		}
	}
	if err := uc.meters.Delete(ctx, period); err != nil {
		log.Error("delete failed", zap.Error(err))
		return fmt.Errorf("delete meter: %w", err)
	}
	log.Info("Success")
	return nil
}

// RequestPhotoUploadURL validates the declaration and returns a signed
// PUT URL. Metadata is NOT persisted until RecordPhoto runs.
func (uc *usecases) RequestPhotoUploadURL(ctx context.Context, in RequestPhotoUploadInput) (*RequestPhotoUploadResult, error) {
	log := uc.logger.With(
		zap.String("method", "RequestPhotoUploadURL"),
		zap.String("period", in.Period),
		zap.String("kind", string(in.Kind)),
	)
	if uc.storage == nil {
		return nil, fmt.Errorf("meters: storage not configured")
	}
	if !entities.IsValidMeterPeriod(in.Period) {
		return nil, entities.ValidationError{Key: "period", Message: "must match YYYY-MM"}
	}
	if !entities.IsKnownMeterPhotoKind(in.Kind) {
		return nil, entities.ValidationError{Key: "kind", Message: "must be one of: global, detail"}
	}
	contentType, err := normalizePhotoContentType(in.ContentType)
	if err != nil {
		return nil, err
	}
	if err := validatePhotoSize(in.SizeBytes); err != nil {
		return nil, err
	}
	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}

	if existing, err := uc.meters.FindByPeriod(ctx, in.Period); err != nil {
		return nil, fmt.Errorf("find meter: %w", err)
	} else if existing == nil {
		return nil, fmt.Errorf("%w: meter reading %q", domainerrors.ErrNotFound, in.Period)
	}

	objectName := entities.MeterPhotoObjectName(in.Period, in.Kind, contentType)
	url, err := uc.storage.SignedPutURL(ctx, objectName, contentType, in.SizeBytes, meterPhotoURLTTL)
	if err != nil {
		log.Error("signed put url failed", zap.Error(err))
		return nil, fmt.Errorf("signed put url: %w", err)
	}
	log.Info("Success")
	return &RequestPhotoUploadResult{
		ObjectName:  objectName,
		UploadURL:   url,
		ContentType: contentType,
		ExpiresAt:   uc.now().Add(meterPhotoURLTTL),
	}, nil
}

// RecordPhoto verifies the GCS object matches the declaration, then
// patches the matching field on the reading. Replacing a photo with a
// different content-type leaves the previous extension orphaned in
// GCS — best-effort cleanup keeps the bucket tidy.
func (uc *usecases) RecordPhoto(ctx context.Context, in RecordPhotoInput) (*entities.MeterReading, error) {
	log := uc.logger.With(
		zap.String("method", "RecordPhoto"),
		zap.String("period", in.Period),
		zap.String("kind", string(in.Kind)),
	)
	if uc.storage == nil {
		return nil, fmt.Errorf("meters: storage not configured")
	}
	if !entities.IsValidMeterPeriod(in.Period) {
		return nil, entities.ValidationError{Key: "period", Message: "must match YYYY-MM"}
	}
	if !entities.IsKnownMeterPhotoKind(in.Kind) {
		return nil, entities.ValidationError{Key: "kind", Message: "must be one of: global, detail"}
	}
	contentType, err := normalizePhotoContentType(in.ContentType)
	if err != nil {
		return nil, err
	}
	if err := validatePhotoSize(in.SizeBytes); err != nil {
		return nil, err
	}
	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}

	existing, err := uc.meters.FindByPeriod(ctx, in.Period)
	if err != nil {
		return nil, fmt.Errorf("find meter: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: meter reading %q", domainerrors.ErrNotFound, in.Period)
	}

	objectName := entities.MeterPhotoObjectName(in.Period, in.Kind, contentType)
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
		if delErr := uc.storage.Delete(ctx, objectName); delErr != nil {
			log.Warn("orphan cleanup failed", zap.Error(delErr))
		}
		return nil, entities.ValidationError{
			Key:     "object",
			Message: fmt.Sprintf("uploaded object metadata mismatch (size=%d, type=%q)", stat.SizeBytes, stat.ContentType),
		}
	}

	prevObject := selectPhotoObject(*existing, in.Kind)

	// Field-level update so two concurrent RecordPhoto calls (global +
	// detail running in parallel from the capture form) don't
	// lost-update each other on the doc.
	if err := uc.meters.SetPhoto(ctx, in.Period, in.Kind, objectName, contentType, in.SizeBytes); err != nil {
		log.Error("store set-photo failed", zap.Error(err))
		return nil, fmt.Errorf("update meter photo: %w", err)
	}

	// Cleanup the previous blob when a re-upload changed the extension —
	// the new object lives at a different path so the old one would
	// otherwise linger.
	if prevObject != "" && prevObject != objectName {
		if err := uc.storage.Delete(ctx, prevObject); err != nil {
			log.Warn("previous photo cleanup failed", zap.Error(err))
		}
	}

	// Re-fetch so the response reflects the field-level mutation plus
	// any other concurrent change (the disjoint photo, etc.).
	updated, err := uc.meters.FindByPeriod(ctx, in.Period)
	if err != nil {
		log.Warn("post-update reload failed", zap.Error(err))
		return existing, nil
	}
	if updated == nil {
		return existing, nil
	}
	log.Info("Success")
	return updated, nil
}

// GetPhotoDownloadURL issues a fresh signed GET URL for the requested
// photo. Returns ErrNotFound if the photo was never recorded.
func (uc *usecases) GetPhotoDownloadURL(ctx context.Context, period string, kind entities.MeterPhotoKind, actorUserID string) (string, time.Time, error) {
	if uc.storage == nil {
		return "", time.Time{}, fmt.Errorf("meters: storage not configured")
	}
	if !entities.IsValidMeterPeriod(period) {
		return "", time.Time{}, entities.ValidationError{Key: "period", Message: "must match YYYY-MM"}
	}
	if !entities.IsKnownMeterPhotoKind(kind) {
		return "", time.Time{}, entities.ValidationError{Key: "kind", Message: "must be one of: global, detail"}
	}
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return "", time.Time{}, err
	}
	existing, err := uc.meters.FindByPeriod(ctx, period)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("find meter: %w", err)
	}
	if existing == nil {
		return "", time.Time{}, fmt.Errorf("%w: meter reading %q", domainerrors.ErrNotFound, period)
	}
	obj := selectPhotoObject(*existing, kind)
	if obj == "" {
		return "", time.Time{}, fmt.Errorf("%w: photo %s for %q", domainerrors.ErrNotFound, kind, period)
	}
	url, err := uc.storage.SignedGetURL(ctx, obj, meterPhotoURLTTL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("signed get url: %w", err)
	}
	return url, uc.now().Add(meterPhotoURLTTL), nil
}

// DeletePhoto drops the GCS blob and clears the associated fields.
// Idempotent — deleting a missing photo is a no-op.
func (uc *usecases) DeletePhoto(ctx context.Context, period string, kind entities.MeterPhotoKind, actorUserID string) (*entities.MeterReading, error) {
	log := uc.logger.With(
		zap.String("method", "DeletePhoto"),
		zap.String("period", period),
		zap.String("kind", string(kind)),
	)
	if uc.storage == nil {
		return nil, fmt.Errorf("meters: storage not configured")
	}
	if !entities.IsValidMeterPeriod(period) {
		return nil, entities.ValidationError{Key: "period", Message: "must match YYYY-MM"}
	}
	if !entities.IsKnownMeterPhotoKind(kind) {
		return nil, entities.ValidationError{Key: "kind", Message: "must be one of: global, detail"}
	}
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	existing, err := uc.meters.FindByPeriod(ctx, period)
	if err != nil {
		return nil, fmt.Errorf("find meter: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: meter reading %q", domainerrors.ErrNotFound, period)
	}
	obj := selectPhotoObject(*existing, kind)
	if obj == "" {
		return existing, nil
	}
	if err := uc.storage.Delete(ctx, obj); err != nil {
		log.Warn("storage delete failed (will still drop metadata)", zap.Error(err))
	}
	if err := uc.meters.ClearPhoto(ctx, period, kind); err != nil {
		log.Error("metadata clear failed", zap.Error(err))
		return nil, fmt.Errorf("clear meter photo: %w", err)
	}
	updated, err := uc.meters.FindByPeriod(ctx, period)
	if err != nil || updated == nil {
		// Fall back to the in-memory mutation so the caller still gets a
		// fresh-looking response when the post-update read trips.
		clearPhoto(existing, kind)
		existing.UpdatedAt = uc.now()
		log.Info("Success")
		return existing, nil
	}
	log.Info("Success")
	return updated, nil
}

// SuggestPhotoValues runs OCR against an already-recorded photo and
// returns the most likely numeric reading(s). For the global meter:
// pick the candidate with the largest font height + plausible digit
// count (filters out serial / model labels). For the 3-meter detail
// panel: cluster numbers into per-meter groups, identify the BLUE
// housing as `common` via pixel sampling, and order the other two by
// distance to common (closer = 1er, farther = RDC) per the panel's
// physical layout.
//
// Heuristic, NOT magic — the user reviews the values before saving.
// Returns an empty suggestion (not an error) when the reader is
// disabled, capped, or hiccups so the UI cleanly falls back to manual
// entry.
func (uc *usecases) SuggestPhotoValues(ctx context.Context, period string, kind entities.MeterPhotoKind, actorUserID string) (*OCRSuggestion, error) {
	log := uc.logger.With(
		zap.String("method", "SuggestPhotoValues"),
		zap.String("period", period),
		zap.String("kind", string(kind)),
	)
	if uc.reader == nil || uc.storage == nil {
		return &OCRSuggestion{}, nil
	}
	if !entities.IsValidMeterPeriod(period) {
		return nil, entities.ValidationError{Key: "period", Message: "must match YYYY-MM"}
	}
	if !entities.IsKnownMeterPhotoKind(kind) {
		return nil, entities.ValidationError{Key: "kind", Message: "must be one of: global, detail"}
	}
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	existing, err := uc.meters.FindByPeriod(ctx, period)
	if err != nil {
		return nil, fmt.Errorf("find meter: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: meter reading %q", domainerrors.ErrNotFound, period)
	}
	obj := selectPhotoObject(*existing, kind)
	if obj == "" {
		return &OCRSuggestion{}, nil
	}
	// Fetch the bytes inline — Vertex AI Gemini takes inline images;
	// ~400 KB per photo, rarely-called endpoint, egress is negligible.
	imageBytes, err := uc.storage.Read(ctx, obj)
	if err != nil {
		log.Warn("photo fetch failed", zap.Error(err))
		return &OCRSuggestion{}, nil
	}
	return uc.analyzeImage(ctx, kind, imageBytes, photoMimeType(*existing, kind)), nil
}

// SuggestRawPhotoValues is the stateless companion to SuggestPhotoValues
// — for the capture flow where the photo hasn't been saved yet. Same
// pipeline applied to raw bytes.
func (uc *usecases) SuggestRawPhotoValues(ctx context.Context, kind entities.MeterPhotoKind, image []byte, actorUserID string) (*OCRSuggestion, error) {
	if uc.reader == nil {
		return &OCRSuggestion{}, nil
	}
	if !entities.IsKnownMeterPhotoKind(kind) {
		return nil, entities.ValidationError{Key: "kind", Message: "must be one of: global, detail"}
	}
	if len(image) == 0 {
		return nil, entities.ValidationError{Key: "image", Message: "empty"}
	}
	if int64(len(image)) > entities.MeterReadingMaxPhotoBytes {
		return nil, entities.ValidationError{
			Key:     "image",
			Message: fmt.Sprintf("exceeds %d bytes (10MB)", entities.MeterReadingMaxPhotoBytes),
		}
	}
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	// Sniff MIME from magic bytes — the route already enforces the
	// allow-list, so this is just to feed Gemini the correct hint.
	mimeType := http.DetectContentType(image)
	if parsed, _, err := mime.ParseMediaType(mimeType); err == nil && parsed != "" {
		mimeType = parsed
	}
	return uc.analyzeImage(ctx, kind, image, mimeType), nil
}

func (uc *usecases) authorize(ctx context.Context, actorUserID string) error {
	return authz.RequireFoyerMember(ctx, uc.foyers, actorUserID)
}

func validateSave(in SaveInput) error {
	details := []entities.Detail{}
	if !entities.IsValidMeterPeriod(in.Period) {
		details = append(details, entities.Detail{Key: "period", Message: "must match YYYY-MM"})
	}
	for k, v := range map[string]float64{
		"global_m3":  in.GlobalM3,
		"common_m3":  in.CommonM3,
		"rdc_m3":     in.RDCM3,
		"premier_m3": in.PremierM3,
	} {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			details = append(details, entities.Detail{Key: k, Message: "must be a finite number ≥ 0"})
		}
	}
	if len(details) > 0 {
		return entities.ValidationError{Key: "save_meter", Message: "invalid input", Details: details}
	}
	return nil
}

// validateNoRollback enforces "real meters only count up": each value
// must be ≥ the prior period's matching value.
func validateNoRollback(in SaveInput, prior *entities.MeterReading) error {
	if prior == nil {
		return nil
	}
	details := []entities.Detail{}
	check := func(key string, curr, before float64) {
		if curr < before {
			details = append(details, entities.Detail{
				Key:     key,
				Message: fmt.Sprintf("ne peut pas descendre sous la lecture précédente (%s : %.3f)", prior.Period, before),
			})
		}
	}
	check("global_m3", in.GlobalM3, prior.GlobalM3)
	check("common_m3", in.CommonM3, prior.CommonM3)
	check("rdc_m3", in.RDCM3, prior.RDCM3)
	check("premier_m3", in.PremierM3, prior.PremierM3)
	if len(details) > 0 {
		return entities.ValidationError{Key: "rollback", Message: "lecture en recul vs. la période précédente", Details: details}
	}
	return nil
}

func selectPhotoObject(m entities.MeterReading, kind entities.MeterPhotoKind) string {
	switch kind {
	case entities.MeterPhotoKindGlobal:
		return m.GlobalPhotoObject
	case entities.MeterPhotoKindDetail:
		return m.DetailPhotoObject
	}
	return ""
}

func clearPhoto(m *entities.MeterReading, kind entities.MeterPhotoKind) {
	switch kind {
	case entities.MeterPhotoKindGlobal:
		m.GlobalPhotoObject = ""
		m.GlobalPhotoContentType = ""
		m.GlobalPhotoSizeBytes = 0
	case entities.MeterPhotoKindDetail:
		m.DetailPhotoObject = ""
		m.DetailPhotoContentType = ""
		m.DetailPhotoSizeBytes = 0
	}
}

func normalizePhotoContentType(raw string) (string, error) {
	parsed, _, err := mime.ParseMediaType(raw)
	if err != nil {
		parsed = strings.ToLower(strings.TrimSpace(raw))
	} else {
		parsed = strings.ToLower(parsed)
	}
	if !entities.IsAllowedMeterPhotoMime(parsed) {
		return "", entities.ValidationError{Key: "content_type", Message: "unsupported (allowed: jpeg, png, heic, heif)"}
	}
	return parsed, nil
}

func validatePhotoSize(sizeBytes int64) error {
	if sizeBytes <= 0 {
		return entities.ValidationError{Key: "size_bytes", Message: "must be > 0"}
	}
	if sizeBytes > entities.MeterReadingMaxPhotoBytes {
		return entities.ValidationError{Key: "size_bytes", Message: fmt.Sprintf("exceeds %d bytes (10MB)", entities.MeterReadingMaxPhotoBytes)}
	}
	return nil
}
