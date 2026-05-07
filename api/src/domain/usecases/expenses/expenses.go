// Package expenses owns the shared-expense logic: input validation, share
// computation across the three distribution modes (Equal, Tantièmes, Custom),
// and persistence.
//
// All amounts flow as integer cents to keep arithmetic exact. The cross-mode
// invariant is the same in every mode: ShareRDC + Share1er == Amount.
package expenses

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// CreateInput captures the user-facing fields. Shares are *only* read when
// DistributionMode == Custom or TrustExplicitShares is set; they're ignored
// otherwise (the usecase recomputes from amount + foyer parts). Name is
// required and serves as the upsert key together with Date.
//
// TrustExplicitShares preserves the supplied ShareRDCCents / Share1erCents
// for any mode — used by the CSV import so a historical "tantieme" row
// keeps its original split even if the foyer parts evolve afterwards. The
// usecase only validates the sum invariant (rdc + 1er == amount).
type CreateInput struct {
	// ActorUserID is the UID of the authenticated foyer member submitting
	// the expense. The Create flow rejects calls where the actor is not a
	// member of either foyer in the copro — admin/CSV-import flows pass an
	// empty string to bypass the check (the AdminKey gate stands in for it
	// at the transport layer).
	ActorUserID         string
	Name                string
	AmountCents         int
	Currency            string
	Date                time.Time
	PaymentDate         *time.Time
	PayerFoyerID        string
	CategoryID          string
	DistributionMode    entities.DistributionMode
	ShareRDCCents       int
	Share1erCents       int
	Note                string
	TrustExplicitShares bool
	// Settled marks the expense as already balanced between foyers — both
	// households paid their share directly. Excluded from the running
	// balance. CSV import sets this for every "Paiement complet" row.
	Settled bool
	// SettledAt is the date the two foyers reconciled accounts. Required
	// (recommended) when Settled is true and known; nil for CSV imports
	// where the legacy spreadsheet doesn't carry that date.
	SettledAt *time.Time
}

// UpsertResult tells the caller whether a brand-new doc was written or an
// existing one was updated.
type UpsertResult struct {
	Expense *entities.Expense
	Created bool
}

// Usecases is the expenses domain contract. The foyer-facing app reads the
// list directly from Firestore (see app/src/lib/live.ts); only mutations
// stay here so share-computation logic remains canonical.
type Usecases interface {
	Create(ctx context.Context, in CreateInput) (*entities.Expense, error)
	// Upsert inserts a new expense or replaces an existing one matched by
	// (Name, Date). Used by the CSV import flow so re-uploading the same
	// spreadsheet doesn't create duplicates.
	Upsert(ctx context.Context, in CreateInput) (*UpsertResult, error)
	// ImportCSV parses the legacy spreadsheet shape and upserts every valid
	// row. defaultPayerFoyerID is applied to all rows since the legacy
	// format doesn't track payer identity.
	ImportCSV(ctx context.Context, r io.Reader, defaultPayerFoyerID string) (*ImportSummary, error)
}

type usecases struct {
	logger     *zap.Logger
	expenses   interfaces.ExpensesStore
	foyers     interfaces.FoyersStore
	copros     interfaces.CoprosStore
	categories interfaces.CategoriesStore
	now        func() time.Time
}

// New builds an expenses usecase.
func New(
	logger *zap.Logger,
	expenses interfaces.ExpensesStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	categories interfaces.CategoriesStore,
) Usecases {
	return &usecases{
		logger:     logger.Named("usecases.expenses"),
		expenses:   expenses,
		foyers:     foyers,
		copros:     copros,
		categories: categories,
		now:        time.Now,
	}
}

