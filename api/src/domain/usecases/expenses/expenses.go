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

	"github.com/titouanfreville/copro-manager/api/src/core/authz"
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
	// ActorUserID is the UID of the authenticated foyer member
	// submitting the expense. Empty bypasses the membership check
	// (admin/CSV-import flows; the AdminKey gate stands in for it at
	// the transport layer).
	ActorUserID string

	// ExpenseDraft is the user-editable subset shared with the
	// validator. Embedding keeps every field accessible as
	// `in.<Field>` at the call sites that pre-date this refactor.
	entities.ExpenseDraft

	// TrustExplicitShares preserves the supplied ShareRDCCents /
	// Share1erCents for any mode — used by the CSV import so a
	// historical "tantieme" row keeps its original split even if the
	// foyer parts evolve. Sum invariant (rdc + 1er == amount) is
	// still enforced.
	TrustExplicitShares bool
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
	// Update replaces every editable field of an existing expense.
	// Identity (ID + CreatedAt) is preserved; UpdatedAt is refreshed.
	// Shares are recomputed from the supplied mode + amount unless
	// in.TrustExplicitShares is set, in which case the supplied shares
	// are taken verbatim (after sum validation).
	Update(ctx context.Context, id string, in CreateInput) (*entities.Expense, error)
	// Delete removes an expense permanently. The actor must be a member of
	// one of the foyers in the copro. Cascades any attachment blobs.
	Delete(ctx context.Context, id, actorUserID string) error
	// Upsert inserts a new expense or replaces an existing one matched by
	// (Name, Date). Used by the CSV import flow so re-uploading the same
	// spreadsheet doesn't create duplicates.
	Upsert(ctx context.Context, in CreateInput) (*UpsertResult, error)
	// ImportCSV parses the legacy spreadsheet shape and upserts every valid
	// row. defaultPayerFoyerID is applied to all rows since the legacy
	// format doesn't track payer identity.
	ImportCSV(ctx context.Context, r io.Reader, defaultPayerFoyerID string) (*ImportSummary, error)
}

// AlertsHook is the narrow contract this package needs from the alerts
// usecase. Defined here (not imported from `usecases/alerts`) so the
// expenses package stays a leaf — Go's structural typing lets the real
// alerts.Usecases value satisfy this interface automatically.
type AlertsHook interface {
	FirePeerExpenseAdded(ctx context.Context, exp entities.Expense, recipientFoyerID string) (*entities.Alert, error)
	ResolveMissingReceipt(ctx context.Context, expenseID string) error
	ResolvePendingCompletion(ctx context.Context, expenseID string) error
	ResolveByExpense(ctx context.Context, expenseID string) error
}

// DocumentsHook is the narrow contract this package needs from the
// documents usecase, used by the expense-delete cascade to wipe linked
// Documents (the unified attachment store) along with their GCS blobs.
type DocumentsHook interface {
	DeleteByLinkedExpense(ctx context.Context, expenseID string) error
}

type usecases struct {
	logger      *zap.Logger
	expenses    interfaces.ExpensesStore
	attachments interfaces.AttachmentsStore
	foyers      interfaces.FoyersStore
	copros      interfaces.CoprosStore
	categories  interfaces.CategoriesStore
	storage     interfaces.StorageService
	settlements interfaces.SettlementsStore
	meters      interfaces.MetersStore
	validator   interfaces.ExpenseValidator
	alerts      AlertsHook
	documents   DocumentsHook
	now         func() time.Time
}

// New builds an expenses usecase. `alerts` and `documents` may be nil
// during local dev or in tests — every hook call is guarded.
func New(
	logger *zap.Logger,
	expenses interfaces.ExpensesStore,
	attachments interfaces.AttachmentsStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	categories interfaces.CategoriesStore,
	storage interfaces.StorageService,
	settlements interfaces.SettlementsStore,
	meters interfaces.MetersStore,
	validator interfaces.ExpenseValidator,
	alerts AlertsHook,
	docs DocumentsHook,
) Usecases {
	return &usecases{
		logger:      logger.Named("usecases.expenses"),
		expenses:    expenses,
		attachments: attachments,
		foyers:      foyers,
		copros:      copros,
		categories:  categories,
		storage:     storage,
		settlements: settlements,
		meters:      meters,
		validator:   validator,
		alerts:      alerts,
		documents:   docs,
		now:         time.Now,
	}
}

