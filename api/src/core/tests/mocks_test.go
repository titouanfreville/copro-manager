package tests_test

import "github.com/stretchr/testify/mock"

// Mock is a test mock used by convey asserter tests.
type Mock struct {
	mock.Mock
}

func (m *Mock) Test(arg string) error {
	args := m.Called(arg)
	return args.Error(0)
}

// Mock2 is a second test mock used by convey asserter tests.
type Mock2 struct {
	mock.Mock
}

func (m *Mock2) Do(arg string) error {
	args := m.Called(arg)
	return args.Error(0)
}
