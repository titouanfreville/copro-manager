package expenses

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

func TestComputeShares(t *testing.T) {
	rdc := &entities.Foyer{ID: "f-rdc", Floor: entities.FoyerFloorRDC, Parts: 350}
	premier := &entities.Foyer{ID: "f-1er", Floor: entities.FoyerFloor1er, Parts: 650}
	copro := &entities.Copro{ID: "c-1", TotalParts: 1000}
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	base := CreateInput{
		ExpenseDraft: entities.ExpenseDraft{
			Name:         "Eau été",
			AmountCents:  10000, // 100€
			Currency:     "EUR",
			Date:         now,
			PayerFoyerID: rdc.ID,
			CategoryID:   "eau",
		},
	}

	Convey("Given an Equal-mode expense with even amount", t, func() {
		in := base
		in.DistributionMode = entities.DistributionModeEqual

		shareRDC, share1er, err := computeShares(in, rdc, premier, copro)

		Convey("Then both foyers split exactly half", func() {
			So(err, ShouldBeNil)
			So(shareRDC, ShouldEqual, 5000)
			So(share1er, ShouldEqual, 5000)
			So(shareRDC+share1er, ShouldEqual, in.AmountCents)
		})
	})

	Convey("Given an Equal-mode expense with odd amount", t, func() {
		in := base
		in.AmountCents = 1001
		in.DistributionMode = entities.DistributionModeEqual

		Convey("When the payer is RDC", func() {
			in.PayerFoyerID = rdc.ID
			shareRDC, share1er, err := computeShares(in, rdc, premier, copro)
			So(err, ShouldBeNil)
			So(shareRDC, ShouldEqual, 501)
			So(share1er, ShouldEqual, 500)
			So(shareRDC+share1er, ShouldEqual, in.AmountCents)
		})

		Convey("When the payer is 1er", func() {
			in.PayerFoyerID = premier.ID
			shareRDC, share1er, err := computeShares(in, rdc, premier, copro)
			So(err, ShouldBeNil)
			So(shareRDC, ShouldEqual, 500)
			So(share1er, ShouldEqual, 501)
			So(shareRDC+share1er, ShouldEqual, in.AmountCents)
		})
	})

	Convey("Given a Tantièmes-mode expense", t, func() {
		in := base
		in.DistributionMode = entities.DistributionModeTantiemes

		Convey("When parts split 350/650 and amount divides cleanly", func() {
			in.AmountCents = 10000
			shareRDC, share1er, err := computeShares(in, rdc, premier, copro)

			So(err, ShouldBeNil)
			So(shareRDC, ShouldEqual, 3500)
			So(share1er, ShouldEqual, 6500)
			So(shareRDC+share1er, ShouldEqual, in.AmountCents)
		})

		Convey("When the integer division leaves a remainder", func() {
			in.AmountCents = 10001
			in.PayerFoyerID = premier.ID
			shareRDC, share1er, err := computeShares(in, rdc, premier, copro)

			Convey("Then the remainder lands on the payer's share", func() {
				So(err, ShouldBeNil)
				So(shareRDC+share1er, ShouldEqual, in.AmountCents)
				// remainder >= 1 cent goes to the payer (1er here)
				So(share1er, ShouldBeGreaterThan, in.AmountCents*premier.Parts/copro.TotalParts)
			})
		})

		Convey("When parts don't sum to total_parts", func() {
			badRDC := &entities.Foyer{ID: "f-rdc", Parts: 100}
			_, _, err := computeShares(in, badRDC, premier, copro)
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a Custom-mode expense", t, func() {
		in := base
		in.DistributionMode = entities.DistributionModeCustom

		Convey("When the supplied shares sum to the total", func() {
			in.ShareRDCCents = 4000
			in.Share1erCents = 6000
			shareRDC, share1er, err := computeShares(in, rdc, premier, copro)

			So(err, ShouldBeNil)
			So(shareRDC, ShouldEqual, 4000)
			So(share1er, ShouldEqual, 6000)
		})

		Convey("When the supplied shares don't sum to the total", func() {
			in.ShareRDCCents = 4000
			in.Share1erCents = 5999
			_, _, err := computeShares(in, rdc, premier, copro)
			So(err, ShouldNotBeNil)
		})

		Convey("When a share is negative", func() {
			in.ShareRDCCents = -1
			in.Share1erCents = 10001
			_, _, err := computeShares(in, rdc, premier, copro)
			So(err, ShouldNotBeNil)
		})
	})
}