// Create validates input, resolves both foyers, computes shares, and writes.
func (uc *usecases) Create(ctx context.Context, in CreateInput) (*entities.Expense, error) {
	log := uc.logger.With(
		zap.String("method", "Create"),
		zap.String("mode", string(in.DistributionMode)),
		zap.Int("amount_cents", in.AmountCents),
	)

	if err := uc.validator.Validate(ctx, in.ExpenseDraft); err != nil {
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

	rdc, premier, err := authz.LoadBothFoyers(ctx, uc.foyers)
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
	if in.ActorUserID != "" && !authz.IsMemberOf(rdc, premier, in.ActorUserID) {
		log.Warn("actor is not a foyer member", zap.String("actor_user_id", in.ActorUserID))
		return nil, entities.AuthorizationError{Code: "not_foyer_member"}
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		log.Error("copro lookup failed", zap.Error(err))
		return nil, fmt.Errorf("copro lookup: %w", err)
	}

	shareRDC, share1er, err := uc.computeSharesOrPending(ctx, in, rdc, premier, copro)
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
		ID:                 uuid.NewString(),
		CoproID:            copro.ID,
		Name:               strings.TrimSpace(in.Name),
		AmountCents:        in.AmountCents,
		Currency:           currency,
		Date:               in.Date,
		PaymentDate:        in.PaymentDate,
		PayerFoyerID:       in.PayerFoyerID,
		CategoryID:         in.CategoryID,
		DistributionMode:   in.DistributionMode,
		ShareRDCCents:      shareRDC,
		Share1erCents:      share1er,
		Settled:            in.Settled,
		SettledAt:          normalizeSettledAt(in.Settled, in.SettledAt),
		Note:               in.Note,
		TemplateID:         in.TemplateID,
		AmountPending:      in.AmountPending,
		MeterReadingPeriod: normalizeMeterPeriod(in.DistributionMode, in.MeterReadingPeriod),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := uc.expenses.Create(ctx, exp); err != nil {
		log.Error("expense create failed", zap.Error(err))
		return nil, fmt.Errorf("create expense: %w", err)
	}

	// peer_expense_added: only when an actual foyer member created this
	// row (cron / CSV import / template materializer all pass empty
	// ActorUserID and shouldn't fan out a "the other foyer added X"
	// alert that would in fact be coming from the system).
	if uc.alerts != nil && in.ActorUserID != "" && !exp.AmountPending {
		// PayerFoyerID is the foyer that fronted the expense; the alert
		// goes to whichever foyer the actor isn't a member of. Whether
		// the actor records their OWN payment or one fronted by the
		// neighbors, the non-actor foyer is the right recipient.
		recipient := otherFoyerID(in.ActorUserID, rdc, premier)
		if recipient != "" {
			if _, err := uc.alerts.FirePeerExpenseAdded(ctx, exp, recipient); err != nil {
				log.Warn("peer alert fire failed", zap.Error(err))
			}
		}
	}

	log.Info("Success", zap.String("expense_id", exp.ID))
	return &exp, nil
}

// otherFoyerID returns the ID of the foyer the actor does NOT belong to.
// Returns "" when the actor isn't a member of either foyer or when one of
// the foyer pointers is nil (defense-in-depth — the auth check in Create
// already filters these cases, but a future caller path could expose the
// nil-deref).
func otherFoyerID(actorUserID string, rdc, premier *entities.Foyer) string {
	if rdc == nil || premier == nil || actorUserID == "" {
		return ""
	}
	for _, mid := range rdc.MemberIDs {
		if mid == actorUserID {
			return premier.ID
		}
	}
	for _, mid := range premier.MemberIDs {
		if mid == actorUserID {
			return rdc.ID
		}
	}
	return ""
}

// normalizeSettledAt enforces the invariant: SettledAt is only meaningful
// when Settled is true. Clears the timestamp on unsettled rows.
func normalizeSettledAt(settled bool, at *time.Time) *time.Time {
	if !settled {
		return nil
	}
	return at
}

// Update mirrors Create's validation + share-computation flow but writes to
// an existing expense doc. Returns ErrNotFound when the id doesn't exist.
//
// Authorization is checked before any resource lookup so non-foyer-members
// can't probe expense IDs (404 vs 403 leak).
func (uc *usecases) Update(ctx context.Context, id string, in CreateInput) (*entities.Expense, error) {
	log := uc.logger.With(
		zap.String("method", "Update"),
		zap.String("expense_id", id),
		zap.String("mode", string(in.DistributionMode)),
	)

	if err := uc.validator.Validate(ctx, in.ExpenseDraft); err != nil {
		log.Warn("validation failed", zap.Error(err))
		return nil, err
	}

	rdc, premier, err := authz.LoadBothFoyers(ctx, uc.foyers)
	if err != nil {
		return nil, err
	}
	if in.ActorUserID != "" && !authz.IsMemberOf(rdc, premier, in.ActorUserID) {
		log.Warn("actor is not a foyer member", zap.String("actor_user_id", in.ActorUserID))
		return nil, entities.AuthorizationError{Code: "not_foyer_member"}
	}

	existing, err := uc.expenses.FindByID(ctx, id)
	if err != nil {
		log.Error("expense lookup failed", zap.Error(err))
		return nil, fmt.Errorf("find expense by id: %w", err)
	}
	if existing == nil {
		log.Warn("expense not found")
		return nil, fmt.Errorf("%w: expense %q", domainerrors.ErrNotFound, id)
	}

	// Once an expense is confirmed (AmountPending=false with a non-zero
	// amount), the user can't revert it to pending — that would zero shares
	// and silently corrupt the running balance. Re-creation is the right
	// path if the row truly needs to start over.
	if !existing.AmountPending && existing.AmountCents > 0 && in.AmountPending {
		return nil, entities.ValidationError{
			Key:     "amount_pending",
			Message: "cannot revert a confirmed expense to pending",
		}
	}

	cat, err := uc.categories.FindByID(ctx, in.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("category lookup: %w", err)
	}
	if cat == nil {
		return nil, entities.ValidationError{Key: "category_id", Message: "not found"}
	}

	if in.PayerFoyerID != rdc.ID && in.PayerFoyerID != premier.ID {
		return nil, entities.ValidationError{Key: "payer_foyer_id", Message: "not a foyer of this copro"}
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return nil, fmt.Errorf("copro lookup: %w", err)
	}

	shareRDC, share1er, err := uc.computeSharesOrPending(ctx, in, rdc, premier, copro)
	if err != nil {
		return nil, err
	}

	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency == "" {
		currency = existing.Currency
	}

	now := uc.now()
	existing.Name = strings.TrimSpace(in.Name)
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
	// Preserve the existing SettledAt when the caller marks the row settled
	// but omits the timestamp — clients that PATCH a partial body shouldn't
	// silently wipe the audit trail.
	existing.SettledAt = mergeSettledAt(in.Settled, in.SettledAt, existing.SettledAt)
	existing.Note = in.Note
	existing.MeterReadingPeriod = normalizeMeterPeriod(in.DistributionMode, in.MeterReadingPeriod)
	// TemplateID is preserved when present — Update doesn't typically
	// clear lineage. Only overwrite when the input carries a value.
	if in.TemplateID != "" {
		existing.TemplateID = in.TemplateID
	}
	// Capture the prior pending-state BEFORE mutating `existing`, otherwise
	// `wasPending` is always false (both operands would be `in.AmountPending`).
	wasPending := existing.AmountPending && !in.AmountPending
	existing.AmountPending = in.AmountPending
	existing.UpdatedAt = now
	if err := uc.expenses.Update(ctx, *existing); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update expense: %w", err)
	}

	// pending_completion: resolve when the row transitioned from
	// pending → confirmed in this Update. Best-effort.
	if uc.alerts != nil && wasPending {
		if err := uc.alerts.ResolvePendingCompletion(ctx, existing.ID); err != nil {
			log.Warn("resolve pending alert failed", zap.Error(err))
		}
	}

	log.Info("Success")
	return existing, nil
}

