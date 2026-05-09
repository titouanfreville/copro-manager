package meters

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder for image.Decode
	_ "image/png"  // register PNG decoder
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// numberCandidate carries everything the assembly logic needs to pick
// good readings out of the raw OCR output.
type numberCandidate struct {
	value      float64
	confidence float64
	// Bounding box in normalized (0..1) image coordinates.
	x      float64
	y      float64
	width  float64
	height float64
	digits int
	// hasDecimal is true when the candidate spans both integer and
	// decimal portions of a meter reading — either because Vision
	// returned a block with an internal '.' / whitespace, we paired
	// two adjacent fragments (the drum integer + the red-background
	// decimal that Vision split off), or the auto-split fallback
	// synthesized one from the residential XXXX.YYY convention.
	hasDecimal bool
	// autoSplit is true ONLY for candidates whose decimal was
	// synthesized by the auto-split fallback (we couldn't find a
	// pairing partner and Vision didn't return a dot). Scoring
	// gives auto-splits a smaller bonus than pair-confirmed or
	// Vision-native decimals — they're a guess, not a fact.
	autoSplit bool
	// score is the picking key — combines digit-count plausibility,
	// font-height (dial digits are physically larger than serial /
	// model labels), OCR confidence, and a bonus tier based on how
	// reliably we believe the decimal portion. Higher = better.
	score float64
}

// digitRunPattern captures the leading numeric content of a block.
// An optional leading "." picks up "decimal-only" blocks like ".452"
// that Vision emits when the meter dial puts the decimal portion on
// a contrasting red-tinted background. Trailing whitespace/dots on
// the captured run are trimmed by the caller.
var digitRunPattern = regexp.MustCompile(`\.?[0-9][0-9\s.,]*`)

// nonDigit strips everything that isn't a digit — used when we need
// just the digit count of a fragment.
var nonDigit = regexp.MustCompile(`[^0-9]`)

// cleanDigitContent extracts the numeric content from an OCR block.
// Returns ok=false when the block has either the intermixed-serial
// signature ("117FA028327": digits, letters, more digits) or the
// model-number signature ("13MA": digits then letters with NO
// whitespace between them). A trailing unit suffix like " m³" — i.e.
// letters separated from digits by whitespace — is fine and gets
// stripped.
//
// More permissive than the old hasLetters filter, which dropped any
// block with a letter and killed real meter readings whose Vision
// block happened to include the unit suffix printed next to the dial.
func cleanDigitContent(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	// Walk once. Tracks:
	//   - firstLetter: position of the first letter we see
	//   - sawDigit: whether we've seen any digit so far
	//   - lastWasDigit: whether the immediately-preceding non-space
	//     character was a digit
	// A digit AFTER a letter rejects (intermixed serial); a letter
	// IMMEDIATELY following a digit (no whitespace separator)
	// rejects (model-number prefix like "13MA"); letters that follow
	// whitespace are accepted as unit suffixes.
	firstLetter := -1
	sawDigit := false
	lastWasDigit := false
	for i, r := range text {
		isLetter := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		isDigit := r >= '0' && r <= '9'
		isSpace := r == ' ' || r == '\t'
		isDecimalPunct := r == '.' || r == ','
		switch {
		case isDigit:
			if firstLetter >= 0 {
				return "", false
			}
			sawDigit = true
			lastWasDigit = true
		case isLetter:
			if firstLetter < 0 {
				firstLetter = i
				if sawDigit && lastWasDigit {
					return "", false
				}
			}
			lastWasDigit = false
		case isSpace:
			lastWasDigit = false
		case isDecimalPunct:
			lastWasDigit = false
		default:
			// "/" "-" "_" etc. between digits are date / version
			// / phone-style separators, never a meter reading. The
			// reject only fires before any letter has been seen —
			// once we're past the digit run into the unit-suffix
			// region (e.g. "m³"), unicode like the superscript 3
			// is fine.
			if sawDigit && firstLetter < 0 {
				return "", false
			}
		}
	}
	// Pull out the leading number-shape and trim its non-digit edges.
	match := digitRunPattern.FindString(text)
	match = strings.Trim(match, ". ")
	if match == "" {
		return "", false
	}
	// Re-attach a leading "." when the original text had one — Vision
	// returns ".452" for the decimal-only red drum portion, and we
	// want to preserve that signal so the candidate later pairs as a
	// decimal partner rather than as a standalone integer.
	if idx := strings.Index(text, match); idx > 0 && text[idx-1] == '.' {
		match = "." + match
	}
	return match, true
}

