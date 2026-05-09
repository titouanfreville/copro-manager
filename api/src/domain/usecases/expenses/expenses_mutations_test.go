package expenses

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

type mockExpensesStore struct{ mock.Mock }

func (m *mockExpensesStore) List(ctx context.Context) ([]entities.Expense, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.Expense), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockExpensesStore) FindByID(ctx context.Context, id string) (*entities.Expense, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.Expense), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockExpensesStore) FindByNameAndDate(ctx context.Context, name string, date time.Time) (*entities.Expense, error) {
	args := m.Called(ctx, name, date)
	if v := args.Get(0); v != nil {
		return v.(*entities.Expense), args.Error(1)
	}
	return nil, args.Error(1)
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
func (m *mockExpensesStore) CountByCategory(ctx context.Context, categoryID string) (int, error) {
	args := m.Called(ctx, categoryID)
	return args.Int(0), args.Error(1)
}

type mockAttachmentsStore struct{ mock.Mock }

func (m *mockAttachmentsStore) List(ctx context.Context, expenseID string) ([]entities.Attachment, error) {
	args := m.Called(ctx, expenseID)
	if v := args.Get(0); v != nil {
		return v.([]entities.Attachment), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAttachmentsStore) FindByID(ctx context.Context, expenseID, attachmentID string) (*entities.Attachment, error) {
	args := m.Called(ctx, expenseID, attachmentID)
	if v := args.Get(0); v != nil {
		return v.(*entities.Attachment), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAttachmentsStore) Count(ctx context.Context, expenseID string) (int, error) {
	args := m.Called(ctx, expenseID)
	return args.Int(0), args.Error(1)
}

func (m *mockAttachmentsStore) CreateIfUnderCap(ctx context.Context, expenseID string, att entities.Attachment, cap int) error {
	return m.Called(ctx, expenseID, att, cap).Error(0)
}

func (m *mockAttachmentsStore) Delete(ctx context.Context, expenseID, attachmentID string) error {
	return m.Called(ctx, expenseID, attachmentID).Error(0)
}

func (m *mockAttachmentsStore) DeleteAll(ctx context.Context, expenseID string) error {
	return m.Called(ctx, expenseID).Error(0)
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

// ─── Tests ──────────────────────────────────────────────────────────

func newUC() (*usecases, *mockExpensesStore, *mockFoyersStore, *mockCoprosStore, *mockCategoriesStore, *mockStorage, *mockAttachmentsStore) {
	exp := &mockExpensesStore{}
	atts := &mockAttachmentsStore{}
	foy := &mockFoyersStore{}
	cop := &mockCoprosStore{}
	cat := &mockCategoriesStore{}
	stor := &mockStorage{}
	uc := &usecases{
		logger:      zap.NewNop(),
		expenses:    exp,
		attachments: atts,
		foyers:      foy,
		copros:      cop,
		categories:  cat,
		storage:     stor,
		now:         func() time.Time { return time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC) },
	}
	return uc, exp, foy, cop, cat, stor, atts
}

var (
	rdc = &entities.Foyer{ID: "rdc", Floor: entities.FoyerFloorRDC, Parts: 500, MemberIDs: []string{"uid-rdc"}}
	one = &entities.Foyer{ID: "1er", Floor: entities.FoyerFloor1er, Parts: 500, MemberIDs: []string{"uid-1er"}}
	cop = &entities.Copro{ID: "c1", TotalParts: 1000}
)

func TestUpdate(t *testing.T) {
	Convey("Given an existing expense", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, copStore, catStore, _, _ := newUC()

		existing := &entities.Expense{
			ID:               "e1",
			CoproID:          "c1",
			Name:             "Eau été",
			AmountCents:      10000,
			Currency:         "EUR",
			Date:             time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
			PayerFoyerID:     "rdc",
			CategoryID:       "eau",
			DistributionMode: entities.DistributionModeEqual,
			ShareRDCCents:    5000,
			Share1erCents:    5000,
			CreatedAt:        time.Date(2025, 7, 2, 0, 0, 0, 0, time.UTC),
			UpdatedAt:        time.Date(2025, 7, 2, 0, 0, 0, 0, time.UTC),
		}

		expStore.On("FindByID", ctx, "e1").Return(existing, nil)
		catStore.On("FindByID", ctx, "eau").Return(&entities.Category{ID: "eau"}, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		copStore.On("GetOrCreateSingleton", ctx).Return(cop, nil)

		Convey("When the actor is a foyer member and inputs are valid", func() {
			expStore.On("Update", ctx, mock.AnythingOfType("entities.Expense")).Return(nil)

			updated, err := uc.Update(ctx, "e1", CreateInput{
				ActorUserID:      "uid-rdc",
				Name:             "Eau été révisée",
				AmountCents:      12000,
				Date:             existing.Date,
				PayerFoyerID:     "1er",
				CategoryID:       "eau",
				DistributionMode: entities.DistributionModeEqual,
			})

			Convey("It writes the new fields and refreshes UpdatedAt", func() {
				So(err, ShouldBeNil)
				So(updated.Name, ShouldEqual, "Eau été révisée")
				So(updated.AmountCents, ShouldEqual, 12000)
				So(updated.PayerFoyerID, ShouldEqual, "1er")
				So(updated.ShareRDCCents+updated.Share1erCents, ShouldEqual, 12000)
				So(updated.UpdatedAt, ShouldHappenAfter, existing.CreatedAt)
				// Identity preserved.
				So(updated.ID, ShouldEqual, "e1")
				So(updated.CreatedAt, ShouldEqual, existing.CreatedAt)
			})
		})

		Convey("When the actor is not a member of either foyer", func() {
			_, err := uc.Update(ctx, "e1", CreateInput{
				ActorUserID:      "intruder",
				Name:             "Eau été",
				AmountCents:      10000,
				Date:             existing.Date,
				PayerFoyerID:     "rdc",
				CategoryID:       "eau",
				DistributionMode: entities.DistributionModeEqual,
			})

			Convey("It returns an authorization error", func() {
				So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
				expStore.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
			})
		})
	})

	Convey("Given a missing expense id", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, _, _ := newUC()
		// Authorization runs first now (defense-in-depth against probing).
		// Foyer mocks are needed even for the not-found path because
		// loadFoyers is called before FindByID.
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "ghost").Return((*entities.Expense)(nil), nil)

		_, err := uc.Update(ctx, "ghost", CreateInput{
			Name:             "x",
			AmountCents:      100,
			Date:             time.Now(),
			PayerFoyerID:     "rdc",
			CategoryID:       "eau",
			DistributionMode: entities.DistributionModeEqual,
		})
		Convey("It returns ErrNotFound", func() {
			So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
		})
	})
}

func TestCreatePending(t *testing.T) {
	Convey("Given a pending expense (amount=0, amount_pending=true)", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, copStore, catStore, _, _ := newUC()

		expStore.On("Create", ctx, mock.AnythingOfType("entities.Expense")).Return(nil)
		catStore.On("FindByID", ctx, "eau").Return(&entities.Category{ID: "eau"}, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		copStore.On("GetOrCreateSingleton", ctx).Return(cop, nil)

		out, err := uc.Create(ctx, CreateInput{
			ActorUserID:      "uid-rdc",
			Name:             "Eau (à compléter)",
			AmountCents:      0,
			Date:             time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
			PayerFoyerID:     "rdc",
			CategoryID:       "eau",
			DistributionMode: entities.DistributionModeEqual,
			AmountPending:    true,
			TemplateID:       "tpl-water",
		})

		Convey("It stores the row with shares 0/0 and the template lineage", func() {
			So(err, ShouldBeNil)
			So(out.AmountPending, ShouldBeTrue)
			So(out.AmountCents, ShouldEqual, 0)
			So(out.ShareRDCCents, ShouldEqual, 0)
			So(out.Share1erCents, ShouldEqual, 0)
			So(out.TemplateID, ShouldEqual, "tpl-water")
		})
	})

	Convey("Rejects amount > 0 when amount_pending is true", t, func() {
		ctx := context.Background()
		uc, _, _, _, _, _, _ := newUC()
		_, err := uc.Create(ctx, CreateInput{
			ActorUserID:      "uid-rdc",
			Name:             "Eau",
			AmountCents:      5000,
			Date:             time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
			PayerFoyerID:     "rdc",
			CategoryID:       "eau",
			DistributionMode: entities.DistributionModeEqual,
			AmountPending:    true,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})
}

func TestUpdateClearsPending(t *testing.T) {
	Convey("Given a pending expense being updated with a real amount", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, copStore, catStore, _, _ := newUC()
		existing := &entities.Expense{
			ID:               "e1",
			CoproID:          "c1",
			Name:             "Eau",
			AmountCents:      0,
			Currency:         "EUR",
			Date:             time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			PayerFoyerID:     "rdc",
			CategoryID:       "eau",
			DistributionMode: entities.DistributionModeEqual,
			AmountPending:    true,
			TemplateID:       "tpl-water",
			CreatedAt:        time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		}
		expStore.On("FindByID", ctx, "e1").Return(existing, nil)
		catStore.On("FindByID", ctx, "eau").Return(&entities.Category{ID: "eau"}, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		copStore.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		expStore.On("Update", ctx, mock.AnythingOfType("entities.Expense")).Return(nil)

		out, err := uc.Update(ctx, "e1", CreateInput{
			ActorUserID:      "uid-rdc",
			Name:             "Eau",
			AmountCents:      8400,
			Date:             existing.Date,
			PayerFoyerID:     "rdc",
			CategoryID:       "eau",
			DistributionMode: entities.DistributionModeEqual,
			// Caller no longer flags pending; shares should be recomputed.
			AmountPending: false,
		})

		Convey("Pending clears and shares recompute from the new amount", func() {
			So(err, ShouldBeNil)
			So(out.AmountPending, ShouldBeFalse)
			So(out.AmountCents, ShouldEqual, 8400)
			So(out.ShareRDCCents+out.Share1erCents, ShouldEqual, 8400)
			// TemplateID is preserved across the pending → final transition.
			So(out.TemplateID, ShouldEqual, "tpl-water")
		})
	})
}

func TestDelete(t *testing.T) {
	Convey("Given an existing expense and an authorized actor", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, stor, attsStore := newUC()

		expStore.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1"}, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		attsStore.On("DeleteAll", ctx, "e1").Return(nil)
		expStore.On("Delete", ctx, "e1").Return(nil)
		stor.On("DeletePrefix", ctx, "expenses/e1/").Return(nil)

		err := uc.Delete(ctx, "e1", "uid-1er")
		Convey("Delete is called and the cascade fires (subcoll + GCS prefix)", func() {
			So(err, ShouldBeNil)
			expStore.AssertCalled(t, "Delete", ctx, "e1")
			attsStore.AssertCalled(t, "DeleteAll", ctx, "e1")
			stor.AssertCalled(t, "DeletePrefix", ctx, "expenses/e1/")
		})
	})

	Convey("Given a missing expense (with valid actor)", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, _, _ := newUC()
		// Authorization runs first, so foyer mocks are required even for the
		// not-found path.
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "ghost").Return((*entities.Expense)(nil), nil)

		err := uc.Delete(ctx, "ghost", "uid-rdc")
		Convey("It returns ErrNotFound and never deletes", func() {
			So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
			expStore.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
		})
	})

	Convey("Given an actor outside the copro", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, stor, _ := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)

		err := uc.Delete(ctx, "e1", "intruder")
		Convey("It returns an authorization error and never touches the expense", func() {
			So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
			// Auth check runs before FindByID now — the expense store is
			// never even consulted.
			expStore.AssertNotCalled(t, "FindByID", mock.Anything, mock.Anything)
			expStore.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
			stor.AssertNotCalled(t, "DeletePrefix", mock.Anything, mock.Anything)
		})
	})
}
