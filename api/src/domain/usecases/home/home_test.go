package home

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
)

func TestHome(t *testing.T) {
	Convey("Given a Home usecase", t, func() {
		uc := New(zap.NewNop())
		ctx := context.Background()

		Convey("When calling Hello", func() {
			result := uc.Hello(ctx)

			Convey("Then it returns the greeting", func() {
				So(result, ShouldEqual, "Copro manager API")
			})
		})
	})
}
