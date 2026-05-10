package contracts

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

type mockContractsStore struct{ mock.Mock }

func (m *mockContractsStore) List(ctx context.Context) ([]entities.Contract, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.Contract), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockContractsStore) FindByID(ctx context.Context, id string) (*entities.Contract, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.Contract), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockContractsStore) Create(ctx context.Context, c entities.Contract) error {
	return m.Called(ctx, c).Error(0)
}
func (m *mockContractsStore) Update(ctx context.Context, c entities.Contract) error {
	return m.Called(ctx, c).Error(0)
}
func (m *mockContractsStore) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockContractsStore) CountByCategory(ctx context.Context, categoryID string) (int, error) {
	args := m.Called(ctx, categoryID)
	return args.Int(0), args.Error(1)
}

type mockCategoriesStore struct{ mock.Mock }

func (m *mockCategoriesStore) FindByID(ctx context.Context, id string) (*entities.Category, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.Category), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockCategoriesStore) List(ctx context.Context) ([]entities.Category, error) {
	return nil, m.Called(ctx).Error(1)
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
	return nil, m.Called(ctx, expenseID).Error(1)
}
func (m *mockDocumentsStore) CountByLinkedContract(ctx context.Context, contractID string) (int, error) {
	args := m.Called(ctx, contractID)
	return args.Int(0), args.Error(1)
}

type mockTemplatesStore struct{ mock.Mock }

func (m *mockTemplatesStore) List(ctx context.Context) ([]entities.ExpenseTemplate, error) {
	return nil, m.Called(ctx).Error(1)
}
func (m *mockTemplatesStore) FindByID(ctx context.Context, id string) (*entities.ExpenseTemplate, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.ExpenseTemplate), args.Error(1)
	}
	return nil, args.Error(1)
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

type mockAlertsHook struct{ mock.Mock }

func (m *mockAlertsHook) ResolveContractExpiring(ctx context.Context, contractID string) error {
	return m.Called(ctx, contractID).Error(0)
}

// ─── Helpers ────────────────────────────────────────────────────────

var (
	rdc     = &entities.Foyer{ID: "rdc", Floor: entities.FoyerFloorRDC, MemberIDs: []string{"uid-rdc"}}
	premier = &entities.Foyer{ID: "1er", Floor: entities.FoyerFloor1er, MemberIDs: []string{"uid-1er"}}
	cop     = &entities.Copro{ID: "c1", TotalParts: 1000}
	now     = time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
)

func newUC() (*usecases, *mockContractsStore, *mockCategoriesStore, *mockFoyersStore, *mockCoprosStore, *mockDocumentsStore, *mockTemplatesStore, *mockAlertsHook) {
	con := &mockContractsStore{}
	cat := &mockCategoriesStore{}
	foy := &mockFoyersStore{}
	cps := &mockCoprosStore{}
	doc := &mockDocumentsStore{}
	tpl := &mockTemplatesStore{}
	alh := &mockAlertsHook{}
	uc := &usecases{
		logger:     zap.NewNop(),
		contracts:  con,
		categories: cat,
		foyers:     foy,
		copros:     cps,
		documents:  doc,
		templates:  tpl,
		alerts:     alh,
		now:        func() time.Time { return now },
	}
	return uc, con, cat, foy, cps, doc, tpl, alh
}

func validInput() CreateInput {
	return CreateInput{
		ActorUserID: "uid-rdc",
		Name:        "Assurance habitation",
		CategoryID:  "assurance",
		Society:     entities.Society{Name: "Maaf", Phone: "0123456789"},
	}
}

// ─── Create ─────────────────────────────────────────────────────────

func TestCreate(t *testing.T) {
	Convey("Given valid input from a foyer member", t, func() {
		ctx := context.Background()
		uc, con, cat, foy, cps, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "assurance").Return(&entities.Category{ID: "assurance"}, nil)
		cps.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		con.On("Create", ctx, mock.AnythingOfType("entities.Contract")).Return(nil)

		c, err := uc.Create(ctx, validInput())
		Convey("It returns the new contract with active status and copro stamped", func() {
			So(err, ShouldBeNil)
			So(c.ID, ShouldNotBeBlank)
			So(c.CoproID, ShouldEqual, "c1")
			So(c.Status, ShouldEqual, entities.ContractStatusActive)
			So(c.Society.Name, ShouldEqual, "Maaf")
		})
	})

	Convey("Rejects when the actor isn't a foyer member", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		in := validInput()
		in.ActorUserID = "uid-stranger"
		_, err := uc.Create(ctx, in)
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})

	Convey("Rejects a too-short name", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		in := validInput()
		in.Name = "x"
		_, err := uc.Create(ctx, in)
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects when society name is empty", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		in := validInput()
		in.Society.Name = "  "
		_, err := uc.Create(ctx, in)
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
		So(err.Error(), ShouldContainSubstring, "society.name")
	})

	Convey("Rejects when category does not exist", t, func() {
		ctx := context.Background()
		uc, _, cat, foy, _, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "ghost").Return((*entities.Category)(nil), nil)
		in := validInput()
		in.CategoryID = "ghost"
		_, err := uc.Create(ctx, in)
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects end_date before start_date", t, func() {
		ctx := context.Background()
		uc, _, cat, foy, _, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "assurance").Return(&entities.Category{ID: "assurance"}, nil)
		in := validInput()
		in.StartDate = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		in.EndDate = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		_, err := uc.Create(ctx, in)
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects a template_id that doesn't exist", t, func() {
		ctx := context.Background()
		uc, _, cat, foy, _, _, tpl, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cat.On("FindByID", ctx, "assurance").Return(&entities.Category{ID: "assurance"}, nil)
		tpl.On("FindByID", ctx, "ghost-tpl").Return((*entities.ExpenseTemplate)(nil), nil)
		in := validInput()
		in.TemplateID = "ghost-tpl"
		_, err := uc.Create(ctx, in)
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
		So(err.Error(), ShouldContainSubstring, "template_id")
	})
}

