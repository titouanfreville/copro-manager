package templates

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/expenses"
)

// ─── Mocks ──────────────────────────────────────────────────────────

type mockTemplatesStore struct{ mock.Mock }

func (m *mockTemplatesStore) List(ctx context.Context) ([]entities.ExpenseTemplate, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.ExpenseTemplate), args.Error(1)
	}
	return nil, args.Error(1)
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
func (m *mockTemplatesStore) CountByCategory(ctx context.Context, categoryID string) (int, error) {
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

// stubExpenses satisfies expenses.Usecases — only Create is exercised by
// the materialization loop. Other methods panic to surface test mistakes.
type stubExpenses struct {
	createCalls []expenses.CreateInput
	createErr   error
}

func (s *stubExpenses) Create(ctx context.Context, in expenses.CreateInput) (*entities.Expense, error) {
	s.createCalls = append(s.createCalls, in)
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &entities.Expense{ID: "exp-id", CoproID: "c1"}, nil
}
func (s *stubExpenses) Update(context.Context, string, expenses.CreateInput) (*entities.Expense, error) {
	panic("unexpected: Update")
}
func (s *stubExpenses) Delete(context.Context, string, string) error { panic("unexpected: Delete") }
func (s *stubExpenses) Upsert(context.Context, expenses.CreateInput) (*expenses.UpsertResult, error) {
	panic("unexpected: Upsert")
}
func (s *stubExpenses) ImportCSV(context.Context, io.Reader, string) (*expenses.ImportSummary, error) {
	panic("unexpected: ImportCSV")
}

// ─── Helpers ────────────────────────────────────────────────────────

var (
	rdc     = &entities.Foyer{ID: "rdc", Floor: entities.FoyerFloorRDC, MemberIDs: []string{"uid-rdc"}}
	premier = &entities.Foyer{ID: "1er", Floor: entities.FoyerFloor1er, MemberIDs: []string{"uid-1er"}}
	cop     = &entities.Copro{ID: "c1", TotalParts: 1000}
	now     = time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
)

func newUC() (*usecases, *mockTemplatesStore, *mockFoyersStore, *mockCoprosStore, *stubExpenses) {
	tpls := &mockTemplatesStore{}
	foy := &mockFoyersStore{}
	cop := &mockCoprosStore{}
	exp := &stubExpenses{}
	uc := &usecases{
		logger:    zap.NewNop(),
		templates: tpls,
		foyers:    foy,
		copros:    cop,
		expenses:  exp,
		now:       func() time.Time { return now },
		location:  time.UTC, // UTC for deterministic test cutoff.
	}
	return uc, tpls, foy, cop, exp
}

// ─── Tests ──────────────────────────────────────────────────────────

func TestCreate(t *testing.T) {
	Convey("Given valid input from a foyer member", t, func() {
		ctx := context.Background()
		uc, tpls, foy, copStore, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		copStore.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		tpls.On("Create", ctx, mock.AnythingOfType("entities.ExpenseTemplate")).Return(nil)

		t1, err := uc.Create(ctx, CreateTemplateInput{
			ActorUserID:        "uid-rdc",
			Name:               "EDF",
			AmountDefaultCents: 0,
			CategoryID:         "elec",
			PayerFoyerID:       "rdc",
			DistributionMode:   entities.DistributionModeEqual,
		})
		Convey("It returns the new template with a fresh ID", func() {
			So(err, ShouldBeNil)
			So(t1.ID, ShouldNotBeBlank)
			So(t1.Currency, ShouldEqual, "EUR")
		})
	})

	Convey("Rejects schedule_active without frequency", t, func() {
		ctx := context.Background()
		uc, _, _, _, _ := newUC()
		_, err := uc.Create(ctx, CreateTemplateInput{
			Name:             "x",
			CategoryID:       "c",
			PayerFoyerID:     "rdc",
			DistributionMode: entities.DistributionModeEqual,
			ScheduleActive:   true,
			DayOfMonth:       5,
			StartDate:        now,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects day_of_month outside 1–31", t, func() {
		ctx := context.Background()
		uc, _, _, _, _ := newUC()
		_, err := uc.Create(ctx, CreateTemplateInput{
			Name:             "x",
			CategoryID:       "c",
			PayerFoyerID:     "rdc",
			DistributionMode: entities.DistributionModeEqual,
			ScheduleActive:   true,
			Frequency:        entities.FrequencyMonthly,
			DayOfMonth:       32,
			StartDate:        now,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects an intruder actor", t, func() {
		ctx := context.Background()
		uc, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		_, err := uc.Create(ctx, CreateTemplateInput{
			ActorUserID:      "intruder",
			Name:             "EDF",
			CategoryID:       "elec",
			PayerFoyerID:     "rdc",
			DistributionMode: entities.DistributionModeEqual,
		})
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})
}

func TestDelete(t *testing.T) {
	Convey("Returns ErrNotFound for ghost id (with valid actor)", t, func() {
		ctx := context.Background()
		uc, tpls, foy, _, _ := newUC()
		// Authorize runs first now (defense-in-depth against ID probing).
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		tpls.On("FindByID", ctx, "ghost").Return((*entities.ExpenseTemplate)(nil), nil)
		err := uc.Delete(ctx, "ghost", "uid-rdc")
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}

// ─── MaterializeRecurring ──────────────────────────────────────────

func TestMaterializeRecurring(t *testing.T) {
	Convey("With a monthly template due today and a default amount", t, func() {
		ctx := context.Background()
		uc, tpls, _, _, exp := newUC()
		dueAt := now.AddDate(0, 0, -2) // 2 days ago, single occurrence due
		tpl := entities.ExpenseTemplate{
			ID:                 "tpl-edf",
			CoproID:            "c1",
			Name:               "EDF",
			AmountDefaultCents: 7500,
			Currency:           "EUR",
			CategoryID:         "elec",
			PayerFoyerID:       "rdc",
			DistributionMode:   entities.DistributionModeEqual,
			ScheduleActive:     true,
			Frequency:          entities.FrequencyMonthly,
			DayOfMonth:         5,
			NextOccurrenceAt:   &dueAt,
		}
		tpls.On("ListDue", ctx, mock.AnythingOfType("time.Time")).Return([]entities.ExpenseTemplate{tpl}, nil)
		tpls.On("Update", ctx, mock.AnythingOfType("entities.ExpenseTemplate")).Return(nil)

		summary, err := uc.MaterializeRecurring(ctx, "")

		Convey("It creates one expense and advances next_occurrence_at by a month", func() {
			So(err, ShouldBeNil)
			So(summary.TemplatesProcessed, ShouldEqual, 1)
			So(summary.ExpensesCreated, ShouldEqual, 1)
			So(len(exp.createCalls), ShouldEqual, 1)
			call := exp.createCalls[0]
			So(call.AmountCents, ShouldEqual, 7500)
			So(call.AmountPending, ShouldBeFalse)
			So(call.TemplateID, ShouldEqual, "tpl-edf")
		})
	})

	Convey("With a monthly template that hasn't fired in 3 months (backfill)", t, func() {
		ctx := context.Background()
		uc, tpls, _, _, exp := newUC()
		dueAt := now.AddDate(0, -3, 0)
		tpl := entities.ExpenseTemplate{
			ID:                 "tpl-water",
			Name:               "Eau",
			AmountDefaultCents: 0, // pending mode
			Currency:           "EUR",
			CategoryID:         "eau",
			PayerFoyerID:       "1er",
			DistributionMode:   entities.DistributionModeTantiemes,
			ScheduleActive:     true,
			Frequency:          entities.FrequencyMonthly,
			DayOfMonth:         5,
			NextOccurrenceAt:   &dueAt,
		}
		tpls.On("ListDue", ctx, mock.AnythingOfType("time.Time")).Return([]entities.ExpenseTemplate{tpl}, nil)
		tpls.On("Update", ctx, mock.AnythingOfType("entities.ExpenseTemplate")).Return(nil)

		summary, err := uc.MaterializeRecurring(ctx, "")

		Convey("It backfills 3 (or 4) pending occurrences", func() {
			So(err, ShouldBeNil)
			// Could be 3 or 4 depending on exact dates around month boundaries.
			So(summary.ExpensesCreated, ShouldBeGreaterThanOrEqualTo, 3)
			So(summary.ExpensesCreated, ShouldBeLessThanOrEqualTo, 4)
			for _, c := range exp.createCalls {
				So(c.AmountPending, ShouldBeTrue)
				So(c.AmountCents, ShouldEqual, 0)
				So(c.TemplateID, ShouldEqual, "tpl-water")
			}
		})
	})

	Convey("With a template whose end_date has passed", t, func() {
		ctx := context.Background()
		uc, tpls, _, _, exp := newUC()
		dueAt := now.AddDate(0, -1, 0)
		endedAt := now.AddDate(0, -2, 0) // already past
		tpl := entities.ExpenseTemplate{
			ID:                 "tpl-old",
			Name:               "Old",
			AmountDefaultCents: 100,
			Currency:           "EUR",
			CategoryID:         "c",
			PayerFoyerID:       "rdc",
			DistributionMode:   entities.DistributionModeEqual,
			ScheduleActive:     true,
			Frequency:          entities.FrequencyMonthly,
			DayOfMonth:         5,
			NextOccurrenceAt:   &dueAt,
			EndDate:            &endedAt,
		}
		var captured entities.ExpenseTemplate
		tpls.On("ListDue", ctx, mock.AnythingOfType("time.Time")).Return([]entities.ExpenseTemplate{tpl}, nil)
		tpls.On("Update", ctx, mock.MatchedBy(func(t entities.ExpenseTemplate) bool {
			captured = t
			return true
		})).Return(nil)

		_, err := uc.MaterializeRecurring(ctx, "")

		Convey("It deactivates the schedule and creates no expenses", func() {
			So(err, ShouldBeNil)
			So(len(exp.createCalls), ShouldEqual, 0)
			So(captured.ScheduleActive, ShouldBeFalse)
		})
	})

	Convey("With nothing due", t, func() {
		ctx := context.Background()
		uc, tpls, _, _, exp := newUC()
		tpls.On("ListDue", ctx, mock.AnythingOfType("time.Time")).Return([]entities.ExpenseTemplate{}, nil)

		summary, err := uc.MaterializeRecurring(ctx, "")
		So(err, ShouldBeNil)
		So(summary.TemplatesProcessed, ShouldEqual, 0)
		So(summary.ExpensesCreated, ShouldEqual, 0)
		So(len(exp.createCalls), ShouldEqual, 0)
	})
}
