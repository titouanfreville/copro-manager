package users

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

type mockUsersStore struct{ mock.Mock }

func (m *mockUsersStore) FindByID(ctx context.Context, id string) (*entities.User, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*entities.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUsersStore) Create(ctx context.Context, user entities.User) error {
	return m.Called(ctx, user).Error(0)
}

func (m *mockUsersStore) List(ctx context.Context) ([]entities.User, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]entities.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUsersStore) ListByIDs(ctx context.Context, ids []string) ([]entities.User, error) {
	args := m.Called(ctx, ids)
	if v := args.Get(0); v != nil {
		return v.([]entities.User), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockAuth struct{ mock.Mock }

func (m *mockAuth) GetOrCreateUserByEmail(ctx context.Context, email, displayName string) (string, string, error) {
	args := m.Called(ctx, email, displayName)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *mockAuth) PasswordResetLink(ctx context.Context, email string) (string, error) {
	args := m.Called(ctx, email)
	return args.String(0), args.Error(1)
}

func TestGetOrCreateByEmail(t *testing.T) {
	Convey("Given a Users usecase", t, func() {
		ctx := context.Background()
		store := &mockUsersStore{}
		auth := &mockAuth{}
		uc := New(zap.NewNop(), store, auth)

		Convey("When auth provisions a new Firebase user and our DB has no doc yet", func() {
			auth.On("GetOrCreateUserByEmail", ctx, "rdc@example.com", "Sophie").Return("uid-new", "p4ss", nil)
			store.On("FindByID", ctx, "uid-new").Return((*entities.User)(nil), nil)
			store.On("Create", ctx, mock.AnythingOfType("entities.User")).Return(nil)

			user, created, err := uc.GetOrCreateByEmail(ctx, "rdc@example.com", "Sophie")

			Convey("Then it creates the user doc and reports created=true", func() {
				So(err, ShouldBeNil)
				So(user.ID, ShouldEqual, "uid-new")
				So(user.Email, ShouldEqual, "rdc@example.com")
				So(created, ShouldBeTrue)
			})
		})

		Convey("When auth returns existing UID and our DB already has the doc", func() {
			auth.On("GetOrCreateUserByEmail", ctx, "known@example.com", "Sophie").Return("uid-known", "", nil)
			store.On("FindByID", ctx, "uid-known").
				Return(&entities.User{ID: "uid-known", Email: "known@example.com"}, nil)

			user, created, err := uc.GetOrCreateByEmail(ctx, "known@example.com", "Sophie")

			Convey("Then no Create is called and created=false", func() {
				So(err, ShouldBeNil)
				So(user.ID, ShouldEqual, "uid-known")
				So(created, ShouldBeFalse)
				store.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			})
		})

		Convey("When the email is malformed", func() {
			user, created, err := uc.GetOrCreateByEmail(ctx, "bad", "Sophie")

			Convey("Then validation error without calling auth", func() {
				So(user, ShouldBeNil)
				So(created, ShouldBeFalse)
				So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
				auth.AssertNotCalled(t, "GetOrCreateUserByEmail", mock.Anything, mock.Anything, mock.Anything)
			})
		})
	})
}

func TestResetPassword(t *testing.T) {
	Convey("Given a Users usecase resetting a password", t, func() {
		ctx := context.Background()
		store := &mockUsersStore{}
		auth := &mockAuth{}
		uc := New(zap.NewNop(), store, auth)

		Convey("When the user exists", func() {
			store.On("FindByID", ctx, "uid-1").Return(&entities.User{ID: "uid-1", Email: "a@x.fr"}, nil)
			auth.On("PasswordResetLink", ctx, "a@x.fr").Return("https://reset?token=xyz", nil)

			link, err := uc.ResetPassword(ctx, "uid-1")

			Convey("Then the link is returned", func() {
				So(err, ShouldBeNil)
				So(link, ShouldEqual, "https://reset?token=xyz")
			})
		})

		Convey("When the user does not exist", func() {
			store.On("FindByID", ctx, "ghost").Return((*entities.User)(nil), nil)

			link, err := uc.ResetPassword(ctx, "ghost")

			Convey("Then ValidationError, no auth call", func() {
				So(link, ShouldEqual, "")
				So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
				auth.AssertNotCalled(t, "PasswordResetLink", mock.Anything, mock.Anything)
			})
		})
	})
}