// fragment is the pre-pairing representation of a numeric OCR result.
// `hasDecimal` records whether the source already carried an integer/
// decimal split (via dot or internal whitespace) so the pairing pass
// doesn't try to merge it with another block.
type fragment struct {
	value      float64
	text       string // canonical "1234" or "1234.567"
	confidence float64
	x, y, w, h float64
	digits     int
	hasDecimal bool
	autoSplit  bool // true when the decimal portion was synthesized
}

// extractNumberCandidates is the OCR pipeline's top-level entry point.
// It produces three classes of candidates, in priority order:
//
//  1. Single-block fragments that already carried a decimal point or
//     internal whitespace (Vision occasionally returns "1234.567" or
//     "1234 567" as one block).
//  2. Paired fragments: an integer block + its adjacent decimal block
//     on the same baseline, which Vision usually splits because the
//     decimal portion of a real meter dial sits on a red-tinted
//     background. Without this pairing, "0252" + "735" come back as
//     two unrelated numbers and the .735 part is lost.
//  3. Standalone digit-only fragments (no pairing partner found).
//
// Score boosts strongly favor (1) and (2) over (3), and penalize the
// short orphan-decimal fragments that lurk in (3).
func extractNumberCandidates(blocks []interfaces.OCRTextBlock) []numberCandidate {
	frags := extractFragments(blocks)
	if len(frags) == 0 {
		return nil
	}

	// Sort by Y then X for deterministic pairing.
	sort.SliceStable(frags, func(i, j int) bool {
		if math.Abs(frags[i].y-frags[j].y) > 0.02 {
			return frags[i].y < frags[j].y
		}
		return frags[i].x < frags[j].x
	})

	paired := make([]bool, len(frags))
	out := []numberCandidate{}

	for i := range frags {
		if paired[i] || frags[i].hasDecimal {
			continue
		}
		if frags[i].digits < 2 || frags[i].digits > 5 {
			continue
		}
		// Look for a right-neighbor that looks like the meter's
		// decimal portion. French residential meters always use 3
		// decimal places (1-liter resolution), so the decimal
		// fragment must be 2-3 digits. Wider tolerances accidentally
		// pair chassis labels (4-digit serials like "5322" living
		// near 5-digit dial readings).
		// Leading-dot fragments (.452, already tagged with
		// hasDecimal=true) are allowed as decimal partners — they
		// were marked decimal because Vision returned a leading dot,
		// not because they're a complete reading on their own.
		for j := range frags {
			if i == j || paired[j] {
				continue
			}
			isLeadingDotDec := frags[j].hasDecimal && strings.HasPrefix(frags[j].text, "0.")
			if frags[j].hasDecimal && !isLeadingDotDec {
				continue
			}
			if frags[j].digits < 2 || frags[j].digits > 3 {
				continue
			}
			if !sameBaseline(frags[i], frags[j]) {
				continue
			}
			if !heightsCompatible(frags[i], frags[j]) {
				continue
			}
			if !isAdjacentRight(frags[i], frags[j]) {
				continue
			}
			merged := mergeIntDecimal(frags[i], frags[j])
			out = append(out, fragmentToCandidate(merged))
			paired[i] = true
			paired[j] = true
			break
		}
	}

	for i, f := range frags {
		if paired[i] {
			continue
		}
		// Drop noisy 1-digit fragments — they're never the reading.
		if f.digits < 2 {
			continue
		}
		out = append(out, fragmentToCandidate(f))
	}
	return out
}

