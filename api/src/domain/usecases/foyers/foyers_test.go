package foyers

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
)

type mockFoyersStore struct{ mock.Mock }

func (m *mockFoyersStore) FindByFloor(ctx context.Context, floor entities.FoyerFloor) (*entities.Foyer, error) {
	args := m.Called(ctx, floor)
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

func (m *mockFoyersStore) Create(ctx context.Context, foyer entities.Foyer) error {
	return m.Called(ctx, foyer).Error(0)
}

func (m *mockFoyersStore) List(ctx context.Context) ([]entities.Foyer, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.Foyer), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockFoyersStore) AddMember(ctx context.Context, foyerID, userID string) error {
	return m.Called(ctx, foyerID, userID).Error(0)
}

func (m *mockFoyersStore) UpdateParts(ctx context.Context, foyerID string, parts int) error {
	return m.Called(ctx, foyerID, parts).Error(0)
}

type mockCoprosStore struct{ mock.Mock }

func (m *mockCoprosStore) GetOrCreateSingleton(ctx context.Context) (*entities.Copro, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.(*entities.Copro), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockUsersService struct{ mock.Mock }

func (m *mockUsersService) GetOrCreateByEmail(ctx context.Context, email, displayName string) (*entities.User, bool, error) {
	args := m.Called(ctx, email, displayName)
	var u *entities.User
	if v := args.Get(0); v != nil {
		u = v.(*entities.User)
	}
	return u, args.Bool(1), args.Error(2)
}

func (m *mockUsersService) FindByID(ctx context.Context, id string) (*entities.User, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUsersService) ListByIDs(ctx context.Context, ids []string) ([]entities.User, error) {
	args := m.Called(ctx, ids)
	if v := args.Get(0); v != nil {
		return v.([]entities.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUsersService) ResetPassword(ctx context.Context, userID string) (string, error) {
	args := m.Called(ctx, userID)
	return args.String(0), args.Error(1)
}

func TestFoyersCreate(t *testing.T) {
	Convey("Given a Foyers usecase creating a foyer with a new member email", t, func() {
		ctx := context.Background()
		store := &mockFoyersStore{}
		copros := &mockCoprosStore{}
		usersSvc := &mockUsersService{}
		uc := New(zap.NewNop(), store, copros, usersSvc)

		validIn := CreateInput{
			Floor: entities.FoyerFloorRDC,
			Name:  "Foyer RDC",
			Parts: 500,
			Member: MemberInput{
				Email:       "rdc@example.com",
				DisplayName: "Sophie",
			},
		}

		Convey("When all dependencies succeed and no foyer exists", func() {
			store.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return((*entities.Foyer)(nil), nil)
			copros.On("GetOrCreateSingleton", ctx).Return(&entities.Copro{ID: "copro-1"}, nil)
			usersSvc.On("GetOrCreateByEmail", ctx, "rdc@example.com", "Sophie").
				Return(&entities.User{ID: "uid-123", Email: "rdc@example.com"}, true, nil)
			usersSvc.On("ResetPassword", ctx, "uid-123").Return("https://reset?token=xyz", nil)
			store.On("Create", ctx, mock.AnythingOfType("entities.Foyer")).Return(nil)

			result, err := uc.Create(ctx, validIn)

			Convey("Then it returns the new foyer with the password-reset link", func() {
				So(err, ShouldBeNil)
				So(result.Foyer.MemberIDs, ShouldResemble, []string{"uid-123"})
				So(result.Foyer.Parts, ShouldEqual, 500)
				So(result.ResetLink, ShouldEqual, "https://reset?token=xyz")
			})
		})

		Convey("When a foyer already exists for this floor", func() {
			store.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(&entities.Foyer{ID: "existing"}, nil)

			result, err := uc.Create(ctx, validIn)

			Convey("Then it returns ErrAlreadyExists and skips downstream calls", func() {
				So(result, ShouldBeNil)
				So(errors.Is(err, domainerrors.ErrAlreadyExists), ShouldBeTrue)
				usersSvc.AssertNotCalled(t, "GetOrCreateByEmail", mock.Anything, mock.Anything, mock.Anything)
			})
		})

		Convey("When the member email is malformed", func() {
			bad := validIn
			bad.Member.Email = "not-an-email"

			result, err := uc.Create(ctx, bad)

			Convey("Then it returns a ValidationError without touching stores", func() {
				So(result, ShouldBeNil)
				So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
				store.AssertNotCalled(t, "FindByFloor", mock.Anything, mock.Anything)
			})
		})
	})

	Convey("Given a Foyers usecase creating a foyer with an existing user_id", t, func() {
		ctx := context.Background()
		store := &mockFoyersStore{}
		copros := &mockCoprosStore{}
		usersSvc := &mockUsersService{}
		uc := New(zap.NewNop(), store, copros, usersSvc)

		in := CreateInput{
			Floor:  entities.FoyerFloor1er,
			Name:   "Foyer 1er",
			Parts:  500,
			Member: MemberInput{UserID: "uid-known"},
		}

		Convey("When all lookups succeed", func() {
			store.On("FindByFloor", ctx, entities.FoyerFloor1er).Return((*entities.Foyer)(nil), nil)
			copros.On("GetOrCreateSingleton", ctx).Return(&entities.Copro{ID: "copro-1"}, nil)
			usersSvc.On("FindByID", ctx, "uid-known").Return(&entities.User{ID: "uid-known"}, nil)
			store.On("Create", ctx, mock.AnythingOfType("entities.Foyer")).Return(nil)

			result, err := uc.Create(ctx, in)

			Convey("Then no reset link is minted and the existing user is bound", func() {
				So(err, ShouldBeNil)
				So(result.Foyer.MemberIDs, ShouldResemble, []string{"uid-known"})
				So(result.ResetLink, ShouldEqual, "")
				usersSvc.AssertNotCalled(t, "ResetPassword", mock.Anything, mock.Anything)
			})
		})
	})
}

func TestFoyersAddMember(t *testing.T) {
	Convey("Given a Foyers usecase adding a member to an existing foyer", t, func() {
		ctx := context.Background()
		store := &mockFoyersStore{}
		copros := &mockCoprosStore{}
		usersSvc := &mockUsersService{}
		uc := New(zap.NewNop(), store, copros, usersSvc)

		Convey("When the foyer exists and the user is brand new", func() {
			store.On("FindByID", ctx, "f1").Return(&entities.Foyer{ID: "f1", MemberIDs: []string{"uid-a"}}, nil)
			usersSvc.On("GetOrCreateByEmail", ctx, "spouse@example.com", "Conjoint").
				Return(&entities.User{ID: "uid-b"}, true, nil)
			usersSvc.On("ResetPassword", ctx, "uid-b").Return("https://reset?token=abc", nil)
			store.On("AddMember", ctx, "f1", "uid-b").Return(nil)

			result, err := uc.AddMember(ctx, "f1", MemberInput{Email: "spouse@example.com", DisplayName: "Conjoint"})

			Convey("Then the member is appended and a reset link is returned", func() {
				So(err, ShouldBeNil)
				So(result.Foyer.MemberIDs, ShouldResemble, []string{"uid-a", "uid-b"})
				So(result.ResetLink, ShouldEqual, "https://reset?token=abc")
			})
		})

		Convey("When the resolved user is already a member", func() {
			store.On("FindByID", ctx, "f1").Return(&entities.Foyer{ID: "f1", MemberIDs: []string{"uid-a"}}, nil)
			usersSvc.On("GetOrCreateByEmail", ctx, "spouse@example.com", "Conjoint").
				Return(&entities.User{ID: "uid-a"}, false, nil)

			result, err := uc.AddMember(ctx, "f1", MemberInput{Email: "spouse@example.com", DisplayName: "Conjoint"})

			Convey("Then ErrAlreadyExists bubbles up without writing", func() {
				So(result, ShouldBeNil)
				So(errors.Is(err, domainerrors.ErrAlreadyExists), ShouldBeTrue)
				store.AssertNotCalled(t, "AddMember", mock.Anything, mock.Anything, mock.Anything)
			})
		})

		Convey("When the foyer does not exist", func() {
			store.On("FindByID", ctx, "ghost").Return((*entities.Foyer)(nil), nil)

			result, err := uc.AddMember(ctx, "ghost", MemberInput{Email: "x@y.fr", DisplayName: "X"})

			Convey("Then it returns ErrNotFound and skips user resolution", func() {
				So(result, ShouldBeNil)
				So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
				usersSvc.AssertNotCalled(t, "GetOrCreateByEmail", mock.Anything, mock.Anything, mock.Anything)
			})
		})
	})
}

func TestFoyersUpdateParts(t *testing.T) {
	Convey("Given a Foyers usecase updating parts", t, func() {
		ctx := context.Background()
		store := &mockFoyersStore{}
		copros := &mockCoprosStore{}
		usersSvc := &mockUsersService{}
		uc := New(zap.NewNop(), store, copros, usersSvc)

		Convey("When the foyer exists and parts is within bounds", func() {
			copros.On("GetOrCreateSingleton", ctx).Return(&entities.Copro{TotalParts: 1000}, nil)
			store.On("FindByID", ctx, "f1").Return(&entities.Foyer{ID: "f1"}, nil)
			store.On("UpdateParts", ctx, "f1", 600).Return(nil)

			err := uc.UpdateParts(ctx, "f1", 600)

			Convey("Then the store is called", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When parts is negative", func() {
			err := uc.UpdateParts(ctx, "f1", -1)

			Convey("Then ValidationError without touching the stores", func() {
				So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
				store.AssertNotCalled(t, "FindByID", mock.Anything, mock.Anything)
				copros.AssertNotCalled(t, "GetOrCreateSingleton", mock.Anything)
			})
		})

		Convey("When parts exceeds copro.total_parts", func() {
			copros.On("GetOrCreateSingleton", ctx).Return(&entities.Copro{TotalParts: 1000}, nil)

			err := uc.UpdateParts(ctx, "f1", 1500)

			Convey("Then ValidationError without touching the foyer store", func() {
				So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
				store.AssertNotCalled(t, "FindByID", mock.Anything, mock.Anything)
				store.AssertNotCalled(t, "UpdateParts", mock.Anything, mock.Anything, mock.Anything)
			})
		})
	})
}

func TestFoyersList(t *testing.T) {
	Convey("Given a Foyers usecase listing foyers", t, func() {
		ctx := context.Background()
		store := &mockFoyersStore{}
		copros := &mockCoprosStore{}
		usersSvc := &mockUsersService{}
		uc := New(zap.NewNop(), store, copros, usersSvc)

		stored := []entities.Foyer{
			{ID: "f1", Floor: entities.FoyerFloorRDC, MemberIDs: []string{"uid-a"}},
			{ID: "f2", Floor: entities.FoyerFloor1er, MemberIDs: []string{"uid-b", "uid-gone"}},
		}

		Convey("When the users service returns matches for two of three IDs", func() {
			store.On("List", ctx).Return(stored, nil)
			usersSvc.On("ListByIDs", ctx, mock.AnythingOfType("[]string")).
				Return([]entities.User{
					{ID: "uid-a", Email: "a@x.fr"},
					{ID: "uid-b", Email: "b@x.fr"},
				}, nil)

			out, err := uc.List(ctx)

			Convey("Then orphaned UIDs are skipped, others are enriched", func() {
				So(err, ShouldBeNil)
				So(out, ShouldHaveLength, 2)
				So(out[0].Members, ShouldHaveLength, 1)
				So(out[0].Members[0].Email, ShouldEqual, "a@x.fr")
				So(out[1].Members, ShouldHaveLength, 1)
				So(out[1].Members[0].ID, ShouldEqual, "uid-b")
			})
		})
	})
}
