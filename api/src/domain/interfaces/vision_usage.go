package interfaces

import "context"

// VisionUsageStore tracks per-month Cloud Vision API call counts so the
// monthly cap (NFR31) is enforceable. Keyed by YYYY-MM bucket; a single
// doc per period is sufficient for the 2-foyer scope.
type VisionUsageStore interface {
	CountForPeriod(ctx context.Context, period string) (int64, error)
	IncrementForPeriod(ctx context.Context, period string) error
}
