package errors

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrNotImplemented = errors.New("not implemented")
	ErrAlreadyExists  = errors.New("already exists")
)
