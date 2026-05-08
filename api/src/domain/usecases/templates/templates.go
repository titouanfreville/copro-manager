// Package templates owns expense-template CRUD and the scheduled
// materialization loop. Templates are saved presets that either pre-fill
// the create-expense form (manual mode, handled client-side) or fire on a
// daily cron (scheduled mode, handled here).
package templates

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/expenses"
)

// materializeBackfillCap bounds how many occurrences a single
// MaterializeRecurring call will mint per template, so a template with a
// far-past StartDate can't run away (e.g. monthly + StartDate=2010 → 200+
// inserts in one run). 12 is "one year of monthlies" — enough to recover
// from a missed cron, not enough to DoS Firestore.
const materializeBackfillCap = 12

// CreateTemplateInput captures the user-facing fields. Schedule fields are
// optional — set ScheduleActive to true and provide Frequency, DayOfMonth,
// StartDate to enable the cron flow.
type CreateTemplateInput struct {
	ActorUserID        string
	Name               string
	AmountDefaultCents int
	Currency           string
	CategoryID         string
	PayerFoyerID       string
	DistributionMode   entities.DistributionMode
	ShareRDCCents      int
	Share1erCents      int
	Note               string

	ScheduleActive bool
	Frequency      entities.Frequency
	DayOfMonth     int
	StartDate      time.Time
	EndDate        *time.Time
}

// MaterializeSummary reports how many templates fired and how many expense
// rows were minted in a single cron run. Errors per template are surfaced
// in `Errors` so a single bad row doesn't block the rest of the cron.
type MaterializeSummary struct {
	TemplatesProcessed int                `json:"templates_processed"`
	ExpensesCreated    int                `json:"expenses_created"`
	Errors             []MaterializeError `json:"errors,omitempty"`
}

// MaterializeError captures one template's failure without aborting the
// whole batch.
type MaterializeError struct {
	TemplateID string `json:"template_id"`
	Message    string `json:"message"`
}

// Usecases is the templates domain contract.
type Usecases interface {
	List(ctx context.Context, actorUserID string) ([]entities.ExpenseTemplate, error)
	Create(ctx context.Context, in CreateTemplateInput) (*entities.ExpenseTemplate, error)
	Update(ctx context.Context, id string, in CreateTemplateInput) (*entities.ExpenseTemplate, error)
	Delete(ctx context.Context, id, actorUserID string) error

	// MaterializeRecurring walks every active scheduled template whose
	// next_occurrence_at is on or before today (Europe/Paris) and creates
	// an expense per due occurrence, advancing next_occurrence_at after
	// each successful Create. Idempotent — re-running it the same day is a
	// no-op once everything has fired. ActorUserID gates user-facing
	// invocations; pass empty for cron callers (the AdminKey gate at
	// transport stands in).
	MaterializeRecurring(ctx context.Context, actorUserID string) (*MaterializeSummary, error)
}

type usecases struct {
	logger    *zap.Logger
	templates interfaces.TemplatesStore
	foyers    interfaces.FoyersStore
	copros    interfaces.CoprosStore
	expenses  expenses.Usecases
	now       func() time.Time
	location  *time.Location
}

// New builds a templates usecase. The materializer pins to Europe/Paris so
// "every 1st of the month at midnight" fires on the calendar day the user
// expects, regardless of the Cloud Run instance's local time (UTC).
func New(
	logger *zap.Logger,
	templates interfaces.TemplatesStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	expenses expenses.Usecases,
) Usecases {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		// Fallback to UTC — Europe/Paris should always be available, but
		// don't crash the app on an exotic build.
		loc = time.UTC
	}
	return &usecases{
		logger:    logger.Named("usecases.templates"),
		templates: templates,
		foyers:    foyers,
		copros:    copros,
		expenses:  expenses,
		now:       time.Now,
		location:  loc,
	}
}

// List returns every template in the copro. Authorized to foyer members
// only — financial template details (amounts, payer, schedule) shouldn't
// leak to non-foyer authenticated users.
func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.ExpenseTemplate, error) {
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	return uc.templates.List(ctx)
}

