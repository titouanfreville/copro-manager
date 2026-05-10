// Package meters persists MeterReading entities in Firestore. Period
// (YYYY-MM) is the doc ID so re-capturing a month overwrites cleanly.
package meters

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const collection = "meter_readings"

type meterDoc struct {
	ID        string  `firestore:"id"`
	CoproID   string  `firestore:"copro_id"`
	Period    string  `firestore:"period"`
	GlobalM3  float64 `firestore:"global_m3"`
	CommonM3  float64 `firestore:"common_m3"`
	RDCM3     float64 `firestore:"rdc_m3"`
	PremierM3 float64 `firestore:"premier_m3"`

	GlobalPhotoObject      string `firestore:"global_photo_object,omitempty"`
	GlobalPhotoContentType string `firestore:"global_photo_content_type,omitempty"`
	GlobalPhotoSizeBytes   int64  `firestore:"global_photo_size_bytes,omitempty"`
	DetailPhotoObject      string `firestore:"detail_photo_object,omitempty"`
	DetailPhotoContentType string `firestore:"detail_photo_content_type,omitempty"`
	DetailPhotoSizeBytes   int64  `firestore:"detail_photo_size_bytes,omitempty"`

	CapturedAt    time.Time `firestore:"captured_at"`
	CapturedByUID string    `firestore:"captured_by_uid"`
	CreatedAt     time.Time `firestore:"created_at"`
	UpdatedAt     time.Time `firestore:"updated_at"`
}

// Store is the Firestore-backed MetersStore implementation.
type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed meters store.
func NewStore(client *fs.Client) interfaces.MetersStore {
	return &Store{client: client}
}

// List returns every reading sorted by Period descending (most recent
// first) — matches the UI's default ordering.
func (s *Store) List(ctx context.Context) ([]entities.MeterReading, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.MeterReading
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("meters: list: %w", err)
		}
		var doc meterDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("meters: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Period > out[j].Period })
	return out, nil
}

