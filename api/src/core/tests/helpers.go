package tests

import (
	"testing"

	"github.com/stretchr/testify/mock"
)

// AssertMockFulfilled asserts that all expectations on the given mocks have been met.
func AssertMockFulfilled(t *testing.T, mocks ...*mock.Mock) bool {
	t.Helper()

	result := true

	for _, m := range mocks {
		if !m.AssertExpectations(t) {
			result = false
		}
	}

	return result
}

// ResetMocks clears all expectations and calls on the given mocks.
func ResetMocks(mocks ...*mock.Mock) {
	for _, m := range mocks {
		m.ExpectedCalls = nil
		m.Calls = nil
	}
}
