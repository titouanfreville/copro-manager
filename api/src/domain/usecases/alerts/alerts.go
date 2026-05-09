// Package alerts owns the in-app notification feed and the Web Push
// fan-out that piggybacks on it. Domain events (expense create, template
// materialize, settlement zero, daily cron) call Fire; the daily cron
// runs ScanTimeBased to age-out missing-receipt and seasonal-balance
// alerts.
package alerts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// FireInput captures everything Fire needs to write an alert + dispatch
// the push fan-out. Title/body are localized French copy used both in
// the in-app card and the push toast.
type FireInput struct {
	Kind             entities.AlertKind
	RecipientFoyerID string
	DedupeKey        string
	Title            string
	Body             string
	DeepLink         string
	Payload          map[string]any
}

// Usecases is the alerts domain contract.
type Usecases interface {
	// Fire writes the alert (idempotent on dedupe key) and best-effort
	// pushes it to every device subscribed for the recipient foyer.
	Fire(ctx context.Context, in FireInput) (*entities.Alert, error)

	List(ctx context.Context, actorUserID string) ([]entities.Alert, error)
	MarkRead(ctx context.Context, id, actorUserID string) error
	Dismiss(ctx context.Context, id, actorUserID string) error
	MarkAllRead(ctx context.Context, actorUserID string) error

	// ScanTimeBased is the daily cron entrypoint. Walks expenses without
	// attachments and fires missing_receipt by stage; on Jul 15 / Dec 15
	// also evaluates the running balance and fires balance_seasonal.
	ScanTimeBased(ctx context.Context) (*ScanSummary, error)

	// ResolveByPrefix exposes the auto-resolve hook other usecases call
	// (e.g. attachment recorded → resolve missing_receipt:{exp.id}:*).
	ResolveByPrefix(ctx context.Context, prefix string) error
	// ResolveByDedupeKey exposes the single-key auto-resolve.
	ResolveByDedupeKey(ctx context.Context, dedupeKey string) error
	// ResolveByExpense clears every alert referencing the given expense_id.
	ResolveByExpense(ctx context.Context, expenseID string) error

	// ─── Convenience fan-out helpers used by domain hooks ────────
	// Each consumer (expenses, templates, settlements) defines its own
	// narrow hook interface that picks just the methods it needs from
	// this list — Go's structural typing makes alerts.Usecases satisfy
	// each automatically, no extra wiring needed.

	// FirePendingCompletion fires when a scheduled template materializes
	// a row with amount_pending=true. Recipient: the payer foyer.
	FirePendingCompletion(ctx context.Context, exp entities.Expense) (*entities.Alert, error)

	// FirePeerExpenseAdded fires once per new expense, alerting the
	// foyer that didn't author it. The recipient is computed from the
	// payer + the actor's foyer; pass `recipientFoyerID` explicitly
	// (caller already knows it) to avoid another store hop.
	FirePeerExpenseAdded(ctx context.Context, exp entities.Expense, recipientFoyerID string) (*entities.Alert, error)

	// ResolveMissingReceipt clears every missing_receipt:* stage for
	// the expense — used when an attachment is recorded.
	ResolveMissingReceipt(ctx context.Context, expenseID string) error

	// ResolvePendingCompletion clears the pending alert for the expense
	// — used when amount_pending flips false on Update.
	ResolvePendingCompletion(ctx context.Context, expenseID string) error

	// ResolveSeasonalAll clears every non-resolved balance_seasonal
	// alert — used by settlements on every Create/Update/Delete that
	// flips the live balance to zero. Idempotent.
	ResolveSeasonalAll(ctx context.Context) error
}

// ScanSummary lets the cron route render a useful response.
type ScanSummary struct {
	MissingReceiptFired      int `json:"missing_receipt_fired"`
	SeasonalFired            int `json:"seasonal_fired"`
	MonthlyMeterReadingFired int `json:"monthly_meter_reading_fired"`
}

type usecases struct {
	logger      *zap.Logger
	alerts      interfaces.AlertsStore
	push        interfaces.PushSubscriptionsStore
	sender      interfaces.PushSender
	expenses    interfaces.ExpensesStore
	attachments interfaces.AttachmentsStore
	settlements interfaces.SettlementsStore
	foyers      interfaces.FoyersStore
	copros      interfaces.CoprosStore
	meters      interfaces.MetersStore
	now         func() time.Time
	location    *time.Location
}

