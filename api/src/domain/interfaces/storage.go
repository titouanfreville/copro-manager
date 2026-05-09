package interfaces

import (
	"context"
	"time"
)

// ObjectStat is the minimum metadata returned by Head — enough to verify
// that a freshly-uploaded blob matches the size and content-type the
// client declared up-front.
type ObjectStat struct {
	SizeBytes   int64
	ContentType string
}

// StorageService is the domain-side contract for the GCS-backed blob
// store. The adapter (api/src/services/storage) implements it; the
// expenses usecase consumes it for signed-URL issuance, post-upload
// verification, and cascading deletes.
type StorageService interface {
	// SignedPutURL returns a V4 signed URL the browser can PUT to. The URL
	// pins the expected content-type and size — GCS will reject mismatched
	// uploads at write time, before the metadata is recorded.
	SignedPutURL(ctx context.Context, objectName, contentType string, sizeBytes int64, ttl time.Duration) (string, error)

	// SignedGetURL returns a short-lived V4 signed URL for read access.
	SignedGetURL(ctx context.Context, objectName string, ttl time.Duration) (string, error)

	// Head returns (stat, true, nil) when the object exists. Returns
	// (zero, false, nil) when absent. Any other error is wrapped.
	Head(ctx context.Context, objectName string) (ObjectStat, bool, error)

	// Delete removes a single object. Idempotent — missing objects are
	// reported as no-ops.
	Delete(ctx context.Context, objectName string) error

	// DeletePrefix removes every object under the given prefix. Used by
	// the expense-delete cascade.
	DeletePrefix(ctx context.Context, prefix string) error

	// Read returns the full object bytes. Used by the OCR path to fetch
	// a meter photo for color-sampling + Vision analysis. Bounded files
	// only — meter photos cap at MeterReadingMaxPhotoBytes (10 MB).
	Read(ctx context.Context, objectName string) ([]byte, error)
}
