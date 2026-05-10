package rules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// NonBlank fails when the trimmed value is empty.
func NonBlank(field, value string) Rule {
	return func() error {
		if strings.TrimSpace(value) == "" {
			return entities.ValidationError{Key: field, Message: "required"}
		}
		return nil
	}
}

// MinLen fails when the trimmed value is shorter than `min` runes.
// Rune-aware so accented characters count once, not as their byte length.
func MinLen(field, value string, min int) Rule {
	return func() error {
		if runeLen(strings.TrimSpace(value)) < min {
			return entities.ValidationError{Key: field, Message: fmt.Sprintf("min %d caractères", min)}
		}
		return nil
	}
}

// MaxLen fails when the trimmed value is longer than `max` runes.
func MaxLen(field, value string, max int) Rule {
	return func() error {
		if runeLen(strings.TrimSpace(value)) > max {
			return entities.ValidationError{Key: field, Message: fmt.Sprintf("max %d caractères", max)}
		}
		return nil
	}
}

// Matches fails when the value doesn't match the supplied regexp.
// Use sparingly — most domains have a more readable check available.
func Matches(field, value string, pattern *regexp.Regexp, message string) Rule {
	return func() error {
		if !pattern.MatchString(value) {
			return entities.ValidationError{Key: field, Message: message}
		}
		return nil
	}
}

func runeLen(s string) int {
	return len([]rune(s))
}
