package errors

import "errors"

var (
	ErrNotFound        = errors.New("not found")
	ErrNotImplemented  = errors.New("not implemented")
	ErrAlreadyExists   = errors.New("already exists")
	ErrFeatureDisabled = errors.New("feature disabled")
	ErrFeatureCapped   = errors.New("feature monthly cap reached")
	// ErrAnalysisFailed marks a recoverable failure of the
	// document-analysis pipeline — Gemini hiccup, malformed JSON
	// response, or empty body. The route maps it to a non-fatal
	// status so the UI can surface "réessayer" instead of a 500.
	ErrAnalysisFailed = errors.New("analysis failed")
)
