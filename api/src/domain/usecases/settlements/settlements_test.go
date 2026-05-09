package settlements

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

type mockSettlementsStore struct{ mock.Mock }

func (m *mockSettlementsStore) List(ctx context.Context) ([]entities.Settlement, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.Settlement), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockSettlementsStore) FindByID(ctx context.Context, id string) (*entities.Settlement, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.Settlement), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockSettlementsStore) FindByExpenseID(ctx context.Context, expenseID string) (*entities.Settlement, error) {
	args := m.Called(ctx, expenseID)
	if v := args.Get(0); v != nil {
		return v.(*entities.Settlement), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockSettlementsStore) Create(ctx context.Context, s entities.Settlement) error {
	return m.Called(ctx, s).Error(0)
}
func (m *mockSettlementsStore) Update(ctx context.Context, s entities.Settlement) error {
	return m.Called(ctx, s).Error(0)
}
func (m *mockSettlementsStore) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockSettlementsStore) PruneExpense(ctx context.Context, expenseID string) error {
	return m.Called(ctx, expenseID).Error(0)
}

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

// ─── Helpers ────────────────────────────────────────────────────────

var (
	rdc     = &entities.Foyer{ID: "rdc", Floor: entities.FoyerFloorRDC, MemberIDs: []string{"uid-rdc"}}
	premier = &entities.Foyer{ID: "1er", Floor: entities.FoyerFloor1er, MemberIDs: []string{"uid-1er"}}
	cop     = &entities.Copro{ID: "c1", TotalParts: 1000}
	now     = time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
)

func newUC() (*usecases, *mockSettlementsStore, *mockExpensesStore, *mockFoyersStore, *mockCoprosStore) {
	st := &mockSettlementsStore{}
	exp := &mockExpensesStore{}
	foy := &mockFoyersStore{}
	cps := &mockCoprosStore{}
	uc := &usecases{
		logger:      zap.NewNop(),
		settlements: st,
		expenses:    exp,
		foyers:      foy,
		copros:      cps,
		now:         func() time.Time { return now },
	}
	return uc, st, exp, foy, cps
}

func validInput(actor string, expenseIDs ...string) CreateInput {
	return CreateInput{
		ActorUserID: actor,
		FromFoyerID: "1er",
		ToFoyerID:   "rdc",
		AmountCents: 12740,
		Date:        now,
		ExpenseIDs:  expenseIDs,
	}
}

// ─── Create ─────────────────────────────────────────────────────────

func TestCreate(t *testing.T) {
	Convey("Given a valid input from a foyer member", t, func() {
		ctx := context.Background()
		uc, st, _, foy, cps := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cps.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		st.On("Create", ctx, mock.AnythingOfType("entities.Settlement")).Return(nil)

		s, err := uc.Create(ctx, validInput("uid-rdc"))
		Convey("It returns the settlement with a fresh ID and EUR currency", func() {
			So(err, ShouldBeNil)
			So(s.ID, ShouldNotBeBlank)
			So(s.Currency, ShouldEqual, "EUR")
			So(s.AmountCents, ShouldEqual, 12740)
			So(s.FromFoyerID, ShouldEqual, "1er")
			So(s.ToFoyerID, ShouldEqual, "rdc")
		})
	})

	Convey("Rejects from == to", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		in := validInput("uid-rdc")
		in.ToFoyerID = in.FromFoyerID
		_, err := uc.Create(ctx, in)
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects amount = 0", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		in := validInput("uid-rdc")
		in.AmountCents = 0
		_, err := uc.Create(ctx, in)
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects a foyer not in the copro", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		in := validInput("uid-rdc")
		in.FromFoyerID = "outsider"
		_, err := uc.Create(ctx, in)
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects an intruder actor", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.Create(ctx, validInput("intruder"))
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})

	Convey("With a linked expense already linked to another settlement", t, func() {
		ctx := context.Background()
		uc, st, exp, foy, cps := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cps.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		exp.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1", CoproID: "c1"}, nil)
		st.On("FindByExpenseID", ctx, "e1").Return(&entities.Settlement{ID: "s-other"}, nil)

		_, err := uc.Create(ctx, validInput("uid-rdc", "e1"))
		Convey("It rejects with a validation error referencing the conflict", func() {
			So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "s-other")
		})
	})

	Convey("With a missing linked expense", t, func() {
		ctx := context.Background()
		uc, _, exp, foy, cps := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		cps.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		exp.On("FindByID", ctx, "ghost").Return((*entities.Expense)(nil), nil)

		_, err := uc.Create(ctx, validInput("uid-rdc", "ghost"))
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})
}

