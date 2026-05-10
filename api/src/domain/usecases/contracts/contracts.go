// Package contracts owns the long-lived service-agreement layer:
// insurance, syndic, energy, maintenance. The four public methods
// (List, Create, Update, Delete) read top-to-bottom as orchestration —
// authorize → validate → build → store — with the technical concerns
// (input rules, normalization, copro stamping) delegated to siblings:
//
//   - validation lives in adapters/validators/contracts.go
//   - entity construction lives in build.go
//
// A non-technical reader should be able to follow the create/update
// flows here without paging through helpers.
package contracts

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/core/authz"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// CreateInput is the route-layer DTO. Its only responsibility on top
// of the entity draft is carrying the actor UID for authorization.
type CreateInput struct {
	ActorUserID string
	entities.ContractDraft
}

// UpdateInput mirrors CreateInput so adding update-only fields later
// stays a one-line change.
type UpdateInput = CreateInput

// Usecases is the contracts domain contract.
type Usecases interface {
	List(ctx context.Context, actorUserID string) ([]entities.Contract, error)
	Create(ctx context.Context, in CreateInput) (*entities.Contract, error)
	Update(ctx context.Context, id string, in UpdateInput) (*entities.Contract, error)
	Delete(ctx context.Context, id, actorUserID string) error
}

// AlertsHook is the narrow contract this package needs from the
// alerts usecase: resolving contract_expiring alerts when the parent
// contract is deleted so the feed doesn't deep-link to a missing
// resource.
type AlertsHook interface {
	ResolveContractExpiring(ctx context.Context, contractID string) error
}

type usecases struct {
	logger    *zap.Logger
	contracts interfaces.ContractsStore
	foyers    interfaces.FoyersStore
	documents interfaces.DocumentsStore
	validator interfaces.ContractValidator
	alerts    AlertsHook
	builder   *builder
}

// New builds a contracts usecase. `alerts` may be nil during tests —
// the delete-cascade hook is guarded.
func New(
	logger *zap.Logger,
	contracts interfaces.ContractsStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	documents interfaces.DocumentsStore,
	validator interfaces.ContractValidator,
	alerts AlertsHook,
) Usecases {
	return &usecases{
		logger:    logger.Named("usecases.contracts"),
		contracts: contracts,
		foyers:    foyers,
		documents: documents,
		validator: validator,
		alerts:    alerts,
		builder:   newBuilder(copros, time.Now),
	}
}

// List returns every contract in the copro. Foyer-membership gated.
func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.Contract, error) {
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	return uc.contracts.List(ctx)
}

// Create validates the input, builds a fresh Contract, persists it.
func (uc *usecases) Create(ctx context.Context, in CreateInput) (*entities.Contract, error) {
	log := uc.logger.With(zap.String("method", "Create"))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	if err := uc.validator.ValidateCreate(ctx, in.ContractDraft); err != nil {
		return nil, err
	}
	c, err := uc.builder.build(ctx, in.ContractDraft)
	if err != nil {
		return nil, fmt.Errorf("build contract: %w", err)
	}
	if err := uc.contracts.Create(ctx, c); err != nil {
		log.Error("store create failed", zap.Error(err))
		return nil, fmt.Errorf("create contract: %w", err)
	}
	log.Info("Success", zap.String("contract_id", c.ID))
	return &c, nil
}

// Update applies a validated draft onto the existing contract and
// persists. Identity (ID, CoproID, CreatedAt) is preserved.
func (uc *usecases) Update(ctx context.Context, id string, in UpdateInput) (*entities.Contract, error) {
	log := uc.logger.With(zap.String("method", "Update"), zap.String("contract_id", id))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	existing, err := uc.contracts.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find contract: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: contract %q", domainerrors.ErrNotFound, id)
	}
	if err := uc.validator.ValidateUpdate(ctx, in.ContractDraft); err != nil {
		return nil, err
	}
	updated := uc.builder.rebuild(*existing, in.ContractDraft)
	if err := uc.contracts.Update(ctx, updated); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update contract: %w", err)
	}
	log.Info("Success")
	return &updated, nil
}

// Delete refuses when documents still reference the contract, then
// removes the row and clears any outstanding contract_expiring alerts.
func (uc *usecases) Delete(ctx context.Context, id, actorUserID string) error {
	log := uc.logger.With(zap.String("method", "Delete"), zap.String("contract_id", id))

	if err := uc.authorize(ctx, actorUserID); err != nil {
		return err
	}
	existing, err := uc.contracts.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("find contract: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%w: contract %q", domainerrors.ErrNotFound, id)
	}
	if err := uc.refuseIfLinkedDocs(ctx, id); err != nil {
		return err
	}
	if err := uc.contracts.Delete(ctx, id); err != nil {
		log.Error("delete failed", zap.Error(err))
		return fmt.Errorf("delete contract: %w", err)
	}
	uc.resolveAlerts(ctx, log, id)
	log.Info("Success")
	return nil
}

func (uc *usecases) authorize(ctx context.Context, actorUserID string) error {
	return authz.RequireFoyerMember(ctx, uc.foyers, actorUserID)
}

// refuseIfLinkedDocs implements the cascade-rejection: a contract
// can't be deleted while documents still back-reference it (the user
// must unlink them first). Mirrors the categories-delete pattern.
func (uc *usecases) refuseIfLinkedDocs(ctx context.Context, contractID string) error {
	if uc.documents == nil {
		return nil
	}
	count, err := uc.documents.CountByLinkedContract(ctx, contractID)
	if err != nil {
		return fmt.Errorf("count linked documents: %w", err)
	}
	if count > 0 {
		return entities.ValidationError{
			Key:     "contract",
			Message: fmt.Sprintf("encore liée à %d document(s) — détache-les d'abord", count),
		}
	}
	return nil
}

// resolveAlerts is best-effort — a transient failure here doesn't
// undo the delete, just leaves alerts in the feed pointing at a
// missing contract until the next prefix-resolve sweep.
func (uc *usecases) resolveAlerts(ctx context.Context, log *zap.Logger, contractID string) {
	if uc.alerts == nil {
		return
	}
	if err := uc.alerts.ResolveContractExpiring(ctx, contractID); err != nil {
		log.Warn("resolve contract_expiring alerts failed", zap.Error(err))
	}
}