// extractFragments parses each OCR block into at most one fragment.
// The block-level processing decides up-front whether the original
// text already carried decimal information (via dot or internal
// whitespace) so the pairing pass can leave it alone.
func extractFragments(blocks []interfaces.OCRTextBlock) []fragment {
	out := []fragment{}
	for _, b := range blocks {
		// Normalize comma decimals to dot before classification —
		// French convention sometimes uses "1,234" for "1.234".
		normalized := strings.ReplaceAll(b.Text, ",", ".")
		match, ok := cleanDigitContent(normalized)
		if !ok {
			continue
		}

		// Detect the integer/decimal split:
		//   - leading dot      → ".452"  (decimal-only red drum)
		//   - explicit dot     → "0252.735"
		//   - internal space   → "0252 735" (rare, but Vision sometimes
		//     keeps the dial gap)
		//   - neither          → fragment without decimal
		leadingDot := strings.HasPrefix(match, ".")
		if leadingDot {
			match = match[1:]
		}
		hasDot := strings.Contains(match, ".")
		hasSpace := strings.ContainsAny(match, " \t")

		var canonical string
		var digits int
		var hasDec bool
		if hasDot {
			// Multi-dot patterns are phone/serial numbers, not meter
			// readings. "53.81.21.35.94" (the FR phone prefix on
			// chassis labels) has more dots than any real dial.
			if strings.Count(match, ".") > 1 {
				continue
			}
			parts := strings.SplitN(match, ".", 2)
			intStr := nonDigit.ReplaceAllString(parts[0], "")
			decStr := nonDigit.ReplaceAllString(parts[1], "")
			if intStr == "" && decStr == "" {
				continue
			}
			if intStr == "" {
				intStr = "0"
			}
			if decStr == "" {
				canonical = intStr
				digits = len(intStr)
				hasDec = false
			} else {
				canonical = intStr + "." + decStr
				digits = len(intStr) + len(decStr)
				hasDec = true
			}
		} else if hasSpace {
			runs := strings.FieldsFunc(match, func(r rune) bool {
				return r == ' ' || r == '\t'
			})
			cleaned := []string{}
			for _, r := range runs {
				d := nonDigit.ReplaceAllString(r, "")
				if d != "" {
					cleaned = append(cleaned, d)
				}
			}
			if len(cleaned) < 2 {
				if len(cleaned) == 1 {
					canonical = cleaned[0]
					digits = len(cleaned[0])
					hasDec = false
				} else {
					continue
				}
			} else {
				// Treat the last run as the decimal portion (red-bg
				// dial digits) and everything before it as integer.
				intStr := strings.Join(cleaned[:len(cleaned)-1], "")
				decStr := cleaned[len(cleaned)-1]
				canonical = intStr + "." + decStr
				digits = len(intStr) + len(decStr)
				hasDec = true
			}
		} else {
			canonical = nonDigit.ReplaceAllString(match, "")
			digits = len(canonical)
			hasDec = false
		}

		if digits < 2 {
			continue
		}
		// A leading-dot fragment (".452") represents a decimal-only
		// drum reading. Its `value` is the decimal magnitude scaled
		// down so the FALLBACK behavior (no integer pair found) is
		// to surface it as a small value the user can spot as wrong,
		// rather than as a 3-digit integer that scores well.
		if leadingDot {
			canonical = "0." + nonDigit.ReplaceAllString(canonical, "")
			hasDec = true
		}
		v, err := strconv.ParseFloat(canonical, 64)
		if err != nil {
			continue
		}
		out = append(out, fragment{
			value:      v,
			text:       canonical,
			confidence: b.Confidence,
			x:          b.X + b.Width/2,
			y:          b.Y + b.Height/2,
			w:          b.Width,
			h:          b.Height,
			digits:     digits,
			hasDecimal: hasDec,
		})
	}
	return out
}

// sameBaseline reports whether two fragments sit on the same
// horizontal line. The tolerance is generous — a handheld phone
// shot of a meter dial often tilts a few degrees, and Vision's
// axis-aligned bounding boxes around the rotated digit clusters
// inflate further, pushing the centers apart vertically. We accept
// up to a full block-height of vertical drift before declaring two
// fragments on different rows.
func sameBaseline(a, b fragment) bool {
	avgH := (a.h + b.h) / 2
	if avgH <= 0 {
		return false
	}
	return math.Abs(a.y-b.y) < avgH*1.0
}

// isAdjacentRight reports whether `b` sits just to the right of `a`.
// We allow a generous horizontal range to cope with the visible gap
// between integer and decimal drums on a real meter face plus the
// bounding-box inflation on tilted photos. Slight overlap is fine
// — Vision occasionally bounds adjacent words with overlapping
// rectangles when the digits are visually close.
func isAdjacentRight(a, b fragment) bool {
	rightOfA := a.x + a.w/2
	leftOfB := b.x - b.w/2
	gap := leftOfB - rightOfA
	avgW := (a.w + b.w) / 2
	if avgW <= 0 {
		return false
	}
	return gap > -avgW*0.8 && gap < avgW*2.5
}

