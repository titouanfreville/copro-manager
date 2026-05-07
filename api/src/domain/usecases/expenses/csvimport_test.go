package expenses

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

func TestParseEURCents(t *testing.T) {
	Convey("Given the legacy CSV's EUR formatting", t, func() {
		Convey("It strips the € suffix and the comma decimal", func() {
			c, err := parseEURCents("346,50 €")
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 34650)
		})
		Convey("It tolerates thin/non-breaking-space thousands separators", func() {
			// Spreadsheets often emit U+00A0 between thousands and the
			// comma; the value below is "1 311,44 €".
			c, err := parseEURCents("1 311,44 €")
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 131144)
		})
		Convey("Empty input returns 0 with no error", func() {
			c, err := parseEURCents("")
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 0)
		})
		Convey("Bare numbers without € work too", func() {
			c, err := parseEURCents("60,00")
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 6000)
		})
		Convey("Unparseable input bubbles the error", func() {
			_, err := parseEURCents("garbage")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestParseFRDate(t *testing.T) {
	Convey("Given DD/MM/YYYY", t, func() {
		t1, err := parseFRDate("24/03/2025")
		So(err, ShouldBeNil)
		So(t1.Year(), ShouldEqual, 2025)
		So(int(t1.Month()), ShouldEqual, 3)
		So(t1.Day(), ShouldEqual, 24)
	})
	Convey("Given ISO YYYY-MM-DD as a fallback", t, func() {
		t1, err := parseFRDate("2025-03-24")
		So(err, ShouldBeNil)
		So(t1.Year(), ShouldEqual, 2025)
	})
	Convey("Empty input is rejected", t, func() {
		_, err := parseFRDate("")
		So(err, ShouldNotBeNil)
	})
}

func TestGuessCategoryID(t *testing.T) {
	cases := []struct {
		item, want string
	}{
		{"Eau Janvier à Mai 2025", "eau"},
		{"Electricité mars", "electricite"},
		{"Électricité octobre", "electricite"},
		{"Taxe foncière", "taxe-fonciere"},
		{"Assurance habitation", "assurance"},
		{"Frais de syndic 2025", "syndic"},
		{"Plantation haie mitoyenne", "travaux"},
		{"Entretien pompe relevage", "travaux"},
		{"Réparation chaudière", "travaux"},
		{"Peinture façade", "travaux"},
		// Items the heuristic can't classify go to the TBD bucket so the
		// operator can re-tag them in the UI rather than having them
		// silently land in Travaux.
		{"Cadeau anniversaire copro", "tbd"},
		{"Truc inconnu", "tbd"},
	}
	Convey("Item-label heuristic maps to seeded category IDs", t, func() {
		for _, c := range cases {
			c := c
			Convey("→ "+c.item, func() {
				So(guessCategoryID(c.item), ShouldEqual, c.want)
			})
		}
	})
}

func TestBuildInputRoundsRoundingArtifacts(t *testing.T) {
	Convey("Given a row whose shares miss the total by 1 cent", t, func() {
		raw := rawRow{
			Item:        "Eau été",
			Date:        "30/05/2025",
			Total:       "100,00 €",
			ChargeRDC:   "33,33 €",
			Charge1er:   "66,66 €", // sums to 99,99 — 1¢ short
			Repartition: "prorata",
		}
		in, err := buildInput(raw, "foyer-rdc")

		Convey("It absorbs the missing cent on the larger share", func() {
			So(err, ShouldBeNil)
			So(in.AmountCents, ShouldEqual, 10000)
			So(in.ShareRDCCents+in.Share1erCents, ShouldEqual, 10000)
			// The larger share (1er, 6666) should pick up the extra cent.
			So(in.Share1erCents, ShouldEqual, 6667)
			So(in.ShareRDCCents, ShouldEqual, 3333)
			So(in.DistributionMode, ShouldEqual, entities.DistributionModeCustom)
			So(in.TrustExplicitShares, ShouldBeTrue)
		})
	})

	Convey("Given a row whose shares miss by more than 1 cent", t, func() {
		raw := rawRow{
			Item:      "Bug row",
			Date:      "01/01/2025",
			Total:     "100,00 €",
			ChargeRDC: "30,00 €",
			Charge1er: "60,00 €",
		}
		_, err := buildInput(raw, "foyer-rdc")
		Convey("It refuses the row", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestMapRepartitionToMode(t *testing.T) {
	cases := []struct {
		csv  string
		want entities.DistributionMode
	}{
		{"50/50", entities.DistributionModeEqual},
		{"tantieme", entities.DistributionModeTantiemes},
		{"Tantième", entities.DistributionModeTantiemes},
		{"prorata", entities.DistributionModeCustom},
		{"", entities.DistributionModeCustom},
		{"weird", entities.DistributionModeCustom},
	}
	Convey("Legacy Répartition labels map to our enum", t, func() {
		for _, c := range cases {
			c := c
			Convey("→ "+c.csv, func() {
				So(mapRepartitionToMode(c.csv), ShouldEqual, c.want)
			})
		}
	})
}
