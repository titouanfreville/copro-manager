package rules

import (
	"fmt"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// IntAtLeast fails when v < min.
func IntAtLeast(field string, v, min int) Rule {
	return func() error {
		if v < min {
			return entities.ValidationError{Key: field, Message: fmt.Sprintf("must be ≥ %d", min)}
		}
		return nil
	}
}

// IntAtMost fails when v > max.
func IntAtMost(field string, v, max int) Rule {
	return func() error {
		if v > max {
			return entities.ValidationError{Key: field, Message: fmt.Sprintf("must be ≤ %d", max)}
		}
		return nil
	}
}

// IntNonNegative is the common shorthand: v ≥ 0.
func IntNonNegative(field string, v int) Rule {
	return IntAtLeast(field, v, 0)
}
