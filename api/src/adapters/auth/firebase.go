// Package auth bridges the domain's AuthProvisioner contract to Firebase Auth.
//
// It is the only place in the application allowed to mint Firebase users —
// other code uses Firebase tokens to verify already-existing identities.
package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	fbauth "firebase.google.com/go/v4/auth"

	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// Password generator. Targets Firebase's strictest password policy: at least
// one lowercase, one uppercase, one digit, one special character. We pick
// one character from each class up front, fill the rest from the union, and
// crypto-shuffle so the guaranteed-class chars don't sit in fixed positions.
const (
	pwdLowers   = "abcdefghijkmnopqrstuvwxyz"
	pwdUppers   = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	pwdDigits   = "23456789"
	pwdSpecials = "!@#$%^&*-_+="
	pwdLength   = 16
)

// FirebaseProvisioner implements interfaces.AuthProvisioner via the Firebase
// Admin SDK. Construction is plain — no logger field, no mutex — because the
// underlying auth.Client is goroutine-safe and any contextual logging is the
// caller's job.
type FirebaseProvisioner struct {
	client *fbauth.Client
}

// NewFirebaseProvisioner returns an AuthProvisioner backed by Firebase Auth.
func NewFirebaseProvisioner(client *fbauth.Client) interfaces.AuthProvisioner {
	return &FirebaseProvisioner{client: client}
}

// GetOrCreateUserByEmail looks up an account by email; if absent it creates
// one with a freshly generated password (returned to the caller exactly
// once). Existing accounts return their UID with an empty password.
func (p *FirebaseProvisioner) GetOrCreateUserByEmail(ctx context.Context, email, displayName string) (string, string, error) {
	user, err := p.client.GetUserByEmail(ctx, email)
	if err == nil {
		return user.UID, "", nil
	}
	if !fbauth.IsUserNotFound(err) {
		return "", "", fmt.Errorf("get user by email: %w", err)
	}

	password, err := generatePassword()
	if err != nil {
		return "", "", fmt.Errorf("generate password: %w", err)
	}

	created, err := p.client.CreateUser(ctx, (&fbauth.UserToCreate{}).
		Email(email).
		EmailVerified(true).
		Password(password).
		DisplayName(displayName).
		Disabled(false))
	if err != nil {
		return "", "", fmt.Errorf("create user: %w", err)
	}

	return created.UID, password, nil
}

// PasswordResetLink mints a Firebase one-shot reset URL for the given email.
func (p *FirebaseProvisioner) PasswordResetLink(ctx context.Context, email string) (string, error) {
	link, err := p.client.PasswordResetLink(ctx, email)
	if err != nil {
		return "", fmt.Errorf("password reset link: %w", err)
	}
	return link, nil
}

func generatePassword() (string, error) {
	classes := []string{pwdLowers, pwdUppers, pwdDigits, pwdSpecials}
	pwd := make([]byte, 0, pwdLength)
	for _, c := range classes {
		ch, err := randChar(c)
		if err != nil {
			return "", err
		}
		pwd = append(pwd, ch)
	}

	all := pwdLowers + pwdUppers + pwdDigits + pwdSpecials
	for len(pwd) < pwdLength {
		ch, err := randChar(all)
		if err != nil {
			return "", err
		}
		pwd = append(pwd, ch)
	}

	if err := cryptoShuffle(pwd); err != nil {
		return "", err
	}
	return string(pwd), nil
}

func randChar(set string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
	if err != nil {
		return 0, fmt.Errorf("random char: %w", err)
	}
	return set[n.Int64()], nil
}

func cryptoShuffle(b []byte) error {
	for i := len(b) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return fmt.Errorf("shuffle: %w", err)
		}
		j := n.Int64()
		b[i], b[j] = b[j], b[i]
	}
	return nil
}
