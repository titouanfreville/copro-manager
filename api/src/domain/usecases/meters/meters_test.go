package meters

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// ─── Mocks ──────────────────────────────────────────────────────────

type mockMetersStore struct{ mock.Mock }

func (m *mockMetersStore) List(ctx context.Context) ([]entities.MeterReading, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.MeterReading), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockMetersStore) FindByPeriod(ctx context.Context, period string) (*entities.MeterReading, error) {
	args := m.Called(ctx, period)
	if v := args.Get(0); v != nil {
		return v.(*entities.MeterReading), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockMetersStore) FindPriorPeriod(ctx context.Context, period string) (*entities.MeterReading, error) {
	args := m.Called(ctx, period)
	if v := args.Get(0); v != nil {
		return v.(*entities.MeterReading), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockMetersStore) FindNextPeriod(ctx context.Context, period string) (*entities.MeterReading, error) {
	args := m.Called(ctx, period)
	if v := args.Get(0); v != nil {
		return v.(*entities.MeterReading), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockMetersStore) Create(ctx context.Context, r entities.MeterReading) error {
	return m.Called(ctx, r).Error(0)
}
func (m *mockMetersStore) Update(ctx context.Context, r entities.MeterReading) error {
	return m.Called(ctx, r).Error(0)
}
func (m *mockMetersStore) Delete(ctx context.Context, period string) error {
	return m.Called(ctx, period).Error(0)
}
func (m *mockMetersStore) SetPhoto(ctx context.Context, period string, kind entities.MeterPhotoKind, object, contentType string, sizeBytes int64) error {
	return m.Called(ctx, period, kind, object, contentType, sizeBytes).Error(0)
}
func (m *mockMetersStore) ClearPhoto(ctx context.Context, period string, kind entities.MeterPhotoKind) error {
	return m.Called(ctx, period, kind).Error(0)
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
func (m *mockExpensesStore) CountByMeterReadingPeriod(ctx context.Context, period string) (int, error) {
	args := m.Called(ctx, period)
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

type mockAlertsHook struct{ mock.Mock }

func (m *mockAlertsHook) ResolveByPrefix(ctx context.Context, prefix string) error {
	return m.Called(ctx, prefix).Error(0)
}

// ─── Helpers ────────────────────────────────────────────────────────

var (
	rdc     = &entities.Foyer{ID: "rdc", Floor: entities.FoyerFloorRDC, MemberIDs: []string{"uid-rdc"}}
	premier = &entities.Foyer{ID: "1er", Floor: entities.FoyerFloor1er, MemberIDs: []string{"uid-1er"}}
	cop     = &entities.Copro{ID: "c1"}
	now     = time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
)

func newUC() (*usecases, *mockMetersStore, *mockExpensesStore, *mockFoyersStore, *mockCoprosStore, *mockAlertsHook) {
	mt := &mockMetersStore{}
	exp := &mockExpensesStore{}
	foy := &mockFoyersStore{}
	cps := &mockCoprosStore{}
	al := &mockAlertsHook{}
	uc := &usecases{
		logger:   zap.NewNop(),
		meters:   mt,
		expenses: exp,
		foyers:   foy,
		copros:   cps,
		alerts:   al,
		now:      func() time.Time { return now },
	}
	return uc, mt, exp, foy, cps, al
}

func validInput() SaveInput {
	return SaveInput{
		ActorUserID: "uid-rdc",
		Period:      "2026-05",
		GlobalM3:    1234.567,
		CommonM3:    100.000,
		RDCM3:       200.500,
		PremierM3:   300.250,
	}
}

// ─── Create ─────────────────────────────────────────────────────────

func TestCreate(t *testing.T) {
	Convey("Given a valid input from a foyer member", t, func() {
		ctx := context.Background()
		uc, mt, _, foy, cps, al := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		mt.On("FindPriorPeriod", ctx, "2026-05").Return(nil, nil)
		mt.On("FindByPeriod", ctx, "2026-05").Return(nil, nil)
		cps.On("GetOrCreateSingleton", ctx).Return(cop, nil)
		mt.On("Create", ctx, mock.AnythingOfType("entities.MeterReading")).Return(nil)
		al.On("ResolveByPrefix", ctx, mock.AnythingOfType("string")).Return(nil)

		m, err := uc.Create(ctx, validInput())
		Convey("It writes a fresh reading and resolves the missing-reading alert", func() {
			So(err, ShouldBeNil)
			So(m.Period, ShouldEqual, "2026-05")
			So(m.CoproID, ShouldEqual, "c1")
			al.AssertCalled(t, "ResolveByPrefix", ctx, "monthly_meter_reading:2026-05:")
		})
	})

	Convey("Rejects a malformed period", t, func() {
		ctx := context.Background()
		uc, _, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		in := validInput()
		in.Period = "2026-13"
		_, err := uc.Create(ctx, in)
		So(err, ShouldNotBeNil)
	})

	Convey("Rejects a roll-back vs. the prior period", t, func() {
		ctx := context.Background()
		uc, mt, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		prior := &entities.MeterReading{Period: "2026-04", RDCM3: 250.000}
		mt.On("FindPriorPeriod", ctx, "2026-05").Return(prior, nil)
		_, err := uc.Create(ctx, validInput()) // RDC=200 < 250 prior
		So(err, ShouldNotBeNil)
	})

	Convey("Rejects a duplicate period", t, func() {
		ctx := context.Background()
		uc, mt, _, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		mt.On("FindPriorPeriod", ctx, "2026-05").Return(nil, nil)
		mt.On("FindByPeriod", ctx, "2026-05").Return(&entities.MeterReading{Period: "2026-05"}, nil)
		_, err := uc.Create(ctx, validInput())
		So(err, ShouldNotBeNil)
	})
}

// ─── Delete ─────────────────────────────────────────────────────────

func TestDelete(t *testing.T) {
	Convey("Refuses to delete a reading still referenced by an expense", t, func() {
		ctx := context.Background()
		uc, mt, exp, foy, _, _ := newUC()
		foy.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foy.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(premier, nil)
		mt.On("FindByPeriod", ctx, "2026-05").Return(&entities.MeterReading{Period: "2026-05"}, nil)
		exp.On("CountByMeterReadingPeriod", ctx, "2026-05").Return(2, nil)

		err := uc.Delete(ctx, "2026-05", "uid-rdc")
		So(err, ShouldNotBeNil)
	})
}
