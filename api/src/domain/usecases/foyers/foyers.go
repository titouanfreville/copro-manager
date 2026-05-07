// Package foyers exposes the business logic for managing the copro's foyers.
//
// The MVP scope is intentionally narrow: create a foyer (with an initial
// member), list foyers (enriched with their User records), add a member,
// and update the tantième share. Member-removal and foyer-deletion are out
// of scope until a real need surfaces.
package foyers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// MemberInput identifies a member to attach to a foyer. Exactly one of
// UserID or (Email + DisplayName) must be set:
//   - UserID picks an existing user from our DB.
//   - Email + DisplayName provisions a new Firebase + DB user when needed.
type MemberInput struct {
	UserID      string
	Email       string
	DisplayName string
}

// CreateInput captures the fields for a brand-new foyer. The Member is the
// foyer's first attached user — every foyer needs at least one member.
type CreateInput struct {
	Floor  entities.FoyerFloor
	Name   string
	Parts  int
	Member MemberInput
}

// CreateResult is what Create returns: the new foyer plus an optional
// password-reset link for the freshly provisioned member. The link is set
// only when a brand-new Firebase user was minted by this call; it lets the
// admin onboard the user without ever surfacing the random initial password.
type CreateResult struct {
	Foyer     entities.Foyer
	ResetLink string
}

// AddMemberResult is what AddMember returns: the updated foyer plus an
// optional reset link (same semantics as CreateResult.ResetLink).
type AddMemberResult struct {
	Foyer     entities.Foyer
	ResetLink string
}

// ListedFoyer is a foyer enriched with the full User record of each member.
type ListedFoyer struct {
	entities.Foyer
	Members []entities.User `json:"members"`
}

// Usecases is the foyers domain contract.
type Usecases interface {
	Create(ctx context.Context, in CreateInput) (*CreateResult, error)
	List(ctx context.Context) ([]ListedFoyer, error)
	AddMember(ctx context.Context, foyerID string, member MemberInput) (*AddMemberResult, error)
	UpdateParts(ctx context.Context, foyerID string, parts int) error
}

type usecases struct {
	logger *zap.Logger
	foyers interfaces.FoyersStore
	copros interfaces.CoprosStore
	users  interfaces.UsersService
}

// New builds a foyers usecase wired to the supplied stores and the users
// service for member resolution.
func New(
	logger *zap.Logger,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	users interfaces.UsersService,
) Usecases {
	return &usecases{
		logger: logger.Named("usecases.foyers"),
		foyers: foyers,
		copros: copros,
		users:  users,
	}
}

const minPartsPerFoyer = 0

// Create validates the input, ensures the initial member is provisioned,
// and writes a new Foyer doc. Refuses to overwrite an existing foyer for
// the same floor.
func (uc *usecases) Create(ctx context.Context, in CreateInput) (*CreateResult, error) {
	log := uc.logger.With(
		zap.String("method", "Create"),
		zap.String("floor", string(in.Floor)),
	)

	if err := validateCreate(in); err != nil {
		log.Warn("validation failed", zap.Error(err))
		return nil, err
	}

	existing, err := uc.foyers.FindByFloor(ctx, in.Floor)
	if err != nil {
		log.Error("foyer lookup failed", zap.Error(err))
		return nil, fmt.Errorf("find foyer by floor: %w", err)
	}
	if existing != nil {
		log.Warn("foyer already exists for this floor")
		return nil, fmt.Errorf("%w: foyer for floor %q", domainerrors.ErrAlreadyExists, in.Floor)
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		log.Error("copro singleton failed", zap.Error(err))
		return nil, fmt.Errorf("get copro: %w", err)
	}

	user, created, err := uc.resolveMember(ctx, in.Member)
	if err != nil {
		log.Error("member resolution failed", zap.Error(err))
		return nil, err
	}

	// Use the floor literal as the doc ID so two concurrent creates for the
	// same floor can't both succeed — Firestore atomically rejects the
	// second `Doc(<floor>).Create(...)` with AlreadyExists.
	foyer := entities.Foyer{
		ID:        string(in.Floor),
		CoproID:   copro.ID,
		Floor:     in.Floor,
		Name:      in.Name,
		Parts:     in.Parts,
		MemberIDs: []string{user.ID},
	}

	if err := uc.foyers.Create(ctx, foyer); err != nil {
		// Race: another caller won. Surface as a clean conflict.
		if errors.Is(err, domainerrors.ErrAlreadyExists) {
			log.Warn("foyer create lost race")
			return nil, fmt.Errorf("%w: foyer for floor %q", domainerrors.ErrAlreadyExists, in.Floor)
		}
		log.Error("foyer doc create failed", zap.Error(err))
		return nil, fmt.Errorf("create foyer doc: %w", err)
	}

	resetLink, err := uc.maybeResetLink(ctx, user.ID, created)
	if err != nil {
		// Don't fail the whole flow — the foyer + user exist; the admin
		// can re-trigger ResetPassword via the dedicated endpoint.
		log.Warn("reset link minting failed", zap.String("user_id", user.ID), zap.Error(err))
	}

	log.Info("Success",
		zap.String("foyer_id", foyer.ID),
		zap.String("copro_id", copro.ID),
		zap.String("user_id", user.ID),
		zap.Bool("user_created", created),
	)

	return &CreateResult{Foyer: foyer, ResetLink: resetLink}, nil
}