// New builds an alerts usecase. push and sender may be nil when the
// VAPID keys aren't configured locally — Fire then skips the fan-out
// and only writes the in-app feed entry.
func New(
	logger *zap.Logger,
	alerts interfaces.AlertsStore,
	push interfaces.PushSubscriptionsStore,
	sender interfaces.PushSender,
	expenses interfaces.ExpensesStore,
	attachments interfaces.AttachmentsStore,
	settlements interfaces.SettlementsStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	meters interfaces.MetersStore,
) Usecases {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		loc = time.UTC
	}
	return &usecases{
		logger:      logger.Named("usecases.alerts"),
		alerts:      alerts,
		push:        push,
		sender:      sender,
		expenses:    expenses,
		attachments: attachments,
		settlements: settlements,
		foyers:      foyers,
		copros:      copros,
		meters:      meters,
		now:         time.Now,
		location:    loc,
	}
}

func (uc *usecases) Fire(ctx context.Context, in FireInput) (*entities.Alert, error) {
	log := uc.logger.With(
		zap.String("method", "Fire"),
		zap.String("kind", string(in.Kind)),
		zap.String("recipient", in.RecipientFoyerID),
		zap.String("dedupe", in.DedupeKey),
	)
	if !entities.IsKnownAlertKind(in.Kind) {
		return nil, fmt.Errorf("alerts: unknown kind %q", in.Kind)
	}
	if in.RecipientFoyerID == "" || in.DedupeKey == "" {
		return nil, fmt.Errorf("alerts: recipient_foyer_id and dedupe_key are required")
	}
	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return nil, fmt.Errorf("copro lookup: %w", err)
	}
	now := uc.now()
	a := entities.Alert{
		ID:               uuid.NewString(),
		CoproID:          copro.ID,
		Kind:             in.Kind,
		RecipientFoyerID: in.RecipientFoyerID,
		DedupeKey:        in.DedupeKey,
		Payload:          in.Payload,
		DeepLink:         in.DeepLink,
		FiredAt:          now,
	}
	saved, created, err := uc.alerts.CreateIfNew(ctx, a)
	if err != nil {
		log.Error("create failed", zap.Error(err))
		return nil, fmt.Errorf("create alert: %w", err)
	}
	if !created {
		// Already fired with this dedupe key — quietly return the existing
		// row so callers can stay idempotent.
		return saved, nil
	}

	// Push fan-out is best-effort: a delivery failure must NOT block the
	// in-app feed entry (NFR21). We log and prune dead subscriptions, but
	// the alert stays.
	if uc.push != nil && uc.sender != nil {
		// Decouple from the request context (fan-out can outlive the
		// originating request) but keep a finite deadline so the
		// goroutine doesn't dangle if web push services hang.
		fanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		go func() {
			defer cancel()
			defer func() {
				if r := recover(); r != nil {
					uc.logger.Error("push: fan-out panic recovered", zap.Any("recover", r))
				}
			}()
			uc.fanOutPush(fanCtx, *saved, in)
		}()
	}
	log.Info("Success", zap.String("alert_id", saved.ID))
	return saved, nil
}

func (uc *usecases) fanOutPush(ctx context.Context, a entities.Alert, in FireInput) {
	subs, err := uc.push.ListByFoyer(ctx, a.RecipientFoyerID)
	if err != nil {
		uc.logger.Warn("push: list subscriptions failed", zap.Error(err))
		return
	}
	if len(subs) == 0 {
		return
	}
	payload := interfaces.PushPayload{
		Title:    in.Title,
		Body:     in.Body,
		DeepLink: a.DeepLink,
		AlertID:  a.ID,
	}
	for _, sub := range subs {
		gone, err := uc.sender.Send(ctx, sub, payload)
		if err != nil {
			// Log a hashed prefix of the endpoint so debug breadcrumbs
			// stay useful without writing the full per-device URL
			// (treated as quasi-PII per NFR16).
			uc.logger.Warn("push: send failed",
				zap.String("endpoint_hash", endpointHash(sub.Endpoint)),
				zap.Error(err))
			continue
		}
		if gone {
			if delErr := uc.push.DeleteByEndpoint(ctx, sub.Endpoint); delErr != nil {
				uc.logger.Warn("push: prune stale failed", zap.Error(delErr))
			}
		}
	}
}

// endpointHash returns the 12-character hex prefix of the SHA-256 of an
// endpoint URL — stable per-endpoint identifier for log correlation
// without logging the full URL (which is quasi-PII).
func endpointHash(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:6])
}

