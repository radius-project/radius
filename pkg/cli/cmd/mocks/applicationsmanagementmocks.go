// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mocks

import (
	"context"
	"reflect"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

type MockApplicationsManagementClient struct {
	ctrl     *gomock.Controller
	recorder *MockApplicationsManagementRecorder
}

type MockApplicationsManagementRecorder struct {
	mock *MockApplicationsManagementClient
}

func NewMockApplicationsManagementClient(ctrl *gomock.Controller) *MockApplicationsManagementClient {
	mock := &MockApplicationsManagementClient{ctrl: ctrl}
	mock.recorder = &MockApplicationsManagementRecorder{mock}
	return mock
}

func (m *MockApplicationsManagementClient) EXPECT() *MockApplicationsManagementRecorder {
	return m.recorder
}

func (m *MockApplicationsManagementClient) ListAllResourcesByType(arg0 context.Context, arg1 string) ([]generated.GenericResource, error) {
	ret := m.ctrl.Call(m, "ListAllResourcesByType", arg0, arg1)
	ret0, _ := ret[0].([]generated.GenericResource)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) ListAllResourcesByType(arg0, arg1 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "ListAllResourcesByType", reflect.TypeOf((*MockApplicationsManagementClient)(nil).ListAllResourcesByType), arg0, arg1)
}

func (m *MockApplicationsManagementClient) ListAllResourceOfTypeInApplication(arg0 context.Context, arg1 string, arg2 string) ([]generated.GenericResource, error) {
	ret := m.ctrl.Call(m, "ListAllResourceOfTypeInApplication", arg0, arg1, arg2)
	ret0, _ := ret[0].([]generated.GenericResource)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) ListAllResourceOfTypeInApplication(arg0, arg1, arg2 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "ListAllResourceOfTypeInApplication", reflect.TypeOf((*MockApplicationsManagementClient)(nil).ListAllResourceOfTypeInApplication), arg0, arg1, arg2)
}

func (m *MockApplicationsManagementClient) ListAllResourcesByApplication(arg0 context.Context, arg1 string) ([]generated.GenericResource, error) {
	ret := m.ctrl.Call(m, "ListAllResourcesByApplication", arg0, arg1)
	ret0, _ := ret[0].([]generated.GenericResource)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) ListAllResourcesByApplication(arg0, arg1 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "ListAllResourcesByApplication", reflect.TypeOf((*MockApplicationsManagementClient)(nil).ListAllResourcesByApplication), arg0, arg1)
}

func (m *MockApplicationsManagementClient) ShowResource(arg0 context.Context, arg1 string, arg2 string) (generated.GenericResource, error) {
	ret := m.ctrl.Call(m, "ShowResource", arg0, arg1)
	ret0, _ := ret[0].(generated.GenericResource)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) ShowResource(arg0, arg1, arg2 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "ShowResource", reflect.TypeOf((*MockApplicationsManagementClient)(nil).ShowResource), arg0, arg1, arg2)
}

func (m *MockApplicationsManagementClient) DeleteResource(arg0 context.Context, arg1 string, arg2 string) (generated.GenericResourcesDeleteResponse, error) {
	ret := m.ctrl.Call(m, "DeleteResource", arg0, arg1)
	ret0, _ := ret[0].(generated.GenericResourcesDeleteResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) DeleteResource(arg0, arg1, arg2 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "DeleteResource", reflect.TypeOf((*MockApplicationsManagementClient)(nil).DeleteResource), arg0, arg1, arg2)
}

func (m *MockApplicationsManagementClient) ListApplications(arg0 context.Context) ([]v20220315privatepreview.ApplicationResource, error) {
	ret := m.ctrl.Call(m, "ListApplications", arg0)
	ret0, _ := ret[0].([]v20220315privatepreview.ApplicationResource)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) ListApplications(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "ListApplications", reflect.TypeOf((*MockApplicationsManagementClient)(nil).ListApplications), arg0)
}

func (m *MockApplicationsManagementClient) ShowApplication(arg0 context.Context, arg1 string) (v20220315privatepreview.ApplicationResource, error) {
	ret := m.ctrl.Call(m, "ShowApplication", arg0, arg1)
	ret0, _ := ret[0].(v20220315privatepreview.ApplicationResource)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) ShowApplication(arg0, arg1 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "ShowApplication", reflect.TypeOf((*MockApplicationsManagementClient)(nil).ShowApplication), arg0, arg1)
}

func (m *MockApplicationsManagementClient) DeleteApplication(arg0 context.Context, arg1 string) (v20220315privatepreview.ApplicationsDeleteResponse, error) {
	ret := m.ctrl.Call(m, "DeleteApplication", arg0, arg1)
	ret0, _ := ret[0].(v20220315privatepreview.ApplicationsDeleteResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) DeleteApplication(arg0, arg1 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "DeleteApplication", reflect.TypeOf((*MockApplicationsManagementClient)(nil).DeleteApplication), arg0, arg1)
}

func (m *MockApplicationsManagementClient) ListEnv(arg0 context.Context) (v20220315privatepreview.ApplicationsDeleteResponse, error) {
	ret := m.ctrl.Call(m, "ListEnv", arg0)
	ret0, _ := ret[0].(v20220315privatepreview.ApplicationsDeleteResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) ListEnv(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "ListEnv", reflect.TypeOf((*MockApplicationsManagementClient)(nil).ListEnv), arg0)
}

func (m *MockApplicationsManagementClient) GetEnvDetails(arg0 context.Context, arg1 string) (v20220315privatepreview.EnvironmentResource, error) {
	ret := m.ctrl.Call(m, "DeleteApplication", arg0, arg1)
	ret0, _ := ret[0].(v20220315privatepreview.EnvironmentResource)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockApplicationsManagementRecorder) GetEnvDetails(arg0, arg1 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "DeleteApplication", reflect.TypeOf((*MockApplicationsManagementClient)(nil).DeleteApplication), arg0, arg1)
}

func (m *MockApplicationsManagementClient) DeleteEnv(arg0 context.Context) (error) {
	ret := m.ctrl.Call(m, "DeleteEnv", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockApplicationsManagementRecorder) DeleteEnv(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCall(mr.mock, "DeleteEnv", reflect.TypeOf((*MockApplicationsManagementClient)(nil).ListEnv), arg0)
}