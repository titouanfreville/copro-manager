package errors

import "errors"

var (
	ErrNotFound        = errors.New("not found")
	ErrNotImplemented  = errors.New("not implemented")
	ErrAlreadyExists   = errors.New("already exists")
	ErrFeatureDisabled = errors.New("feature disabled")
	ErrFeatureCapped   = errors.New("feature monthly cap reached")
)