// heightsCompatible rejects pairings where one fragment is much
// shorter than the other. The threshold is permissive (40% of the
// taller block) so a 4-digit integer drum still pairs with a
// 3-digit decimal drum even when Vision's bounding boxes differ by
// a stripe of red-background border.
func heightsCompatible(a, b fragment) bool {
	minH, maxH := math.Min(a.h, b.h), math.Max(a.h, b.h)
	if maxH == 0 {
		return false
	}
	return minH/maxH > 0.4
}

// mergeIntDecimal combines two same-baseline fragments into one
// "integer.decimal" candidate. The merged bounding box spans both
// originals so downstream geometry (color sampling, distance from
// `common`) sees the full meter dial region.
func mergeIntDecimal(intF, decF fragment) fragment {
	intStr := nonDigit.ReplaceAllString(intF.text, "")
	decStr := nonDigit.ReplaceAllString(decF.text, "")
	// Leading-dot fragments (".452") are stored as "0.452" so the
	// fallback value is sensible when no integer pair is found. When
	// merging, strip the synthetic "0" prefix or we'd get
	// "01234.0452" instead of "1234.452".
	if strings.HasPrefix(decF.text, "0.") {
		decStr = strings.TrimPrefix(decStr, "0")
	}
	canonical := intStr + "." + decStr
	v, _ := strconv.ParseFloat(canonical, 64)

	leftX := math.Min(intF.x-intF.w/2, decF.x-decF.w/2)
	rightX := math.Max(intF.x+intF.w/2, decF.x+decF.w/2)
	topY := math.Min(intF.y-intF.h/2, decF.y-decF.h/2)
	bottomY := math.Max(intF.y+intF.h/2, decF.y+decF.h/2)

	return fragment{
		value:      v,
		text:       canonical,
		confidence: math.Min(intF.confidence, decF.confidence),
		x:          (leftX + rightX) / 2,
		y:          (topY + bottomY) / 2,
		w:          rightX - leftX,
		h:          bottomY - topY,
		digits:     intF.digits + decF.digits,
		hasDecimal: true,
	}
}

func fragmentToCandidate(f fragment) numberCandidate {
	c := numberCandidate{
		value:      f.value,
		confidence: f.confidence,
		x:          f.x,
		y:          f.y,
		width:      f.w,
		height:     f.h,
		digits:     f.digits,
		hasDecimal: f.hasDecimal,
		autoSplit:  f.autoSplit,
	}
	c.score = scoreCandidate(c)
	return c
}

// scoreCandidate produces a single number ranking candidates. A real
// meter reading typically:
//   - has BOTH integer and decimal portions (the .YYY in 1234.YYY)
//   - is 5-7 digits total (4 m³ digits + 3 liter digits)
//   - is drawn in the LARGEST font on the meter face (drum digits
//     visually dominate the model/serial labels)
//   - lands with decent OCR confidence (clean black-on-white drum)
//
// The decimal-presence bonus is intentionally large: it's the single
// most reliable signal that we're looking at the dial reading and not
// at a chassis-printed model number like "103942".
func scoreCandidate(c numberCandidate) float64 {
	// Digit-count fit: peak at 5-9 digits. Covers XXX.YYY (smaller
	// meters), XXXX.YYY (typical residential), XXXXX.YYY (older
	// meters), and XXXXXX.YYY (building-main meters with
	// high accumulated consumption). Tapers off on either side.
	digitFit := 0.2
	switch {
	case c.digits >= 5 && c.digits <= 9:
		digitFit = 1.0
	case c.digits == 4 || c.digits == 10:
		digitFit = 0.5
	}
	// Height in normalized coords. The dial reading is typically the
	// LARGEST text on the meter face; rotated photos can push the
	// AABB-projected height to 30%+ of the image. We let height
	// dominate the score so a tall reading drum beats a chassis
	// label even when both carry decimals.
	heightFit := math.Min(c.height*15, 4.0)
	// Confidence as-is. Vision sometimes returns 0 when uncertain.
	conf := c.confidence
	if conf <= 0 {
		conf = 0.5
	}

	score := 2.0*heightFit + 1.5*digitFit + 0.5*conf

	// Decimal-presence bonus, tiered by reliability. Pair-confirmed
	// and Vision-native decimals are facts (Vision saw a dot or two
	// adjacent fragments aligned). Auto-split decimals are a guess
	// based on the residential XXXX.YYY convention — still better
	// than no decimal, but should lose to a paired alternative when
	// both are competing in the same image.
	switch {
	case c.hasDecimal && !c.autoSplit:
		score += 3.0
	case c.hasDecimal && c.autoSplit:
		score += 2.0
	}

	// Penalize tiny standalone runs (orphan decimals that didn't get
	// paired, or partial OCR misreads).
	if !c.hasDecimal && c.digits <= 3 {
		score -= 2.0
	}

	return score
}