func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.Alert, error) {
	foyer, err := uc.actorFoyer(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	return uc.alerts.ListByFoyer(ctx, foyer.ID, false)
}

func (uc *usecases) MarkRead(ctx context.Context, id, actorUserID string) error {
	a, err := uc.fetchForActor(ctx, id, actorUserID)
	if err != nil {
		return err
	}
	if a.ReadAt != nil {
		return nil
	}
	// Field-level update so a concurrent Dismiss on the same row from
	// another device doesn't race-clobber dismissed_at.
	return uc.alerts.MarkRead(ctx, a.ID, uc.now())
}

func (uc *usecases) Dismiss(ctx context.Context, id, actorUserID string) error {
	a, err := uc.fetchForActor(ctx, id, actorUserID)
	if err != nil {
		return err
	}
	if a.DismissedAt != nil {
		return nil
	}
	return uc.alerts.MarkDismissed(ctx, a.ID, uc.now())
}

func (uc *usecases) MarkAllRead(ctx context.Context, actorUserID string) error {
	foyer, err := uc.actorFoyer(ctx, actorUserID)
	if err != nil {
		return err
	}
	rows, err := uc.alerts.ListByFoyer(ctx, foyer.ID, false)
	if err != nil {
		return fmt.Errorf("list for mark-all: %w", err)
	}
	now := uc.now()
	for i := range rows {
		if rows[i].ReadAt != nil {
			continue
		}
		if err := uc.alerts.MarkRead(ctx, rows[i].ID, now); err != nil {
			return fmt.Errorf("mark read: %w", err)
		}
	}
	return nil
}

func (uc *usecases) ResolveByPrefix(ctx context.Context, prefix string) error {
	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return err
	}
	now := uc.now()
	return uc.alerts.ResolveByPrefix(ctx, copro.ID, prefix, entities.Alert{ResolvedAt: &now})
}

func (uc *usecases) ResolveByDedupeKey(ctx context.Context, dedupeKey string) error {
	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return err
	}
	now := uc.now()
	return uc.alerts.ResolveByDedupeKey(ctx, copro.ID, dedupeKey, entities.Alert{ResolvedAt: &now})
}

func (uc *usecases) ResolveByExpense(ctx context.Context, expenseID string) error {
	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return err
	}
	now := uc.now()
	return uc.alerts.ResolveByExpense(ctx, copro.ID, expenseID, entities.Alert{ResolvedAt: &now})
}

// ─── Convenience hooks ─────────────────────────────────────────────

func (uc *usecases) FirePendingCompletion(ctx context.Context, exp entities.Expense) (*entities.Alert, error) {
	return uc.Fire(ctx, FireInput{
		Kind:             entities.AlertKindPendingCompletion,
		RecipientFoyerID: exp.PayerFoyerID,
		DedupeKey:        entities.DedupeKeyPendingCompletion(exp.ID),
		Title:            "Dépense à compléter",
		Body:             fmt.Sprintf("« %s » attend son montant.", exp.Name),
		DeepLink:         "/expenses?focus=" + exp.ID,
		Payload: map[string]any{
			"expense_id":     exp.ID,
			"expense_name":   exp.Name,
			"payer_foyer_id": exp.PayerFoyerID,
		},
	})
}

func (uc *usecases) FirePeerExpenseAdded(ctx context.Context, exp entities.Expense, recipientFoyerID string) (*entities.Alert, error) {
	if recipientFoyerID == "" {
		return nil, fmt.Errorf("alerts: peer recipient required")
	}
	return uc.Fire(ctx, FireInput{
		Kind:             entities.AlertKindPeerExpenseAdded,
		RecipientFoyerID: recipientFoyerID,
		DedupeKey:        entities.DedupeKeyPeerExpenseAdded(exp.ID),
		Title:            "Nouvelle dépense",
		Body:             fmt.Sprintf("« %s » · %.2f €", exp.Name, float64(exp.AmountCents)/100),
		DeepLink:         "/expenses?focus=" + exp.ID,
		Payload: map[string]any{
			"expense_id":      exp.ID,
			"expense_name":    exp.Name,
			"amount_cents":    exp.AmountCents,
			"author_foyer_id": exp.PayerFoyerID,
		},
	})
}

func (uc *usecases) ResolveMissingReceipt(ctx context.Context, expenseID string) error {
	return uc.ResolveByPrefix(ctx, entities.DedupeKeyMissingReceiptPrefix(expenseID))
}

func (uc *usecases) ResolvePendingCompletion(ctx context.Context, expenseID string) error {
	return uc.ResolveByDedupeKey(ctx, entities.DedupeKeyPendingCompletion(expenseID))
}

func (uc *usecases) ResolveSeasonalAll(ctx context.Context) error {
	return uc.ResolveByPrefix(ctx, string(entities.AlertKindBalanceSeasonal)+":")
}