func (uc *usecases) Create(ctx context.Context, in CreateTemplateInput) (*entities.ExpenseTemplate, error) {
	log := uc.logger.With(zap.String("method", "Create"))

	if err := uc.validateInput(in); err != nil {
		log.Warn("validation failed", zap.Error(err))
		return nil, err
	}
	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		log.Warn("actor unauthorized", zap.Error(err))
		return nil, err
	}
	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return nil, fmt.Errorf("copro lookup: %w", err)
	}

	now := uc.now()
	t := buildEntityFromInput(in, copro.ID, uuid.NewString(), now)
	if err := uc.templates.Create(ctx, t); err != nil {
		log.Error("create failed", zap.Error(err))
		return nil, fmt.Errorf("create template: %w", err)
	}
	log.Info("Success", zap.String("template_id", t.ID))
	return &t, nil
}

func (uc *usecases) Update(ctx context.Context, id string, in CreateTemplateInput) (*entities.ExpenseTemplate, error) {
	log := uc.logger.With(zap.String("method", "Update"), zap.String("template_id", id))

	if err := uc.validateInput(in); err != nil {
		return nil, err
	}
	// Authorize before resource lookup so non-members can't probe template
	// IDs (404 vs 403 leak).
	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	existing, err := uc.templates.FindByID(ctx, id)
	if err != nil {
		log.Error("lookup failed", zap.Error(err))
		return nil, fmt.Errorf("find template: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: template %q", domainerrors.ErrNotFound, id)
	}

	now := uc.now()
	updated := buildEntityFromInput(in, existing.CoproID, existing.ID, existing.CreatedAt)
	updated.UpdatedAt = now
	// Preserve the running next_occurrence_at when the schedule continues
	// AND the user didn't change the StartDate. Edit-in-place clients
	// typically echo the existing cursor as `start_date`, which we treat
	// the same as "no change" so the cursor isn't reset to its original
	// anchor. A truly different StartDate (or a zero one) means the user
	// either reset the cadence or bumped it forward — let the input win.
	if existing.ScheduleActive && updated.ScheduleActive && existing.NextOccurrenceAt != nil &&
		(in.StartDate.IsZero() || in.StartDate.Equal(*existing.NextOccurrenceAt)) {
		updated.NextOccurrenceAt = existing.NextOccurrenceAt
	}

	if err := uc.templates.Update(ctx, updated); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update template: %w", err)
	}
	log.Info("Success")
	return &updated, nil
}

func (uc *usecases) Delete(ctx context.Context, id, actorUserID string) error {
	log := uc.logger.With(zap.String("method", "Delete"), zap.String("template_id", id))

	// Authorize before resource lookup.
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return err
	}
	existing, err := uc.templates.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("find template: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%w: template %q", domainerrors.ErrNotFound, id)
	}
	if err := uc.templates.Delete(ctx, id); err != nil {
		log.Error("delete failed", zap.Error(err))
		return fmt.Errorf("delete template: %w", err)
	}
	log.Info("Success")
	return nil
}

// MaterializeRecurring is the cron / lazy-on-load entry point.
//
// Idempotency is best-effort: per-iteration cursor advance + per-template
// error isolation prevent a single failure from cascading or being replayed
// from scratch on the next tick. Concurrency safety is delegated to the
// transport layer (single Cloud Scheduler job) — two concurrent runs can
// still race past the cursor read; the user-facing Cloud Scheduler is
// configured `attemptDeadline=5m` so retries don't pile up.
func (uc *usecases) MaterializeRecurring(ctx context.Context, actorUserID string) (*MaterializeSummary, error) {
	log := uc.logger.With(zap.String("method", "MaterializeRecurring"))

	if err := uc.authorize(ctx, actorUserID); err != nil {
		log.Warn("actor unauthorized", zap.Error(err))
		return nil, err
	}

	// Cutoff is end-of-day in Europe/Paris so a template "for the 1st of
	// each month" fires on the local calendar day, not on the 30th of the
	// previous month at 22:00 UTC.
	now := uc.now().In(uc.location)
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, uc.location)

	due, err := uc.templates.ListDue(ctx, cutoff)
	if err != nil {
		log.Error("list due failed", zap.Error(err))
		return nil, fmt.Errorf("list due: %w", err)
	}

	summary := &MaterializeSummary{TemplatesProcessed: len(due)}

	for i := range due {
		t := due[i]
		created, err := uc.materializeOne(ctx, &t, cutoff)
		summary.ExpensesCreated += created
		if err != nil {
			// Continue past per-template failures so a single bad template
			// doesn't block the rest of the daily cron.
			log.Error("materialize one failed", zap.String("template_id", t.ID), zap.Error(err))
			summary.Errors = append(summary.Errors, MaterializeError{
				TemplateID: t.ID,
				Message:    err.Error(),
			})
		}
	}

	log.Info("Success",
		zap.Int("templates_processed", summary.TemplatesProcessed),
		zap.Int("expenses_created", summary.ExpensesCreated),
		zap.Int("errors", len(summary.Errors)),
	)
	return summary, nil
}