// FindByPeriod looks up a reading by its YYYY-MM doc ID.
func (s *Store) FindByPeriod(ctx context.Context, period string) (*entities.MeterReading, error) {
	snap, err := s.client.Collection(collection).Doc(period).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("meters: get %q: %w", period, err)
	}
	var doc meterDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("meters: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

// FindPriorPeriod returns the most recent reading whose Period is
// strictly less than `period`. Lexicographic ordering is correct because
// every Period is fixed-width YYYY-MM.
func (s *Store) FindPriorPeriod(ctx context.Context, period string) (*entities.MeterReading, error) {
	iter := s.client.Collection(collection).
		Where("period", "<", period).
		OrderBy("period", fs.Desc).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	snap, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("meters: prior period for %q: %w", period, err)
	}
	var doc meterDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("meters: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

// FindNextPeriod is the forward symmetric of FindPriorPeriod — the
// earliest reading whose Period is strictly greater than `period`.
func (s *Store) FindNextPeriod(ctx context.Context, period string) (*entities.MeterReading, error) {
	iter := s.client.Collection(collection).
		Where("period", ">", period).
		OrderBy("period", fs.Asc).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	snap, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("meters: next period for %q: %w", period, err)
	}
	var doc meterDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("meters: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

// Create inserts a fresh reading. Fails if a doc with the same Period
// already exists — caller must explicitly Update to overwrite.
func (s *Store) Create(ctx context.Context, m entities.MeterReading) error {
	if _, err := s.client.Collection(collection).Doc(m.Period).Create(ctx, entityToDoc(m)); err != nil {
		return fmt.Errorf("meters: create: %w", err)
	}
	return nil
}

// Update overwrites an existing reading. Caller is responsible for
// having loaded the row first.
func (s *Store) Update(ctx context.Context, m entities.MeterReading) error {
	if _, err := s.client.Collection(collection).Doc(m.Period).Set(ctx, entityToDoc(m)); err != nil {
		return fmt.Errorf("meters: update: %w", err)
	}
	return nil
}

// Delete removes the reading by Period. Idempotent.
func (s *Store) Delete(ctx context.Context, period string) error {
	if _, err := s.client.Collection(collection).Doc(period).Delete(ctx); err != nil {
		return fmt.Errorf("meters: delete: %w", err)
	}
	return nil
}

// SetPhoto patches just the photo-related fields for the given kind. A
// field-level update keeps concurrent two-photo uploads (global +
// detail running in parallel from the capture form) from clobbering
// each other — the previous read-modify-write on the full doc would
// otherwise drop one of the two photos.
func (s *Store) SetPhoto(ctx context.Context, period string, kind entities.MeterPhotoKind, object, contentType string, sizeBytes int64) error {
	objField, ctField, sizeField, ok := photoFieldNames(kind)
	if !ok {
		return fmt.Errorf("meters: set photo: unknown kind %q", kind)
	}
	_, err := s.client.Collection(collection).Doc(period).Update(ctx, []fs.Update{
		{Path: objField, Value: object},
		{Path: ctField, Value: contentType},
		{Path: sizeField, Value: sizeBytes},
		{Path: "updated_at", Value: time.Now()},
	})
	if err != nil {
		return fmt.Errorf("meters: set photo: %w", err)
	}
	return nil
}

// ClearPhoto wipes the photo fields for the given kind. Same field-
// level mechanism so an unrelated photo on the same doc isn't touched.
func (s *Store) ClearPhoto(ctx context.Context, period string, kind entities.MeterPhotoKind) error {
	objField, ctField, sizeField, ok := photoFieldNames(kind)
	if !ok {
		return fmt.Errorf("meters: clear photo: unknown kind %q", kind)
	}
	_, err := s.client.Collection(collection).Doc(period).Update(ctx, []fs.Update{
		{Path: objField, Value: ""},
		{Path: ctField, Value: ""},
		{Path: sizeField, Value: int64(0)},
		{Path: "updated_at", Value: time.Now()},
	})
	if err != nil {
		return fmt.Errorf("meters: clear photo: %w", err)
	}
	return nil
}

// photoFieldNames maps a MeterPhotoKind to the Firestore field paths
// for its (object, content_type, size_bytes) triplet.
func photoFieldNames(kind entities.MeterPhotoKind) (string, string, string, bool) {
	switch kind {
	case entities.MeterPhotoKindGlobal:
		return "global_photo_object", "global_photo_content_type", "global_photo_size_bytes", true
	case entities.MeterPhotoKindDetail:
		return "detail_photo_object", "detail_photo_content_type", "detail_photo_size_bytes", true
	}
	return "", "", "", false
}

func docToEntity(d meterDoc) entities.MeterReading {
	return entities.MeterReading{
		ID:                     d.ID,
		CoproID:                d.CoproID,
		Period:                 d.Period,
		GlobalM3:               d.GlobalM3,
		CommonM3:               d.CommonM3,
		RDCM3:                  d.RDCM3,
		PremierM3:              d.PremierM3,
		GlobalPhotoObject:      d.GlobalPhotoObject,
		GlobalPhotoContentType: d.GlobalPhotoContentType,
		GlobalPhotoSizeBytes:   d.GlobalPhotoSizeBytes,
		DetailPhotoObject:      d.DetailPhotoObject,
		DetailPhotoContentType: d.DetailPhotoContentType,
		DetailPhotoSizeBytes:   d.DetailPhotoSizeBytes,
		CapturedAt:             d.CapturedAt,
		CapturedByUID:          d.CapturedByUID,
		CreatedAt:              d.CreatedAt,
		UpdatedAt:              d.UpdatedAt,
	}
}

func entityToDoc(m entities.MeterReading) meterDoc {
	return meterDoc{
		ID:                     m.ID,
		CoproID:                m.CoproID,
		Period:                 m.Period,
		GlobalM3:               m.GlobalM3,
		CommonM3:               m.CommonM3,
		RDCM3:                  m.RDCM3,
		PremierM3:              m.PremierM3,
		GlobalPhotoObject:      m.GlobalPhotoObject,
		GlobalPhotoContentType: m.GlobalPhotoContentType,
		GlobalPhotoSizeBytes:   m.GlobalPhotoSizeBytes,
		DetailPhotoObject:      m.DetailPhotoObject,
		DetailPhotoContentType: m.DetailPhotoContentType,
		DetailPhotoSizeBytes:   m.DetailPhotoSizeBytes,
		CapturedAt:             m.CapturedAt,
		CapturedByUID:          m.CapturedByUID,
		CreatedAt:              m.CreatedAt,
		UpdatedAt:              m.UpdatedAt,
	}
}
