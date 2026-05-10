package meters

import (
	"context"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// analyzeImage delegates to the configured MeterReader (Vertex AI
// Gemini in production) and shapes its scalar response into the
// per-slot OCRSuggestion the route hands back to the SvelteKit UI.
//
// Confidence is replicated across slots so the wire format
// (`{values: [...], confidence: [...]}`) the frontend already consumes
// is preserved — Gemini reports a single confidence for the whole
// reading; the UI's `res.confidence?.[i]` lookups continue to work.
//
// Returns an empty suggestion (never nil) on any failure short of
// authorization or input validation, so the UI cleanly falls back to
// manual entry without surfacing infrastructure errors to the user.
func (uc *usecases) analyzeImage(
	ctx context.Context,
	kind entities.MeterPhotoKind,
	imageBytes []byte,
	mimeType string,
) *OCRSuggestion {
	log := uc.logger.With(
		zap.String("method", "analyzeImage"),
		zap.String("kind", string(kind)),
		zap.Int("image_bytes", len(imageBytes)),
	)
	values, confidence, err := uc.reader.ReadMeterPhoto(ctx, kind, imageBytes, mimeType)
	if err != nil {
		log.Warn("meter reader failed", zap.Error(err))
		return &OCRSuggestion{}
	}
	if len(values) == 0 {
		return &OCRSuggestion{}
	}
	conf := make([]float64, len(values))
	for i := range conf {
		conf[i] = confidence
	}
	return &OCRSuggestion{Values: values, Confidence: conf}
}

// photoMimeType returns the persisted Content-Type for a meter
// photo, defaulting to image/jpeg when the field is absent (older
// records). Used by SuggestPhotoValues to feed Gemini the right
// inline-data hint.
func photoMimeType(m entities.MeterReading, kind entities.MeterPhotoKind) string {
	switch kind {
	case entities.MeterPhotoKindGlobal:
		if m.GlobalPhotoContentType != "" {
			return m.GlobalPhotoContentType
		}
	case entities.MeterPhotoKindDetail:
		if m.DetailPhotoContentType != "" {
			return m.DetailPhotoContentType
		}
	}
	return "image/jpeg"
}