// Create validates input, resolves both foyers, computes shares, and writes.
func (uc *usecases) Create(ctx context.Context, in CreateInput) (*entities.Expense, error) {
	log := uc.logger.With(
		zap.String("method", "Create"),
		zap.String("mode", string(in.DistributionMode)),
		zap.Int("amount_cents", in.AmountCents),
	)

	if err := validateInput(in); err != nil {
		log.Warn("validation failed", zap.Error(err))
		return nil, err
	}

	cat, err := uc.categories.FindByID(ctx, in.CategoryID)
	if err != nil {
		log.Error("category lookup failed", zap.Error(err))
		return nil, fmt.Errorf("category lookup: %w", err)
	}
	if cat == nil {
		log.Warn("category not found")
		return nil, entities.ValidationError{Key: "category_id", Message: "not found"}
	}

	rdc, premier, err := uc.loadFoyers(ctx)
	if err != nil {
		log.Error("foyer load failed", zap.Error(err))
		return nil, err
	}

	if in.PayerFoyerID != rdc.ID && in.PayerFoyerID != premier.ID {
		log.Warn("payer not in copro")
		return nil, entities.ValidationError{Key: "payer_foyer_id", Message: "not a foyer of this copro"}
	}

	// Authorization: the actor must belong to one of the copro's foyers.
	// Both foyers are equal participants per the PRD, so a member of foyer A
	// may legitimately attribute payment to foyer B (e.g. record an expense
	// fronted by the other household). The check only excludes random
	// authenticated Firebase users who aren't members of either foyer.
	if in.ActorUserID != "" && !isFoyerMember(in.ActorUserID, rdc, premier) {
		log.Warn("actor is not a foyer member", zap.String("actor_user_id", in.ActorUserID))
		return nil, entities.AuthorizationError{Code: "not_foyer_member"}
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		log.Error("copro lookup failed", zap.Error(err))
		return nil, fmt.Errorf("copro lookup: %w", err)
	}

	shareRDC, share1er, err := computeShares(in, rdc, premier, copro)
	if err != nil {
		log.Warn("share computation failed", zap.Error(err))
		return nil, err
	}

	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency == "" {
		currency = "EUR"
	}

	now := uc.now()
	exp := entities.Expense{
		ID:               uuid.NewString(),
		CoproID:          copro.ID,
		Name:             strings.TrimSpace(in.Name),
		AmountCents:      in.AmountCents,
		Currency:         currency,
		Date:             in.Date,
		PaymentDate:      in.PaymentDate,
		PayerFoyerID:     in.PayerFoyerID,
		CategoryID:       in.CategoryID,
		DistributionMode: in.DistributionMode,
		ShareRDCCents:    shareRDC,
		Share1erCents:    share1er,
		Settled:          in.Settled,
		SettledAt:        normalizeSettledAt(in.Settled, in.SettledAt),
		Note:             in.Note,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := uc.expenses.Create(ctx, exp); err != nil {
		log.Error("expense create failed", zap.Error(err))
		return nil, fmt.Errorf("create expense: %w", err)
	}

	log.Info("Success", zap.String("expense_id", exp.ID))
	return &exp, nil
}

// normalizeSettledAt enforces the invariant: SettledAt is only meaningful
// when Settled is true. Clears the timestamp on unsettled rows.
func normalizeSettledAt(settled bool, at *time.Time) *time.Time {
	if !settled {
		return nil
	}
	return at
}

// Upsert performs the same validation + computation as Create, but matches
// existing rows by (Name, Date) and updates them in place. Used by the CSV
// import — re-uploading the same spreadsheet is a no-op rather than a
// duplicator.
func (uc *usecases) Upsert(ctx context.Context, in CreateInput) (*UpsertResult, error) {
	log := uc.logger.With(
		zap.String("method", "Upsert"),
		zap.String("name", in.Name),
		zap.Time("date", in.Date),
	)

	if err := validateInput(in); err != nil {
		log.Warn("validation failed", zap.Error(err))
		return nil, err
	}

	cat, err := uc.categories.FindByID(ctx, in.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("category lookup: %w", err)
	}
	if cat == nil {
		return nil, entities.ValidationError{Key: "category_id", Message: "not found"}
	}

	rdc, premier, err := uc.loadFoyers(ctx)
	if err != nil {
		return nil, err
	}
	if in.PayerFoyerID != rdc.ID && in.PayerFoyerID != premier.ID {
		return nil, entities.ValidationError{Key: "payer_foyer_id", Message: "not a foyer of this copro"}
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return nil, fmt.Errorf("copro lookup: %w", err)
	}

	shareRDC, share1er, err := computeShares(in, rdc, premier, copro)
	if err != nil {
		return nil, err
	}

	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency == "" {
		currency = "EUR"
	}

	name := strings.TrimSpace(in.Name)
	existing, err := uc.expenses.FindByNameAndDate(ctx, name, in.Date)
	if err != nil {
		log.Error("upsert lookup failed", zap.Error(err))
		return nil, fmt.Errorf("find by name+date: %w", err)
	}

	now := uc.now()
	if existing != nil {
		// Preserve identity + creation time; refresh everything else.
		existing.CoproID = copro.ID
		existing.Name = name
		existing.AmountCents = in.AmountCents
		existing.Currency = currency
		existing.Date = in.Date
		existing.PaymentDate = in.PaymentDate
		existing.PayerFoyerID = in.PayerFoyerID
		existing.CategoryID = in.CategoryID
		existing.DistributionMode = in.DistributionMode
		existing.ShareRDCCents = shareRDC
		existing.Share1erCents = share1er
		existing.Settled = in.Settled
		existing.SettledAt = normalizeSettledAt(in.Settled, in.SettledAt)
		existing.Note = in.Note
		existing.UpdatedAt = now

		if err := uc.expenses.Update(ctx, *existing); err != nil {
			log.Error("update failed", zap.Error(err))
			return nil, fmt.Errorf("update expense: %w", err)
		}
		log.Info("Success", zap.String("expense_id", existing.ID), zap.Bool("created", false))
		return &UpsertResult{Expense: existing, Created: false}, nil
	}

	exp := entities.Expense{
		ID:               uuid.NewString(),
		CoproID:          copro.ID,
		Name:             name,
		AmountCents:      in.AmountCents,
		Currency:         currency,
		Date:             in.Date,
		PaymentDate:      in.PaymentDate,
		PayerFoyerID:     in.PayerFoyerID,
		CategoryID:       in.CategoryID,
		DistributionMode: in.DistributionMode,
		ShareRDCCents:    shareRDC,
		Share1erCents:    share1er,
		Settled:          in.Settled,
		SettledAt:        normalizeSettledAt(in.Settled, in.SettledAt),
		Note:             in.Note,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := uc.expenses.Create(ctx, exp); err != nil {
		log.Error("create failed", zap.Error(err))
		return nil, fmt.Errorf("create expense: %w", err)
	}
	log.Info("Success", zap.String("expense_id", exp.ID), zap.Bool("created", true))
	return &UpsertResult{Expense: &exp, Created: true}, nil
}

// isFoyerMember reports whether the given UID belongs to either of the
// copro's foyers. Used to gate user-facing mutations.
func isFoyerMember(uid string, rdc, premier *entities.Foyer) bool {
	for _, mid := range rdc.MemberIDs {
		if mid == uid {
			return true
		}
	}
	for _, mid := range premier.MemberIDs {
		if mid == uid {
			return true
		}
	}
	return false
}

// loadFoyers fetches RDC and 1er via FindByFloor. Both must exist before any
// expense can be recorded — admin must seed at least one foyer per floor.
func (uc *usecases) loadFoyers(ctx context.Context) (*entities.Foyer, *entities.Foyer, error) {
	rdc, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloorRDC)
	if err != nil {
		return nil, nil, fmt.Errorf("find rdc: %w", err)
	}
	premier, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloor1er)
	if err != nil {
		return nil, nil, fmt.Errorf("find 1er: %w", err)
	}
	if rdc == nil || premier == nil {
		return nil, nil, fmt.Errorf("%w: both RDC and 1er foyers must exist", domainerrors.ErrNotFound)
	}
	return rdc, premier, nil
}