// ScanTimeBased runs the daily checks. Idempotent — every alert it
// produces uses a stable dedupe key so re-running on the same day is a
// no-op.
func (uc *usecases) ScanTimeBased(ctx context.Context) (*ScanSummary, error) {
	log := uc.logger.With(zap.String("method", "ScanTimeBased"))
	summary := &ScanSummary{}

	// Missing-receipt: walk every non-settled, non-pending expense whose
	// attachment subcollection is empty. Compute the stage from age and
	// fire only when today aligns with a cadence step.
	if err := uc.scanMissingReceipts(ctx, summary); err != nil {
		log.Error("missing-receipt scan failed", zap.Error(err))
		return summary, err
	}

	// Balance seasonal: fires only on Jul 15 + Dec 15 in Europe/Paris.
	if err := uc.scanBalanceSeasonal(ctx, summary); err != nil {
		log.Error("seasonal scan failed", zap.Error(err))
		return summary, err
	}

	// Monthly meter reading: fires on the 28th of every month in
	// Europe/Paris if the current period (YYYY-MM) has no MeterReading.
	if err := uc.scanMonthlyMeterReading(ctx, summary); err != nil {
		log.Error("monthly meter reading scan failed", zap.Error(err))
		return summary, err
	}

	log.Info("Success",
		zap.Int("missing_receipt_fired", summary.MissingReceiptFired),
		zap.Int("seasonal_fired", summary.SeasonalFired),
		zap.Int("monthly_meter_reading_fired", summary.MonthlyMeterReadingFired),
	)
	return summary, nil
}

func (uc *usecases) scanMissingReceipts(ctx context.Context, summary *ScanSummary) error {
	now := uc.now().In(uc.location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, uc.location)

	expenses, err := uc.expenses.List(ctx)
	if err != nil {
		return fmt.Errorf("list expenses: %w", err)
	}
	for _, e := range expenses {
		if e.Settled || e.AmountPending {
			continue
		}
		// Skip rows that already have an attachment.
		if uc.attachments != nil {
			n, err := uc.attachments.Count(ctx, e.ID)
			if err != nil {
				return fmt.Errorf("count attachments: %w", err)
			}
			if n > 0 {
				continue
			}
		} else if len(e.Attachments) > 0 {
			continue
		}
		created := time.Date(e.CreatedAt.Year(), e.CreatedAt.Month(), e.CreatedAt.Day(), 0, 0, 0, 0, uc.location)
		days := int(today.Sub(created).Hours() / 24)
		stage := entities.MissingReceiptStage(days)
		if stage == "" {
			continue
		}
		// Fire (idempotent — re-runs on the same day are no-ops).
		_, err := uc.Fire(ctx, FireInput{
			Kind:             entities.AlertKindMissingReceipt,
			RecipientFoyerID: e.PayerFoyerID,
			DedupeKey:        entities.DedupeKeyMissingReceipt(e.ID, stage),
			Title:            "Justificatif manquant",
			Body:             fmt.Sprintf("« %s » est sans justificatif depuis %d jours.", e.Name, days),
			DeepLink:         "/expenses?focus=" + e.ID,
			Payload: map[string]any{
				"expense_id":   e.ID,
				"expense_name": e.Name,
				"stage":        stage,
				"amount_cents": e.AmountCents,
			},
		})
		if err != nil {
			return fmt.Errorf("fire missing_receipt: %w", err)
		}
		summary.MissingReceiptFired++
	}
	return nil
}

