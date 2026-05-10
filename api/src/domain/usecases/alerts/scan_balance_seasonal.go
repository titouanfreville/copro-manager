package alerts

import (
	"context"
	"fmt"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/core/authz"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// scanBalanceSeasonal fires the balance_seasonal alert on Jul 15 and
// Dec 15 (Europe/Paris) when the live ledger balance is non-zero.
// Recipients: both foyers — either can act. Per-recipient idempotency
// via the `:foyerID` suffix on the dedupe key.
func (uc *usecases) scanBalanceSeasonal(ctx context.Context, summary *ScanSummary) error {
	now := uc.now().In(uc.location)
	half := seasonalHalf(now)
	if half == "" {
		return nil // not a fire date
	}

	net, rdc, premier, err := uc.computeLiveBalance(ctx)
	if err != nil {
		return err
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
			DedupeKey:        dedupe + ":" + foyerID,
			Title:            "Solde à équilibrer",
			Body:             body,
			DeepLink:         "/expenses",
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

// seasonalHalf returns "h1" on Jul 15 and "h2" on Dec 15, empty
// otherwise. The two anchors are calendar-aligned so both households
// know to expect them.
func seasonalHalf(now time.Time) string {
	switch {
	case now.Month() == time.July && now.Day() == 15:
		return "h1"
	case now.Month() == time.December && now.Day() == 15:
		return "h2"
	}
	return ""
}

// computeLiveBalance mirrors the frontend $lib/balance formula —
// settlements net the running expense delta. Returns net from RDC's
// perspective: positive → 1er owes RDC; negative → RDC owes 1er.
func (uc *usecases) computeLiveBalance(ctx context.Context) (net int, rdc, premier *entities.Foyer, err error) {
	expenses, err := uc.expenses.List(ctx)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("list expenses: %w", err)
	}
	settlements, err := uc.settlements.List(ctx)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("list settlements: %w", err)
	}
	rdc, premier, err = authz.LoadBothFoyers(ctx, uc.foyers)
	if err != nil {
		return 0, nil, nil, err
	}
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
	return net, rdc, premier, nil
}
