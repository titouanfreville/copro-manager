package documents

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// ─── Mocks ──────────────────────────────────────────────────────────

type mockDocumentsStore struct{ mock.Mock }

func (m *mockDocumentsStore) List(ctx context.Context) ([]entities.Document, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.Document), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockDocumentsStore) FindByID(ctx context.Context, id string) (*entities.Document, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.Document), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockDocumentsStore) Create(ctx context.Context, d entities.Document) error {
	return m.Called(ctx, d).Error(0)
}
func (m *mockDocumentsStore) Update(ctx context.Context, d entities.Document) error {
	return m.Called(ctx, d).Error(0)
}
func (m *mockDocumentsStore) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockDocumentsStore) CountByCategory(ctx context.Context, categoryID string) (int, error) {
	args := m.Called(ctx, categoryID)
	return args.Int(0), args.Error(1)
}

type mockCategoriesStore struct{ mock.Mock }

func (m *mockCategoriesStore) List(ctx context.Context) ([]entities.Category, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.Category), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockCategoriesStore) FindByID(ctx context.Context, id string) (*entities.Category, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.Category), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockCategoriesStore) EnsureSeeded(ctx context.Context, seed []entities.Category) error {
	return m.Called(ctx, seed).Error(0)
}
func (m *mockCategoriesStore) Create(ctx context.Context, c entities.Category) error {
	return m.Called(ctx, c).Error(0)
}
func (m *mockCategoriesStore) Update(ctx context.Context, c entities.Category) error {
	return m.Called(ctx, c).Error(0)
}
func (m *mockCategoriesStore) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

type mockFoyersStore struct{ mock.Mock }

func (m *mockFoyersStore) FindByFloor(ctx context.Context, f entities.FoyerFloor) (*entities.Foyer, error) {
	args := m.Called(ctx, f)
	if v := args.Get(0); v != nil {
		return v.(*entities.Foyer), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockFoyersStore) FindByID(ctx context.Context, id string) (*entities.Foyer, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.Foyer), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockFoyersStore) Create(ctx context.Context, f entities.Foyer) error {
	return m.Called(ctx, f).Error(0)
}
func (m *mockFoyersStore) List(ctx context.Context) ([]entities.Foyer, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.Foyer), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockFoyersStore) AddMember(ctx context.Context, fid, uid string) error {
	return m.Called(ctx, fid, uid).Error(0)
}
func (m *mockFoyersStore) UpdateParts(ctx context.Context, fid string, parts int) error {
	return m.Called(ctx, fid, parts).Error(0)
}

type mockCoprosStore struct{ mock.Mock }

