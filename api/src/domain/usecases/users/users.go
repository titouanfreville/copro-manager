// Package users exposes the User domain — our DB-side projection of an
// authenticated person. The User.ID is the Firebase UID; this package owns
// the get-or-create flow that keeps Firebase Auth and the users collection
// in sync, plus admin-side reset.
package users

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// minPasswordLen is the minimum password length we accept on the admin
// set-password path. Firebase's own default is 6; we bump to 8 so admin
// shortcuts don't slip below something the user could plausibly guess.
const minPasswordLen = 8

// Usecases is the users domain contract.
type Usecases interface {
	// GetOrCreateByEmail returns the User for a given email — creating both
	// the Firebase Auth account and our User doc as needed. The boolean is
	// true only when a brand-new Firebase user was minted by this call.
	// The provisioner-supplied password is intentionally NOT surfaced —
	// callers route brand-new users through ResetPassword instead.
	GetOrCreateByEmail(ctx context.Context, email, displayName string) (*entities.User, bool, error)
	FindByID(ctx context.Context, id string) (*entities.User, error)
	List(ctx context.Context) ([]entities.User, error)
	ListByIDs(ctx context.Context, ids []string) ([]entities.User, error)
	// ResetPassword mints a Firebase one-shot reset link for the given user's
	// email. The admin operator forwards it via any channel.
	ResetPassword(ctx context.Context, userID string) (string, error)
	// SetPassword writes a chosen password to the user's Firebase Auth
	// account. Admin escape hatch for the "user is on the phone, no time
	// to wait for the reset email" case. Returns ValidationError when the
	// password is too short.
	SetPassword(ctx context.Context, userID, password string) error
}

type usecases struct {
	logger *zap.Logger
	users  interfaces.UsersStore
	auth   interfaces.AuthProvisioner
}

// New builds a users usecase wired to the supplied store + provisioner.
func New(logger *zap.Logger, users interfaces.UsersStore, auth interfaces.AuthProvisioner) Usecases {
	return &usecases{
		logger: logger.Named("usecases.users"),
		users:  users,
		auth:   auth,
	}
}

func (uc *usecases) GetOrCreateByEmail(ctx context.Context, email, displayName string) (*entities.User, bool, error) {
	// Don't bind the email on the parent log — NFR16 forbids personal data in
	// logs at INFO level or higher. Identify by UID once the provisioner
	// returns one.
	log := uc.logger.With(zap.String("method", "GetOrCreateByEmail"))

	if !looksLikeEmail(email) {
		log.Warn("validation failed: invalid email")
		return nil, false, entities.ValidationError{Key: "email", Message: "invalid email"}
	}
	if strings.TrimSpace(displayName) == "" {
		log.Warn("validation failed: missing display name")
		return nil, false, entities.ValidationError{Key: "display_name", Message: "required"}
	}

	uid, password, err := uc.auth.GetOrCreateUserByEmail(ctx, email, displayName)
	if err != nil {
		log.Error("auth provisioning failed", zap.Error(err))
		return nil, false, fmt.Errorf("provision firebase user: %w", err)
	}
	// `password` is the random one-shot value Firebase minted for new users.
	// We never return it to callers (NFR13 spirit + audit AC) — onboarding
	// happens via ResetPassword.
	_ = password

	existing, err := uc.users.FindByID(ctx, uid)
	if err != nil {
		log.Error("user lookup failed", zap.String("uid", uid), zap.Error(err))
		return nil, false, fmt.Errorf("lookup user: %w", err)
	}
	if existing != nil {
		log.Info("Success", zap.String("uid", uid), zap.Bool("created", false))
		return existing, false, nil
	}

	user := entities.User{
		ID:          uid,
		Email:       email,
		DisplayName: displayName,
	}
	if err := uc.users.Create(ctx, user); err != nil {
		log.Error("user create failed", zap.String("uid", uid), zap.Error(err))
		return nil, false, fmt.Errorf("create user: %w", err)
	}

	log.Info("Success", zap.String("uid", uid), zap.Bool("created", true))
	return &user, true, nil
}

func (uc *usecases) FindByID(ctx context.Context, id string) (*entities.User, error) {
	return uc.users.FindByID(ctx, id)
}

func (uc *usecases) List(ctx context.Context) ([]entities.User, error) {
	return uc.users.List(ctx)
}

func (uc *usecases) ListByIDs(ctx context.Context, ids []string) ([]entities.User, error) {
	return uc.users.ListByIDs(ctx, ids)
}

func (uc *usecases) SetPassword(ctx context.Context, userID, password string) error {
	log := uc.logger.With(zap.String("method", "SetPassword"), zap.String("user_id", userID))

	if len(password) < minPasswordLen {
		log.Warn("validation failed: password too short")
		return entities.ValidationError{Key: "password", Message: fmt.Sprintf("min %d caractères", minPasswordLen)}
	}

	user, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		log.Error("user lookup failed", zap.Error(err))
		return fmt.Errorf("lookup user: %w", err)
	}
	if user == nil {
		log.Warn("user not found")
		return entities.ValidationError{Key: "user_id", Message: "not found"}
	}

	if err := uc.auth.SetPassword(ctx, user.Email, password); err != nil {
		log.Error("set password failed", zap.Error(err))
		return fmt.Errorf("set password: %w", err)
	}

	log.Info("Success")
	return nil
}

func (uc *usecases) ResetPassword(ctx context.Context, userID string) (string, error) {
	log := uc.logger.With(zap.String("method", "ResetPassword"), zap.String("user_id", userID))

	user, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		log.Error("user lookup failed", zap.Error(err))
		return "", fmt.Errorf("lookup user: %w", err)
	}
	if user == nil {
		log.Warn("user not found")
		return "", entities.ValidationError{Key: "user_id", Message: "not found"}
	}

	link, err := uc.auth.PasswordResetLink(ctx, user.Email)
	if err != nil {
		log.Error("password reset link failed", zap.Error(err))
		return "", fmt.Errorf("password reset link: %w", err)
	}

	log.Info("Success")
	return link, nil
}

func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	return strings.IndexByte(s[at+1:], '.') > 0
}
