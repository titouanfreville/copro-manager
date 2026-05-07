package rest_test

import (
	"context"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/core/tests"
)

func Test_DataBinding(t *testing.T) {
	Convey("Given a request with", t, func() {
		expectedTest := "someData"
		expectedInt := 12
		expectedTestVal := true
		expectedArray := []string{"tests", "12"}
		expectedObject := testObject{Test: true, Val: 12.89}

		jsonBody := `{
			"tests": "someData",
			"anint": 12,
			"testval": true,
			"array": ["tests", "12"],
			"object": {"tests": true, "val": 12.89}
		}
		`
		// Cannot be indented as it will make body parsing fail
		multipartBody := `--xxx
Content-Disposition: form-data; name="tests"

someData
--xxx
Content-Disposition: form-data; name="anint"

12
--xxx
Content-Disposition: form-data; name="testval"

true
--xxx--
Content-Disposition: form-data; name="array"

[tests, 12]
--xxx--
Content-Disposition: form-data; name="object"

{tests=true,val=12.89}
--xxx--
		`

		Convey("JSON data", func() {
			req := httptest.NewRequest(
				"POST", "http://any.fr",
				strings.NewReader(jsonBody),
			)

			Convey("I should be able to retrieve existing key in correct type/object", func() {
				var test mainReqData

				So(rest.Bind().JSONData(req, &test), ShouldBeNil)
				So(test.Test, ShouldEqual, expectedTest)
				So(test.SomeInt, ShouldEqual, expectedInt)
				So(test.TestBool, ShouldEqual, expectedTestVal)
				So(test.TestArray, tests.ShouldBeEquivalent, expectedArray)
				So(test.TestObject, tests.ShouldBeEquivalent, expectedObject)
			})

			Convey("I should have an error if value pass cannot be targeted by marshaling", func() {
				var test string
				var someInt int
				var testVal bool
				var array []string
				var object map[string]interface{}
				err := rest.Bind().JSONData(req, &test, &someInt, &testVal, &array, &object)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "json: cannot unmarshal object into Go value of type")
			})
		})

		Convey("FORM data", func() {
			req := httptest.NewRequest(
				"POST", "http://any.fr",
				strings.NewReader(multipartBody),
			)
			req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")

			Convey("I should be able to retrieve existing key in correct type/object", func() {
				var test mainReqData

				So(rest.Bind().FormData(req, &test), ShouldBeNil)
				So(test.Test, ShouldEqual, expectedTest)
				So(test.SomeInt, ShouldEqual, expectedInt)
				So(test.TestBool, ShouldEqual, expectedTestVal)
			})

			Convey("I should not be able to retrieve objects/array", func() {
				var test mainReqData

				So(rest.Bind().FormData(req, &test), ShouldBeNil)
				So(test.TestObject, ShouldBeZeroValue)
				So(test.TestArray, ShouldBeZeroValue)
			})

			Convey("I should error when passing non object element", func() {
				var test string
				var someInt int
				var testVal bool
				var array []string
				var object map[string]interface{}
				err := rest.Bind().FormData(req, &test, &someInt, &testVal, &array, &object)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "expected type")
				So(err.Error(), ShouldContainSubstring, ", got unconvertible type 'map[string]interface {}")
			})

			Convey("I should error when passing non multipart request element", func() {
				req.Body = io.NopCloser(strings.NewReader(jsonBody))
				var test string
				var someInt int
				var testVal bool
				var array []string
				var object map[string]interface{}
				err := rest.Bind().FormData(req, &test, &someInt, &testVal, &array, &object)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, "multipart: NextPart: EOF")
			})

			Convey("I should get an error if uuid form data is not an UUID", func() {
				data := url.Values{}
				data.Set("uuid", "wrongUUID")
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Body = io.NopCloser(strings.NewReader(data.Encode()))
				var test mainReqData
				So(rest.Bind().FormData(req, &test), ShouldBeError)
			})

			Convey("I should be able to retrieve objects/array in application/x-www-form-urlencoded type", func() {
				expectedUUID := uuid.New()

				data := url.Values{}
				data.Set("tests", "someData")
				data.Set("anint", "12")
				data.Set("testval", "true")
				data.Set("uuid", expectedUUID.String())
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Body = io.NopCloser(strings.NewReader(data.Encode()))
				var test mainReqData
				So(rest.Bind().FormData(req, &test), ShouldBeNil)
				So(test.Test, ShouldEqual, expectedTest)
				So(test.SomeInt, ShouldEqual, expectedInt)
				So(test.TestBool, ShouldEqual, expectedTestVal)
				So(test.TestUUID.String(), ShouldEqual, expectedUUID.String())
			})
		})

		Convey("JSON or FORM data", func() {
			reqJSON := httptest.NewRequest(
				"POST", "http://any.fr",
				strings.NewReader(jsonBody),
			)
			reqJSON.Header.Set("Content-Type", "application/json")

			reqFORM := httptest.NewRequest(
				"POST", "http://any.fr",
				strings.NewReader(multipartBody),
			)
			reqFORM.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")

			Convey("I should be able to retrieve from both JSON && FORM", func() {
				var test mainReqData
				So(rest.Bind().RequestData(reqFORM, &test), ShouldBeNil)
				So(test.Test, ShouldEqual, expectedTest)
				So(test.SomeInt, ShouldEqual, expectedInt)
				So(test.TestBool, ShouldEqual, expectedTestVal)

				So(rest.Bind().RequestData(reqJSON, &test), ShouldBeNil)
				So(test.TestArray, tests.ShouldBeEquivalent, expectedArray)
				So(test.TestObject, tests.ShouldBeEquivalent, expectedObject)
			})

			Convey("I should have an error if binding from JSON failed", func() {
				var test string

				err := rest.Bind().RequestData(reqJSON, &test)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "json: cannot unmarshal object into Go value of type")
			})

			Convey("I should have an error if binding from FORM failed", func() {
				var testFail failedReqData

				err := rest.Bind().RequestData(reqFORM, &testFail)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "error(s) decoding:")
				So(err.Error(), ShouldContainSubstring, "cannot parse")
				So(err.Error(), ShouldContainSubstring, "invalid syntax")
			})

			Convey("I should have an error if binding expect form but data are not correctly formatted", func() {
				var test mainReqData
				reqFORM.Body = io.NopCloser(strings.NewReader(jsonBody))
				err := rest.Bind().RequestData(reqFORM, &test)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, "multipart: NextPart: EOF")
			})
		})
	})
}

