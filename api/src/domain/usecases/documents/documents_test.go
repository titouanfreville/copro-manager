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
func (m *mockDocumentsStore) CountByLinkedExpense(ctx context.Context, expenseID string) (int, error) {
	args := m.Called(ctx, expenseID)
	return args.Int(0), args.Error(1)
}
func (m *mockDocumentsStore) ListByLinkedExpense(ctx context.Context, expenseID string) ([]entities.Document, error) {
	args := m.Called(ctx, expenseID)
	if v := args.Get(0); v != nil {
		return v.([]entities.Document), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockDocumentsStore) CountByLinkedContract(ctx context.Context, contractID string) (int, error) {
	args := m.Called(ctx, contractID)
	return args.Int(0), args.Error(1)
}
func (m *mockDocumentsStore) SetAnalysis(ctx context.Context, id string, analysis *entities.DocumentAnalysis) error {
	return m.Called(ctx, id, analysis).Error(0)
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
	return nil, m.Called(ctx, id).Error(1)
}
func (m *mockFoyersStore) Create(ctx context.Context, f entities.Foyer) error {
	return m.Called(ctx, f).Error(0)
}
func (m *mockFoyersStore) List(ctx context.Context) ([]entities.Foyer, error) {
	return nil, m.Called(ctx).Error(1)
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

type mockExpensesStore struct{ mock.Mock }

func (m *mockExpensesStore) List(ctx context.Context) ([]entities.Expense, error) {
	return nil, m.Called(ctx).Error(1)
}
func (m *mockExpensesStore) FindByID(ctx context.Context, id string) (*entities.Expense, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.Expense), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockExpensesStore) FindByNameAndDate(ctx context.Context, n string, d time.Time) (*entities.Expense, error) {
	return nil, m.Called(ctx, n, d).Error(1)
}
func (m *mockExpensesStore) Create(ctx context.Context, e entities.Expense) error {
	return m.Called(ctx, e).Error(0)
}
func (m *mockExpensesStore) Update(ctx context.Context, e entities.Expense) error {
	return m.Called(ctx, e).Error(0)
}
func (m *mockExpensesStore) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockExpensesStore) CountByCategory(ctx context.Context, id string) (int, error) {
	return m.Called(ctx, id).Int(0), m.Called(ctx, id).Error(1)
}
func (m *mockExpensesStore) CountByMeterReadingPeriod(ctx context.Context, p string) (int, error) {
	return m.Called(ctx, p).Int(0), m.Called(ctx, p).Error(1)
}

type mockStorage struct{ mock.Mock }

