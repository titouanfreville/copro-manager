package expenses

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// maxImportAmountCents caps a single CSV row's total at 1M EUR. The legacy
// spreadsheet's biggest row is on the order of a few thousand euros; anything
// above this cap is almost certainly a parse error or a typo we shouldn't
// silently propagate into the ledger.
const maxImportAmountCents = 100_000_000

// ImportSummary is what the CSV import returns to the caller. SkipReasons
// is a histogram of why rows were skipped — the legacy CSV often has rows
// missing a date or marked Paiement complet=FALSE, and surfacing those
// counts helps the operator understand "why did only 4 of my 15 rows
// import?" without digging through logs.
type ImportSummary struct {
	Created     int            `json:"created"`
	Updated     int            `json:"updated"`
	Skipped     int            `json:"skipped"`
	SkipReasons map[string]int `json:"skip_reasons"`
	Errors      []ImportError  `json:"errors"`
	Processed   int            `json:"processed"`
}

// Skip-reason keys (kept stable so the frontend can label them).
const (
	skipPaymentNotComplete = "payment_not_complete"
	skipMissingFields      = "missing_required_field"
	skipMissingDate        = "missing_date"
)

// ImportError captures a single row's failure without aborting the whole
// batch — the import is best-effort so a malformed line doesn't cost the
// rest of the upload.
type ImportError struct {
	Line    int    `json:"line"`
	Item    string `json:"item"`
	Message string `json:"message"`
}

// ImportCSV parses the legacy spreadsheet shape (Item, Date, Date paiement,
// Paiement complet, Total, Répartition, Prorata appliqué, Charge RDC,
// Charge 1er, Notes, Devis/Factures) and upserts every "TRUE" row that has
// a name + date + total. The defaultPayerFoyerID is applied to every row
// since the legacy CSV doesn't track payer identity.
func (uc *usecases) ImportCSV(
	ctx context.Context,
	r io.Reader,
	defaultPayerFoyerID string,
) (*ImportSummary, error) {
	log := uc.logger.With(zap.String("method", "ImportCSV"), zap.String("payer_foyer_id", defaultPayerFoyerID))

	if strings.TrimSpace(defaultPayerFoyerID) == "" {
		return nil, entities.ValidationError{Key: "payer_foyer_id", Message: "required"}
	}

	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // some rows have trailing empties; tolerate

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("csv: read header: %w", err)
	}
	cols, err := mapColumns(header)
	if err != nil {
		return nil, err
	}

	summary := &ImportSummary{
		// Initialize as non-nil so the JSON wire format is `[]` not `null`
		// — saves the frontend from defensive checks.
		Errors:      []ImportError{},
		SkipReasons: map[string]int{},
	}
	lineNo := 1 // header was line 1

	skip := func(reason string) {
		summary.Skipped++
		summary.SkipReasons[reason]++
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		lineNo++
		// Count every attempted row so the summary reconciles:
		// Processed == Created + Updated + Skipped + len(Errors).
		summary.Processed++
		if err != nil {
			summary.Errors = append(summary.Errors, ImportError{
				Line:    lineNo,
				Message: fmt.Sprintf("read row: %v", err),
			})
			continue
		}

		raw := pickRow(row, cols)

		// Skip rows that aren't real expenses.
		if !isTrue(raw.PaymentComplete) {
			skip(skipPaymentNotComplete)
			continue
		}
		if strings.TrimSpace(raw.Item) == "" || strings.TrimSpace(raw.Total) == "" {
			skip(skipMissingFields)
			continue
		}
		if strings.TrimSpace(raw.Date) == "" {
			skip(skipMissingDate)
			continue
		}

		in, err := buildInput(raw, defaultPayerFoyerID)
		if err != nil {
			summary.Errors = append(summary.Errors, ImportError{
				Line:    lineNo,
				Item:    raw.Item,
				Message: err.Error(),
			})
			continue
		}

		res, err := uc.Upsert(ctx, in)
		if err != nil {
			summary.Errors = append(summary.Errors, ImportError{
				Line:    lineNo,
				Item:    raw.Item,
				Message: err.Error(),
			})
			continue
		}
		if res.Created {
			summary.Created++
		} else {
			summary.Updated++
		}
	}

	log.Info("Success",
		zap.Int("processed", summary.Processed),
		zap.Int("created", summary.Created),
		zap.Int("updated", summary.Updated),
		zap.Int("skipped", summary.Skipped),
		zap.Int("errors", len(summary.Errors)),
	)
	return summary, nil
}

// columnMap holds the index of each known header in the row slice. The CSV
// is hand-edited so column order may shift between versions; mapping by
// header name keeps us resilient to that.
type columnMap struct {
	Item            int
	Date            int
	PaymentDate     int
	PaymentComplete int
	Total           int
	Repartition     int
	ChargeRDC       int
	Charge1er       int
	Notes           int
}

type rawRow struct {
	Item            string
	Date            string
	PaymentDate     string
	PaymentComplete string
	Total           string
	Repartition     string
	ChargeRDC       string
	Charge1er       string
	Notes           string
}

