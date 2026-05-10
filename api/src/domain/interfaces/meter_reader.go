package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// MeterReader interprets a residential water-meter photo and returns
// the digit reading(s) directly. Replaces the previous OCRService:
// instead of returning raw text blocks for the usecase to interpret,
// the implementation (Gemini multimodal) does the interpretation
// itself, eliminating the heuristics layer that used to live in
// `usecases/meters/ocr.go`.
//
// For kind=global the slice carries one value (the building's main
// dial). For kind=detail the slice carries three values in fixed
// order: [common, rdc, premier]. Confidence ∈ [0, 1] is the model's
// self-assessment of legibility — the UI uses it to decide whether
// to pre-fill the form or warn the user.
//
// Both entry points feed inline bytes; the saved-photo path reads
// from GCS first (one extra `storage.Read`, kept on Cloud Run egress
// since photos are <400 KB after client-side compression). No GCS
// URI variant — the call volume doesn't justify the second method.
type MeterReader interface {
	ReadMeterPhoto(
		ctx context.Context,
		kind entities.MeterPhotoKind,
		image []byte,
		mimeType string,
	) (values []float64, confidence float64, err error)
}