// pickBest returns the highest-scoring candidate, or nil when the
// slice is empty.
func pickBest(candidates []numberCandidate) *numberCandidate {
	if len(candidates) == 0 {
		return nil
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}
	return &best
}

// pickRowBest clusters candidates by Y coordinate, picks one per row,
// returns rows top-to-bottom up to wantRows. Used as a fallback when
// the blue-meter anchor doesn't pan out.
func pickRowBest(candidates []numberCandidate, wantRows int) []numberCandidate {
	if len(candidates) == 0 || wantRows <= 0 {
		return nil
	}
	const yTol = 0.08
	sorted := make([]numberCandidate, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].y < sorted[j].y })

	rows := [][]numberCandidate{}
	for _, c := range sorted {
		placed := false
		for ri := range rows {
			meanY := 0.0
			for _, m := range rows[ri] {
				meanY += m.y
			}
			meanY /= float64(len(rows[ri]))
			if math.Abs(c.y-meanY) <= yTol {
				rows[ri] = append(rows[ri], c)
				placed = true
				break
			}
		}
		if !placed {
			rows = append(rows, []numberCandidate{c})
		}
	}
	best := make([]numberCandidate, 0, len(rows))
	for _, row := range rows {
		b := row[0]
		for _, c := range row[1:] {
			if c.score > b.score {
				b = c
			}
		}
		best = append(best, b)
	}
	if len(best) > wantRows {
		best = best[:wantRows]
	}
	return best
}

// assignDetailValues maps the OCR candidates onto the three sub-meters
// using the user's spatial rule:
//   - common (the only blue housing) is the anchor
//   - of the other two, the one CLOSER to common is `1er`
//   - the FARTHER one is `RDC`
//
// Returns values in [common, RDC, 1er] order to match the API contract.
// Falls back to top-to-bottom row clustering when:
//   - the image can't be decoded
//   - no candidate scores blue enough (e.g. unusual lighting)
//   - we can't gather 3 distinct candidates
func assignDetailValues(candidates []numberCandidate, image []byte) ([]float64, []float64) {
	if len(candidates) == 0 {
		return nil, nil
	}
	// Pre-sort by score so distinct candidates per spatial bucket
	// pick the best reading.
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	// Try the blue-anchor strategy first.
	if image != nil {
		if values, conf, ok := assignByBlueAnchor(candidates, image); ok {
			return values, conf
		}
	}

	// Fallback: top-to-bottom rows, position-based ordering. Always
	// return 3 slots so the API contract `[common, RDC, 1er]` is
	// position-stable; missing slots come back as 0.0 + confidence 0
	// and the client treats confidence=0 as "skip auto-fill".
	picked := pickRowBest(candidates, 3)
	values := []float64{0, 0, 0}
	conf := []float64{0, 0, 0}
	for i := 0; i < len(picked) && i < 3; i++ {
		values[i] = picked[i].value
		conf[i] = picked[i].confidence
	}
	return values, conf
}