// ─── Update ─────────────────────────────────────────────────────────

func TestUpdate(t *testing.T) {
	Convey("Given an existing settlement", t, func() {
		ctx := context.Background()
		uc, st, _, foy, _ := newUC()
		existing := &entities.Settlement{
			ID:          "s1",
			CoproID:     "c1",
			FromFoyerID: "1er",
			ToFoyerID:   "rdc",
			AmountCents: 10000,
			Currency:    "EUR",
			Date:        time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			CreatedAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		}
		st.On("FindByID", ctx, "s1").Return(existing, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		st.On("Update", ctx, mock.AnythingOfType("entities.Settlement")).Return(nil)

		out, err := uc.Update(ctx, "s1", CreateInput{
			ActorUserID: "uid-rdc",
			FromFoyerID: "1er",
			ToFoyerID:   "rdc",
			AmountCents: 8000,
			Date:        existing.Date,
		})
		Convey("It writes the new amount and refreshes UpdatedAt", func() {
			So(err, ShouldBeNil)
			So(out.AmountCents, ShouldEqual, 8000)
			So(out.UpdatedAt, ShouldHappenAfter, existing.CreatedAt)
			So(out.ID, ShouldEqual, "s1")
			So(out.CreatedAt, ShouldEqual, existing.CreatedAt)
		})
	})

	Convey("Returns ErrNotFound for a ghost id", t, func() {
		ctx := context.Background()
		uc, st, _, foy, _ := newUC()
		st.On("FindByID", ctx, "ghost").Return((*entities.Settlement)(nil), nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.Update(ctx, "ghost", validInput("uid-rdc"))
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})

	Convey("Update preserves a link that already pointed to this settlement (no false conflict)", t, func() {
		ctx := context.Background()
		uc, st, exp, foy, _ := newUC()
		existing := &entities.Settlement{
			ID:          "s1",
			CoproID:     "c1",
			FromFoyerID: "1er",
			ToFoyerID:   "rdc",
			AmountCents: 10000,
			Currency:    "EUR",
			Date:        now,
			ExpenseIDs:  []string{"e1"},
			CreatedAt:   now,
		}
		st.On("FindByID", ctx, "s1").Return(existing, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		exp.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1", CoproID: "c1"}, nil)
		// FindByExpenseID returns the SAME settlement we're editing — must not collide.
		st.On("FindByExpenseID", ctx, "e1").Return(existing, nil)
		st.On("Update", ctx, mock.AnythingOfType("entities.Settlement")).Return(nil)

		// checkExpenseLinks now resolves the copro singleton up-front to
		// enforce the cross-tenant guard; mock it.
		copStore := uc.copros.(*mockCoprosStore)
		copStore.On("GetOrCreateSingleton", ctx).Return(cop, nil)

		_, err := uc.Update(ctx, "s1", CreateInput{
			ActorUserID: "uid-rdc",
			FromFoyerID: "1er",
			ToFoyerID:   "rdc",
			AmountCents: 12000,
			Date:        now,
			ExpenseIDs:  []string{"e1"},
		})
		So(err, ShouldBeNil)
	})
}

// ─── Delete ─────────────────────────────────────────────────────────

func TestDelete(t *testing.T) {
	Convey("Deletes an existing settlement", t, func() {
		ctx := context.Background()
		uc, st, _, foy, _ := newUC()
		st.On("FindByID", ctx, "s1").Return(&entities.Settlement{ID: "s1"}, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		st.On("Delete", ctx, "s1").Return(nil)

		err := uc.Delete(ctx, "s1", "uid-rdc")
		So(err, ShouldBeNil)
		st.AssertCalled(t, "Delete", ctx, "s1")
	})

	Convey("Returns ErrNotFound for a ghost id", t, func() {
		ctx := context.Background()
		uc, st, _, foy, _ := newUC()
		st.On("FindByID", ctx, "ghost").Return((*entities.Settlement)(nil), nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		err := uc.Delete(ctx, "ghost", "uid-rdc")
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})

	Convey("Rejects an intruder actor", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		err := uc.Delete(ctx, "s1", "intruder")
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})
}
