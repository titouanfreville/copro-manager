package rules

import (
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// OneOf fails when `value` is set but isn't in the allowed set. An
// empty `value` short-circuits as allowed — pair with NonBlank if the
// field is required.
func OneOf[T comparable](field string, value T, allowed []T) Rule {
	return func() error {
		var zero T
		if value == zero {
			return nil
		}
		for _, a := range allowed {
			if value == a {
				return nil
			}
		}
		return entities.ValidationError{Key: field, Message: "unknown value"}
	}
}