// computeShares applies the chosen distribution mode and returns the
// (RDC, 1er) cents pair. The invariant ShareRDC + Share1er == Amount is
// enforced for every mode (rounding remainder routes to the payer).
//
// When TrustExplicitShares is set, the supplied shares are taken verbatim
// regardless of mode — the historical preservation path used by the CSV
// import.
func computeShares(in CreateInput, rdc, premier *entities.Foyer, copro *entities.Copro) (int, int, error) {
	if in.TrustExplicitShares {
		if in.ShareRDCCents+in.Share1erCents != in.AmountCents {
			return 0, 0, entities.ValidationError{
				Key:     "shares",
				Message: fmt.Sprintf("share_rdc_cents + share_1er_cents (%d) ≠ amount_cents (%d)", in.ShareRDCCents+in.Share1erCents, in.AmountCents),
			}
		}
		if in.ShareRDCCents < 0 || in.Share1erCents < 0 {
			return 0, 0, entities.ValidationError{Key: "shares", Message: "shares must be >= 0"}
		}
		return in.ShareRDCCents, in.Share1erCents, nil
	}

	switch in.DistributionMode {
	case entities.DistributionModeEqual:
		half := in.AmountCents / 2
		remainder := in.AmountCents - 2*half
		shareRDC, share1er := half, half
		if remainder != 0 {
			if in.PayerFoyerID == rdc.ID {
				shareRDC += remainder
			} else {
				share1er += remainder
			}
		}
		return shareRDC, share1er, nil

	case entities.DistributionModeTantiemes:
		if copro.TotalParts <= 0 {
			return 0, 0, entities.ValidationError{Key: "copro.total_parts", Message: "must be > 0"}
		}
		if rdc.Parts+premier.Parts != copro.TotalParts {
			return 0, 0, entities.ValidationError{
				Key:     "foyers.parts",
				Message: fmt.Sprintf("Σ parts (%d) ≠ copro.total_parts (%d)", rdc.Parts+premier.Parts, copro.TotalParts),
			}
		}
		// Integer math: amount * parts / total. Allocate the remainder to payer.
		shareRDC := in.AmountCents * rdc.Parts / copro.TotalParts
		share1er := in.AmountCents * premier.Parts / copro.TotalParts
		remainder := in.AmountCents - shareRDC - share1er
		if remainder != 0 {
			if in.PayerFoyerID == rdc.ID {
				shareRDC += remainder
			} else {
				share1er += remainder
			}
		}
		return shareRDC, share1er, nil

	case entities.DistributionModeCustom:
		if in.ShareRDCCents+in.Share1erCents != in.AmountCents {
			return 0, 0, entities.ValidationError{
				Key:     "shares",
				Message: fmt.Sprintf("share_rdc_cents + share_1er_cents (%d) ≠ amount_cents (%d)", in.ShareRDCCents+in.Share1erCents, in.AmountCents),
			}
		}
		if in.ShareRDCCents < 0 || in.Share1erCents < 0 {
			return 0, 0, entities.ValidationError{Key: "shares", Message: "shares must be >= 0"}
		}
		return in.ShareRDCCents, in.Share1erCents, nil

	default:
		return 0, 0, entities.ValidationError{Key: "distribution_mode", Message: "unknown mode"}
	}
}

func validateInput(in CreateInput) error {
	details := []entities.Detail{}
	if strings.TrimSpace(in.Name) == "" {
		details = append(details, entities.Detail{Key: "name", Message: "required"})
	}
	if in.AmountCents <= 0 {
		details = append(details, entities.Detail{Key: "amount_cents", Message: "must be > 0"})
	}
	if !entities.IsKnownDistributionMode(in.DistributionMode) {
		details = append(details, entities.Detail{Key: "distribution_mode", Message: "unknown mode"})
	}
	if strings.TrimSpace(in.PayerFoyerID) == "" {
		details = append(details, entities.Detail{Key: "payer_foyer_id", Message: "required"})
	}
	if strings.TrimSpace(in.CategoryID) == "" {
		details = append(details, entities.Detail{Key: "category_id", Message: "required"})
	}
	if in.Date.IsZero() {
		details = append(details, entities.Detail{Key: "date", Message: "required"})
	}
	if len(details) > 0 {
		return entities.ValidationError{
			Key:     "create_expense",
			Message: "invalid input",
			Details: details,
		}
	}
	return nil
}
