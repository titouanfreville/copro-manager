package alerts

import (
	"context"
	"fmt"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/core/authz"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// scanContractExpiring fires once per active contract whose end_date
// is within ContractExpiringSoonDays. Both foyers receive the alert
// (the contract binds the building, not a household). Idempotent via
// the (contract_id, end_date) dedupe key — renewing the contract
// (writing a new end_date) yields a fresh dedupe so the next 30-day
// window will fire. Cancelled / already-expired contracts are skipped
// by Contract.IsExpiringSoon.
func (uc *usecases) scanContractExpiring(ctx context.Context, summary *ScanSummary) error {
	if uc.contracts == nil {
		return nil
	}
	now := uc.now().In(uc.location)
	contracts, err := uc.contracts.List(ctx)
	if err != nil {
		return fmt.Errorf("list contracts: %w", err)
	}
	if len(contracts) == 0 {
		return nil
	}

	rdc, premier, err := authz.LoadBothFoyers(ctx, uc.foyers)
	if err != nil {
		return err
	}

	for _, c := range contracts {
		if !c.IsExpiringSoon(now) {
			continue
		}
		if err := uc.fireContractExpiring(ctx, c, now, []string{rdc.ID, premier.ID}, summary); err != nil {
			return err
		}
	}
	return nil
}

func (uc *usecases) fireContractExpiring(ctx context.Context, c entities.Contract, now time.Time, recipients []string, summary *ScanSummary) error {
	dedupe := entities.DedupeKeyContractExpiring(c.ID, c.EndDate)
	days := entities.DaysUntil(now, c.EndDate)
	body := contractExpiringBody(c.Name, c.EndDate, days)
	for _, foyerID := range recipients {
		_, err := uc.Fire(ctx, FireInput{
			Kind:             entities.AlertKindContractExpiring,
			RecipientFoyerID: foyerID,
			DedupeKey:        dedupe + ":" + foyerID,
			Title:            "Contrat à renouveler",
			Body:             body,
			DeepLink:         "/contracts?focus=" + c.ID,
			Payload: map[string]any{
				"contract_id":   c.ID,
				"contract_name": c.Name,
				"end_date":      c.EndDate.Format("2006-01-02"),
				"society_name":  c.Society.Name,
			},
		})
		if err != nil {
			return fmt.Errorf("fire contract_expiring: %w", err)
		}
		summary.ContractExpiringFired++
	}
	return nil
}

// contractExpiringBody renders the alert body. Days is the date-only
// delta from today to end_date; the message reads naturally for the
// boundary cases (same-day, tomorrow) instead of "expire dans 0 jour(s)".
func contractExpiringBody(name string, endDate time.Time, days int) string {
	dateLabel := endDate.Format("02/01/2006")
	switch {
	case days <= 0:
		return fmt.Sprintf("« %s » expire aujourd'hui (%s).", name, dateLabel)
	case days == 1:
		return fmt.Sprintf("« %s » expire demain (%s).", name, dateLabel)
	default:
		return fmt.Sprintf("« %s » expire dans %d jours (%s).", name, days, dateLabel)
	}
}
