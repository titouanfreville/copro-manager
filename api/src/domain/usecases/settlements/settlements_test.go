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
	return nil, m.Called(ctx, id).Error(1)
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

type mockValidator struct{ mock.Mock }

func (m *mockValidator) Validate(ctx context.Context, d entities.SettlementDraft, selfID string) error {
	return m.Called(ctx, d, selfID).Error(0)
}

type mockAlerts struct{ mock.Mock }

func (m *mockAlerts) ResolveSeasonalAll(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

// ─── Helpers ────────────────────────────────────────────────────────

var (
	rdc     = &entities.Foyer{ID: "rdc", Floor: entities.FoyerFloorRDC, MemberIDs: []string{"uid-rdc"}}
	premier = &entities.Foyer{ID: "1er", Floor: entities.FoyerFloor1er, MemberIDs: []string{"uid-1er"}}
	cop     = &entities.Copro{ID: "c1", TotalParts: 1000}
	now     = time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
)

func newUC() (*usecases, *mockSettlementsStore, *mockExpensesStore, *mockFoyersStore, *mockCoprosStore, *mockValidator, *mockAlerts) {
	st := &mockSettlementsStore{}
	exp := &mockExpensesStore{}
	foy := &mockFoyersStore{}
	cps := &mockCoprosStore{}
	val := &mockValidator{}
	alerts := &mockAlerts{}
	clock := func() time.Time { return now }
	logger := zap.NewNop()
	uc := &usecases{
		logger:      logger,
		settlements: st,
		foyers:      foy,
		validator:   val,
		builder:     newBuilder(cps, clock),
		resolver:    newSeasonalResolver(logger, exp, st, foy, alerts),
	}
	return uc, st, exp, foy, cps, val, alerts
}

func validInput(actor string) CreateInput {
	return CreateInput{
		ActorUserID: actor,
		SettlementDraft: entities.SettlementDraft{
			FromFoyerID: "1er",
			ToFoyerID:   "rdc",
			AmountCents: 12740,
			Date:        now,
		},
	}
}

// ─── Create ─────────────────────────────────────────────────────────

func TestCreate(t *testing.T) {
	Convey("Persists a fresh settlement and triggers the seasonal cascade", t, func() {
		ctx := context.Background()
		uc, st, exp, foy, cps, val, alerts := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		val.On("Validate", ctx, mock.AnythingOfType("entities.SettlementDraft"), "").Return(nil)
		cps.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		st.On("Create", ctx, mock.AnythingOfType("entities.Settlement")).Return(nil)
		// Seasonal resolver reads the live ledger; return empty so net=0
		// triggers the resolve.
		exp.On("List", ctx).Return([]entities.Expense{}, nil)
		st.On("List", ctx).Return([]entities.Settlement{}, nil)
		alerts.On("ResolveSeasonalAll", ctx).Return(nil)

		out, err := uc.Create(ctx, validInput("uid-rdc"))
		So(err, ShouldBeNil)
		So(out.ID, ShouldNotBeBlank)
		So(out.CoproID, ShouldEqual, "c1")
		So(out.Currency, ShouldEqual, "EUR")
	})

	Convey("Refuses unauthenticated foreign actor", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)

		_, err := uc.Create(ctx, validInput("stranger"))
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})

	Convey("Surfaces validator errors verbatim", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, val, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		val.On("Validate", ctx, mock.AnythingOfType("entities.SettlementDraft"), "").
			Return(entities.ValidationError{Key: "amount_cents", Message: "must be > 0"})

		_, err := uc.Create(ctx, validInput("uid-rdc"))
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})
}

// ─── Update ─────────────────────────────────────────────────────────

func TestUpdate(t *testing.T) {
	Convey("Updates the settlement and bumps UpdatedAt", t, func() {
		ctx := context.Background()
		uc, st, exp, foy, _, val, alerts := newUC()
		existing := &entities.Settlement{
			ID: "s1", CoproID: "c1", FromFoyerID: "1er", ToFoyerID: "rdc",
			AmountCents: 5000, Currency: "EUR", Date: now.Add(-7 * 24 * time.Hour),
			CreatedAt: now.Add(-7 * 24 * time.Hour), UpdatedAt: now.Add(-7 * 24 * time.Hour),
		}
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		st.On("FindByID", ctx, "s1").Return(existing, nil)
		val.On("Validate", ctx, mock.AnythingOfType("entities.SettlementDraft"), "s1").Return(nil)
		st.On("Update", ctx, mock.AnythingOfType("entities.Settlement")).Return(nil)
		exp.On("List", ctx).Return([]entities.Expense{}, nil)
		st.On("List", ctx).Return([]entities.Settlement{}, nil)
		alerts.On("ResolveSeasonalAll", ctx).Return(nil)

		out, err := uc.Update(ctx, "s1", validInput("uid-rdc"))
		So(err, ShouldBeNil)
		So(out.ID, ShouldEqual, "s1")
		So(out.UpdatedAt, ShouldEqual, now)
		So(out.CreatedAt, ShouldEqual, existing.CreatedAt)
	})

	Convey("Returns ErrNotFound for a ghost id", t, func() {
		ctx := context.Background()
		uc, st, _, foy, _, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		st.On("FindByID", ctx, "ghost").Return((*entities.Settlement)(nil), nil)

		_, err := uc.Update(ctx, "ghost", validInput("uid-rdc"))
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}

// ─── Delete ─────────────────────────────────────────────────────────

func TestDelete(t *testing.T) {
	Convey("Removes the settlement and re-runs the cascade", t, func() {
		ctx := context.Background()
		uc, st, exp, foy, _, _, alerts := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		st.On("FindByID", ctx, "s1").Return(&entities.Settlement{ID: "s1"}, nil)
		st.On("Delete", ctx, "s1").Return(nil)
		exp.On("List", ctx).Return([]entities.Expense{}, nil)
		st.On("List", ctx).Return([]entities.Settlement{}, nil)
		alerts.On("ResolveSeasonalAll", ctx).Return(nil)

		err := uc.Delete(ctx, "s1", "uid-rdc")
		So(err, ShouldBeNil)
	})
}
