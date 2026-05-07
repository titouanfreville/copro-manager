package entities

import "errors"

// Detail holds additional information about a validation error.
type Detail struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

// ValidationError represents a domain validation error.
type ValidationError struct {
	Key     string   `json:"key"`
	Message string   `json:"message"`
	Details []Detail `json:"details,omitempty"`
}

func (ve ValidationError) Error() string {
	return ve.Key + " " + ve.Message
}

func (ve ValidationError) Is(tgt error) bool {
	var target ValidationError

	return errors.As(tgt, &target)
}

// AuthorizationError represents an authorization failure.
type AuthorizationError struct {
	Code string `json:"code"`
}

func (ae AuthorizationError) Error() string {
	return "authorization error: " + ae.Code
}

func (ae AuthorizationError) Is(tgt error) bool {
	var target AuthorizationError

	return errors.As(tgt, &target)
}
