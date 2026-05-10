// Package authz centralizes authorization checks shared across
// usecases. The foyer-membership check is the single rule every
// foyer-facing mutation runs through, so it lives here rather than
// being copy-pasted into each usecase.
package authz

import (
	"context"
	"fmt"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// RequireFoyerMember rejects callers that aren't members of either
// foyer in the copro. Empty `actorUserID` short-circuits as allowed —
// admin / cron / CSV-import paths pass an empty actor and are gated
// upstream by the AdminKey middleware.
//
// Returns:
//   - nil — actor allowed
//   - entities.AuthorizationError{Code:"not_foyer_member"} — actor isn't a member
//   - wrapped store error — Firestore lookup failed
//   - wrapped ErrNotFound — one or both foyers missing (bootstrap state)
func RequireFoyerMember(ctx context.Context, foyers interfaces.FoyersStore, actorUserID string) error {
	if actorUserID == "" {
		return nil
	}
	rdc, premier, err := loadBoth(ctx, foyers)
	if err != nil {
		return err
	}
	if isMember(rdc, actorUserID) || isMember(premier, actorUserID) {
		return nil
	}
	return entities.AuthorizationError{Code: "not_foyer_member"}
}

// LoadBothFoyers returns the RDC and 1er foyer in one call. Used by
// usecases that need both sides for downstream logic (share
// computation, recipient resolution).
func LoadBothFoyers(ctx context.Context, foyers interfaces.FoyersStore) (rdc, premier *entities.Foyer, err error) {
	return loadBoth(ctx, foyers)
}

func loadBoth(ctx context.Context, foyers interfaces.FoyersStore) (*entities.Foyer, *entities.Foyer, error) {
	rdc, err := foyers.FindByFloor(ctx, entities.FoyerFloorRDC)
	if err != nil {
		return nil, nil, fmt.Errorf("find rdc: %w", err)
	}
	premier, err := foyers.FindByFloor(ctx, entities.FoyerFloor1er)
	if err != nil {
		return nil, nil, fmt.Errorf("find 1er: %w", err)
	}
	if rdc == nil || premier == nil {
		return nil, nil, fmt.Errorf("%w: both foyers must exist", domainerrors.ErrNotFound)
	}
	return rdc, premier, nil
}

func isMember(f *entities.Foyer, uid string) bool {
	for _, mid := range f.MemberIDs {
		if mid == uid {
			return true
		}
	}
	return false
}

// IsMemberOf returns true when the UID belongs to either of the
// supplied foyers. Exported for usecases that have already loaded the
// foyer pair (via LoadBothFoyers) and want to gate without redoing
// the lookup.
func IsMemberOf(rdc, premier *entities.Foyer, uid string) bool {
	return isMember(rdc, uid) || isMember(premier, uid)
}