// applyColorSplit refines candidates whose digit run lacks a decimal
// separator. The pipeline samples pixels around each candidate's
// bounding box and looks for a meter-dial-decimal red tint somewhere
// in the right portion of the box. When red is confirmed, we apply
// the standard residential XXXX.YYY convention (last 3 digits =
// decimal m³). When no red is detected, the candidate stays
// integer-only so the user can correct it manually rather than
// receiving a confidently-wrong split based on a guessed digit count.
//
// Two-stage detection: first verify red exists ANYWHERE in the right
// half of the candidate's bounding-box neighborhood (so the dial-
// drum confirmation works even when Vision's bbox is tight on the
// digits and tiny); then split using the convention. If we ever get
// per-symbol coordinates from Vision the strip-precise variant can
// supersede this — for now, color-confirmed-convention is the
// reliable middle ground given the data.
func applyColorSplit(candidates []numberCandidate, imageBytes []byte) []numberCandidate {
	if len(candidates) == 0 || len(imageBytes) == 0 {
		return candidates
	}
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return candidates
	}
	bounds := img.Bounds()
	imgW := float64(bounds.Dx())
	imgH := float64(bounds.Dy())
	if imgW <= 0 || imgH <= 0 {
		return candidates
	}

	out := make([]numberCandidate, len(candidates))
	copy(out, candidates)
	for i := range out {
		c := &out[i]
		if c.hasDecimal || c.digits < 4 {
			continue
		}
		if !rightHalfHasRed(img, imgW, imgH, *c) {
			continue
		}
		digitsOnly := canonicalDigits(*c)
		if len(digitsOnly) <= 3 {
			continue
		}
		intPart := digitsOnly[:len(digitsOnly)-3]
		decPart := digitsOnly[len(digitsOnly)-3:]
		canonical := intPart + "." + decPart
		v, err := strconv.ParseFloat(canonical, 64)
		if err != nil {
			continue
		}
		c.value = v
		c.hasDecimal = true
		c.autoSplit = true // color-confirmed but not pair-confirmed
		c.score = scoreCandidate(*c)
	}
	return out
}

// canonicalDigits returns the candidate's underlying digit text. The
// raw digit count is stashed on the candidate; the value is the
// integer interpretation, so we re-derive the leading-zero-padded
// digit string from those two.
func canonicalDigits(c numberCandidate) string {
	return fmt.Sprintf("%0*d", c.digits, int64(math.Round(c.value)))
}

// rightHalfHasRed reports whether the right ~half of the candidate's
// bounding box (expanded vertically to capture the drum housing
// around tight digit boxes) contains pixels that look like the
// meter's red decimal drum. A few percent of red pixels is enough
// to confirm "this is the dial, not a chassis label".
func rightHalfHasRed(img image.Image, imgW, imgH float64, c numberCandidate) bool {
	cx0 := c.x - c.width/2
	cx1 := c.x + c.width/2
	cy0 := c.y - c.height/2
	cy1 := c.y + c.height/2
	// Expand vertically — Vision's word-level bounding boxes hug the
	// digits tightly, so the red drum housing extends slightly above
	// and below the box. The expansion is ~2× the box height on each
	// side; tight bounding boxes (h=2% of image) need this generous
	// pad to capture the drum bg around the digits.
	pad := math.Max(c.height*2, 0.015)
	cy0 -= pad
	cy1 += pad
	// Right half — the decimal drum is on the right side of every
	// French residential meter dial.
	splitX := c.x
	px0 := int(math.Max(0, splitX*imgW))
	px1 := int(math.Min(imgW, cx1*imgW))
	py0 := int(math.Max(0, cy0*imgH))
	py1 := int(math.Min(imgH, cy1*imgH))
	// Falls back to the full bounding box when the right-half region
	// is degenerate — happens when Vision returns a near-zero-width
	// bbox.
	if px1-px0 < 4 || py1-py0 < 4 {
		px0 = int(math.Max(0, cx0*imgW))
		py0 = int(math.Max(0, cy0*imgH))
		if px1-px0 < 4 || py1-py0 < 4 {
			return false
		}
	}

	stride := 1
	area := (px1 - px0) * (py1 - py0)
	if area > 4000 {
		stride = int(math.Sqrt(float64(area) / 4000))
		if stride < 1 {
			stride = 1
		}
	}
	redPixels := 0
	totalPixels := 0
	for y := py0; y < py1; y += stride {
		for x := px0; x < px1; x += stride {
			r, g, b, _ := img.At(x, y).RGBA()
			r8 := int(r >> 8)
			g8 := int(g >> 8)
			b8 := int(b >> 8)
			// "Red drum" pixel: R clearly dominates G and B AND the
			// pixel is lit. Thresholds tuned against the user's
			// real photos — meter-dial red sometimes reads as
			// pink/coral on overexposed phone shots, hence the
			// permissive R-G margin.
			if r8 > g8+12 && r8 > b8+12 && r8 > 70 {
				redPixels++
			}
			totalPixels++
		}
	}
	if totalPixels == 0 {
		return false
	}
	// Even a small fraction of red pixels signals the dial drum,
	// because most of the drum window is filled with black digits
	// and only a fraction shows the red background through gaps.
	return float64(redPixels)/float64(totalPixels) > 0.05
}

