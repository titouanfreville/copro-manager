package meters

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// block is a tiny constructor for OCR blocks in test fixtures —
// keeps the table-driven tests below readable.
func block(text string, x, y, w, h, conf float64) interfaces.OCRTextBlock {
	return interfaces.OCRTextBlock{
		Text:       text,
		Confidence: conf,
		X:          x,
		Y:          y,
		Width:      w,
		Height:     h,
	}
}

// TestPairingMergesAdjacentIntegerAndDecimal covers the user-reported
// regression: residential meter dials put the decimal portion on a
// red background and Vision returns it as a separate block. The
// pairing pass must merge them so the user gets `1234.567` instead of
// just `1234.000`.
func TestPairingMergesAdjacentIntegerAndDecimal(t *testing.T) {
	Convey("Adjacent integer + decimal blocks on the same baseline merge", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("0252", 0.30, 0.45, 0.10, 0.05, 0.95),
			block("735", 0.41, 0.45, 0.05, 0.05, 0.85),
		}
		cands := extractNumberCandidates(blocks)
		So(len(cands), ShouldEqual, 1)
		So(cands[0].value, ShouldEqual, 252.735)
		So(cands[0].hasDecimal, ShouldBeTrue)
	})

	Convey("Vision-supplied dot is preserved without a pairing pass", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("0252.735", 0.30, 0.45, 0.15, 0.05, 0.95),
		}
		cands := extractNumberCandidates(blocks)
		So(len(cands), ShouldEqual, 1)
		So(cands[0].value, ShouldEqual, 252.735)
		So(cands[0].hasDecimal, ShouldBeTrue)
	})

	Convey("Internal whitespace acts like a decimal separator", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("0252 735", 0.30, 0.45, 0.15, 0.05, 0.95),
		}
		cands := extractNumberCandidates(blocks)
		So(len(cands), ShouldEqual, 1)
		So(cands[0].value, ShouldEqual, 252.735)
		So(cands[0].hasDecimal, ShouldBeTrue)
	})

	Convey("Different baselines stay unpaired", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("0252", 0.30, 0.20, 0.10, 0.05, 0.95),
			block("735", 0.41, 0.45, 0.05, 0.05, 0.85), // 0.25 below — not same row
		}
		cands := extractNumberCandidates(blocks)
		// Both emit standalone, neither merged.
		So(len(cands), ShouldEqual, 2)
		for _, c := range cands {
			So(c.hasDecimal, ShouldBeFalse)
		}
	})

	Convey("Far-apart fragments stay unpaired even on the same baseline", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("0252", 0.10, 0.45, 0.05, 0.05, 0.95),
			block("735", 0.85, 0.45, 0.05, 0.05, 0.85), // way to the right
		}
		cands := extractNumberCandidates(blocks)
		// Both standalone — the gap rules out adjacency.
		So(len(cands), ShouldEqual, 2)
	})
}

// TestNoConventionAutoSplit covers the user-driven decision: don't
// invent decimals from a digit-count convention because Vision
// occasionally drops a digit and the resulting value is wildly wrong
// (e.g. 6 digits read instead of 7 → 10× off). Long no-decimal runs
// stay integer-only at the extractNumberCandidates level; the
// color-aware split lives in analyzeImage where the image bytes are
// available.
func TestNoConventionAutoSplit(t *testing.T) {
	Convey("Long no-decimal runs are NOT split without color evidence", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("0007500", 0.23, 0.51, 0.06, 0.02, 0.98),
		}
		cands := extractNumberCandidates(blocks)
		So(len(cands), ShouldEqual, 1)
		So(cands[0].hasDecimal, ShouldBeFalse)
		So(cands[0].value, ShouldEqual, 7500)
	})

	Convey("Multi-dot serial numbers (53.81.21.35.94) are dropped, not treated as decimal", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("53.81.21.35.94", 0.35, 0.67, 0.10, 0.04, 0.94),
			block("00739901", 0.40, 0.68, 0.07, 0.03, 0.85),
		}
		cands := extractNumberCandidates(blocks)
		// The phone-style "decimal" pattern is rejected by the
		// multi-dot check, leaving only the dial reading.
		So(len(cands), ShouldEqual, 1)
		So(cands[0].value, ShouldEqual, 739901)
	})
}