func mapColumns(header []string) (columnMap, error) {
	idx := func(needles ...string) int {
		for i, h := range header {
			h = strings.ToLower(strings.TrimSpace(h))
			for _, n := range needles {
				if strings.Contains(h, n) {
					return i
				}
			}
		}
		return -1
	}

	cols := columnMap{
		Item: idx("item"),
		// Date and PaymentDate must come from explicit passes below — we
		// can't seed Date from `idx("date paiement")` because the substring
		// match would silently capture the payment-date column when no
		// invoice-date column exists, corrupting every row.
		Date:            -1,
		PaymentDate:     -1,
		PaymentComplete: idx("paiement complet"),
		Total:           idx("total"),
		Repartition:     idx("répartition", "repartition"),
		ChargeRDC:       idx("charge rdc"),
		Charge1er:       idx("charge 1er"),
		Notes:           idx("notes"),
	}

	// "Date" must match exactly — "Date paiement" must NOT be picked here.
	for i, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), "date") {
			cols.Date = i
			break
		}
	}
	// "Date paiement" is optional.
	for i, h := range header {
		if strings.Contains(strings.ToLower(strings.TrimSpace(h)), "date paiement") {
			cols.PaymentDate = i
			break
		}
	}

	if cols.Item < 0 || cols.Date < 0 || cols.Total < 0 {
		return cols, entities.ValidationError{
			Key:     "csv_header",
			Message: "missing required columns (Item / Date / Total)",
		}
	}
	return cols, nil
}

func pickRow(row []string, cols columnMap) rawRow {
	get := func(i int) string {
		if i < 0 || i >= len(row) {
			return ""
		}
		return row[i]
	}
	return rawRow{
		Item:            get(cols.Item),
		Date:            get(cols.Date),
		PaymentDate:     get(cols.PaymentDate),
		PaymentComplete: get(cols.PaymentComplete),
		Total:           get(cols.Total),
		Repartition:     get(cols.Repartition),
		ChargeRDC:       get(cols.ChargeRDC),
		Charge1er:       get(cols.Charge1er),
		Notes:           get(cols.Notes),
	}
}

func buildInput(raw rawRow, payerFoyerID string) (CreateInput, error) {
	date, err := parseFRDate(raw.Date)
	if err != nil {
		return CreateInput{}, fmt.Errorf("date: %w", err)
	}
	var paymentDate *time.Time
	if strings.TrimSpace(raw.PaymentDate) != "" {
		pd, err := parseFRDate(raw.PaymentDate)
		if err != nil {
			return CreateInput{}, fmt.Errorf("payment_date: %w", err)
		}
		paymentDate = &pd
	}

	amountCents, err := parseEURCents(raw.Total)
	if err != nil {
		return CreateInput{}, fmt.Errorf("total: %w", err)
	}
	rdcCents, err := parseEURCents(raw.ChargeRDC)
	if err != nil {
		// CSV may have empty share columns for non-50/50 modes — fall back
		// to half, but the upsert validator will reject if it doesn't sum.
		return CreateInput{}, fmt.Errorf("charge rdc: %w", err)
	}
	premierCents, err := parseEURCents(raw.Charge1er)
	if err != nil {
		return CreateInput{}, fmt.Errorf("charge 1er: %w", err)
	}

	if rdcCents+premierCents != amountCents {
		// Tolerate ±1 cent rounding (common in legacy spreadsheets) by
		// nudging the larger share toward the total.
		diff := amountCents - (rdcCents + premierCents)
		if diff > 1 || diff < -1 {
			return CreateInput{}, fmt.Errorf(
				"share mismatch: rdc %d + 1er %d ≠ total %d",
				rdcCents, premierCents, amountCents,
			)
		}
		// Allocate the cent to whichever is larger (arbitrary tiebreaker
		// when equal: RDC).
		if rdcCents >= premierCents {
			rdcCents += diff
		} else {
			premierCents += diff
		}
	}

	categoryID := guessCategoryID(raw.Item)
	note := strings.TrimSpace(raw.Notes)

	mode := mapRepartitionToMode(raw.Repartition)

	return CreateInput{
		Name:             strings.TrimSpace(raw.Item),
		AmountCents:      amountCents,
		Currency:         "EUR",
		Date:             date,
		PaymentDate:      paymentDate,
		PayerFoyerID:     payerFoyerID,
		CategoryID:       categoryID,
		DistributionMode: mode,
		ShareRDCCents:    rdcCents,
		Share1erCents:    premierCents,
		Note:             note,
		// Preserve the historical split exactly. For tantieme rows this
		// matters most: foyer parts may evolve later but the imported
		// expense should still reflect the ratio in force when the bill
		// was paid.
		TrustExplicitShares: true,
		// "Paiement complet (2 parties) = TRUE" on the source spreadsheet
		// means both households already settled their share directly with
		// the supplier — these expenses must NOT skew the running balance.
		Settled: true,
	}, nil
}

