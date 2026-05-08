package entities

import (
	"fmt"
	"time"
)

// Frequency is the cadence at which a scheduled template fires.
type Frequency string

const (
	FrequencyMonthly   Frequency = "monthly"
	FrequencyQuarterly Frequency = "quarterly"
	FrequencyYearly    Frequency = "yearly"
)

// IsKnownFrequency reports whether the value is one of the supported cadences.
func IsKnownFrequency(f Frequency) bool {
	switch f {
	case FrequencyMonthly, FrequencyQuarterly, FrequencyYearly:
		return true
	}
	return false
}

// AdvanceDate returns the date one Frequency-step after `from`, clamping the
// day of month to the supplied target (or the last day of the target month
// when the target overflows — e.g. Jan 31 + 1 month → Feb 28/29 rather than
// Go's default Mar 3). Returns an error on unknown frequency so a corrupt
// stored value can't drive an infinite loop in the materializer.
//
// `dayOfMonth` must be 1–31; the function clamps to lastDay(year, month) of
// the resulting month if dayOfMonth exceeds it.
func AdvanceDate(from time.Time, f Frequency, dayOfMonth int) (time.Time, error) {
	if !IsKnownFrequency(f) {
		return time.Time{}, fmt.Errorf("entities: unknown frequency %q", f)
	}
	if dayOfMonth < 1 {
		dayOfMonth = from.Day()
	}

	var year, monthOffset int
	switch f {
	case FrequencyMonthly:
		monthOffset = 1
	case FrequencyQuarterly:
		monthOffset = 3
	case FrequencyYearly:
		year = 1
	}

	// Step into the next year/month combination, then clamp the day to the
	// month's actual last day. We rebuild via time.Date so an overflow
	// (e.g. Feb 30) doesn't silently roll forward into March.
	nextYear := from.Year() + year
	nextMonth := time.Month(int(from.Month()) + monthOffset)
	for nextMonth > 12 {
		nextMonth -= 12
		nextYear++
	}
	last := lastDayOfMonth(nextYear, nextMonth)
	day := dayOfMonth
	if day > last {
		day = last
	}

	return time.Date(
		nextYear, nextMonth, day,
		from.Hour(), from.Minute(), from.Second(), from.Nanosecond(),
		from.Location(),
	), nil
}

// LastDayOfMonth returns the number of days in the given month. Uses the
// "first day of next month minus 1 day" trick to dodge leap-year edges.
func LastDayOfMonth(year int, month time.Month) int {
	firstNext := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
	last := firstNext.AddDate(0, 0, -1)
	return last.Day()
}

// lastDayOfMonth (unexported alias — kept for callers within this file).
func lastDayOfMonth(year int, month time.Month) int {
	return LastDayOfMonth(year, month)
}

// ExpenseTemplate is a saved preset for creating expenses, in two modes:
//
//  1. Manual instantiation: the user picks a template from a list, the
//     create form pre-fills everything (payer, category, mode, optional
//     default amount), and the user types/confirms the actual amount.
//
//  2. Scheduled auto-creation: when ScheduleActive is true, the daily cron
//     job creates expenses on each NextOccurrenceAt, advancing it by
//     Frequency. If AmountDefaultCents > 0 the row is born complete; if
//     AmountDefaultCents == 0 it's born with AmountPending=true and waits
//     for a foyer member to fill in the actual amount.
//
// Schedule fields are all-or-none: when ScheduleActive is true, Frequency,
// DayOfMonth and NextOccurrenceAt must be set. Validated at write time.
type ExpenseTemplate struct {
	ID                 string           `json:"id"`
	CoproID            string           `json:"copro_id"`
	Name               string           `json:"name"`
	AmountDefaultCents int              `json:"amount_default_cents"`
	Currency           string           `json:"currency"`
	CategoryID         string           `json:"category_id"`
	PayerFoyerID       string           `json:"payer_foyer_id"`
	DistributionMode   DistributionMode `json:"distribution_mode"`
	ShareRDCCents      int              `json:"share_rdc_cents,omitempty"`
	Share1erCents      int              `json:"share_1er_cents,omitempty"`
	Note               string           `json:"note,omitempty"`

	ScheduleActive   bool       `json:"schedule_active"`
	Frequency        Frequency  `json:"frequency,omitempty"`
	DayOfMonth       int        `json:"day_of_month,omitempty"`
	NextOccurrenceAt *time.Time `json:"next_occurrence_at,omitempty"`
	EndDate          *time.Time `json:"end_date,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