// ─── Update ─────────────────────────────────────────────────────────

func TestUpdate(t *testing.T) {
	Convey("Updates an existing contract and bumps UpdatedAt", t, func() {
		ctx := context.Background()
		uc, con, cat, foy, _, _, _, _ := newUC()
		existing := &entities.Contract{
			ID: "ctr-1", CoproID: "c1", Name: "Old", CategoryID: "assurance",
			Society:   entities.Society{Name: "Old Co"},
			Status:    entities.ContractStatusActive,
			CreatedAt: now.Add(-30 * 24 * time.Hour),
			UpdatedAt: now.Add(-30 * 24 * time.Hour),
		}
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		con.On("FindByID", ctx, "ctr-1").Return(existing, nil)
		cat.On("FindByID", ctx, "assurance").Return(&entities.Category{ID: "assurance"}, nil)
		con.On("Update", ctx, mock.AnythingOfType("entities.Contract")).Return(nil)

		out, err := uc.Update(ctx, "ctr-1", validInput())
		So(err, ShouldBeNil)
		So(out.Name, ShouldEqual, "Assurance habitation")
		So(out.UpdatedAt, ShouldEqual, now)
		So(out.CreatedAt, ShouldEqual, existing.CreatedAt)
	})

	Convey("Returns ErrNotFound for a ghost id", t, func() {
		ctx := context.Background()
		uc, con, _, foy, _, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		con.On("FindByID", ctx, "ghost").Return((*entities.Contract)(nil), nil)
		_, err := uc.Update(ctx, "ghost", validInput())
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}

// ─── Delete ─────────────────────────────────────────────────────────

func TestDelete(t *testing.T) {
	Convey("Refuses delete when documents still link to the contract", t, func() {
		ctx := context.Background()
		uc, con, _, foy, _, doc, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		con.On("FindByID", ctx, "ctr-1").Return(&entities.Contract{ID: "ctr-1"}, nil)
		doc.On("CountByLinkedContract", ctx, "ctr-1").Return(2, nil)

		err := uc.Delete(ctx, "ctr-1", "uid-rdc")
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
		So(err.Error(), ShouldContainSubstring, "2 document")
	})

	Convey("Deletes when no docs linked, then resolves outstanding alerts", t, func() {
		ctx := context.Background()
		uc, con, _, foy, _, doc, _, alh := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		con.On("FindByID", ctx, "ctr-1").Return(&entities.Contract{ID: "ctr-1"}, nil)
		doc.On("CountByLinkedContract", ctx, "ctr-1").Return(0, nil)
		con.On("Delete", ctx, "ctr-1").Return(nil)
		alh.On("ResolveContractExpiring", ctx, "ctr-1").Return(nil)

		err := uc.Delete(ctx, "ctr-1", "uid-rdc")
		So(err, ShouldBeNil)
		alh.AssertCalled(t, "ResolveContractExpiring", ctx, "ctr-1")
	})

	Convey("Returns ErrNotFound for a ghost id", t, func() {
		ctx := context.Background()
		uc, con, _, foy, _, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		con.On("FindByID", ctx, "ghost").Return((*entities.Contract)(nil), nil)
		err := uc.Delete(ctx, "ghost", "uid-rdc")
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}

// ─── Helpers (entity behaviour) ─────────────────────────────────────

func TestIsExpiringSoon(t *testing.T) {
	Convey("Returns true when end_date is within 30 days, regardless of TZ", t, func() {
		paris, _ := time.LoadLocation("Europe/Paris")
		ref := time.Date(2026, 5, 8, 23, 30, 0, 0, paris) // late evening Paris
		c := entities.Contract{
			Status:  entities.ContractStatusActive,
			EndDate: time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC), // 30 days later
		}
		So(c.IsExpiringSoon(ref), ShouldBeTrue)
	})

	Convey("Returns false when contract is cancelled", t, func() {
		ref := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
		c := entities.Contract{
			Status:  entities.ContractStatusCancelled,
			EndDate: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		}
		So(c.IsExpiringSoon(ref), ShouldBeFalse)
	})

	Convey("Returns false when end_date is missing", t, func() {
		ref := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
		c := entities.Contract{Status: entities.ContractStatusActive}
		So(c.IsExpiringSoon(ref), ShouldBeFalse)
	})
}

func TestTruncate(t *testing.T) {
	Convey("Preserves UTF-8 boundaries when cutting at a multibyte rune", t, func() {
		// "é" is two bytes in UTF-8. Truncating at 5 bytes lands inside
		// the second "é" (4 bytes "Soci" + first byte of "é") — must
		// step back to a rune boundary.
		got := truncate("Société", 5)
		So(got, ShouldEqual, "Soci")
	})

	Convey("Returns the input untouched when shorter than the cap", t, func() {
		So(truncate("hello", 100), ShouldEqual, "hello")
	})
}