// AddMember attaches a user to an existing foyer. Refuses to attach the
// same user twice — admin operations are intentional.
func (uc *usecases) AddMember(ctx context.Context, foyerID string, member MemberInput) (*AddMemberResult, error) {
	log := uc.logger.With(
		zap.String("method", "AddMember"),
		zap.String("foyer_id", foyerID),
	)

	foyer, err := uc.foyers.FindByID(ctx, foyerID)
	if err != nil {
		log.Error("foyer lookup failed", zap.Error(err))
		return nil, fmt.Errorf("find foyer by id: %w", err)
	}
	if foyer == nil {
		log.Warn("foyer not found")
		return nil, fmt.Errorf("%w: foyer %q", domainerrors.ErrNotFound, foyerID)
	}

	user, created, err := uc.resolveMember(ctx, member)
	if err != nil {
		log.Error("member resolution failed", zap.Error(err))
		return nil, err
	}

	for _, mid := range foyer.MemberIDs {
		if mid == user.ID {
			log.Warn("user already member of foyer")
			return nil, fmt.Errorf("%w: user already member", domainerrors.ErrAlreadyExists)
		}
	}

	if err := uc.foyers.AddMember(ctx, foyerID, user.ID); err != nil {
		log.Error("add member failed", zap.Error(err))
		return nil, fmt.Errorf("add member: %w", err)
	}

	foyer.MemberIDs = append(foyer.MemberIDs, user.ID)

	resetLink, err := uc.maybeResetLink(ctx, user.ID, created)
	if err != nil {
		log.Warn("reset link minting failed", zap.String("user_id", user.ID), zap.Error(err))
	}

	log.Info("Success", zap.String("user_id", user.ID), zap.Bool("user_created", created))

	return &AddMemberResult{Foyer: *foyer, ResetLink: resetLink}, nil
}

// maybeResetLink mints a one-shot password-reset URL for newly provisioned
// users. We never return the auto-generated random password from the auth
// provisioner — onboarding happens via the reset link instead so the random
// password never lands in HTTP responses, browser DevTools, or proxy logs.
func (uc *usecases) maybeResetLink(ctx context.Context, userID string, created bool) (string, error) {
	if !created {
		return "", nil
	}
	return uc.users.ResetPassword(ctx, userID)
}

// UpdateParts sets the foyer's tantième share. Validates the per-foyer bounds
// (0 ≤ parts ≤ copro.TotalParts). The cross-foyer invariant
// (Σ parts == Copro.TotalParts) is the operator's responsibility — a typo on
// one foyer is caught at expense-create time when computeShares enforces it.
func (uc *usecases) UpdateParts(ctx context.Context, foyerID string, parts int) error {
	log := uc.logger.With(zap.String("method", "UpdateParts"), zap.String("foyer_id", foyerID), zap.Int("parts", parts))

	if parts < minPartsPerFoyer {
		log.Warn("validation failed: negative parts")
		return entities.ValidationError{Key: "parts", Message: "must be >= 0"}
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		log.Error("copro lookup failed", zap.Error(err))
		return fmt.Errorf("copro lookup: %w", err)
	}
	if parts > copro.TotalParts {
		log.Warn("validation failed: parts above total_parts", zap.Int("total_parts", copro.TotalParts))
		return entities.ValidationError{Key: "parts", Message: fmt.Sprintf("must be <= copro.total_parts (%d)", copro.TotalParts)}
	}

	foyer, err := uc.foyers.FindByID(ctx, foyerID)
	if err != nil {
		log.Error("foyer lookup failed", zap.Error(err))
		return fmt.Errorf("find foyer by id: %w", err)
	}
	if foyer == nil {
		log.Warn("foyer not found")
		return fmt.Errorf("%w: foyer %q", domainerrors.ErrNotFound, foyerID)
	}

	if err := uc.foyers.UpdateParts(ctx, foyerID, parts); err != nil {
		log.Error("update parts failed", zap.Error(err))
		return fmt.Errorf("update parts: %w", err)
	}

	log.Info("Success")
	return nil
}

