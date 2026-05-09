package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// MetersStore persists MeterReading entities — one per YYYY-MM period.
// Period is the upsert key, so a re-capture overwrites in place rather
// than fanning out new docs.
type MetersStore interface {
	List(ctx context.Context) ([]entities.MeterReading, error)
	FindByPeriod(ctx context.Context, period string) (*entities.MeterReading, error)
	// FindPriorPeriod returns the most recent reading whose Period is
	// strictly less than `period` (lexicographic compare works because
	// the format is fixed-width YYYY-MM). Returns (nil, nil) when no
	// prior reading exists.
	FindPriorPeriod(ctx context.Context, period string) (*entities.MeterReading, error)
	// FindNextPeriod is the symmetric forward lookup — the closest
	// reading whose Period is strictly greater than `period`. Returns
	// (nil, nil) when `period` is the latest recorded.
	FindNextPeriod(ctx context.Context, period string) (*entities.MeterReading, error)
	Create(ctx context.Context, m entities.MeterReading) error
	Update(ctx context.Context, m entities.MeterReading) error
	Delete(ctx context.Context, period string) error

	// SetPhoto patches just the (object, content_type, size, updated_at)
	// fields for the given kind. Field-level update so two concurrent
	// uploads (one global, one detail) don't lost-update each other —
	// the read-modify-write on the full doc would otherwise drop one of
	// the two photos.
	SetPhoto(ctx context.Context, period string, kind entities.MeterPhotoKind, object, contentType string, sizeBytes int64) error
	// ClearPhoto wipes just the photo fields for the given kind.
	ClearPhoto(ctx context.Context, period string, kind entities.MeterPhotoKind) error
}