func Test_URLParameterBinding(t *testing.T) {
	Convey("Given some request with URL with parameters", t, func() {
		req := httptest.NewRequest("GET", "http://any.fr?tests=tests&anint=666&val=T", nil)
		chiRequestContext := chi.NewRouteContext()
		expectedStringID := "satan"
		expectedIntID := 666
		chiRequestContext.URLParams.Add("someID", "666")
		chiRequestContext.URLParams.Add("someIDString", expectedStringID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiRequestContext))

		Convey("I should be able to recover 1 parameter", func() {
			var test string
			So(rest.Bind().URLParam(req, "someIDString", &test), ShouldBeNil)
			So(test, ShouldEqual, "satan")
		})

		Convey("I should have an error if asked parameters does not exists", func() {
			var test string
			So(rest.Bind().URLParam(req, "flagada", &test), tests.ShouldBeLikeError, rest.ErrNotFound)
		})

		Convey("I should have an error when url arg cannot be parsed to type", func() {
			var test chan string
			So(rest.Bind().URLParam(req, "someID", &test), tests.ShouldBeLikeError, rest.ErrUnsupportedType)

			var anInt int8
			So(rest.Bind().URLParam(req, "someIDString", &anInt), tests.ShouldBeLikeError, rest.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when parsing to non pointer", func() {
			var test string
			So(rest.Bind().URLParam(req, "someIDString", test), tests.ShouldBeLikeError, rest.ErrUnsupportedType)
		})

		Convey("I should be able to recover multiple arguments", func() {
			var test string
			var anInt int
			So(rest.Bind().URLParams(req, map[string]interface{}{"someIDString": &test, "someID": &anInt}), ShouldBeNil)
			So(test, ShouldEqual, expectedStringID)
			So(anInt, ShouldEqual, expectedIntID)
		})

		Convey("I should have an error when at least one url arg cannot be parsed to type", func() {
			var test chan string
			var test2 string
			var anInt int64
			So(rest.Bind().URLParams(req, map[string]interface{}{"someIDString": &test, "someID": &anInt}), tests.ShouldBeLikeError, rest.ErrUnsupportedType)
			So(rest.Bind().URLParams(req, map[string]interface{}{"someIDString": &anInt, "someID": &test2}), tests.ShouldBeLikeError, rest.ErrInvalidTypeConversion)
		})
	})
}