// mergeSettledAt is the Update-time analog of normalizeSettledAt: when the
// caller marks the row settled but doesn't echo the timestamp, fall back to
// the existing one. Wipe SettledAt entirely when Settled flips to false.
func mergeSettledAt(settled bool, in *time.Time, existing *time.Time) *time.Time {
	if !settled {
		return nil
	}
	if in == nil {
		return existing
	}
	return in
}

// Delete removes an expense after authorizing the actor as a foyer member.
// Idempotent at the storage layer — deleting a missing doc is a no-op,
// surfaced here as ErrNotFound so the route handler can return 404.
//
// Cascades any GCS attachment blobs under the expense's prefix. The blob
// cleanup is best-effort: failures are logged but do not roll back the
// metadata delete (orphaned blobs are recoverable; rolling back a
// successful delete is worse UX than a stale prefix).
func (uc *usecases) Delete(ctx context.Context, id, actorUserID string) error {
	log := uc.logger.With(zap.String("method", "Delete"), zap.String("expense_id", id))

	// Authorize before resource lookup so non-foyer-members can't probe
	// expense existence (404 vs 403 leak).
	if actorUserID != "" {
		rdc, premier, err := authz.LoadBothFoyers(ctx, uc.foyers)
		if err != nil {
			return err
		}
		if !authz.IsMemberOf(rdc, premier, actorUserID) {
			log.Warn("actor is not a foyer member", zap.String("actor_user_id", actorUserID))
			return entities.AuthorizationError{Code: "not_foyer_member"}
		}
	}

	existing, err := uc.expenses.FindByID(ctx, id)
	if err != nil {
		log.Error("expense lookup failed", zap.Error(err))
		return fmt.Errorf("find expense by id: %w", err)
	}
	if existing == nil {
		log.Warn("expense not found")
		return fmt.Errorf("%w: expense %q", domainerrors.ErrNotFound, id)
	}

	// Cascade BEFORE deleting the parent so child references are reachable.
	// Best-effort: if any cleanup leg fails we still drop the parent so the
	// user's "delete" action isn't blocked by orphan-cleanup hiccups.
	//
	// Three legs to cover both legacy and migrated state:
	//   1. Linked Documents — the unified attachment store; deletes both
	//      the Firestore record and the GCS blob it points at (which lives
	//      under either documents/ or the legacy expenses/ prefix).
	//   2. Legacy attachments subcollection — drained at boot by the
	//      migration but called here too in case any survived.
	//   3. Legacy GCS prefix expenses/{id}/ — for any blob whose Document
	//      record was already migrated and deleted via leg 1, this is a
	//      no-op; otherwise it cleans the byproducts.
	if uc.documents != nil {
		if err := uc.documents.DeleteByLinkedExpense(ctx, id); err != nil {
			log.Warn("linked-documents cleanup failed (orphan docs may remain)", zap.Error(err))
		}
	}
	if uc.attachments != nil {
		if err := uc.attachments.DeleteAll(ctx, id); err != nil {
			log.Warn("attachment subcollection cleanup failed (orphan docs may remain)", zap.Error(err))
		}
	}
	if uc.storage != nil {
		if err := uc.storage.DeletePrefix(ctx, attachmentPrefix(id)); err != nil {
			log.Warn("attachment blob cleanup failed (orphan blobs may remain)", zap.Error(err))
		}
	}

	if err := uc.expenses.Delete(ctx, id); err != nil {
		log.Error("delete failed", zap.Error(err))
		return fmt.Errorf("delete expense: %w", err)
	}

	// Cascade-prune any settlement that was audit-linking this expense.
	// Best-effort: a transient failure leaves a dangling reference, which
	// the next prune call would clean up.
	if uc.settlements != nil {
		if err := uc.settlements.PruneExpense(ctx, id); err != nil {
			log.Warn("settlement link prune failed (dangling reference may remain)", zap.Error(err))
		}
	}

	// Resolve every alert that referenced this expense — missing_receipt
	// stages, pending_completion, peer_expense_added all become moot once
	// the row is gone.
	if uc.alerts != nil {
		if err := uc.alerts.ResolveByExpense(ctx, id); err != nil {
			log.Warn("alert auto-resolve failed", zap.Error(err))
		}
	}

	log.Info("Success")
	return nil
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

	if err := uc.validator.Validate(ctx, in.ExpenseDraft); err != nil {
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

	rdc, premier, err := authz.LoadBothFoyers(ctx, uc.foyers)
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

	shareRDC, share1er, err := uc.computeSharesOrPending(ctx, in, rdc, premier, copro)
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
		existing.MeterReadingPeriod = normalizeMeterPeriod(in.DistributionMode, in.MeterReadingPeriod)
		existing.UpdatedAt = now

		if err := uc.expenses.Update(ctx, *existing); err != nil {
			log.Error("update failed", zap.Error(err))
			return nil, fmt.Errorf("update expense: %w", err)
		}
		log.Info("Success", zap.String("expense_id", existing.ID), zap.Bool("created", false))
		return &UpsertResult{Expense: existing, Created: false}, nil
	}

	exp := entities.Expense{
		ID:                 uuid.NewString(),
		CoproID:            copro.ID,
		Name:               name,
		AmountCents:        in.AmountCents,
		Currency:           currency,
		Date:               in.Date,
		PaymentDate:        in.PaymentDate,
		PayerFoyerID:       in.PayerFoyerID,
		CategoryID:         in.CategoryID,
		DistributionMode:   in.DistributionMode,
		ShareRDCCents:      shareRDC,
		Share1erCents:      share1er,
		Settled:            in.Settled,
		SettledAt:          normalizeSettledAt(in.Settled, in.SettledAt),
		Note:               in.Note,
		MeterReadingPeriod: normalizeMeterPeriod(in.DistributionMode, in.MeterReadingPeriod),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := uc.expenses.Create(ctx, exp); err != nil {
		log.Error("create failed", zap.Error(err))
		return nil, fmt.Errorf("create expense: %w", err)
	}
	log.Info("Success", zap.String("expense_id", exp.ID), zap.Bool("created", true))
	return &UpsertResult{Expense: &exp, Created: true}, nil
}

