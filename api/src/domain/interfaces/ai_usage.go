package interfaces

import "context"

// AIUsageStore tracks per-month Vertex AI / Gemini call counts so the
// monthly cap (NFR31) is enforceable. Keyed by YYYY-MM bucket; one doc
// per period covers the 2-foyer scope without contention.
//
// Same shape as the previous VisionUsageStore — kept under a generic
// name so the document analyzer and future chat assistant can share
// the counter without a second collection.
type AIUsageStore interface {
	CountForPeriod(ctx context.Context, period string) (int64, error)
	IncrementForPeriod(ctx context.Context, period string) error
}