// assignByBlueAnchor decodes the image, samples meter-housing colors
// around each candidate, picks the bluest as `common`, and orders the
// other two by distance from common (closer = 1er, farther = RDC).
// Returns ok=false when the image can't be decoded or when fewer than
// 3 spatially-distinct candidates land on plausibly-different meters.
func assignByBlueAnchor(candidates []numberCandidate, imageBytes []byte) ([]float64, []float64, bool) {
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, nil, false
	}
	bounds := img.Bounds()
	imgW := float64(bounds.Dx())
	imgH := float64(bounds.Dy())
	if imgW <= 0 || imgH <= 0 {
		return nil, nil, false
	}

	// Collapse candidates into "meter clusters" — each meter typically
	// has multiple OCR-detected number runs (the main dial reading
	// plus secondary indicators / decimals). One cluster per spatial
	// neighborhood.
	clusters := clusterByPosition(candidates, 0.10)
	if len(clusters) < 3 {
		return nil, nil, false
	}

	// Score each cluster's blueness by sampling pixels around its
	// representative bounding box.
	type ranked struct {
		cluster   meterCluster
		blueScore float64
	}
	scored := make([]ranked, 0, len(clusters))
	for _, c := range clusters {
		score := sampleBlueness(img, imgW, imgH, c)
		scored = append(scored, ranked{cluster: c, blueScore: score})
	}

	// Pick the bluest cluster as `common`. If the bluest barely beats
	// the others (no clear blue signal), bail out and let the fallback
	// handle it.
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].blueScore > scored[j].blueScore })
	if len(scored) < 3 {
		return nil, nil, false
	}
	if scored[0].blueScore < 0.15 {
		// No cluster is convincingly blue — abort the anchor strategy.
		return nil, nil, false
	}
	common := scored[0].cluster

	// Of the rest, sort by distance to common.
	others := []meterCluster{scored[1].cluster, scored[2].cluster}
	sort.SliceStable(others, func(i, j int) bool {
		return distanceSq(common, others[i]) < distanceSq(common, others[j])
	})
	ner := others[0] // closer → 1er
	rdc := others[1] // farther → RDC

	values := []float64{common.best.value, rdc.best.value, ner.best.value}
	conf := []float64{common.best.confidence, rdc.best.confidence, ner.best.confidence}
	return values, conf, true
}

// meterCluster is a spatial group of OCR candidates that all belong
// to the same physical meter — the dial reading, decimal portion,
// secondary indicators, etc.
type meterCluster struct {
	best numberCandidate // highest-scoring candidate in the cluster
	x    float64         // mean X across cluster members
	y    float64         // mean Y across cluster members
}

// clusterByPosition greedy-groups candidates whose positions are
// within `tol` of each other (Euclidean in normalized image coords).
// The first candidate seeds the cluster; subsequent ones either join
// or start a new one.
func clusterByPosition(candidates []numberCandidate, tol float64) []meterCluster {
	if len(candidates) == 0 {
		return nil
	}
	type bucket struct {
		members []numberCandidate
	}
	buckets := []bucket{}
	for _, c := range candidates {
		placed := false
		for bi := range buckets {
			meanX, meanY := 0.0, 0.0
			for _, m := range buckets[bi].members {
				meanX += m.x
				meanY += m.y
			}
			n := float64(len(buckets[bi].members))
			meanX /= n
			meanY /= n
			dx := c.x - meanX
			dy := c.y - meanY
			if math.Sqrt(dx*dx+dy*dy) <= tol {
				buckets[bi].members = append(buckets[bi].members, c)
				placed = true
				break
			}
		}
		if !placed {
			buckets = append(buckets, bucket{members: []numberCandidate{c}})
		}
	}
	out := make([]meterCluster, 0, len(buckets))
	for _, b := range buckets {
		// Pick the highest-scoring candidate per cluster as the
		// representative reading.
		best := b.members[0]
		for _, m := range b.members[1:] {
			if m.score > best.score {
				best = m
			}
		}
		mx, my := 0.0, 0.0
		for _, m := range b.members {
			mx += m.x
			my += m.y
		}
		mx /= float64(len(b.members))
		my /= float64(len(b.members))
		out = append(out, meterCluster{best: best, x: mx, y: my})
	}
	return out
}

