package rules

import (
	"time"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// DateNotBefore fails when `value` precedes `floor`. Both zero values
// short-circuit as allowed (typical "field is optional" semantics).
func DateNotBefore(field string, value, floor time.Time) Rule {
	return func() error {
		if value.IsZero() || floor.IsZero() {
			return nil
		}
		if value.Before(floor) {
			return entities.ValidationError{Key: field, Message: "must be on or after start_date"}
		}
		return nil
	}
}
