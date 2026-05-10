package alerts

import (
	"context"
	"fmt"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// scanMissingReceipts fires the missing_receipt alert on its
// escalating cadence (D+3 / D+10 / W+15 stages) for any non-settled,
// non-pending expense whose attachments list is empty. Idempotent
// via the (expense_id, stage) dedupe key.
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
		hasAttachment, err := uc.expenseHasAttachment(ctx, e)
		if err != nil {
			return err
		}
		if hasAttachment {
			continue
		}
		stage := stageForExpense(today, e, uc.location)
		if stage == "" {
			continue
		}
		if err := uc.fireMissingReceipt(ctx, e, stage, daysSince(today, e, uc.location)); err != nil {
			return err
		}
		summary.MissingReceiptFired++
	}
	return nil
}

// expenseHasAttachment reports whether the expense already has at
// least one attachment in the legacy subcollection or — since the
// migration — as a Document with linked_expense_id. The store
// dependency is checked at call time so a usecase wired without it
// (e.g. tests) falls back to the inline `e.Attachments` field.
func (uc *usecases) expenseHasAttachment(ctx context.Context, e entities.Expense) (bool, error) {
	if uc.attachments != nil {
		n, err := uc.attachments.Count(ctx, e.ID)
		if err != nil {
			return false, fmt.Errorf("count attachments: %w", err)
		}
		return n > 0, nil
	}
	return len(e.Attachments) > 0, nil
}

func stageForExpense(today time.Time, e entities.Expense, loc *time.Location) string {
	created := time.Date(e.CreatedAt.Year(), e.CreatedAt.Month(), e.CreatedAt.Day(), 0, 0, 0, 0, loc)
	days := int(today.Sub(created).Hours() / 24)
	return entities.MissingReceiptStage(days)
}

func daysSince(today time.Time, e entities.Expense, loc *time.Location) int {
	created := time.Date(e.CreatedAt.Year(), e.CreatedAt.Month(), e.CreatedAt.Day(), 0, 0, 0, 0, loc)
	return int(today.Sub(created).Hours() / 24)
}

func (uc *usecases) fireMissingReceipt(ctx context.Context, e entities.Expense, stage string, days int) error {
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
	return nil
}
