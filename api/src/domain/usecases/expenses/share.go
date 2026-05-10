// Package expenses — share computation lives here, separate from the
// CRUD orchestration so the formula for each distribution mode is
// readable as a single block. Three modes compute synchronously
// (Equal / Tantièmes / Custom); water_3_meters needs the meters store
// and dispatches via computeSharesOrPending.
package expenses

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// computeShares applies the chosen distribution mode and returns the
// (RDC, 1er) cents pair for the modes that can run synchronously
// without an extra Firestore lookup. The invariant
// ShareRDC + Share1er == AmountCents is enforced for every mode
// (rounding remainder routes to the payer).
//
// When TrustExplicitShares is set, the supplied shares are taken
// verbatim regardless of mode — the historical preservation path
// used by the CSV import. Without that flag the `water_3_meters`
// mode is rejected here: it requires ctx + the meters store, so
// callers must use computeSharesOrPending (which dispatches) instead
// of this function directly.
func computeShares(in CreateInput, rdc, premier *entities.Foyer, copro *entities.Copro) (int, int, error) {
	if in.TrustExplicitShares {
		return validatedExplicitShares(in)
	}
	switch in.DistributionMode {
	case entities.DistributionModeEqual:
		shareRDC, share1er := equalShares(in, rdc)
		return shareRDC, share1er, nil
	case entities.DistributionModeTantiemes:
		return tantiemesShares(in, rdc, premier, copro)
	case entities.DistributionModeCustom:
		shareRDC, share1er, err := validatedExplicitShares(in)
		return shareRDC, share1er, err
	case entities.DistributionModeWater3Meters:
		// Reachable only via the synchronous Upsert path (CSV import) —
		// when TrustExplicitShares is false, the formula needs ctx + the
		// meters store and the caller must dispatch via the usecase
		// method.
		return 0, 0, entities.ValidationError{
			Key:     "distribution_mode",
			Message: "water_3_meters: shares must be supplied explicitly when imported (use trust_explicit_shares)",
		}
	default:
		return 0, 0, entities.ValidationError{Key: "distribution_mode", Message: "unknown mode"}
	}
}

// computeSharesOrPending wraps computeShares with the pending
// short-circuit (pending rows store 0/0 regardless of mode) and the
// water_3_meters dispatch (formula needs the meters store).
func (uc *usecases) computeSharesOrPending(ctx context.Context, in CreateInput, rdc, premier *entities.Foyer, copro *entities.Copro) (int, int, error) {
	if in.AmountPending {
		return 0, 0, nil
	}
	if in.TrustExplicitShares {
		// CSV-import path: shares preserved verbatim regardless of mode.
		return computeShares(in, rdc, premier, copro)
	}
	if in.DistributionMode == entities.DistributionModeWater3Meters {
		return uc.computeWaterShares(ctx, in, rdc)
	}
	return computeShares(in, rdc, premier, copro)
}

// equalShares splits the amount in half and routes the rounding
// remainder (1¢) to the payer.
func equalShares(in CreateInput, rdc *entities.Foyer) (int, int) {
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
	return shareRDC, share1er
}

// tantiemesShares applies the foyer parts ratio. The integer math
// `amount * parts / total` plus a remainder allocation to the payer
// keeps Σ shares == amount even with non-divisible totals.
func tantiemesShares(in CreateInput, rdc, premier *entities.Foyer, copro *entities.Copro) (int, int, error) {
	if copro.TotalParts <= 0 {
		return 0, 0, entities.ValidationError{Key: "copro.total_parts", Message: "must be > 0"}
	}
	if rdc.Parts+premier.Parts != copro.TotalParts {
		return 0, 0, entities.ValidationError{
			Key:     "foyers.parts",
			Message: fmt.Sprintf("Σ parts (%d) ≠ copro.total_parts (%d)", rdc.Parts+premier.Parts, copro.TotalParts),
		}
	}
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
}