// materializeOne fires up to `materializeBackfillCap` occurrences for a
// single template, persisting the cursor after EACH successful Create so a
// transient failure mid-loop doesn't replay the already-materialized
// occurrences on the next cron run.
func (uc *usecases) materializeOne(ctx context.Context, t *entities.ExpenseTemplate, cutoff time.Time) (int, error) {
	if t.NextOccurrenceAt == nil || !entities.IsKnownFrequency(t.Frequency) {
		return 0, nil
	}
	created := 0
	cursor := *t.NextOccurrenceAt
	for !cursor.After(cutoff) {
		if created >= materializeBackfillCap {
			break
		}
		if t.EndDate != nil && cursor.After(*t.EndDate) {
			t.ScheduleActive = false
			break
		}

		input := expenses.CreateInput{
			Name:             t.Name,
			AmountCents:      t.AmountDefaultCents,
			Currency:         t.Currency,
			Date:             cursor,
			PayerFoyerID:     t.PayerFoyerID,
			CategoryID:       t.CategoryID,
			DistributionMode: t.DistributionMode,
			ShareRDCCents:    t.ShareRDCCents,
			Share1erCents:    t.Share1erCents,
			Note:             t.Note,
			TemplateID:       t.ID,
			AmountPending:    t.AmountDefaultCents == 0,
		}
		if _, err := uc.expenses.Create(ctx, input); err != nil {
			return created, fmt.Errorf("create expense from template %q: %w", t.ID, err)
		}
		created++

		next, err := entities.AdvanceDate(cursor, t.Frequency, t.DayOfMonth)
		if err != nil {
			// Persist whatever progress we made so the next cron tick
			// doesn't replay these `created` rows.
			t.NextOccurrenceAt = &cursor
			t.UpdatedAt = uc.now()
			_ = uc.templates.Update(ctx, *t)
			return created, fmt.Errorf("advance date for template %q: %w", t.ID, err)
		}
		cursor = next

		// Persist the cursor after EVERY successful Create so a failure on
		// the next iteration replays at most one occurrence (the failing
		// one), not the whole loop.
		t.NextOccurrenceAt = &cursor
		t.UpdatedAt = uc.now()
		if err := uc.templates.Update(ctx, *t); err != nil {
			return created, fmt.Errorf("advance template cursor %q: %w", t.ID, err)
		}
	}

	// Final ScheduleActive=false write (when EndDate is reached) — only if
	// the loop exited via the EndDate branch.
	if !t.ScheduleActive {
		t.UpdatedAt = uc.now()
		if err := uc.templates.Update(ctx, *t); err != nil {
			return created, fmt.Errorf("disable expired template %q: %w", t.ID, err)
		}
	}
	return created, nil
}

// authorize replicates the foyer-member gate used by the expenses usecase.
// Empty actor short-circuits (cron callers, AdminKey gate at transport
// stands in).
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

