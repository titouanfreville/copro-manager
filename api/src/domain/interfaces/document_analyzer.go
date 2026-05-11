package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// DocumentAnalyzer classifies an uploaded document (image or PDF) and
// extracts structured fields per kind (expense / contract / other).
// The Gemini-backed implementation lives in `services/gemini`.
//
// Inputs:
//   - image: the raw document bytes (image or PDF; size already gated
//     by the upload pipeline at <= 10 MB).
//   - mimeType: the IANA MIME the client uploaded; falls back to
//     image/jpeg if empty.
//
// Output: a fully-populated DocumentAnalysis (Kind always set, plus
// per-kind extraction nested when applicable). Errors only on
// infrastructure failure or feature gating (ErrFeatureDisabled /
// ErrFeatureCapped); a classification of "other" with low confidence
// is a valid success result, not an error.
type DocumentAnalyzer interface {
	AnalyzeDocument(
		ctx context.Context,
		image []byte,
		mimeType string,
	) (*entities.DocumentAnalysis, error)
}
