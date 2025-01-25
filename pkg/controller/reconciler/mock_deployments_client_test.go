/*
Copyright 2024 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler

import (
	"context"
	"net/http"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcoreruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/google/uuid"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
)

// This file contains mocks for the DeploymentClient interface.

func NewMockDeploymentClient() *mockDeploymentClient {
	return &mockDeploymentClient{
		resourceDeployments: map[string]sdkclients.ClientCreateOrUpdateResponse{},
		operations:          map[string]*operationState{},

		lock: &sync.Mutex{},
	}
}

var _ DeploymentClient = (*mockDeploymentClient)(nil)

type mockDeploymentClient struct {
	resourceDeployments map[string]sdkclients.ClientCreateOrUpdateResponse
	operations          map[string]*operationState

	lock *sync.Mutex
}

func (dc *mockDeploymentClient) ResourceDeployments() ResourceDeploymentsClient {
	return &mockResourceDeploymentsClient{mock: dc}
}

func (dc *mockDeploymentClient) CompleteOperation(operationID string, update func(state *operationState)) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	state, ok := dc.operations[operationID]
	if !ok {
		panic("operation not found: " + operationID)
	}

	if update != nil {
		update(state)
	}

	state.complete = true
}

var _ ResourceDeploymentsClient = (*mockResourceDeploymentsClient)(nil)

type mockResourceDeploymentsClient struct {
	mock *mockDeploymentClient
}

func (rdc *mockResourceDeploymentsClient) CreateOrUpdate(ctx context.Context, parameters sdkclients.Deployment, resourceID, apiVersion string) (Poller[sdkclients.ClientCreateOrUpdateResponse], error) {
	rdc.mock.lock.Lock()
	defer rdc.mock.lock.Unlock()

	value := sdkclients.ClientCreateOrUpdateResponse{
		DeploymentExtended: armresources.DeploymentExtended{
			ID:         &resourceID,
			Properties: &armresources.DeploymentPropertiesExtended{},
		},
	}
	state := &operationState{
		kind:       http.MethodPut,
		resourceID: resourceID,
		value:      value,
	}

	operationID := uuid.New().String()
	rdc.mock.resourceDeployments[resourceID] = value
	rdc.mock.operations[operationID] = state

	return &mockDeploymentClientPoller[sdkclients.ClientCreateOrUpdateResponse]{
		mock:        rdc.mock,
		operationID: operationID,
		state:       state,
	}, nil
}

func (rdc *mockResourceDeploymentsClient) ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[sdkclients.ClientCreateOrUpdateResponse], error) {
	rdc.mock.lock.Lock()
	defer rdc.mock.lock.Unlock()

	state, ok := rdc.mock.operations[resumeToken]
	if !ok {
		return nil, &azcore.ResponseError{StatusCode: http.StatusNotFound}
	}

	return &mockDeploymentClientPoller[sdkclients.ClientCreateOrUpdateResponse]{
		operationID: resumeToken,
		mock:        rdc.mock,
		state:       state,
	}, nil
}

func (rdc *mockResourceDeploymentsClient) Delete(ctx context.Context, resourceID, apiVersion string) (Poller[sdkclients.ClientDeleteResponse], error) {
	rdc.mock.lock.Lock()
	defer rdc.mock.lock.Unlock()

	state := &operationState{
		kind:       http.MethodDelete,
		resourceID: resourceID,
		value: sdkclients.ClientDeleteResponse{
			DeploymentExtended: armresources.DeploymentExtended{
				ID:         &resourceID,
				Properties: &armresources.DeploymentPropertiesExtended{},
			},
		},
	}

	operationID := uuid.New().String()
	rdc.mock.operations[operationID] = state

	return &mockDeploymentClientPoller[sdkclients.ClientDeleteResponse]{
		mock:        rdc.mock,
		operationID: operationID,
		state:       state,
	}, nil
}

func (rdc *mockResourceDeploymentsClient) ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[sdkclients.ClientDeleteResponse], error) {
	rdc.mock.lock.Lock()
	defer rdc.mock.lock.Unlock()

	state, ok := rdc.mock.operations[resumeToken]
	if !ok {
		return nil, &azcore.ResponseError{StatusCode: http.StatusNotFound}
	}

	return &mockDeploymentClientPoller[sdkclients.ClientDeleteResponse]{
		operationID: resumeToken,
		mock:        rdc.mock,
		state:       state,
	}, nil
}

var _ Poller[sdkclients.ClientCreateOrUpdateResponse] = (*azcoreruntime.Poller[sdkclients.ClientCreateOrUpdateResponse])(nil)

type mockDeploymentClientPoller[T any] struct {
	operationID string
	mock        *mockDeploymentClient
	state       *operationState
}

func (mp *mockDeploymentClientPoller[T]) Done() bool {
	mp.mock.lock.Lock()
	defer mp.mock.lock.Unlock()

	return mp.state.complete // Status updates are delivered via the Poll function.
}

func (mp *mockDeploymentClientPoller[T]) Poll(ctx context.Context) (*http.Response, error) {
	mp.mock.lock.Lock()
	defer mp.mock.lock.Unlock()

	// NOTE: this is ok because our code ignores the actual result.
	mp.state = mp.mock.operations[mp.operationID]
	return nil, nil
}

func (mp *mockDeploymentClientPoller[T]) Result(ctx context.Context) (T, error) {
	mp.mock.lock.Lock()
	defer mp.mock.lock.Unlock()

	if mp.state.complete && mp.state.err != nil {
		return mp.state.value.(T), mp.state.err
	} else if mp.state.complete {
		return mp.state.value.(T), nil
	}

	panic("operation not done")
}

func (mp *mockDeploymentClientPoller[T]) ResumeToken() (string, error) {
	return mp.operationID, nil
}
