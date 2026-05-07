package tests_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"

	"github.com/titouanfreville/copro-manager/api/src/core/tests"
)

func Test_ErrorShouldBeLike(t *testing.T) {
	Convey("Given some errors", t, func() {
		mainError := errors.New("main error")
		error1 := errors.New("wrong_1")
		error2 := errors.New("wrong_2")
		error1Wrap := fmt.Errorf("wrap: %w", error1)
		error2Wrap := fmt.Errorf("wrap: %w", error2)

		Convey("should success", func() {
			So(mainError, tests.ShouldBeLikeError, mainError)
			So(error1, tests.ShouldBeLikeError, error1)
			So(error1Wrap, tests.ShouldBeLikeError, error1)
		})

		Convey("should fail", func() {
			So(tests.ShouldBeLikeError(mainError, error1), ShouldNotBeZeroValue)
			So(tests.ShouldBeLikeError(mainError, error1Wrap), ShouldNotBeZeroValue)
			So(tests.ShouldBeLikeError(error1, error2), ShouldNotBeZeroValue)
			So(tests.ShouldBeLikeError(error1Wrap, error2Wrap), ShouldNotBeZeroValue)
			So(tests.ShouldBeLikeError(error1Wrap), ShouldNotBeZeroValue)
			So(tests.ShouldBeLikeError("tests", error2Wrap), ShouldNotBeZeroValue)
			So(tests.ShouldBeLikeError(mainError, "tests"), ShouldNotBeZeroValue)
		})
	})
}

func Test_ShouldBeFullFilled(t *testing.T) {
	m1 := &Mock{}
	m2 := &Mock2{}

	testifyMocks := []*mock.Mock{&m1.Mock, &m2.Mock}

	Convey("Given mocks: ", t, func() {
		Convey("single instance on testify", func() {
			Convey("should success if full filled ", func() {
				m1.On("Test", "some").Return(nil).Twice()
				_ = m1.Test("some")
				_ = m1.Test("some")
				So(&m1.Mock, tests.ShouldBeFullFilled)
			})

			Convey("should fail if not full filled", func() {
				m1.On("Test", "some").Return(nil).Twice()
				_ = m1.Test("some")
				So(tests.ShouldBeFullFilled(&m1.Mock), ShouldNotBeZeroValue)
			})
		})

		Convey("multiple instance on testify", func() {
			Convey("should success if full filled ", func() {
				m1.On("Test", "some").Return(nil).Twice()
				_ = m1.Test("some")
				_ = m1.Test("some")
				m2.On("Do", "some").Return(nil).Once()
				_ = m2.Do("some")
				So(testifyMocks, tests.ShouldBeFullFilled)
			})

			Convey("should fail if not full filled", func() {
				m1.On("Test", "some").Return(nil).Twice()
				_ = m1.Test("some")
				m2.On("Do", "some").Return(nil).Once()
				_ = m2.Do("some")
				So(tests.ShouldBeFullFilled(testifyMocks), ShouldNotBeZeroValue)
			})
		})

		Convey("unknown should fail", func() {
			So(tests.ShouldBeFullFilled("tests"), ShouldNotBeZeroValue)
		})

		tests.ResetMocks(testifyMocks...)
	})
}

func Test_ShouldBeEquivalent(t *testing.T) {
	type TestType struct {
		A int
		B string
		C bool
		D map[string]string
		E []interface{}
	}

	type TestDate struct {
		A time.Time
		B time.Time
		C bool
		D map[string]string
	}

	Convey("Given value to compare: ", t, func() {
		testVal := TestType{
			A: 10,
			B: "tests",
			C: true,
			D: map[string]string{"fun": "kykong", "st": "status", "one": "way"},
			E: []interface{}{"tests", 123, 40.9, false, true, 'c'},
		}

		testDateVal := TestDate{
			A: time.Now(),
			B: time.Now().Add(time.Nanosecond*100 + time.Hour*2),
			C: true,
			D: map[string]string{"fun": "kykong", "st": "status", "one": "way"},
		}

		Convey("equivalent structure should be deemed as equal", func() {
			So(testVal, tests.ShouldBeEquivalent, testVal)

			expectedCorrectDateVal := testDateVal
			expectedCorrectDateVal.A = expectedCorrectDateVal.A.Round(time.Nanosecond)
			So(ShouldResemble(expectedCorrectDateVal, testDateVal), ShouldNotEqual, "")
			So(testDateVal, tests.ShouldBeEquivalent, testDateVal)
		})

		Convey("different structure should be refused the right to equality", func() {
			cp := testVal
			cp.B = "azfazniofaejizoga"
			So(tests.ShouldBeEquivalent(testVal, cp), ShouldStartWith, "items does not match.")
		})
	})
}