// TestPairingHandlesUnitsAndLeadingDot covers the second-round
// regression: Vision often returns the decimal portion as ".452"
// (with a leading dot) or attaches a "m³" unit suffix to the integer
// block. The old `hasLetters` filter dropped both cases. The new
// `cleanDigitContent` keeps them so pairing can still work.
func TestPairingHandlesUnitsAndLeadingDot(t *testing.T) {
	Convey("Trailing m³ unit is stripped, integer fragment still pairs", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("0252 m³", 0.30, 0.45, 0.10, 0.05, 0.95),
			block("735", 0.41, 0.45, 0.05, 0.05, 0.85),
		}
		cands := extractNumberCandidates(blocks)
		So(len(cands), ShouldEqual, 1)
		So(cands[0].value, ShouldEqual, 252.735)
		So(cands[0].hasDecimal, ShouldBeTrue)
	})

	Convey("Leading-dot decimal fragment merges with the integer to the left", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("0252", 0.30, 0.45, 0.10, 0.05, 0.95),
			block(".735", 0.41, 0.45, 0.05, 0.05, 0.85),
		}
		cands := extractNumberCandidates(blocks)
		// One merged "252.735" — the leading-dot fragment found its integer.
		var found *numberCandidate
		for i := range cands {
			if cands[i].hasDecimal && cands[i].value > 100 {
				found = &cands[i]
			}
		}
		So(found, ShouldNotBeNil)
		So(found.value, ShouldEqual, 252.735)
	})

	Convey("Serial-number patterns are still rejected", t, func() {
		blocks := []interfaces.OCRTextBlock{
			block("117FA028327", 0.30, 0.20, 0.20, 0.04, 0.95),
			block("0252.735", 0.30, 0.55, 0.18, 0.06, 0.95),
		}
		cands := extractNumberCandidates(blocks)
		So(len(cands), ShouldEqual, 1)
		So(cands[0].value, ShouldEqual, 252.735)
	})

	Convey("Tilted-photo baseline drift is tolerated", t, func() {
		// The decimal block sits slightly lower than the integer
		// block — typical of a phone shot held at an angle.
		blocks := []interfaces.OCRTextBlock{
			block("0252", 0.30, 0.45, 0.10, 0.05, 0.95),
			block("735", 0.41, 0.49, 0.05, 0.05, 0.85), // Y differs by ~0.04, ≈ block height
		}
		cands := extractNumberCandidates(blocks)
		So(len(cands), ShouldEqual, 1)
		So(cands[0].hasDecimal, ShouldBeTrue)
		So(cands[0].value, ShouldEqual, 252.735)
	})
}

// TestScoringPrefersDecimalCandidates covers the user-reported
// regression where chassis-printed model numbers like "103942"
// outrank the actual dial reading. With the decimal-presence bonus,
// any merged candidate beats a standalone of similar size.
func TestScoringPrefersDecimalCandidates(t *testing.T) {
	Convey("A merged 7-digit reading outranks a standalone 6-digit model number at the same size", t, func() {
		blocks := []interfaces.OCRTextBlock{
			// Real dial reading split across two blocks
			block("0252", 0.30, 0.45, 0.10, 0.05, 0.95),
			block("735", 0.41, 0.45, 0.05, 0.05, 0.85),
			// Chassis-printed model number, similar font height
			block("103942", 0.60, 0.20, 0.12, 0.05, 0.95),
		}
		cands := extractNumberCandidates(blocks)
		So(len(cands), ShouldEqual, 2)
		best := pickBest(cands)
		So(best, ShouldNotBeNil)
		So(best.value, ShouldEqual, 252.735)
	})

	Convey("Letter-trailing blocks survive but lose to a real reading", t, func() {
		// "13MA" has letters DIRECTLY after digits (no whitespace) —
		// the model-number signature. It's dropped.
		// "103942 N" has letters AFTER whitespace, so it could be a
		// unit-suffixed reading; we keep it. But it has no decimal
		// pair → loses to 0252.735 in scoring.
		blocks := []interfaces.OCRTextBlock{
			block("13MA", 0.20, 0.20, 0.06, 0.04, 0.95),
			block("103942 N", 0.27, 0.20, 0.15, 0.04, 0.95),
			block("0252.735", 0.30, 0.55, 0.18, 0.06, 0.95),
		}
		cands := extractNumberCandidates(blocks)
		best := pickBest(cands)
		So(best, ShouldNotBeNil)
		So(best.value, ShouldEqual, 252.735)
	})

	Convey("Standalone short runs are penalized", t, func() {
		// Two candidates: a 3-digit standalone and a 5-digit merged
		// reading. Merged should win even though both are similar
		// height/confidence.
		blocks := []interfaces.OCRTextBlock{
			block("671", 0.20, 0.30, 0.06, 0.05, 0.90),
			block("0007", 0.50, 0.50, 0.08, 0.05, 0.90),
			block("452", 0.59, 0.50, 0.05, 0.05, 0.90),
		}
		cands := extractNumberCandidates(blocks)
		best := pickBest(cands)
		So(best, ShouldNotBeNil)
		So(best.value, ShouldEqual, 7.452)
	})
}
