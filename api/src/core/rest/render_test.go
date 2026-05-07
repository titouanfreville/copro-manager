package rest_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
)

func Test_RenderJSON(t *testing.T) {
	Convey("Given an object and an HTTP code", t, func() {
		Convey("I should be able to have a correct json response", func() {
			status := 418
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			obj := "foo"

			rest.Render().JSON(status, w, r, obj)

			So(w.Code, ShouldEqual, status)
			So(w.Header().Get("Content-Type"), ShouldContainSubstring, "application/json")
			So(w.Body.String(), ShouldEqual, "\"foo\"\n")
		})
	})
}

func Test_RenderNoContent(t *testing.T) {
	Convey("Given a NoContent call", t, func() {
		Convey("I should get an empty response with the given status", func() {
			w := httptest.NewRecorder()

			rest.Render().NoContent(http.StatusNoContent, w)

			So(w.Code, ShouldEqual, http.StatusNoContent)
			So(w.Body.String(), ShouldBeEmpty)
		})
	})
}
