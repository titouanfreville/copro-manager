// Package contracts owns the long-lived service-agreement layer:
// insurance, syndic, energy, maintenance, and any other contract the
// foyer needs at hand. Reads come live from Firestore via
// app/src/lib/live; only mutations stay here so name normalization,
// status validation, and the copro_id stamping live in one place.
package contracts

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const (
	nameMinLen = 2
	nameMaxLen = 120
	noteMaxLen = 2000
	urlMaxLen  = 256
	textMaxLen = 256
)

// CreateInput captures the user-facing fields for Create. ID is server-
// generated; Status defaults to `active`. Empty StartDate / EndDate
// keep the doc open-ended.
type CreateInput struct {
	ActorUserID string
	Name        string
	CategoryID  string

	Society entities.Society
	Contact entities.Contact

	StartDate time.Time
	EndDate   time.Time

	AmountCents      int
	BillingFrequency entities.BillingFrequency

	TemplateID string
	Status     entities.ContractStatus
	Note       string
}

// UpdateInput mirrors CreateInput. The usecase pulls the existing
// Contract first to preserve CreatedAt + ID; everything else is
// replaced wholesale (no patch semantics — keeps validation rules
// linear).
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
	logger     *zap.Logger
	contracts  interfaces.ContractsStore
	categories interfaces.CategoriesStore
	foyers     interfaces.FoyersStore
	copros     interfaces.CoprosStore
	documents  interfaces.DocumentsStore
	templates  interfaces.TemplatesStore
	alerts     AlertsHook
	now        func() time.Time
}

// New builds a contracts usecase. `documents` powers the delete
// cascade-rejection (refuse delete when docs are still linked);
// `templates` validates the optional `template_id` referential
// integrity; `alerts` may be nil during local dev — hook calls
// are guarded.
func New(
	logger *zap.Logger,
	contracts interfaces.ContractsStore,
	categories interfaces.CategoriesStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	documents interfaces.DocumentsStore,
	templates interfaces.TemplatesStore,
	alerts AlertsHook,
) Usecases {
	return &usecases{
		logger:     logger.Named("usecases.contracts"),
		contracts:  contracts,
		categories: categories,
		foyers:     foyers,
		copros:     copros,
		documents:  documents,
		templates:  templates,
		alerts:     alerts,
		now:        time.Now,
	}
}

func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.Contract, error) {
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	return uc.contracts.List(ctx)
}

func (uc *usecases) Create(ctx context.Context, in CreateInput) (*entities.Contract, error) {
	log := uc.logger.With(zap.String("method", "Create"))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		log.Warn("actor unauthorized", zap.Error(err))
		return nil, err
	}
	if err := uc.validateInput(ctx, in); err != nil {
		return nil, err
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return nil, fmt.Errorf("copro lookup: %w", err)
	}

	now := uc.now()
	c := entities.Contract{
		ID:               uuid.NewString(),
		CoproID:          copro.ID,
		Name:             strings.TrimSpace(in.Name),
		CategoryID:       in.CategoryID,
		Society:          normalizeSociety(in.Society),
		Contact:          normalizeContact(in.Contact),
		StartDate:        in.StartDate,
		EndDate:          in.EndDate,
		AmountCents:      in.AmountCents,
		BillingFrequency: in.BillingFrequency,
		TemplateID:       strings.TrimSpace(in.TemplateID),
		Status:           defaultStatus(in.Status),
		Note:             truncate(strings.TrimSpace(in.Note), noteMaxLen),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := uc.contracts.Create(ctx, c); err != nil {
		log.Error("store create failed", zap.Error(err))
		return nil, fmt.Errorf("create contract: %w", err)
	}
	log.Info("Success", zap.String("contract_id", c.ID))
	return &c, nil
}

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
	if err := uc.validateInput(ctx, in); err != nil {
		return nil, err
	}

	existing.Name = strings.TrimSpace(in.Name)
	existing.CategoryID = in.CategoryID
	existing.Society = normalizeSociety(in.Society)
	existing.Contact = normalizeContact(in.Contact)
	existing.StartDate = in.StartDate
	existing.EndDate = in.EndDate
	existing.AmountCents = in.AmountCents
	existing.BillingFrequency = in.BillingFrequency
	existing.TemplateID = strings.TrimSpace(in.TemplateID)
	existing.Status = defaultStatus(in.Status)
	existing.Note = truncate(strings.TrimSpace(in.Note), noteMaxLen)
	existing.UpdatedAt = uc.now()

	if err := uc.contracts.Update(ctx, *existing); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update contract: %w", err)
	}
	log.Info("Success")
	return existing, nil
}

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

	// Cascade-rejection: refuse delete if any Document still
	// back-references this contract. Mirrors the categories pattern —
	// the user must unlink the documents first so the archive doesn't
	// end up with dangling `linked_contract_id` values.
	if uc.documents != nil {
		count, err := uc.documents.CountByLinkedContract(ctx, id)
		if err != nil {
			return fmt.Errorf("count linked documents: %w", err)
		}
		if count > 0 {
			return entities.ValidationError{
				Key:     "contract",
				Message: fmt.Sprintf("encore liée à %d document(s) — détache-les d'abord", count),
			}
		}
	}

	if err := uc.contracts.Delete(ctx, id); err != nil {
		log.Error("delete failed", zap.Error(err))
		return fmt.Errorf("delete contract: %w", err)
	}

	// Resolve any outstanding contract_expiring alerts so the feed
	// doesn't keep deep-linking to a deleted contract. Best-effort —
	// a transient failure here doesn't undo the delete.
	if uc.alerts != nil {
		if err := uc.alerts.ResolveContractExpiring(ctx, id); err != nil {
			log.Warn("resolve contract_expiring alerts failed", zap.Error(err))
		}
	}

	log.Info("Success")
	return nil
}