// mapRepartitionToMode maps the legacy CSV's "Répartition" cell to our
// DistributionMode enum:
//
//	"50/50"    → equal
//	"tantieme" → tantiemes
//	anything else (incl. "prorata") → custom
//
// Prorata in the legacy spreadsheet was computed from water meters; the
// formula isn't stored, only the resulting amounts. We import as custom
// (with the explicit shares) rather than reverse-engineering the formula.
func mapRepartitionToMode(s string) entities.DistributionMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "50/50", "egal", "égal", "egalite", "égalité", "equal":
		return entities.DistributionModeEqual
	case "tantieme", "tantième", "tantiemes", "tantièmes":
		return entities.DistributionModeTantiemes
	default:
		return entities.DistributionModeCustom
	}
}

func parseFRDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	// Primary format: 24/03/2025 (DD/MM/YYYY). Roundtrip-check so Go's
	// lenient parser doesn't silently roll an invalid date forward
	// (e.g. "31/02/2025" → "03/03/2025").
	if t, err := time.Parse("02/01/2006", s); err == nil {
		if t.Format("02/01/2006") == s {
			return t.UTC(), nil
		}
		return time.Time{}, fmt.Errorf("invalid date %q (out of range)", s)
	}
	// Tolerate ISO YYYY-MM-DD as a fallback (with the same roundtrip check).
	if t, err := time.Parse("2006-01-02", s); err == nil {
		if t.Format("2006-01-02") == s {
			return t.UTC(), nil
		}
		return time.Time{}, fmt.Errorf("invalid date %q (out of range)", s)
	}
	return time.Time{}, fmt.Errorf("unrecognized date format %q (expected DD/MM/YYYY)", s)
}

// parseEURCents accepts the spreadsheet's "346,50 €", "1 311,44 €",
// "1 234,56 €" or bare "60,00" and returns integer cents. Returns 0 with
// no error for empty input — callers guard on emptiness elsewhere.
func parseEURCents(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	// Strip currency suffix and any whitespace (incl. NBSP used by FR locale).
	s = strings.NewReplacer(
		"€", "",
		" ", "",
		" ", "",
	).Replace(s)
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".")

	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("invalid amount %q (NaN/Inf)", s)
	}
	if f < 0 {
		return 0, fmt.Errorf("amount must be >= 0 (got %f)", f)
	}
	// Round half-up to nearest cent.
	cents := int(math.Round(f * 100))
	if cents > maxImportAmountCents {
		return 0, fmt.Errorf("amount %q exceeds the per-row import cap (≥ 1M EUR)", s)
	}
	return cents, nil
}

func isTrue(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TRUE", "VRAI", "1", "OUI", "YES":
		return true
	}
	return false
}

// guessCategoryID maps a free-text Item label to one of the seeded category
// IDs. Items the heuristic can't classify get the "tbd" (À catégoriser)
// category — making them easy to find and re-tag in the UI rather than
// silently misclassifying them as Travaux.
//
// Matching runs on word tokens so "Cadeau" doesn't fall into the "eau"
// bucket via a naive substring match.
func guessCategoryID(item string) string {
	for _, t := range tokenize(item) {
		switch {
		case t == "eau" || t == "eaux":
			return "eau"
		case strings.HasPrefix(t, "électr"),
			strings.HasPrefix(t, "electr"),
			t == "élec", t == "elec":
			return "electricite"
		case strings.HasPrefix(t, "taxe"):
			return "taxe-fonciere"
		case strings.HasPrefix(t, "assur"):
			return "assurance"
		case t == "syndic" || t == "syndicat":
			return "syndic"
		case t == "travaux", t == "travail",
			strings.HasPrefix(t, "entretien"),
			strings.HasPrefix(t, "réparation"),
			strings.HasPrefix(t, "reparation"),
			strings.HasPrefix(t, "plantation"),
			strings.HasPrefix(t, "peinture"),
			strings.HasPrefix(t, "jardin"),
			strings.HasPrefix(t, "maintenance"):
			return "travaux"
		}
	}
	return "tbd"
}

// tokenize lowercases the input and splits on any non-letter rune. Used by
// the category heuristic so substring collisions ("Cadeau" → "eau") don't
// produce false positives.
func tokenize(s string) []string {
	s = strings.ToLower(s)
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		// IsLetter covers French accented characters; everything else
		// (space, hyphen, punctuation, digits) acts as a delimiter.
		if isLetter(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

func isLetter(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	}
	// Cheap fallthrough for accented Latin letters used in French expense
	// labels. We rely on Unicode categories for anything beyond ASCII.
	return r > 127 && (isAccentedLatin(r))
}

func isAccentedLatin(r rune) bool {
	// Letters in the Latin-1 supplement and Latin Extended-A blocks that
	// aren't punctuation. Conservative — this only needs to recognize the
	// French-language characters likely to appear in expense labels.
	if r >= 0x00C0 && r <= 0x017F {
		return true
	}
	return false
}