func (uc *usecases) validateInput(in CreateTemplateInput) error {
	details := []entities.Detail{}
	if strings.TrimSpace(in.Name) == "" {
		details = append(details, entities.Detail{Key: "name", Message: "required"})
	}
	if in.AmountDefaultCents < 0 {
		details = append(details, entities.Detail{Key: "amount_default_cents", Message: "must be >= 0"})
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
	if in.DistributionMode == entities.DistributionModeCustom {
		if in.ShareRDCCents < 0 || in.Share1erCents < 0 {
			details = append(details, entities.Detail{Key: "shares", Message: "must be >= 0"})
		}
		// Custom-mode templates with amount_default > 0 enforce sum invariant.
		if in.AmountDefaultCents > 0 && in.ShareRDCCents+in.Share1erCents != in.AmountDefaultCents {
			details = append(details, entities.Detail{Key: "shares", Message: fmt.Sprintf("share_rdc + share_1er (%d) ≠ amount_default (%d)", in.ShareRDCCents+in.Share1erCents, in.AmountDefaultCents)})
		}
	}
	if in.ScheduleActive {
		if !entities.IsKnownFrequency(in.Frequency) {
			details = append(details, entities.Detail{Key: "frequency", Message: "unknown — required when schedule active"})
		}
		// 1–31 is now valid because AdvanceDate clamps to the month's last
		// day for Feb / 30-day months.
		if in.DayOfMonth < 1 || in.DayOfMonth > 31 {
			details = append(details, entities.Detail{Key: "day_of_month", Message: "must be 1–31"})
		}
		if in.StartDate.IsZero() {
			details = append(details, entities.Detail{Key: "start_date", Message: "required when schedule active"})
		}
		// The first occurrence is anchored at StartDate. If the user
		// supplied a separate DayOfMonth, it must match — otherwise the
		// first fire and subsequent fires would land on different days.
		if !in.StartDate.IsZero() && in.DayOfMonth >= 1 && in.DayOfMonth <= 31 &&
			in.StartDate.Day() != in.DayOfMonth &&
			// Exception: if DayOfMonth doesn't fit in StartDate's month
			// (e.g. 31 in Feb), we tolerate the StartDate's clamped day.
			in.DayOfMonth <= entities.LastDayOfMonth(in.StartDate.Year(), in.StartDate.Month()) {
			details = append(details, entities.Detail{
				Key:     "day_of_month",
				Message: fmt.Sprintf("day_of_month (%d) must match start_date.Day() (%d)", in.DayOfMonth, in.StartDate.Day()),
			})
		}
		if in.EndDate != nil && !in.EndDate.IsZero() && in.EndDate.Before(in.StartDate) {
			details = append(details, entities.Detail{Key: "end_date", Message: "must be on or after start_date"})
		}
	}
	if len(details) > 0 {
		return entities.ValidationError{
			Key:     "create_template",
			Message: "invalid input",
			Details: details,
		}
	}
	return nil
}

func buildEntityFromInput(in CreateTemplateInput, coproID, id string, createdAt time.Time) entities.ExpenseTemplate {
	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency == "" {
		currency = "EUR"
	}
	t := entities.ExpenseTemplate{
		ID:                 id,
		CoproID:            coproID,
		Name:               strings.TrimSpace(in.Name),
		AmountDefaultCents: in.AmountDefaultCents,
		Currency:           currency,
		CategoryID:         in.CategoryID,
		PayerFoyerID:       in.PayerFoyerID,
		DistributionMode:   in.DistributionMode,
		ShareRDCCents:      in.ShareRDCCents,
		Share1erCents:      in.Share1erCents,
		Note:               strings.TrimSpace(in.Note),
		ScheduleActive:     in.ScheduleActive,
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt,
	}
	if in.ScheduleActive {
		t.Frequency = in.Frequency
		t.DayOfMonth = in.DayOfMonth
		next := in.StartDate
		t.NextOccurrenceAt = &next
		if in.EndDate != nil && !in.EndDate.IsZero() {
			end := *in.EndDate
			t.EndDate = &end
		}
	}
	return t
}

// Compile-time guarantees we keep using the imported `errors` symbol if a
// future contributor swaps fmt.Errorf for errors.Join. Avoids `unused`
// linter noise without polluting the export surface.
var _ = errors.New
