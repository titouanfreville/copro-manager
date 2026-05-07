package firebase

import (
	"context"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
)

// App is the minimal Firebase app surface we depend on.
type App interface {
	Auth(ctx context.Context) (*auth.Client, error)
}

// AuthClient describes the Firebase Auth operations the API uses.
// Defining it as an interface keeps the middleware unit-testable.
type AuthClient interface {
	VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error)
	GetUser(ctx context.Context, uid string) (*auth.UserRecord, error)
}

// NewApp creates the root Firebase app. Credentials are picked up from
// Application Default Credentials (gcloud auth application-default login locally,
// service account on Cloud Run).
func NewApp() (App, error) {
	return firebase.NewApp(context.Background(), nil)
}

// NewAuthClient returns a Firebase Auth client bound to the given app.
func NewAuthClient(app App) (AuthClient, error) {
	return app.Auth(context.Background())
}

// NewAdminClient returns the concrete *auth.Client used by admin-only adapters
// that need write operations (CreateUser, GetUserByEmail) the AuthClient
// interface intentionally does not expose to the rest of the app.
func NewAdminClient(app App) (*auth.Client, error) {
	return app.Auth(context.Background())
}
