// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mocks

import (
	"context"
	"reflect"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/workspaces"
)

type MockConnectionsFactory struct {
	ctrl     *gomock.Controller
	recorder *MockConnectionsFactoryRecorder
}

type MockConnectionsFactoryRecorder struct {
	mock *MockConnectionsFactory
}

func NewMockConnectionsFactory(ctrl *gomock.Controller) *MockConnectionsFactory {
	mock := &MockConnectionsFactory{ctrl: ctrl}
	mock.recorder = &MockConnectionsFactoryRecorder{mock}
	return mock
}

func (m *MockConnectionsFactory) EXPECT() *MockConnectionsFactoryRecorder {
	return m.recorder
}

func (m *MockConnectionsFactory) CreateDeploymentClient(arg0 context.Context, arg1 workspaces.Workspace) (clients.DeploymentClient, error) {
	ret := m.ctrl.Call(m, "CreateDeploymentClient", arg0, arg1)
	ret0, _ := ret[0].(clients.DeploymentClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (m *MockConnectionsFactoryRecorder) CreateDeploymentClient(arg0, arg1 interface{}) *gomock.Call {
	return m.mock.ctrl.RecordCall(m.mock, "CreateDeploymentClient", reflect.TypeOf((*MockConnectionsFactory)(nil).CreateDeploymentClient), arg0, arg1)
}

func (m *MockConnectionsFactory) CreateDiagnosticsClient(arg0 context.Context, arg1 workspaces.Workspace) (clients.DiagnosticsClient, error) {
	ret := m.ctrl.Call(m, "CreateDiagnosticsClient", arg0, arg1)
	ret0, _ := ret[0].(clients.DiagnosticsClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (m *MockConnectionsFactoryRecorder) CreateDiagnosticsClient(arg0, arg1 interface{}) *gomock.Call {
	return m.mock.ctrl.RecordCall(m.mock, "CreateDiagnosticsClient", reflect.TypeOf((*MockConnectionsFactory)(nil).CreateDiagnosticsClient), arg0, arg1)
}

func (m *MockConnectionsFactory) CreateApplicationsManagementClient(arg0 context.Context, arg1 workspaces.Workspace) (clients.ApplicationsManagementClient, error) {
	ret := m.ctrl.Call(m, "CreateApplicationsManagementClient", arg0, arg1)
	ret0, _ := ret[0].(clients.ApplicationsManagementClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (m *MockConnectionsFactoryRecorder) CreateApplicationsManagementClient(arg0, arg1 interface{}) *gomock.Call {
	return m.mock.ctrl.RecordCall(m.mock, "CreateApplicationsManagementClient", reflect.TypeOf((*MockConnectionsFactory)(nil).CreateApplicationsManagementClient), arg0, arg1)
}

func (m *MockConnectionsFactory) CreateServerLifecycleClient(arg0 context.Context, arg1 workspaces.Workspace) (clients.ServerLifecycleClient, error) {
	ret := m.ctrl.Call(m, "CreateServerLifecycleClient", arg0, arg1)
	ret0, _ := ret[0].(clients.ServerLifecycleClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (m *MockConnectionsFactoryRecorder) CreateServerLifecycleClient(arg0, arg1 interface{}) *gomock.Call {
	return m.mock.ctrl.RecordCall(m.mock, "CreateServerLifecycleClient", reflect.TypeOf((*MockConnectionsFactory)(nil).CreateServerLifecycleClient), arg0, arg1)
}