func Test_URLArgsBinding(t *testing.T) {
	Convey("Given some request with URL arguments", t, func() {
		req := httptest.NewRequest("GET", "http://any.fr?tests=tests&anint=666&val=T", nil)

		Convey("I should be able to recover 1 argument", func() {
			var test string
			So(rest.Bind().URLArg(req, "tests", &test), ShouldBeNil)
			So(test, ShouldEqual, "tests")
		})

		Convey("I should not change value if asked url arg does not exists", func() {
			test := "Satan"
			So(rest.Bind().URLArg(req, "flagada", &test), ShouldBeNil)
			So(test, ShouldEqual, "Satan")
		})

		Convey("I should have an error when url arg cannot be parsed to type", func() {
			var test chan string
			So(rest.Bind().URLArg(req, "tests", &test), tests.ShouldBeLikeError, rest.ErrUnsupportedType)

			var anInt int8
			So(rest.Bind().URLArg(req, "tests", &anInt), tests.ShouldBeLikeError, rest.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when parsing to non pointer", func() {
			var test string
			So(rest.Bind().URLArg(req, "tests", test), tests.ShouldBeLikeError, rest.ErrUnsupportedType)
		})

		Convey("I should be able to recover multiple arguments", func() {
			var test string
			var anInt int
			So(rest.Bind().URLArgs(req, map[string]interface{}{"tests": &test, "anint": &anInt}), ShouldBeNil)
			So(test, ShouldEqual, "tests")
			So(anInt, ShouldEqual, 666)
		})

		Convey("I should have an error when at least one url arg cannot be parsed to type", func() {
			var test chan string
			var test2 string
			var anInt int64
			So(rest.Bind().URLArgs(req, map[string]interface{}{"tests": &test, "anint": &anInt}), tests.ShouldBeLikeError, rest.ErrUnsupportedType)
			So(rest.Bind().URLArgs(req, map[string]interface{}{"tests": &anInt, "anint": &test2}), tests.ShouldBeLikeError, rest.ErrInvalidTypeConversion)
		})
	})
}

type testObject struct {
	Test bool    `json:"tests" mapstructure:"tests"`
	Val  float64 `json:"val" mapstructure:"val"`
}

type mainReqData struct {
	TestObject testObject `json:"object" mapstructure:"object"`
	Test       string     `json:"tests" mapstructure:"tests"`
	SomeInt    int        `json:"anint" mapstructure:"anint"`
	TestBool   bool       `json:"testval" mapstructure:"testval"`
	TestArray  []string   `json:"array" mapstructure:"array"`
	TestUUID   uuid.UUID  `json:"uuid" mapstructure:"uuid"`
}

type failedReqData struct {
	TestObject testObject `json:"object" mapstructure:"object"`
	Test       string     `json:"testval" mapstructure:"testval"`
	SomeInt    int        `json:"tests" mapstructure:"tests"`
	TestBool   bool       `json:"anint" mapstructure:"anint"`
	TestArray  []string   `json:"array" mapstructure:"array"`
}
