// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/radius-project/radius/pkg/ucp/store (interfaces: StorageClient)

// Package store is a generated GoMock package.
package store

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockStorageClient is a mock of StorageClient interface.
type MockStorageClient struct {
	ctrl     *gomock.Controller
	recorder *MockStorageClientMockRecorder
}

// MockStorageClientMockRecorder is the mock recorder for MockStorageClient.
type MockStorageClientMockRecorder struct {
	mock *MockStorageClient
}

// NewMockStorageClient creates a new mock instance.
func NewMockStorageClient(ctrl *gomock.Controller) *MockStorageClient {
	mock := &MockStorageClient{ctrl: ctrl}
	mock.recorder = &MockStorageClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStorageClient) EXPECT() *MockStorageClientMockRecorder {
	return m.recorder
}

// Delete mocks base method.
func (m *MockStorageClient) Delete(arg0 context.Context, arg1 string, arg2 ...DeleteOptions) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Delete", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Delete indicates an expected call of Delete.
func (mr *MockStorageClientMockRecorder) Delete(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockStorageClient)(nil).Delete), varargs...)
}

// Get mocks base method.
func (m *MockStorageClient) Get(arg0 context.Context, arg1 string, arg2 ...GetOptions) (*Object, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Get", varargs...)
	ret0, _ := ret[0].(*Object)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockStorageClientMockRecorder) Get(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockStorageClient)(nil).Get), varargs...)
}

// Query mocks base method.
func (m *MockStorageClient) Query(arg0 context.Context, arg1 Query, arg2 ...QueryOptions) (*ObjectQueryResult, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Query", varargs...)
	ret0, _ := ret[0].(*ObjectQueryResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Query indicates an expected call of Query.
func (mr *MockStorageClientMockRecorder) Query(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Query", reflect.TypeOf((*MockStorageClient)(nil).Query), varargs...)
}

// Save mocks base method.
func (m *MockStorageClient) Save(arg0 context.Context, arg1 *Object, arg2 ...SaveOptions) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Save", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Save indicates an expected call of Save.
func (mr *MockStorageClientMockRecorder) Save(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Save", reflect.TypeOf((*MockStorageClient)(nil).Save), varargs...)
}
