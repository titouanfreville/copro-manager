package interfaces

import "context"

// OCRTextBlock is one detected piece of text in a meter photo, with the
// bounding-box coordinates the position-based assignment heuristic
// needs.
type OCRTextBlock struct {
	// Text is the raw detected text (we don't pre-filter to digits at
	// the service boundary — meter labels like "RDC" can be useful
	// downstream).
	Text string
	// Confidence ∈ [0, 1] from the underlying OCR engine. May be 0 when
	// the engine doesn't expose a value.
	Confidence float64
	// Bounding box, in normalized (0..1) image coordinates so consumers
	// don't need the source image's pixel dimensions.
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// OCRService runs text detection against an image. Two entry points:
//
//   - DetectText takes a GCS URI; Vision pulls the bytes itself, no
//     egress through Cloud Run. Used by the "re-run OCR on a saved
//     reading" flow on the edit page.
//   - DetectTextFromBytes takes the raw bytes inline. Used by the
//     capture flow on the new-meter page where the photo isn't yet
//     persisted (the user wants OCR to pre-fill the form BEFORE save,
//     so there's no meter doc / no GCS object to point at).
type OCRService interface {
	DetectText(ctx context.Context, gcsURI string) ([]OCRTextBlock, error)
	DetectTextFromBytes(ctx context.Context, image []byte) ([]OCRTextBlock, error)
}