// validatedExplicitShares enforces the sum invariant + non-negative
// guard for both Custom mode and the TrustExplicitShares pathway.
func validatedExplicitShares(in CreateInput) (int, int, error) {
	if in.ShareRDCCents+in.Share1erCents != in.AmountCents {
		return 0, 0, entities.ValidationError{
			Key: "shares",
			Message: fmt.Sprintf(
				"share_rdc_cents + share_1er_cents (%d) ≠ amount_cents (%d)",
				in.ShareRDCCents+in.Share1erCents, in.AmountCents,
			),
		}
	}
	if in.ShareRDCCents < 0 || in.Share1erCents < 0 {
		return 0, 0, entities.ValidationError{Key: "shares", Message: "shares must be >= 0"}
	}
	return in.ShareRDCCents, in.Share1erCents, nil
}

// computeWaterShares applies the 3-meter water formula:
//
//	Δcommon = curr.common - prev.common
//	Δrdc    = curr.rdc    - prev.rdc
//	Δ1er    = curr.premier - prev.premier
//	total   = Δcommon + Δrdc + Δ1er
//	share_rdc = round((Δrdc + Δcommon/2) / total × amount)
//	share_1er = amount - share_rdc   (carries the rounding remainder)
//
// Returns ValidationError on missing readings, total ≤ 0, or any
// negative delta (real meters only count up; a roll-back means data
// entry error or meter replacement and the user must reconcile
// manually).
func (uc *usecases) computeWaterShares(ctx context.Context, in CreateInput, _ *entities.Foyer) (int, int, error) {
	if uc.meters == nil {
		return 0, 0, fmt.Errorf("expenses: water_3_meters requires the meters store but none was wired")
	}
	if !entities.IsValidMeterPeriod(in.MeterReadingPeriod) {
		return 0, 0, entities.ValidationError{Key: "meter_reading_period", Message: "must match YYYY-MM"}
	}
	curr, err := uc.meters.FindByPeriod(ctx, in.MeterReadingPeriod)
	if err != nil {
		return 0, 0, fmt.Errorf("find meter %q: %w", in.MeterReadingPeriod, err)
	}
	if curr == nil {
		return 0, 0, entities.ValidationError{
			Key:     "meter_reading_period",
			Message: fmt.Sprintf("aucune lecture pour la période %s — capture-la avant", in.MeterReadingPeriod),
		}
	}
	prev, err := uc.meters.FindPriorPeriod(ctx, in.MeterReadingPeriod)
	if err != nil {
		return 0, 0, fmt.Errorf("find prior period: %w", err)
	}
	if prev == nil {
		return 0, 0, entities.ValidationError{
			Key:     "meter_reading_period",
			Message: "aucune période antérieure — bascule en mode manuel pour la première facture d'eau",
		}
	}
	dCommon := curr.CommonM3 - prev.CommonM3
	dRDC := curr.RDCM3 - prev.RDCM3
	d1er := curr.PremierM3 - prev.PremierM3
	if dCommon < 0 || dRDC < 0 || d1er < 0 {
		return 0, 0, entities.ValidationError{
			Key:     "meter_reading_period",
			Message: "delta négatif détecté entre deux lectures — corrige la lecture avant de calculer la facture",
		}
	}
	total := dCommon + dRDC + d1er
	if total <= 0 {
		return 0, 0, entities.ValidationError{
			Key:     "meter_reading_period",
			Message: "consommation totale nulle entre les deux lectures — bascule en mode manuel",
		}
	}
	rdcShare := (dRDC + dCommon/2) / total
	shareRDCCents := int(math.Round(rdcShare * float64(in.AmountCents)))
	if shareRDCCents < 0 {
		shareRDCCents = 0
	}
	if shareRDCCents > in.AmountCents {
		shareRDCCents = in.AmountCents
	}
	share1erCents := in.AmountCents - shareRDCCents
	return shareRDCCents, share1erCents, nil
}

// normalizeMeterPeriod returns the period only when the mode is
// `water_3_meters`. Other modes drop the field — keeps the persisted
// doc clean and avoids surprising readers who switch a row back from
// water_3_meters to (say) custom and notice an orphan period.
func normalizeMeterPeriod(mode entities.DistributionMode, period string) string {
	if mode != entities.DistributionModeWater3Meters {
		return ""
	}
	return strings.TrimSpace(period)
}
