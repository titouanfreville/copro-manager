package alerts

import (
	"context"
	"fmt"

	"github.com/titouanfreville/copro-manager/api/src/core/authz"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// scanMonthlyMeterReading fires the monthly_meter_reading alert from
// the 28th of the month (Europe/Paris) onward if no MeterReading
// exists for the current YYYY-MM. Recipients: both foyers — water
// consumption is shared. Per-recipient idempotency via the `:foyerID`
// suffix on the dedupe key. The `>= 28` guard plus dedupe makes the
// alert self-healing across cron misses.
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

	rdc, premier, err := authz.LoadBothFoyers(ctx, uc.foyers)
	if err != nil {
		return err
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
