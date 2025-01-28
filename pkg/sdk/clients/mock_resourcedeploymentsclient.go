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

package clients

import (
	"context"
	"net/http"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/google/uuid"
)

// This file contains mocks for the ResourceDeploymentsClient interface.

func NewMockResourceDeploymentsClient() *MockResourceDeploymentsClient {
	return &MockResourceDeploymentsClient{
		resourceDeployments: map[string]*ClientCreateOrUpdateResponse{},
		operations:          map[string]*OperationState{},

		lock: &sync.Mutex{},
	}
}

var _ ResourceDeploymentsClient = (*MockResourceDeploymentsClient)(nil)

type MockResourceDeploymentsClient struct {
	resourceDeployments map[string]*ClientCreateOrUpdateResponse
	operations          map[string]*OperationState

	lock *sync.Mutex
}

func (rdc *MockResourceDeploymentsClient) CompleteOperation(operationID string, update func(state *OperationState)) {
	rdc.lock.Lock()
	defer rdc.lock.Unlock()

	state, ok := rdc.operations[operationID]
	if !ok {
		panic("operation not found: " + operationID)
	}

	if update != nil {
		update(state)
	}

	state.Complete = true
}

func (rdc *MockResourceDeploymentsClient) CreateOrUpdate(ctx context.Context, parameters Deployment, resourceID, apiVersion string) (Poller[ClientCreateOrUpdateResponse], error) {
	rdc.lock.Lock()
	defer rdc.lock.Unlock()

	value := ClientCreateOrUpdateResponse{
		DeploymentExtended: armresources.DeploymentExtended{
			ID:         &resourceID,
			Properties: &armresources.DeploymentPropertiesExtended{},
		},
	}
	state := &OperationState{
		Kind:       http.MethodPut,
		ResourceID: resourceID,
		Value:      value,
	}

	operationID := uuid.New().String()
	rdc.resourceDeployments[resourceID] = &value
	rdc.operations[operationID] = state

	return &MockResourceDeploymentsClientPoller[ClientCreateOrUpdateResponse]{
		mock:        rdc,
		operationID: operationID,
		state:       state,
	}, nil
}

func (rdc *MockResourceDeploymentsClient) ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[ClientCreateOrUpdateResponse], error) {
	rdc.lock.Lock()
	defer rdc.lock.Unlock()

	state, ok := rdc.operations[resumeToken]
	if !ok {
		return nil, &azcore.ResponseError{StatusCode: http.StatusNotFound}
	}

	return &MockResourceDeploymentsClientPoller[ClientCreateOrUpdateResponse]{
		operationID: resumeToken,
		mock:        rdc,
		state:       state,
	}, nil
}

func (rdc *MockResourceDeploymentsClient) Delete(ctx context.Context, resourceID, apiVersion string) (Poller[ClientDeleteResponse], error) {
	rdc.lock.Lock()
	defer rdc.lock.Unlock()

	state := &OperationState{
		Kind:       http.MethodDelete,
		ResourceID: resourceID,
		Value: ClientDeleteResponse{
			DeploymentExtended: armresources.DeploymentExtended{
				ID:         &resourceID,
				Properties: &armresources.DeploymentPropertiesExtended{},
			},
		},
	}

	operationID := uuid.New().String()
	rdc.operations[operationID] = state

	return &MockResourceDeploymentsClientPoller[ClientDeleteResponse]{
		mock:        rdc,
		operationID: operationID,
		state:       state,
	}, nil
}

func (rdc *MockResourceDeploymentsClient) ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[ClientDeleteResponse], error) {
	rdc.lock.Lock()
	defer rdc.lock.Unlock()

	state, ok := rdc.operations[resumeToken]
	if !ok {
		return nil, &azcore.ResponseError{StatusCode: http.StatusNotFound}
	}

	return &MockResourceDeploymentsClientPoller[ClientDeleteResponse]{
		operationID: resumeToken,
		mock:        rdc,
		state:       state,
	}, nil
}

func (rdc *MockResourceDeploymentsClient) GetResource(resourceID string) (*ClientCreateOrUpdateResponse, bool) {
	resource, ok := rdc.resourceDeployments[resourceID]

	return resource, ok
}

type MockResourceDeploymentsClientPoller[T any] struct {
	operationID string
	mock        *MockResourceDeploymentsClient
	state       *OperationState
}

var _ Poller[ClientCreateOrUpdateResponse] = (*MockResourceDeploymentsClientPoller[ClientCreateOrUpdateResponse])(nil)

func (mp *MockResourceDeploymentsClientPoller[T]) Done() bool {
	mp.mock.lock.Lock()
	defer mp.mock.lock.Unlock()

	return mp.state.Complete // Status updates are delivered via the Poll function.
}

func (mp *MockResourceDeploymentsClientPoller[T]) Poll(ctx context.Context) (*http.Response, error) {
	mp.mock.lock.Lock()
	defer mp.mock.lock.Unlock()

	// NOTE: this is ok because our code ignores the actual result.
	mp.state = mp.mock.operations[mp.operationID]
	return nil, nil
}

func (mp *MockResourceDeploymentsClientPoller[T]) Result(ctx context.Context) (T, error) {
	mp.mock.lock.Lock()
	defer mp.mock.lock.Unlock()

	if mp.state.Complete && mp.state.Err != nil {
		return mp.state.Value.(T), mp.state.Err
	} else if mp.state.Complete {
		return mp.state.Value.(T), nil
	}

	panic("operation not done")
}

func (mp *MockResourceDeploymentsClientPoller[T]) ResumeToken() (string, error) {
	return mp.operationID, nil
}

func (mp *MockResourceDeploymentsClientPoller[T]) PollUntilDone(ctx context.Context, options *PollUntilDoneOptions) (T, error) {
	return mp.Result(ctx)
}