func (uc *usecases) scanBalanceSeasonal(ctx context.Context, summary *ScanSummary) error {
	now := uc.now().In(uc.location)
	month, day := now.Month(), now.Day()
	var half string
	switch {
	case month == time.July && day == 15:
		half = "h1"
	case month == time.December && day == 15:
		half = "h2"
	default:
		return nil // not a fire date
	}

	// Compute the live balance from RDC's perspective. Mirrors the
	// frontend formula in $lib/balance to keep the two paths semantically
	// identical.
	expenses, err := uc.expenses.List(ctx)
	if err != nil {
		return fmt.Errorf("list expenses: %w", err)
	}
	settlements, err := uc.settlements.List(ctx)
	if err != nil {
		return fmt.Errorf("list settlements: %w", err)
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
	net := 0
	for _, e := range expenses {
		if e.Settled || e.AmountPending {
			continue
		}
		switch e.PayerFoyerID {
		case rdc.ID:
			net += e.Share1erCents
		case premier.ID:
			net -= e.ShareRDCCents
		}
	}
	for _, s := range settlements {
		if s.FromFoyerID == premier.ID && s.ToFoyerID == rdc.ID {
			net -= s.AmountCents
		} else if s.FromFoyerID == rdc.ID && s.ToFoyerID == premier.ID {
			net += s.AmountCents
		}
	}
	if net == 0 {
		return nil // even — no nudge
	}

	owedBy, owedTo, abs := premier.ID, rdc.ID, net
	if net < 0 {
		owedBy, owedTo, abs = rdc.ID, premier.ID, -net
	}
	body := fmt.Sprintf("Le compte commun n'est pas équilibré (%.2f €) — pensez à régler.", float64(abs)/100)
	dedupe := entities.DedupeKeyBalanceSeasonal(now.Year(), half)
	for _, foyerID := range []string{rdc.ID, premier.ID} {
		_, err := uc.Fire(ctx, FireInput{
			Kind:             entities.AlertKindBalanceSeasonal,
			RecipientFoyerID: foyerID,
			// One dedupe key per (year, half) — but we'd dedupe across
			// recipients too if we used the bare key. Suffix the foyer to
			// allow per-recipient idempotency.
			DedupeKey: dedupe + ":" + foyerID,
			Title:     "Solde à équilibrer",
			Body:      body,
			DeepLink:  "/expenses",
			Payload: map[string]any{
				"year":      now.Year(),
				"half":      half,
				"net_cents": net,
				"owed_by":   owedBy,
				"owed_to":   owedTo,
			},
		})
		if err != nil {
			return fmt.Errorf("fire balance_seasonal: %w", err)
		}
		summary.SeasonalFired++
	}
	return nil
}

// scanMonthlyMeterReading fires the monthly_meter_reading alert from
// the 28th of the month (Europe/Paris) onward if no MeterReading
// exists for the current YYYY-MM. Recipient: both foyers — water
// consumption is shared. Per-recipient idempotency via the `:foyerID`
// suffix on the dedupe key (mirrors balance_seasonal). The `>= 28`
// guard plus dedupe makes the alert self-healing across cron misses.
func (uc *usecases) scanMonthlyMeterReading(ctx context.Context, summary *ScanSummary) error {
	if uc.meters == nil {
		return nil
	}
	now := uc.now().In(uc.location)
	if now.Day() < 28 {
		return nil
	}
	period := fmt.Sprintf("%04d-%02d", now.Year(), int(now.Month()))
	existing, err := uc.meters.FindByPeriod(ctx, period)
	if err != nil {
		return fmt.Errorf("find meter %q: %w", period, err)
	}
	if existing != nil {
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

	dedupe := entities.DedupeKeyMonthlyMeterReading(period)
	body := fmt.Sprintf("Aucune lecture de compteur pour %s — pense à relever les sous-compteurs avant la facture.", period)
	for _, foyerID := range []string{rdc.ID, premier.ID} {
		_, err := uc.Fire(ctx, FireInput{
			Kind:             entities.AlertKindMonthlyMeterReading,
			RecipientFoyerID: foyerID,
			DedupeKey:        dedupe + ":" + foyerID,
			Title:            "Lecture des sous-compteurs",
			Body:             body,
			DeepLink:         "/meters/new?period=" + period,
			Payload: map[string]any{
				"period": period,
			},
		})
		if err != nil {
			return fmt.Errorf("fire monthly_meter_reading: %w", err)
		}
		summary.MonthlyMeterReadingFired++
	}
	return nil
}

func (uc *usecases) actorFoyer(ctx context.Context, actorUserID string) (*entities.Foyer, error) {
	if actorUserID == "" {
		return nil, entities.AuthorizationError{Code: "actor_required"}
	}
	rdc, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloorRDC)
	if err != nil {
		return nil, fmt.Errorf("find rdc: %w", err)
	}
	premier, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloor1er)
	if err != nil {
		return nil, fmt.Errorf("find 1er: %w", err)
	}
	if rdc == nil || premier == nil {
		return nil, fmt.Errorf("%w: both foyers must exist", domainerrors.ErrNotFound)
	}
	for _, mid := range rdc.MemberIDs {
		if mid == actorUserID {
			return rdc, nil
		}
	}
	for _, mid := range premier.MemberIDs {
		if mid == actorUserID {
			return premier, nil
		}
	}
	return nil, entities.AuthorizationError{Code: "not_foyer_member"}
}

func (uc *usecases) fetchForActor(ctx context.Context, id, actorUserID string) (*entities.Alert, error) {
	foyer, err := uc.actorFoyer(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	a, err := uc.alerts.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find alert: %w", err)
	}
	if a == nil {
		return nil, fmt.Errorf("%w: alert %q", domainerrors.ErrNotFound, id)
	}
	if a.RecipientFoyerID != foyer.ID {
		return nil, entities.AuthorizationError{Code: "not_recipient"}
	}
	return a, nil
}