// List returns every foyer with its members enriched from the users store.
// Members not found (orphan UID) are silently skipped.
func (uc *usecases) List(ctx context.Context) ([]ListedFoyer, error) {
	log := uc.logger.With(zap.String("method", "List"))

	foyers, err := uc.foyers.List(ctx)
	if err != nil {
		log.Error("foyer list failed", zap.Error(err))
		return nil, fmt.Errorf("list foyers: %w", err)
	}

	idSet := map[string]struct{}{}
	for _, f := range foyers {
		for _, mid := range f.MemberIDs {
			idSet[mid] = struct{}{}
		}
	}
	allIDs := make([]string, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}

	users, err := uc.users.ListByIDs(ctx, allIDs)
	if err != nil {
		log.Error("members lookup failed", zap.Error(err))
		return nil, fmt.Errorf("list users: %w", err)
	}
	usersByID := make(map[string]entities.User, len(users))
	for _, u := range users {
		usersByID[u.ID] = u
	}

	out := make([]ListedFoyer, 0, len(foyers))
	for _, f := range foyers {
		members := make([]entities.User, 0, len(f.MemberIDs))
		for _, mid := range f.MemberIDs {
			if u, ok := usersByID[mid]; ok {
				members = append(members, u)
			}
		}
		out = append(out, ListedFoyer{Foyer: f, Members: members})
	}

	log.Info("Success", zap.Int("count", len(out)))
	return out, nil
}

// resolveMember fans MemberInput into a User: if UserID is set we trust it
// and look up the existing record; otherwise we get-or-create from email.
// The boolean reports whether a brand-new user was provisioned by this
// call — callers gate the reset-link minting on it.
func (uc *usecases) resolveMember(ctx context.Context, m MemberInput) (*entities.User, bool, error) {
	if m.UserID != "" {
		user, err := uc.users.FindByID(ctx, m.UserID)
		if err != nil {
			return nil, false, fmt.Errorf("find user by id: %w", err)
		}
		if user == nil {
			return nil, false, entities.ValidationError{Key: "user_id", Message: "not found"}
		}
		return user, false, nil
	}

	user, created, err := uc.users.GetOrCreateByEmail(ctx, m.Email, m.DisplayName)
	if err != nil {
		return nil, false, err
	}
	return user, created, nil
}

func validateCreate(in CreateInput) error {
	details := []entities.Detail{}
	if !isKnownFloor(in.Floor) {
		details = append(details, entities.Detail{Key: "floor", Message: "unknown floor"})
	}
	if strings.TrimSpace(in.Name) == "" {
		details = append(details, entities.Detail{Key: "name", Message: "required"})
	}
	if in.Parts < minPartsPerFoyer {
		details = append(details, entities.Detail{Key: "parts", Message: "must be >= 0"})
	}
	if err := validateMemberInput(in.Member); err != nil {
		details = append(details, entities.Detail{Key: "member", Message: err.Error()})
	}
	if len(details) > 0 {
		return entities.ValidationError{
			Key:     "create_foyer",
			Message: "invalid input",
			Details: details,
		}
	}
	return nil
}

func validateMemberInput(m MemberInput) error {
	if m.UserID != "" {
		return nil
	}
	if !looksLikeEmail(m.Email) {
		return fmt.Errorf("invalid email")
	}
	if strings.TrimSpace(m.DisplayName) == "" {
		return fmt.Errorf("display name required")
	}
	return nil
}

func isKnownFloor(f entities.FoyerFloor) bool {
	for _, known := range entities.AllFoyerFloors() {
		if f == known {
			return true
		}
	}
	return false
}

func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	return strings.IndexByte(s[at+1:], '.') > 0
}
