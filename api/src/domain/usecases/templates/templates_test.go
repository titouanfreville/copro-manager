package templates

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
	args := m.Called(ctx, cutoff)
	if v := args.Get(0); v != nil {
		return v.([]entities.ExpenseTemplate), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockTemplatesStore) CountByCategory(ctx context.Context, id string) (int, error) {
	args := m.Called(ctx, id)
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

func (m *mockValidator) Validate(ctx context.Context, d entities.ExpenseTemplateDraft) error {
	return m.Called(ctx, d).Error(0)
}

type stubExpenses struct{}

// stubExpenses implements templates.ExpensesHook — the narrow
// hook the materializer uses; not the full expenses.Usecases.
func (s *stubExpenses) Create(context.Context, entities.ExpenseDraft) (*entities.Expense, error) {
	return &entities.Expense{ID: "e1"}, nil
}

// ─── Helpers ────────────────────────────────────────────────────────

var (
	rdc     = &entities.Foyer{ID: "rdc", Floor: entities.FoyerFloorRDC, MemberIDs: []string{"uid-rdc"}}
	premier = &entities.Foyer{ID: "1er", Floor: entities.FoyerFloor1er, MemberIDs: []string{"uid-1er"}}
	cop     = &entities.Copro{ID: "c1", TotalParts: 1000}
	now     = time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
)

func newUC() (*usecases, *mockTemplatesStore, *mockFoyersStore, *mockCoprosStore, *mockValidator) {
	tpls := &mockTemplatesStore{}
	foy := &mockFoyersStore{}
	cps := &mockCoprosStore{}
	val := &mockValidator{}
	clock := func() time.Time { return now }
	logger := zap.NewNop()
	uc := &usecases{
		logger:       logger,
		templates:    tpls,
		foyers:       foy,
		validator:    val,
		builder:      newBuilder(cps, clock),
		materializer: newMaterializer(logger, tpls, &stubExpenses{}, nil, clock),
		location:     time.UTC,
	}
	return uc, tpls, foy, cps, val
}

func validInput(actor string) CreateTemplateInput {
	return CreateTemplateInput{
		ActorUserID: actor,
		ExpenseTemplateDraft: entities.ExpenseTemplateDraft{
			Name:               "Eau",
			AmountDefaultCents: 5000,
			CategoryID:         "eau",
			PayerFoyerID:       "rdc",
			DistributionMode:   entities.DistributionModeEqual,
		},
	}
}

// ─── Create ─────────────────────────────────────────────────────────

func TestCreate(t *testing.T) {
	Convey("Persists a fresh template with EUR default and stamped copro", t, func() {
		ctx := context.Background()
		uc, tpls, foy, cps, val := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		val.On("Validate", ctx, mock.AnythingOfType("entities.ExpenseTemplateDraft")).Return(nil)
		cps.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		tpls.On("Create", ctx, mock.AnythingOfType("entities.ExpenseTemplate")).Return(nil)

		out, err := uc.Create(ctx, validInput("uid-rdc"))
		So(err, ShouldBeNil)
		So(out.ID, ShouldNotBeBlank)
		So(out.CoproID, ShouldEqual, "c1")
		So(out.Currency, ShouldEqual, "EUR")
	})

	Convey("Surfaces validator errors verbatim", t, func() {
		ctx := context.Background()
		uc, _, foy, _, val := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		val.On("Validate", ctx, mock.AnythingOfType("entities.ExpenseTemplateDraft")).
			Return(entities.ValidationError{Key: "name", Message: "required"})
		_, err := uc.Create(ctx, validInput("uid-rdc"))
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Refuses unauthenticated foreign actor", t, func() {
		ctx := context.Background()
		uc, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.Create(ctx, validInput("stranger"))
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})
}

// ─── Update ─────────────────────────────────────────────────────────

func TestUpdate(t *testing.T) {
	Convey("Updates the template and bumps UpdatedAt", t, func() {
		ctx := context.Background()
		uc, tpls, foy, _, val := newUC()
		existing := &entities.ExpenseTemplate{
			ID: "t1", CoproID: "c1", Name: "Old",
			DistributionMode: entities.DistributionModeEqual,
			Currency:         "EUR",
			CreatedAt:        now.Add(-30 * 24 * time.Hour),
			UpdatedAt:        now.Add(-30 * 24 * time.Hour),
		}
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		tpls.On("FindByID", ctx, "t1").Return(existing, nil)
		val.On("Validate", ctx, mock.AnythingOfType("entities.ExpenseTemplateDraft")).Return(nil)
		tpls.On("Update", ctx, mock.AnythingOfType("entities.ExpenseTemplate")).Return(nil)

		out, err := uc.Update(ctx, "t1", validInput("uid-rdc"))
		So(err, ShouldBeNil)
		So(out.ID, ShouldEqual, "t1")
		So(out.UpdatedAt, ShouldEqual, now)
	})

	Convey("Returns ErrNotFound for a ghost id", t, func() {
		ctx := context.Background()
		uc, tpls, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		tpls.On("FindByID", ctx, "ghost").Return((*entities.ExpenseTemplate)(nil), nil)
		_, err := uc.Update(ctx, "ghost", validInput("uid-rdc"))
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}

// ─── MaterializeRecurring ───────────────────────────────────────────

func TestMaterializeRecurring(t *testing.T) {
	Convey("No-op when no templates are due", t, func() {
		ctx := context.Background()
		uc, tpls, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		tpls.On("ListDue", ctx, mock.AnythingOfType("time.Time")).Return([]entities.ExpenseTemplate{}, nil)

		summary, err := uc.MaterializeRecurring(ctx, "uid-rdc")
		So(err, ShouldBeNil)
		So(summary.TemplatesProcessed, ShouldEqual, 0)
		So(summary.ExpensesCreated, ShouldEqual, 0)
	})
}