// distanceSq is the squared Euclidean distance between two clusters'
// centers — squared because we only use it for ordering.
func distanceSq(a, b meterCluster) float64 {
	dx := a.x - b.x
	dy := a.y - b.y
	return dx*dx + dy*dy
}

// sampleBlueness returns a 0..1 score for how blue the meter housing
// around the cluster appears. We sample a region centered on the
// cluster's best-bounding-box, expanded by 4× to cover the meter
// housing rather than just the digit window itself.
func sampleBlueness(img image.Image, imgW, imgH float64, c meterCluster) float64 {
	bb := c.best
	// Expand the bounding box outward — the digit window sits inside
	// the housing, so we need a noticeably larger region to capture
	// the housing's color.
	expand := 2.0
	cx := bb.x
	cy := bb.y
	w := math.Max(bb.width*expand, 0.08)
	h := math.Max(bb.height*expand, 0.08)
	x0 := math.Max(0, cx-w/2)
	y0 := math.Max(0, cy-h/2)
	x1 := math.Min(1, cx+w/2)
	y1 := math.Min(1, cy+h/2)

	px0 := int(x0 * imgW)
	py0 := int(y0 * imgH)
	px1 := int(x1 * imgW)
	py1 := int(y1 * imgH)
	// Step size so we sample at most ~2500 pixels regardless of image
	// resolution — keeps the OCR endpoint snappy on phone-sized JPEGs.
	stride := 1
	pxCount := (px1 - px0) * (py1 - py0)
	if pxCount > 2500 {
		stride = int(math.Sqrt(float64(pxCount) / 2500))
		if stride < 1 {
			stride = 1
		}
	}

	bluePixels := 0
	totalPixels := 0
	for y := py0; y < py1; y += stride {
		for x := px0; x < px1; x += stride {
			r, g, b, _ := img.At(x, y).RGBA()
			if isBluish(r, g, b) {
				bluePixels++
			}
			totalPixels++
		}
	}
	if totalPixels == 0 {
		return 0
	}
	return float64(bluePixels) / float64(totalPixels)
}

// isBluish classifies a pixel as "meter-housing-blue" based on the
// blue meter's color profile (saturated mid-blue). Uses the 16-bit
// RGB values that color.RGBA returns from img.At().RGBA().
//
// The blue meter housing in the user's photos is roughly RGB(60, 120,
// 200) at full lighting, with hue in the 200-220° range.
func isBluish(r, g, b uint32) bool {
	// Convert to 8-bit for HSV; precision is fine.
	r8 := float64(r >> 8)
	g8 := float64(g >> 8)
	b8 := float64(b >> 8)
	maxC := math.Max(r8, math.Max(g8, b8))
	minC := math.Min(r8, math.Min(g8, b8))
	delta := maxC - minC
	if maxC == 0 {
		return false
	}
	saturation := delta / maxC
	// Reject grayscale-ish pixels (white plastic, dirt, shadow).
	if saturation < 0.25 {
		return false
	}
	// Reject very dark pixels (deep shadow can read low-saturation
	// blue but isn't part of the housing).
	if maxC < 60 {
		return false
	}
	// Hue calculation — only meaningful with non-zero delta.
	var hue float64
	switch maxC {
	case r8:
		hue = 60 * math.Mod((g8-b8)/delta, 6)
	case g8:
		hue = 60 * (((b8 - r8) / delta) + 2)
	case b8:
		hue = 60 * (((r8 - g8) / delta) + 4)
	}
	if hue < 0 {
		hue += 360
	}
	// Blue band: ~190-250° covers cyan-blue through indigo. The user's
	// blue meter sits ~210°.
	return hue >= 190 && hue <= 250
}
