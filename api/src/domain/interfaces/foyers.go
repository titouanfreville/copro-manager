// Package interfaces collects the contracts the domain expects its outer
// layers (adapters, services) to satisfy. Domain code only depends on these,
// keeping it free of Firestore / Firebase / chi imports.
package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// FoyersStore persists foyers. Implementations must be transparently
// idempotent on Floor: FindByFloor returns nil + nil when no doc exists.
type FoyersStore interface {
	FindByFloor(ctx context.Context, floor entities.FoyerFloor) (*entities.Foyer, error)
	FindByID(ctx context.Context, id string) (*entities.Foyer, error)
	Create(ctx context.Context, foyer entities.Foyer) error
	List(ctx context.Context) ([]entities.Foyer, error)
	// AddMember atomically appends a User.ID to the foyer's MemberIDs slice
	// (idempotent at the storage layer — duplicate appends are no-ops).
	AddMember(ctx context.Context, foyerID, userID string) error
	// UpdateParts overwrites the foyer's Parts value. The caller is responsible
	// for invariants spanning multiple foyers (Σ parts == TotalParts).
	UpdateParts(ctx context.Context, foyerID string, parts int) error
}

// CoprosStore returns the singleton Copro and creates it on demand.
// The admin-driven foyer creation flow needs the CoproID to bind a new
// foyer, but never edits the copro itself.
type CoprosStore interface {
	GetOrCreateSingleton(ctx context.Context) (*entities.Copro, error)
}

// UsersStore persists the User entity. Doc IDs are the Firebase UID — there
// is no separate identifier namespace. FindByID returns (nil, nil) when no
// doc exists for that UID (it may exist in Firebase Auth but lack a
// metadata doc here yet).
type UsersStore interface {
	FindByID(ctx context.Context, id string) (*entities.User, error)
	Create(ctx context.Context, user entities.User) error
	List(ctx context.Context) ([]entities.User, error)
	ListByIDs(ctx context.Context, ids []string) ([]entities.User, error)
}

// AuthProvisioner manages Firebase Auth users on behalf of admin flows.
type AuthProvisioner interface {
	// GetOrCreateUserByEmail returns the UID of an existing user or creates one
	// with the supplied display name and a freshly generated password. When a
	// new user is provisioned, the generated password is returned exactly once;
	// for an existing user, password is empty.
	GetOrCreateUserByEmail(ctx context.Context, email, displayName string) (uid string, password string, err error)

	// PasswordResetLink mints a one-shot password-reset URL for the given
	// email. The admin operator copies and sends it via any channel.
	PasswordResetLink(ctx context.Context, email string) (string, error)
}

// UsersService is the slice of the users domain that other usecases (foyers)
// depend on for member resolution. Domain-level contract — kept narrow on
// purpose so foyers don't pull in the full users surface.
type UsersService interface {
	// GetOrCreateByEmail returns the User. The second return value flags
	// whether THIS call provisioned a brand-new Firebase + DB user (true)
	// versus reusing an existing one (false). Callers that need to issue a
	// password-reset link as part of onboarding key off this flag.
	GetOrCreateByEmail(ctx context.Context, email, displayName string) (user *entities.User, created bool, err error)
	FindByID(ctx context.Context, id string) (*entities.User, error)
	ListByIDs(ctx context.Context, ids []string) ([]entities.User, error)
	// ResetPassword mints a Firebase one-shot reset URL for the given user.
	// Used by the onboarding flow to hand the new user a self-serve way to
	// set their password — we never expose the auto-generated random
	// password we minted at provisioning time.
	ResetPassword(ctx context.Context, userID string) (string, error)
}
