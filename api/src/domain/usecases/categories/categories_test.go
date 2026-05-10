package categories

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
)

// ─── Mocks ──────────────────────────────────────────────────────────

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
func (m *mockCategoriesStore) Create(ctx context.Context, c entities.Category) error {
	return m.Called(ctx, c).Error(0)
}
func (m *mockCategoriesStore) Update(ctx context.Context, c entities.Category) error {
	return m.Called(ctx, c).Error(0)
}
func (m *mockCategoriesStore) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockCategoriesStore) EnsureSeeded(ctx context.Context, seed []entities.Category) error {
	return m.Called(ctx, seed).Error(0)
}

type mockExpensesStore struct{ mock.Mock }

func (m *mockExpensesStore) List(ctx context.Context) ([]entities.Expense, error) {
	return nil, m.Called(ctx).Error(1)
}
func (m *mockExpensesStore) FindByID(ctx context.Context, id string) (*entities.Expense, error) {
	return nil, m.Called(ctx, id).Error(1)
}
func (m *mockExpensesStore) FindByNameAndDate(ctx context.Context, name string, date time.Time) (*entities.Expense, error) {
	return nil, m.Called(ctx, name, date).Error(1)
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
func (m *mockExpensesStore) CountByMeterReadingPeriod(ctx context.Context, period string) (int, error) {
	args := m.Called(ctx, period)
	return args.Int(0), args.Error(1)
}

type mockTemplatesStore struct{ mock.Mock }

func (m *mockTemplatesStore) List(ctx context.Context) ([]entities.ExpenseTemplate, error) {
	return nil, m.Called(ctx).Error(1)
}
func (m *mockTemplatesStore) FindByID(ctx context.Context, id string) (*entities.ExpenseTemplate, error) {
	return nil, m.Called(ctx, id).Error(1)
}
func (m *mockTemplatesStore) Create(ctx context.Context, t entities.ExpenseTemplate) error {
	return m.Called(ctx, t).Error(0)
}
func (m *mockTemplatesStore) Update(ctx context.Context, t entities.ExpenseTemplate) error {
	return m.Called(ctx, t).Error(0)
}
func (m *mockTemplatesStore) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockTemplatesStore) ListDue(ctx context.Context, cutoff time.Time) ([]entities.ExpenseTemplate, error) {
	return nil, m.Called(ctx, cutoff).Error(1)
}
func (m *mockTemplatesStore) CountByCategory(ctx context.Context, categoryID string) (int, error) {
	args := m.Called(ctx, categoryID)
	return args.Int(0), args.Error(1)
}

type mockDocumentsStore struct{ mock.Mock }

