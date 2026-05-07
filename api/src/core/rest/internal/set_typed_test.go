package internal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/titouanfreville/copro-manager/api/src/core/rest/internal"
	"github.com/titouanfreville/copro-manager/api/src/core/tests"
)

func Test_SetTyped(t *testing.T) {
	var emptyStrList []string

	Convey("Given I want to bind string/string list to typed interface", t, func() {
		Convey("I should be able to retrieve string from single string value", func() {
			var actual string
			expected := "tests"
			So(internal.SetTyped(expected, emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)

			expected = "anotherTest"
			So(internal.SetTyped(expected, []string{"some", "tests"}, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve string list from string list value", func() {
			var actual []string
			expected := []string{"tests", "some", "value"}
			So(internal.SetTyped("efnuio", expected, &actual), ShouldBeNil)
			So(actual, tests.ShouldBeEquivalent, expected)

			expected = []string{"Tankyou", "Satan"}
			So(internal.SetTyped("re, re fa sol la re", expected, &actual), ShouldBeNil)
			So(actual, tests.ShouldBeEquivalent, expected)
		})

		Convey("I should be able to retrieve int from single string value", func() {
			var actual int
			expected := 666
			So(internal.SetTyped("666", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve int8 from single string value", func() {
			var actual int8
			expected := 8
			So(internal.SetTyped("8", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve int16 from single string value", func() {
			var actual int16
			expected := 666
			So(internal.SetTyped("666", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve int32 from single string value", func() {
			var actual int32
			expected := 666
			So(internal.SetTyped("666", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve int64 from single string value", func() {
			var actual int64
			expected := 666
			So(internal.SetTyped("666", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve uint from single string value", func() {
			var actual uint
			expected := 666
			So(internal.SetTyped("666", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve uint8 from single string value", func() {
			var actual uint8
			expected := 126
			So(internal.SetTyped("126", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve uint16 from single string value", func() {
			var actual uint16
			expected := 666
			So(internal.SetTyped("666", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve uint32 from single string value", func() {
			var actual uint32
			expected := 666
			So(internal.SetTyped("666", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve uint64 from single string value", func() {
			var actual uint64
			expected := 666
			So(internal.SetTyped("666", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve float32 from single string value", func() {
			var actual float32
			So(internal.SetTyped("666.7", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldAlmostEqual, 666.7, 0.001)
		})

		Convey("I should be able to retrieve float64 from single string value", func() {
			var actual float64
			expected := 666.7
			So(internal.SetTyped("666.7", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		})

		Convey("I should be able to retrieve bool from single string value", func() {
			var actual bool
			So(internal.SetTyped("T", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldBeTrue)

			So(internal.SetTyped("F", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldBeFalse)

			So(internal.SetTyped("true", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldBeTrue)

			So(internal.SetTyped("false", emptyStrList, &actual), ShouldBeNil)
			So(actual, ShouldBeFalse)
		})

		Convey("I should have an error when provided value cannot be bind to int and expecting int", func() {
			var actual int
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to int8 and expecting int8", func() {
			var actual int8
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to int16 and expecting int16", func() {
			var actual int16
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to int32 and expecting int32", func() {
			var actual int32
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to int64 and expecting int64", func() {
			var actual int64
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to uint and expecting uint", func() {
			var actual uint
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to uint8 and expecting uint8", func() {
			var actual uint8
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to uint16 and expecting uint16", func() {
			var actual uint16
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to uint32 and expecting uint32", func() {
			var actual uint32
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to uint64 and expecting uint64", func() {
			var actual uint64
			So(internal.SetTyped("ceci n'est pas un entier", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to float32 and expecting float32", func() {
			var actual float32
			So(internal.SetTyped("ceci n'est pas un float", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to float64 and expecting float64", func() {
			var actual float64
			So(internal.SetTyped("ceci n'est pas un float", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error when provided value cannot be bind to bool and expecting bool", func() {
			var actual bool
			So(internal.SetTyped("ceci n'est pas un bool", emptyStrList, &actual), tests.ShouldBeLikeError, internal.ErrInvalidTypeConversion)
		})

		Convey("I should have an error if type is not supported", func() {
			var actual chan string
			So(internal.SetTyped("any", []string{}, &actual), tests.ShouldBeLikeError, internal.ErrUnsupportedType)
		})
	})
}
