// Package templates owns the expense-template layer: saved presets
// the user fires manually (client-side form pre-fill) or that the
// daily materializer cron auto-fires for scheduled templates.
//
// Validation lives in adapters/validators/templates.go; entity
// construction lives in build.go; the cron materializer lives in
// materialize.go. This file is pure orchestration.
package templates

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

// CreateTemplateInput is the route-layer DTO. Actor UID rides
// alongside the draft so the validator only sees draft data.
type CreateTemplateInput struct {
	ActorUserID string
	entities.ExpenseTemplateDraft
}

// Usecases is the templates domain contract.
type Usecases interface {
	List(ctx context.Context, actorUserID string) ([]entities.ExpenseTemplate, error)
	Create(ctx context.Context, in CreateTemplateInput) (*entities.ExpenseTemplate, error)
	Update(ctx context.Context, id string, in CreateTemplateInput) (*entities.ExpenseTemplate, error)
	Delete(ctx context.Context, id, actorUserID string) error
	// MaterializeRecurring walks every active scheduled template whose
	// next_occurrence_at is on or before today (Europe/Paris) and
	// creates an expense per due occurrence, advancing
	// next_occurrence_at after each successful Create. Idempotent.
	// ActorUserID gates user-facing invocations; pass empty for cron
	// callers (the AdminKey gate at transport stands in).
	MaterializeRecurring(ctx context.Context, actorUserID string) (*MaterializeSummary, error)
}

// AlertsHook is the narrow contract this package needs from the
// alerts usecase. Defined here so templates stays a leaf.
type AlertsHook interface {
	FirePendingCompletion(ctx context.Context, exp entities.Expense) (*entities.Alert, error)
}

// ExpensesHook is the narrow contract this package needs from the
// expenses usecase: mint a fresh Expense row from a draft. Defined
// here (with an entity-only signature) so the templates package
// stays free of any usecase-to-usecase import. The composition
// root wires an adapter that bridges to expenses.Usecases.
type ExpensesHook interface {
	Create(ctx context.Context, draft entities.ExpenseDraft) (*entities.Expense, error)
}

type usecases struct {
	logger       *zap.Logger
	templates    interfaces.TemplatesStore
	foyers       interfaces.FoyersStore
	validator    interfaces.TemplateValidator
	builder      *builder
	materializer *materializer
	location     *time.Location
}

// New builds a templates usecase. The materializer pins to
// Europe/Paris so "every 1st of the month at midnight" fires on the
// calendar day the user expects, not the UTC equivalent.
//
// `alerts` may be nil during tests — the materializer guards every
// hook call.
func New(
	logger *zap.Logger,
	templates interfaces.TemplatesStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	expenses ExpensesHook,
	validator interfaces.TemplateValidator,
	alerts AlertsHook,
) Usecases {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		// Fallback to UTC — Europe/Paris should always be available,
		// but don't crash the app on an exotic build.
		loc = time.UTC
	}
	now := time.Now
	return &usecases{
		logger:       logger.Named("usecases.templates"),
		templates:    templates,
		foyers:       foyers,
		validator:    validator,
		builder:      newBuilder(copros, now),
		materializer: newMaterializer(logger, templates, expenses, alerts, now),
		location:     loc,
	}
}

// List returns every template in the copro. Foyer-membership gated —
// financial template details (amounts, payer, schedule) shouldn't
// leak to non-foyer authenticated users.
func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.ExpenseTemplate, error) {
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	return uc.templates.List(ctx)
}

// Create validates the draft, builds a fresh template, persists.
func (uc *usecases) Create(ctx context.Context, in CreateTemplateInput) (*entities.ExpenseTemplate, error) {
	log := uc.logger.With(zap.String("method", "Create"))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	if err := uc.validator.Validate(ctx, in.ExpenseTemplateDraft); err != nil {
		return nil, err
	}
	t, err := uc.builder.build(ctx, in.ExpenseTemplateDraft)
	if err != nil {
		return nil, fmt.Errorf("build template: %w", err)
	}
	if err := uc.templates.Create(ctx, t); err != nil {
		log.Error("create failed", zap.Error(err))
		return nil, fmt.Errorf("create template: %w", err)
	}
	log.Info("Success", zap.String("template_id", t.ID))
	return &t, nil
}

// Update validates the draft, rebuilds the existing template
// (preserving identity + the running schedule cursor), persists.
func (uc *usecases) Update(ctx context.Context, id string, in CreateTemplateInput) (*entities.ExpenseTemplate, error) {
	log := uc.logger.With(zap.String("method", "Update"), zap.String("template_id", id))

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
	if err := uc.validator.Validate(ctx, in.ExpenseTemplateDraft); err != nil {
		return nil, err
	}
	updated := uc.builder.rebuild(*existing, in.ExpenseTemplateDraft)
	if err := uc.templates.Update(ctx, updated); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update template: %w", err)
	}
	log.Info("Success")
	return &updated, nil
}

// Delete removes the row.
func (uc *usecases) Delete(ctx context.Context, id, actorUserID string) error {
	log := uc.logger.With(zap.String("method", "Delete"), zap.String("template_id", id))

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

// MaterializeRecurring is the cron / lazy-on-load entry point. The
// orchestration here is just the auth gate + cutoff calculation;
// the actual fire loop lives in materialize.go.
func (uc *usecases) MaterializeRecurring(ctx context.Context, actorUserID string) (*MaterializeSummary, error) {
	log := uc.logger.With(zap.String("method", "MaterializeRecurring"))

	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	cutoff := endOfDay(uc.materializer.now().In(uc.location), uc.location)
	summary, err := uc.materializer.run(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	log.Info("Success",
		zap.Int("templates_processed", summary.TemplatesProcessed),
		zap.Int("expenses_created", summary.ExpensesCreated),
		zap.Int("errors", len(summary.Errors)),
	)
	return summary, nil
}

func (uc *usecases) authorize(ctx context.Context, actorUserID string) error {
	return authz.RequireFoyerMember(ctx, uc.foyers, actorUserID)
}

// endOfDay returns 23:59:59 in the supplied location for the given
// instant's calendar day. The materializer cutoff: a template "for
// the 1st of each month" fires on the local calendar day, not on
// the previous day at 22:00 UTC.
func endOfDay(t time.Time, loc *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, loc)
}