func (uc *usecases) authorize(ctx context.Context, actorUserID string) error {
	if actorUserID == "" {
		return nil
	}
	rdc, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloorRDC)
	if err != nil {
		return fmt.Errorf("find rdc: %w", err)
	}
	premier, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloor1er)
	if err != nil {
		return fmt.Errorf("find 1er: %w", err)
	}
	if rdc == nil || premier == nil {
		return fmt.Errorf("%w: both foyers must exist", domainerrors.ErrNotFound)
	}
	for _, mid := range rdc.MemberIDs {
		if mid == actorUserID {
			return nil
		}
	}
	for _, mid := range premier.MemberIDs {
		if mid == actorUserID {
			return nil
		}
	}
	return entities.AuthorizationError{Code: "not_foyer_member"}
}

func (uc *usecases) validateInput(ctx context.Context, in CreateInput) error {
	name := strings.TrimSpace(in.Name)
	if len(name) < nameMinLen {
		return entities.ValidationError{Key: "name", Message: fmt.Sprintf("min %d caractères", nameMinLen)}
	}
	if len(name) > nameMaxLen {
		return entities.ValidationError{Key: "name", Message: fmt.Sprintf("max %d caractères", nameMaxLen)}
	}
	if strings.TrimSpace(in.Society.Name) == "" {
		return entities.ValidationError{Key: "society.name", Message: "required"}
	}
	if strings.TrimSpace(in.CategoryID) == "" {
		return entities.ValidationError{Key: "category_id", Message: "required"}
	}
	cat, err := uc.categories.FindByID(ctx, in.CategoryID)
	if err != nil {
		return fmt.Errorf("category lookup: %w", err)
	}
	if cat == nil {
		return entities.ValidationError{Key: "category_id", Message: "not found"}
	}
	if in.AmountCents < 0 {
		return entities.ValidationError{Key: "amount_cents", Message: "must be ≥ 0"}
	}
	if in.BillingFrequency != "" && !entities.IsKnownBillingFrequency(in.BillingFrequency) {
		return entities.ValidationError{Key: "billing_frequency", Message: "unknown frequency"}
	}
	if in.Status != "" && !entities.IsKnownContractStatus(in.Status) {
		return entities.ValidationError{Key: "status", Message: "unknown status"}
	}
	if !in.StartDate.IsZero() && !in.EndDate.IsZero() && in.EndDate.Before(in.StartDate) {
		return entities.ValidationError{Key: "end_date", Message: "must be on or after start_date"}
	}
	// Optional template link: if set, the template must exist so the
	// contract row doesn't accumulate dangling FKs the moment someone
	// deletes a template.
	if tid := strings.TrimSpace(in.TemplateID); tid != "" && uc.templates != nil {
		t, err := uc.templates.FindByID(ctx, tid)
		if err != nil {
			return fmt.Errorf("template lookup: %w", err)
		}
		if t == nil {
			return entities.ValidationError{Key: "template_id", Message: "not found"}
		}
	}
	return nil
}

func defaultStatus(s entities.ContractStatus) entities.ContractStatus {
	if s == "" {
		return entities.ContractStatusActive
	}
	return s
}

func normalizeSociety(s entities.Society) entities.Society {
	return entities.Society{
		Name:    truncate(strings.TrimSpace(s.Name), textMaxLen),
		Phone:   truncate(strings.TrimSpace(s.Phone), textMaxLen),
		Email:   truncate(strings.TrimSpace(s.Email), textMaxLen),
		Website: truncate(strings.TrimSpace(s.Website), urlMaxLen),
		Address: truncate(strings.TrimSpace(s.Address), noteMaxLen),
	}
}

func normalizeContact(c entities.Contact) entities.Contact {
	return entities.Contact{
		Name:  truncate(strings.TrimSpace(c.Name), textMaxLen),
		Role:  truncate(strings.TrimSpace(c.Role), textMaxLen),
		Phone: truncate(strings.TrimSpace(c.Phone), textMaxLen),
		Email: truncate(strings.TrimSpace(c.Email), textMaxLen),
	}
}

// truncate caps a string at `maxBytes` without splitting a multi-byte
// rune in half — essential for French text where one accent corrupts
// to invalid UTF-8 if the byte cut lands inside its 2-byte sequence.
func truncate(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
