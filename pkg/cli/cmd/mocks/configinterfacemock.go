// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mocks

import (
	"context"
	"reflect"

	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
)

type MockConfigInterface struct {
	ctrl     *gomock.Controller
	recorder *MockConfigInterfaceRecorder
}

type MockConfigInterfaceRecorder struct {
	mock *MockConfigInterface
}

func NewMockConfigInterface(ctrl *gomock.Controller) *MockConfigInterface {
	mock := &MockConfigInterface{ctrl: ctrl}
	mock.recorder = &MockConfigInterfaceRecorder{mock}
	return mock
}

func (m *MockConfigInterface) EXPECT() *MockConfigInterfaceRecorder {
	return m.recorder
}

func (m *MockConfigInterface) ConfigFromContext(arg0 context.Context) *viper.Viper {
	ret := m.ctrl.Call(m, "ConfigFromContext", arg0)
	ret0, _ := ret[0].(*viper.Viper)
	return ret0
}

func (mr *MockConfigInterfaceRecorder) ConfigFromContext(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "DoSomething", reflect.TypeOf((*MockConfigInterface)(nil).ConfigFromContext), arg0)
}