func (m *mockCoprosStore) GetOrCreateSingleton(ctx context.Context) (*entities.Copro, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.(*entities.Copro), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockStorage struct{ mock.Mock }

func (m *mockStorage) SignedPutURL(ctx context.Context, objectName, contentType string, sizeBytes int64, ttl time.Duration) (string, error) {
	args := m.Called(ctx, objectName, contentType, sizeBytes, ttl)
	return args.String(0), args.Error(1)
}
func (m *mockStorage) SignedGetURL(ctx context.Context, objectName string, ttl time.Duration) (string, error) {
	args := m.Called(ctx, objectName, ttl)
	return args.String(0), args.Error(1)
}
func (m *mockStorage) Head(ctx context.Context, objectName string) (interfaces.ObjectStat, bool, error) {
	args := m.Called(ctx, objectName)
	stat, _ := args.Get(0).(interfaces.ObjectStat)
	return stat, args.Bool(1), args.Error(2)
}
func (m *mockStorage) Delete(ctx context.Context, objectName string) error {
	return m.Called(ctx, objectName).Error(0)
}
func (m *mockStorage) DeletePrefix(ctx context.Context, prefix string) error {
	return m.Called(ctx, prefix).Error(0)
}
func (m *mockStorage) Read(ctx context.Context, objectName string) ([]byte, error) {
	args := m.Called(ctx, objectName)
	if v := args.Get(0); v != nil {
		return v.([]byte), args.Error(1)
	}
	return nil, args.Error(1)
}

// ─── Helpers ────────────────────────────────────────────────────────

var (
	rdc     = &entities.Foyer{ID: "rdc", Floor: entities.FoyerFloorRDC, MemberIDs: []string{"uid-rdc"}}
	premier = &entities.Foyer{ID: "1er", Floor: entities.FoyerFloor1er, MemberIDs: []string{"uid-1er"}}
	cop     = &entities.Copro{ID: "c1", TotalParts: 1000}
	now     = time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
)

func newUC() (*usecases, *mockDocumentsStore, *mockCategoriesStore, *mockFoyersStore, *mockCoprosStore, *mockStorage) {
	docs := &mockDocumentsStore{}
	cats := &mockCategoriesStore{}
	foy := &mockFoyersStore{}
	cps := &mockCoprosStore{}
	stor := &mockStorage{}
	uc := &usecases{
		logger:     zap.NewNop(),
		documents:  docs,
		categories: cats,
		foyers:     foy,
		copros:     cps,
		storage:    stor,
		now:        func() time.Time { return now },
	}
	return uc, docs, cats, foy, cps, stor
}

// ─── RequestUploadURL ──────────────────────────────────────────────

func TestRequestUploadURL(t *testing.T) {
	Convey("Given a valid declaration from a foyer member", t, func() {
		ctx := context.Background()
		uc, _, cats, foy, _, stor := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cats.On("FindByID", ctx, "syndic").Return(&entities.Category{ID: "syndic"}, nil)
		stor.On("SignedPutURL",
			ctx,
			mock.MatchedBy(func(name string) bool {
				return len(name) > len("documents/") && name[:len("documents/")] == "documents/"
			}),
			"application/pdf",
			int64(4242),
			mock.AnythingOfType("time.Duration"),
		).Return("https://signed.example/put", nil)

		out, err := uc.RequestUploadURL(ctx, RequestUploadInput{
			ActorUserID:      "uid-rdc",
			Title:            "Contrat 2026",
			CategoryID:       "syndic",
			Group:            "Contrat",
			OriginalFilename: "syndic-2026.pdf",
			ContentType:      "application/pdf",
			SizeBytes:        4242,
		})

		Convey("It returns a stable object name and signed URL", func() {
			So(err, ShouldBeNil)
			So(out.DocumentID, ShouldNotBeBlank)
			So(out.ObjectName, ShouldStartWith, "documents/")
			So(out.ObjectName, ShouldEndWith, ".pdf")
			So(out.UploadURL, ShouldEqual, "https://signed.example/put")
		})
	})

	Convey("Rejects an unsupported MIME type", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.RequestUploadURL(ctx, RequestUploadInput{
			ActorUserID: "uid-rdc",
			Title:       "x",
			CategoryID:  "syndic",
			ContentType: "application/zip",
			SizeBytes:   100,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects oversized files (>10MB)", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.RequestUploadURL(ctx, RequestUploadInput{
			ActorUserID: "uid-rdc",
			Title:       "x",
			CategoryID:  "syndic",
			ContentType: "application/pdf",
			SizeBytes:   entities.DocumentMaxSizeBytes + 1,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects a missing title", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.RequestUploadURL(ctx, RequestUploadInput{
			ActorUserID: "uid-rdc",
			Title:       "   ",
			CategoryID:  "syndic",
			ContentType: "application/pdf",
			SizeBytes:   100,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects a missing category", t, func() {
		ctx := context.Background()
		uc, _, cats, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cats.On("FindByID", ctx, "ghost").Return((*entities.Category)(nil), nil)
		_, err := uc.RequestUploadURL(ctx, RequestUploadInput{
			ActorUserID: "uid-rdc",
			Title:       "x",
			CategoryID:  "ghost",
			ContentType: "application/pdf",
			SizeBytes:   100,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects an intruder actor", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.RequestUploadURL(ctx, RequestUploadInput{
			ActorUserID: "intruder",
			Title:       "x",
			CategoryID:  "syndic",
			ContentType: "application/pdf",
			SizeBytes:   100,
		})
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})
}

// ─── Record ─────────────────────────────────────────────────────────

func TestRecord(t *testing.T) {
	Convey("Given a successful upload (HEAD matches declaration)", t, func() {
		ctx := context.Background()
		uc, docs, cats, foy, cps, stor := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cats.On("FindByID", ctx, "syndic").Return(&entities.Category{ID: "syndic"}, nil)
		cps.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		stor.On("Head", ctx, "documents/doc1.pdf").Return(
			interfaces.ObjectStat{SizeBytes: 4242, ContentType: "application/pdf"}, true, nil,
		)
		docs.On("Create", ctx, mock.AnythingOfType("entities.Document")).Return(nil)

		d, err := uc.Record(ctx, RecordDocumentInput{
			ActorUserID:      "uid-rdc",
			DocumentID:       "doc1",
			Title:            "Contrat 2026",
			CategoryID:       "syndic",
			Group:            "  Contrat  ",
			ContentType:      "application/pdf",
			SizeBytes:        4242,
			OriginalFilename: "syndic-2026.pdf",
		})
		Convey("It records the metadata with normalized group", func() {
			So(err, ShouldBeNil)
			So(d.ID, ShouldEqual, "doc1")
			So(d.ObjectName, ShouldEqual, "documents/doc1.pdf")
			So(d.UploadedBy, ShouldEqual, "uid-rdc")
			// Group lowercased + trimmed.
			So(d.Group, ShouldEqual, "contrat")
		})
	})

	Convey("When HEAD reports a size mismatch, the orphan blob is cleaned up", t, func() {
		ctx := context.Background()
		uc, docs, cats, foy, _, stor := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cats.On("FindByID", ctx, "syndic").Return(&entities.Category{ID: "syndic"}, nil)
		stor.On("Head", ctx, "documents/doc1.pdf").Return(
			interfaces.ObjectStat{SizeBytes: 9999, ContentType: "application/pdf"}, true, nil,
		)
		stor.On("Delete", ctx, "documents/doc1.pdf").Return(nil)

		_, err := uc.Record(ctx, RecordDocumentInput{
			ActorUserID: "uid-rdc",
			DocumentID:  "doc1",
			Title:       "x",
			CategoryID:  "syndic",
			ContentType: "application/pdf",
			SizeBytes:   4242,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
		docs.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		stor.AssertCalled(t, "Delete", ctx, "documents/doc1.pdf")
	})

	Convey("When the object isn't there, returns a validation error", t, func() {
		ctx := context.Background()
		uc, _, cats, foy, _, stor := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cats.On("FindByID", ctx, "syndic").Return(&entities.Category{ID: "syndic"}, nil)
		stor.On("Head", ctx, "documents/doc1.pdf").Return(interfaces.ObjectStat{}, false, nil)

		_, err := uc.Record(ctx, RecordDocumentInput{
			ActorUserID: "uid-rdc",
			DocumentID:  "doc1",
			Title:       "x",
			CategoryID:  "syndic",
			ContentType: "application/pdf",
			SizeBytes:   100,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})
}

// ─── Update ─────────────────────────────────────────────────────────

func TestUpdate(t *testing.T) {
	Convey("Given an existing document", t, func() {
		ctx := context.Background()
		uc, docs, cats, foy, _, _ := newUC()
		existing := &entities.Document{
			ID:          "doc1",
			CoproID:     "c1",
			CategoryID:  "syndic",
			Title:       "Contrat 2026",
			Group:       "contrat",
			ObjectName:  "documents/doc1.pdf",
			ContentType: "application/pdf",
			SizeBytes:   4242,
			UploadedAt:  now,
		}
		docs.On("FindByID", ctx, "doc1").Return(existing, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cats.On("FindByID", ctx, "syndic").Return(&entities.Category{ID: "syndic"}, nil)
		docs.On("Update", ctx, mock.AnythingOfType("entities.Document")).Return(nil)

		out, err := uc.Update(ctx, "doc1", UpdateDocumentInput{
			ActorUserID: "uid-rdc",
			Title:       "Contrat 2027 reconduit",
			Description: "Avenant signé le 1er mai",
			CategoryID:  "syndic",
			Group:       "AVENANT", // capitals — should normalize
		})
		Convey("It edits metadata + normalizes the group", func() {
			So(err, ShouldBeNil)
			So(out.Title, ShouldEqual, "Contrat 2027 reconduit")
			So(out.Group, ShouldEqual, "avenant")
		})
	})

	Convey("Returns ErrNotFound for ghost id", t, func() {
		ctx := context.Background()
		uc, docs, _, foy, _, _ := newUC()
		docs.On("FindByID", ctx, "ghost").Return((*entities.Document)(nil), nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.Update(ctx, "ghost", UpdateDocumentInput{
			ActorUserID: "uid-rdc",
			Title:       "x",
			CategoryID:  "syndic",
		})
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}

// ─── Delete ─────────────────────────────────────────────────────────

func TestDelete(t *testing.T) {
	Convey("Drops both metadata and the GCS object", t, func() {
		ctx := context.Background()
		uc, docs, _, foy, _, stor := newUC()
		docs.On("FindByID", ctx, "doc1").Return(&entities.Document{
			ID:         "doc1",
			ObjectName: "documents/doc1.pdf",
		}, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		stor.On("Delete", ctx, "documents/doc1.pdf").Return(nil)
		docs.On("Delete", ctx, "doc1").Return(nil)

		err := uc.Delete(ctx, "doc1", "uid-rdc")
		So(err, ShouldBeNil)
		stor.AssertCalled(t, "Delete", ctx, "documents/doc1.pdf")
		docs.AssertCalled(t, "Delete", ctx, "doc1")
	})

	Convey("Returns ErrNotFound for ghost id", t, func() {
		ctx := context.Background()
		uc, docs, _, foy, _, _ := newUC()
		docs.On("FindByID", ctx, "ghost").Return((*entities.Document)(nil), nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		err := uc.Delete(ctx, "ghost", "uid-rdc")
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})

	Convey("Rejects an intruder actor", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		err := uc.Delete(ctx, "doc1", "intruder")
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})
}

// ─── GetDownloadURL ────────────────────────────────────────────────

func TestGetDownloadURL(t *testing.T) {
	Convey("Returns a signed URL when the doc exists", t, func() {
		ctx := context.Background()
		uc, docs, _, foy, _, stor := newUC()
		docs.On("FindByID", ctx, "doc1").Return(&entities.Document{
			ID: "doc1", ObjectName: "documents/doc1.pdf",
		}, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		stor.On("SignedGetURL", ctx, "documents/doc1.pdf", mock.AnythingOfType("time.Duration")).
			Return("https://signed.example/get", nil)

		url, _, err := uc.GetDownloadURL(ctx, "doc1", "uid-rdc")
		So(err, ShouldBeNil)
		So(url, ShouldEqual, "https://signed.example/get")
	})

	Convey("Returns ErrNotFound for ghost id", t, func() {
		ctx := context.Background()
		uc, docs, _, foy, _, _ := newUC()
		docs.On("FindByID", ctx, "ghost").Return((*entities.Document)(nil), nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, _, err := uc.GetDownloadURL(ctx, "ghost", "uid-rdc")
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}
