package tests

import (
	"fmt"
	"testing"

	"github.com/go-errors/errors"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/mock"
)

// ShouldBeLikeError asserts errors look alike using goerrors.Is method.
func ShouldBeLikeError(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return "ShouldBeLikeError assertion expect 1 and only 1 asserter"
	}

	resp := ""
	expActComp := fmt.Sprintf("\nActual: %#v\nExpected: %#v", actual, expected[0])

	actualErr, ok := actual.(error)
	if !ok {
		resp += "Actual value should be an error"
	}

	expectedErr, ok2 := expected[0].(error)
	if !ok2 {
		resp += "Expected value should be an error"
	}

	if resp != "" {
		return resp + expActComp
	}

	if !errors.Is(actualErr, expectedErr) {
		return "Actual error do not match expected err Is." + expActComp
	}

	return ""
}

// ShouldBeFullFilled asserts provided testify mock(s) have all expectations met.
func ShouldBeFullFilled(actual interface{}, _ ...interface{}) string {
	expActComp := fmt.Sprintf("\nProvided: %#v", actual)

	singleTestifyMock, isTestify := actual.(*mock.Mock)
	listTestifyMock, isTestifyList := actual.([]*mock.Mock)

	if !isTestify && !isTestifyList {
		return "Provided value is not a testify or mock list." + expActComp
	}

	if isTestify && !AssertMockFulfilled(&testing.T{}, singleTestifyMock) {
		return "Some expectation were not met."
	}

	if isTestifyList {
		for pos, localMock := range listTestifyMock {
			if !AssertMockFulfilled(&testing.T{}, localMock) {
				return fmt.Sprintf("Some expectation were not met for mock number: %d.", pos)
			}
		}
	}

	return ""
}

// ShouldBeEquivalent asserts items are equivalent using go-cmp.
func ShouldBeEquivalent(actual interface{}, expected ...interface{}) string {
	lenExpected := len(expected)
	if lenExpected < 1 {
		return "assertion expect a comparison object and optional gocmp options list"
	}

	var opts cmp.Options

	if lenExpected > 1 {
		for _, candidate := range expected[1:] {
			opt, ok := candidate.(cmp.Option)
			if !ok {
				return fmt.Sprintf("options is not a cmp.Option type. got: %T", candidate)
			}

			opts = append(opts, opt)
		}
	}

	if !cmp.Equal(actual, expected[0], opts...) {
		return fmt.Sprintf("items does not match.\nActual: %#v\nExpected: %#v", actual, expected[0])
	}

	return ""
}