func (m *mockStorage) SignedPutURL(ctx context.Context, name, contentType string, size int64, ttl time.Duration) (string, error) {
	args := m.Called(ctx, name, contentType, size, ttl)
	return args.String(0), args.Error(1)
}
func (m *mockStorage) SignedGetURL(ctx context.Context, name string, ttl time.Duration) (string, error) {
	args := m.Called(ctx, name, ttl)
	return args.String(0), args.Error(1)
}
func (m *mockStorage) Head(ctx context.Context, name string) (interfaces.ObjectStat, bool, error) {
	args := m.Called(ctx, name)
	return args.Get(0).(interfaces.ObjectStat), args.Bool(1), args.Error(2)
}
func (m *mockStorage) Delete(ctx context.Context, name string) error {
	return m.Called(ctx, name).Error(0)
}
func (m *mockStorage) DeletePrefix(ctx context.Context, prefix string) error {
	return m.Called(ctx, prefix).Error(0)
}
func (m *mockStorage) Read(ctx context.Context, name string) ([]byte, error) {
	args := m.Called(ctx, name)
	if v := args.Get(0); v != nil {
		return v.([]byte), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockValidator struct{ mock.Mock }

func (m *mockValidator) ValidateUpload(ctx context.Context, d entities.DocumentDraft) error {
	return m.Called(ctx, d).Error(0)
}
func (m *mockValidator) ValidateUpdate(ctx context.Context, d entities.DocumentMetadataDraft) error {
	return m.Called(ctx, d).Error(0)
}

type mockAnalyzer struct{ mock.Mock }

func (m *mockAnalyzer) AnalyzeDocument(ctx context.Context, image []byte, mimeType string) (*entities.DocumentAnalysis, error) {
	args := m.Called(ctx, image, mimeType)
	if v := args.Get(0); v != nil {
		return v.(*entities.DocumentAnalysis), args.Error(1)
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

func newUC() (*usecases, *mockDocumentsStore, *mockFoyersStore, *mockCoprosStore, *mockExpensesStore, *mockStorage, *mockValidator) {
	docs := &mockDocumentsStore{}
	foy := &mockFoyersStore{}
	cps := &mockCoprosStore{}
	exp := &mockExpensesStore{}
	stor := &mockStorage{}
	val := &mockValidator{}
	clock := func() time.Time { return now }
	uc := &usecases{
		logger:    zap.NewNop(),
		documents: docs,
		foyers:    foy,
		storage:   stor,
		validator: val,
		builder:   newBuilder(cps, exp, clock),
		now:       clock,
	}
	return uc, docs, foy, cps, exp, stor, val
}

// newUCWithAnalyzer is a variant of newUC that wires a mock analyzer
// into the usecase so the Analyze() path can be exercised. The other
// dependencies are returned for the rare test that needs to set
// authorization or storage expectations alongside the analyzer.
func newUCWithAnalyzer() (*usecases, *mockDocumentsStore, *mockFoyersStore, *mockStorage, *mockAnalyzer) {
	uc, docs, foy, _, _, stor, _ := newUC()
	an := &mockAnalyzer{}
	uc.analyzer = an
	return uc, docs, foy, stor, an
}

// ─── RequestUploadURL ──────────────────────────────────────────────

func TestRequestUploadURL(t *testing.T) {
	Convey("Mints a signed URL when validation passes", t, func() {
		ctx := context.Background()
		uc, _, foy, _, _, stor, val := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		val.On("ValidateUpload", ctx, mock.AnythingOfType("entities.DocumentDraft")).Return(nil)
		stor.On("SignedPutURL",
			ctx,
			mock.MatchedBy(func(name string) bool { return len(name) > 0 }),
			"image/jpeg",
			int64(2048),
			documentURLTTL,
		).Return("https://signed/put", nil)

		out, err := uc.RequestUploadURL(ctx, RequestUploadInput{
			ActorUserID: "uid-rdc",
			DocumentDraft: entities.DocumentDraft{
				Title:       "Bill",
				CategoryID:  "syndic",
				ContentType: "image/jpeg",
				SizeBytes:   2048,
			},
		})
		So(err, ShouldBeNil)
		So(out.UploadURL, ShouldEqual, "https://signed/put")
		So(out.DocumentID, ShouldNotBeBlank)
		So(out.ContentType, ShouldEqual, "image/jpeg")
	})

	Convey("Surfaces validator errors verbatim", t, func() {
		ctx := context.Background()
		uc, _, foy, _, _, _, val := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		val.On("ValidateUpload", ctx, mock.AnythingOfType("entities.DocumentDraft")).
			Return(entities.ValidationError{Key: "title", Message: "required"})

		_, err := uc.RequestUploadURL(ctx, RequestUploadInput{
			ActorUserID: "uid-rdc",
			DocumentDraft: entities.DocumentDraft{
				CategoryID:  "syndic",
				ContentType: "image/jpeg",
				SizeBytes:   2048,
			},
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Pulls linked-expense defaults before validating", t, func() {
		ctx := context.Background()
		uc, _, foy, _, exp, stor, val := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		exp.On("FindByID", ctx, "exp-1").Return(&entities.Expense{ID: "exp-1", Name: "Plombier", CategoryID: "travaux"}, nil)
		val.On("ValidateUpload", ctx, mock.MatchedBy(func(d entities.DocumentDraft) bool {
			return d.Title == "Plombier" && d.CategoryID == "travaux"
		})).Return(nil)
		stor.On("SignedPutURL", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return("https://signed/put", nil)

		_, err := uc.RequestUploadURL(ctx, RequestUploadInput{
			ActorUserID: "uid-rdc",
			DocumentDraft: entities.DocumentDraft{
				ContentType:     "image/jpeg",
				SizeBytes:       2048,
				LinkedExpenseID: "exp-1",
			},
		})
		So(err, ShouldBeNil)
	})

	Convey("Refuses unauthenticated foreign actor", t, func() {
		ctx := context.Background()
		uc, _, foy, _, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)

		_, err := uc.RequestUploadURL(ctx, RequestUploadInput{ActorUserID: "stranger"})
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})
}

// ─── Record ─────────────────────────────────────────────────────────

func TestRecord(t *testing.T) {
	Convey("Persists when GCS HEAD matches the declaration", t, func() {
		ctx := context.Background()
		uc, docs, foy, cps, _, stor, val := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		val.On("ValidateUpload", ctx, mock.AnythingOfType("entities.DocumentDraft")).Return(nil)
		cps.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		stor.On("Head", ctx, mock.AnythingOfType("string")).
			Return(interfaces.ObjectStat{ContentType: "image/jpeg", SizeBytes: 2048}, true, nil)
		docs.On("Create", ctx, mock.AnythingOfType("entities.Document")).Return(nil)

		out, err := uc.Record(ctx, RecordDocumentInput{
			ActorUserID: "uid-rdc",
			DocumentID:  "doc-abc",
			DocumentDraft: entities.DocumentDraft{
				Title:       "Bill",
				CategoryID:  "syndic",
				ContentType: "image/jpeg",
				SizeBytes:   2048,
			},
		})
		So(err, ShouldBeNil)
		So(out.ID, ShouldEqual, "doc-abc")
		So(out.CoproID, ShouldEqual, "c1")
	})

	Convey("Rejects on HEAD mismatch and cleans the orphan blob", t, func() {
		ctx := context.Background()
		uc, _, foy, _, _, stor, val := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		val.On("ValidateUpload", ctx, mock.AnythingOfType("entities.DocumentDraft")).Return(nil)
		stor.On("Head", ctx, mock.AnythingOfType("string")).
			Return(interfaces.ObjectStat{ContentType: "image/png", SizeBytes: 4096}, true, nil)
		stor.On("Delete", ctx, mock.AnythingOfType("string")).Return(nil)

		_, err := uc.Record(ctx, RecordDocumentInput{
			ActorUserID: "uid-rdc",
			DocumentID:  "doc-abc",
			DocumentDraft: entities.DocumentDraft{
				Title:       "Bill",
				CategoryID:  "syndic",
				ContentType: "image/jpeg",
				SizeBytes:   2048,
			},
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
		stor.AssertCalled(t, "Delete", ctx, mock.AnythingOfType("string"))
	})
}

// ─── Update ─────────────────────────────────────────────────────────

func TestUpdate(t *testing.T) {
	Convey("Patches metadata fields onto the existing doc", t, func() {
		ctx := context.Background()
		uc, docs, foy, _, _, _, val := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		docs.On("FindByID", ctx, "doc-abc").Return(&entities.Document{ID: "doc-abc", CoproID: "c1", ObjectName: "documents/doc-abc.jpg"}, nil)
		val.On("ValidateUpdate", ctx, mock.AnythingOfType("entities.DocumentMetadataDraft")).Return(nil)
		docs.On("Update", ctx, mock.AnythingOfType("entities.Document")).Return(nil)

		out, err := uc.Update(ctx, "doc-abc", UpdateDocumentInput{
			ActorUserID: "uid-rdc",
			DocumentMetadataDraft: entities.DocumentMetadataDraft{
				Title:      "Renamed",
				CategoryID: "syndic",
			},
		})
		So(err, ShouldBeNil)
		So(out.Title, ShouldEqual, "Renamed")
		So(out.ObjectName, ShouldEqual, "documents/doc-abc.jpg") // preserved
	})

	Convey("Returns ErrNotFound for a ghost id", t, func() {
		ctx := context.Background()
		uc, docs, foy, _, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		docs.On("FindByID", ctx, "ghost").Return((*entities.Document)(nil), nil)

		_, err := uc.Update(ctx, "ghost", UpdateDocumentInput{ActorUserID: "uid-rdc"})
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}

// ─── Delete ─────────────────────────────────────────────────────────

func TestDelete(t *testing.T) {
	Convey("Deletes the GCS blob and the metadata", t, func() {
		ctx := context.Background()
		uc, docs, foy, _, _, stor, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		docs.On("FindByID", ctx, "doc-abc").Return(&entities.Document{ID: "doc-abc", ObjectName: "documents/doc-abc.jpg"}, nil)
		stor.On("Delete", ctx, "documents/doc-abc.jpg").Return(nil)
		docs.On("Delete", ctx, "doc-abc").Return(nil)

		err := uc.Delete(ctx, "doc-abc", "uid-rdc")
		So(err, ShouldBeNil)
	})
}

// ─── Analyze ────────────────────────────────────────────────────────

func TestAnalyze(t *testing.T) {
	ctx := context.Background()
	docID := "abcd1234-doc-id"

	existingFresh := func() *entities.Document {
		return &entities.Document{
			ID:          docID,
			ObjectName:  "documents/" + docID + ".jpg",
			ContentType: "image/jpeg",
		}
	}

	expenseVerdict := &entities.DocumentAnalysis{
		Kind:       entities.DocumentKindExpense,
		Confidence: 0.92,
		Model:      "gemini-2.5-flash",
		Expense: &entities.ExpenseExtraction{
			AmountEUR: 127.50,
			Date:      "2026-03-15",
			Vendor:    "EDF",
		},
	}

	Convey("First analysis: reads bytes, calls analyzer, persists, returns enriched doc", t, func() {
		uc, docs, foy, stor, an := newUCWithAnalyzer()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		docs.On("FindByID", ctx, docID).Return(existingFresh(), nil)
		stor.On("Read", ctx, "documents/"+docID+".jpg").Return([]byte("jpeg-bytes"), nil)
		an.On("AnalyzeDocument", ctx, []byte("jpeg-bytes"), "image/jpeg").Return(expenseVerdict, nil)
		docs.On("SetAnalysis", ctx, docID, mock.MatchedBy(func(a *entities.DocumentAnalysis) bool {
			return a != nil && a.Kind == entities.DocumentKindExpense
		})).Return(nil)

		out, err := uc.Analyze(ctx, docID, "uid-rdc", false)
		So(err, ShouldBeNil)
		So(out.Analysis.Kind, ShouldEqual, entities.DocumentKindExpense)
		So(out.Analysis.Expense.AmountEUR, ShouldEqual, 127.50)
		// Catch refactor regressions where the impl drops calls but
		// the test still passes because On(...) registrations silently
		// no-op (BH-MED-8).
		an.AssertExpectations(t)
		docs.AssertExpectations(t)
		stor.AssertExpectations(t)
	})

	Convey("Cached analysis: returns existing without calling analyzer or storage", t, func() {
		uc, docs, foy, stor, an := newUCWithAnalyzer()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cached := existingFresh()
		cached.Analysis = expenseVerdict
		docs.On("FindByID", ctx, docID).Return(cached, nil)

		out, err := uc.Analyze(ctx, docID, "uid-rdc", false)
		So(err, ShouldBeNil)
		So(out.Analysis, ShouldEqual, expenseVerdict)
		an.AssertNotCalled(t, "AnalyzeDocument", mock.Anything, mock.Anything, mock.Anything)
		stor.AssertNotCalled(t, "Read", mock.Anything, mock.Anything)
		// Cache hit must NOT re-persist the verdict — a regression
		// that does would write to Firestore on every fetch.
		docs.AssertNotCalled(t, "SetAnalysis", mock.Anything, mock.Anything, mock.Anything)
	})

	Convey("force=true bypasses cache and re-analyzes", t, func() {
		uc, docs, foy, stor, an := newUCWithAnalyzer()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cached := existingFresh()
		cached.Analysis = expenseVerdict
		docs.On("FindByID", ctx, docID).Return(cached, nil)
		stor.On("Read", ctx, "documents/"+docID+".jpg").Return([]byte("jpeg-bytes"), nil)
		newVerdict := &entities.DocumentAnalysis{Kind: entities.DocumentKindOther, Confidence: 0.4, Reason: "AG minutes"}
		an.On("AnalyzeDocument", ctx, []byte("jpeg-bytes"), "image/jpeg").Return(newVerdict, nil)
		docs.On("SetAnalysis", ctx, docID, mock.AnythingOfType("*entities.DocumentAnalysis")).Return(nil)

		out, err := uc.Analyze(ctx, docID, "uid-rdc", true)
		So(err, ShouldBeNil)
		So(out.Analysis.Kind, ShouldEqual, entities.DocumentKindOther)
		an.AssertExpectations(t)
		docs.AssertExpectations(t)
		stor.AssertExpectations(t)
	})

	Convey("Analyzer nil → ErrFeatureDisabled (only after authz passes)", t, func() {
		uc, _, foy, _, _, _, _ := newUC() // analyzer left nil
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.Analyze(ctx, docID, "uid-rdc", false)
		So(errors.Is(err, domainerrors.ErrFeatureDisabled), ShouldBeTrue)
	})

	Convey("Unauthenticated caller is rejected before feature-state is exposed", t, func() {
		uc, _, foy, _, _, _, _ := newUC()
		// Empty actor → loadBothFoyers still happens, but the lookup
		// inside RequireFoyerMember returns AuthorizationError before
		// any analyzer/storage/id checks would leak posture.
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.Analyze(ctx, docID, "stranger-uid", false)
		So(err, ShouldNotBeNil)
		// The first gate is authorization; feature-disabled never fires.
		So(errors.Is(err, domainerrors.ErrFeatureDisabled), ShouldBeFalse)
	})

	Convey("Document not found → ErrNotFound", t, func() {
		uc, docs, foy, _, _ := newUCWithAnalyzer()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		docs.On("FindByID", ctx, "ghost").Return((*entities.Document)(nil), nil)

		_, err := uc.Analyze(ctx, "ghost", "uid-rdc", false)
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})

	Convey("Analyzer error surfaces (not silently swallowed)", t, func() {
		uc, docs, foy, stor, an := newUCWithAnalyzer()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		docs.On("FindByID", ctx, docID).Return(existingFresh(), nil)
		stor.On("Read", ctx, "documents/"+docID+".jpg").Return([]byte("jpeg-bytes"), nil)
		an.On("AnalyzeDocument", ctx, []byte("jpeg-bytes"), "image/jpeg").
			Return((*entities.DocumentAnalysis)(nil), domainerrors.ErrFeatureCapped)

		_, err := uc.Analyze(ctx, docID, "uid-rdc", false)
		So(errors.Is(err, domainerrors.ErrFeatureCapped), ShouldBeTrue)
	})

	Convey("Storage read failure short-circuits with a wrapped error", t, func() {
		uc, docs, foy, stor, an := newUCWithAnalyzer()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		docs.On("FindByID", ctx, docID).Return(existingFresh(), nil)
		stor.On("Read", ctx, "documents/"+docID+".jpg").Return(([]byte)(nil), errors.New("gcs boom"))

		_, err := uc.Analyze(ctx, docID, "uid-rdc", false)
		So(err, ShouldNotBeNil)
		an.AssertNotCalled(t, "AnalyzeDocument", mock.Anything, mock.Anything, mock.Anything)
	})
}
