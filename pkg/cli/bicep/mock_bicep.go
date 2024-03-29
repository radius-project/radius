// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/radius-project/radius/pkg/cli/bicep (interfaces: Interface)

// Package bicep is a generated GoMock package.
package bicep

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockInterface is a mock of Interface interface.
type MockInterface struct {
	ctrl     *gomock.Controller
	recorder *MockInterfaceMockRecorder
}

// MockInterfaceMockRecorder is the mock recorder for MockInterface.
type MockInterfaceMockRecorder struct {
	mock *MockInterface
}

// NewMockInterface creates a new mock instance.
func NewMockInterface(ctrl *gomock.Controller) *MockInterface {
	mock := &MockInterface{ctrl: ctrl}
	mock.recorder = &MockInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockInterface) EXPECT() *MockInterfaceMockRecorder {
	return m.recorder
}

// PrepareTemplate mocks base method.
func (m *MockInterface) PrepareTemplate(arg0 string) (map[string]interface{}, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PrepareTemplate", arg0)
	ret0, _ := ret[0].(map[string]interface{})
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PrepareTemplate indicates an expected call of PrepareTemplate.
func (mr *MockInterfaceMockRecorder) PrepareTemplate(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PrepareTemplate", reflect.TypeOf((*MockInterface)(nil).PrepareTemplate), arg0)
}