func (m *mockDocumentsStore) List(ctx context.Context) ([]entities.Document, error) {
	return nil, m.Called(ctx).Error(1)
}
func (m *mockDocumentsStore) FindByID(ctx context.Context, id string) (*entities.Document, error) {
	return nil, m.Called(ctx, id).Error(1)
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

// ─── Helpers ────────────────────────────────────────────────────────

var (
	rdc     = &entities.Foyer{ID: "rdc", Floor: entities.FoyerFloorRDC, MemberIDs: []string{"uid-rdc"}}
	premier = &entities.Foyer{ID: "1er", Floor: entities.FoyerFloor1er, MemberIDs: []string{"uid-1er"}}
)

func newUC() (*usecases, *mockCategoriesStore, *mockExpensesStore, *mockTemplatesStore, *mockDocumentsStore, *mockFoyersStore) {
	cat := &mockCategoriesStore{}
	exp := &mockExpensesStore{}
	tpl := &mockTemplatesStore{}
	doc := &mockDocumentsStore{}
	foy := &mockFoyersStore{}
	uc := &usecases{
		logger:    zap.NewNop(),
		store:     cat,
		expenses:  exp,
		templates: tpl,
		documents: doc,
		foyers:    foy,
	}
	return uc, cat, exp, tpl, doc, foy
}

// ─── Create ─────────────────────────────────────────────────────────

func TestCreate(t *testing.T) {
	Convey("Given a valid name from a foyer member", t, func() {
		ctx := context.Background()
		uc, cat, _, _, _, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("List", ctx).Return([]entities.Category{}, nil)
		cat.On("Create", ctx, mock.AnythingOfType("entities.Category")).Return(nil)

		c, err := uc.Create(ctx, CreateCategoryInput{
			ActorUserID:             "uid-rdc",
			Name:                    "Garage",
			DefaultDistributionMode: entities.DistributionModeEqual,
		})
		Convey("It returns the new category as Custom", func() {
			So(err, ShouldBeNil)
			So(c.ID, ShouldNotBeBlank)
			So(c.Predefined, ShouldBeFalse)
			So(c.Name, ShouldEqual, "Garage")
		})
	})

	Convey("Rejects a duplicate name (case-insensitive)", t, func() {
		ctx := context.Background()
		uc, cat, _, _, _, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("List", ctx).Return([]entities.Category{
			{ID: "garage", Name: "Garage", Predefined: false},
		}, nil)

		_, err := uc.Create(ctx, CreateCategoryInput{
			ActorUserID: "uid-rdc",
			Name:        "garage", // lowercase variant
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects too-short names", t, func() {
		ctx := context.Background()
		uc, _, _, _, _, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.Create(ctx, CreateCategoryInput{ActorUserID: "uid-rdc", Name: "x"})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects an intruder actor", t, func() {
		ctx := context.Background()
		uc, _, _, _, _, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.Create(ctx, CreateCategoryInput{ActorUserID: "intruder", Name: "Garage"})
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})
}

// ─── Update ─────────────────────────────────────────────────────────

func TestUpdate(t *testing.T) {
	Convey("Predefined: only default mode is mutable; name stays", t, func() {
		ctx := context.Background()
		uc, cat, _, _, _, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "eau").Return(&entities.Category{
			ID: "eau", Name: "Eau", Predefined: true,
			DefaultDistributionMode: entities.DistributionModeEqual,
		}, nil)
		cat.On("Update", ctx, mock.MatchedBy(func(c entities.Category) bool {
			return c.Name == "Eau" && c.DefaultDistributionMode == entities.DistributionModeTantiemes
		})).Return(nil)

		out, err := uc.Update(ctx, "eau", UpdateCategoryInput{
			ActorUserID:             "uid-rdc",
			Name:                    "TENTATIVE DE RENOMMAGE", // ignored
			DefaultDistributionMode: entities.DistributionModeTantiemes,
		})
		So(err, ShouldBeNil)
		So(out.Name, ShouldEqual, "Eau")
		So(out.DefaultDistributionMode, ShouldEqual, entities.DistributionModeTantiemes)
	})

	Convey("Custom: name + default mode both mutable", t, func() {
		ctx := context.Background()
		uc, cat, _, _, _, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "garage-id").Return(&entities.Category{
			ID: "garage-id", Name: "Garage", Predefined: false,
		}, nil)
		cat.On("List", ctx).Return([]entities.Category{
			{ID: "garage-id", Name: "Garage"},
		}, nil)
		cat.On("Update", ctx, mock.AnythingOfType("entities.Category")).Return(nil)

		out, err := uc.Update(ctx, "garage-id", UpdateCategoryInput{
			ActorUserID:             "uid-rdc",
			Name:                    "Parking",
			DefaultDistributionMode: entities.DistributionModeEqual,
		})
		So(err, ShouldBeNil)
		So(out.Name, ShouldEqual, "Parking")
	})

	Convey("Returns ErrNotFound for ghost id", t, func() {
		ctx := context.Background()
		uc, cat, _, _, _, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "ghost").Return((*entities.Category)(nil), nil)
		_, err := uc.Update(ctx, "ghost", UpdateCategoryInput{
			ActorUserID: "uid-rdc",
			Name:        "x",
		})
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}

// ─── Delete ─────────────────────────────────────────────────────────

func TestDelete(t *testing.T) {
	Convey("Rejects predefined categories unconditionally", t, func() {
		ctx := context.Background()
		uc, cat, _, _, _, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "eau").Return(&entities.Category{
			ID: "eau", Name: "Eau", Predefined: true,
		}, nil)
		err := uc.Delete(ctx, "eau", "uid-rdc")
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects custom category that's referenced", t, func() {
		ctx := context.Background()
		uc, cat, exp, tpl, doc, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "garage-id").Return(&entities.Category{
			ID: "garage-id", Name: "Garage", Predefined: false,
		}, nil)
		exp.On("CountByCategory", ctx, "garage-id").Return(3, nil)
		tpl.On("CountByCategory", ctx, "garage-id").Return(0, nil)
		doc.On("CountByCategory", ctx, "garage-id").Return(0, nil)

		err := uc.Delete(ctx, "garage-id", "uid-rdc")
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
		So(err.Error(), ShouldContainSubstring, "3 dépense")
	})

	Convey("Deletes an unreferenced custom category", t, func() {
		ctx := context.Background()
		uc, cat, exp, tpl, doc, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "garage-id").Return(&entities.Category{
			ID: "garage-id", Name: "Garage", Predefined: false,
		}, nil)
		exp.On("CountByCategory", ctx, "garage-id").Return(0, nil)
		tpl.On("CountByCategory", ctx, "garage-id").Return(0, nil)
		doc.On("CountByCategory", ctx, "garage-id").Return(0, nil)
		cat.On("Delete", ctx, "garage-id").Return(nil)

		err := uc.Delete(ctx, "garage-id", "uid-rdc")
		So(err, ShouldBeNil)
	})

	Convey("Returns ErrNotFound for ghost id", t, func() {
		ctx := context.Background()
		uc, cat, _, _, _, foy := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "ghost").Return((*entities.Category)(nil), nil)
		err := uc.Delete(ctx, "ghost", "uid-rdc")
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}
