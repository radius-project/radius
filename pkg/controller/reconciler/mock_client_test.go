/*
Copyright 2023.

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
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// This file contains mocks for the RadiusClient interface.

func NewMockRadiusClient() *mockRadiusClient {
	return &mockRadiusClient{
		applications: map[string]corerpv20231001preview.ApplicationResource{},
		containers:   map[string]corerpv20231001preview.ContainerResource{},
		environments: map[string]corerpv20231001preview.EnvironmentResource{},
		groups:       map[string]ucpv20231001preview.ResourceGroupResource{},
		resources:    map[string]generated.GenericResource{},
		operations:   map[string]*operationState{},

		lock: &sync.Mutex{},
	}
}

var _ RadiusClient = (*mockRadiusClient)(nil)

type mockRadiusClient struct {
	applications map[string]corerpv20231001preview.ApplicationResource
	containers   map[string]corerpv20231001preview.ContainerResource
	environments map[string]corerpv20231001preview.EnvironmentResource
	groups       map[string]ucpv20231001preview.ResourceGroupResource
	resources    map[string]generated.GenericResource
	operations   map[string]*operationState

	lock *sync.Mutex
}

type operationState struct {
	complete bool
	value    any
	// Ideally we'd use azcore.ResponseError here, but it's tricky to set up in tests.
	err        error
	resourceID string
	kind       string
}

func (rc *mockRadiusClient) Update(exec func()) {
	rc.lock.Lock()
	defer rc.lock.Unlock()

	exec()
}

func (rc *mockRadiusClient) Applications(scope string) ApplicationClient {
	return &mockApplicationClient{mock: rc, scope: scope}
}

func (rc *mockRadiusClient) Containers(scope string) ContainerClient {
	return &mockContainerClient{mock: rc, scope: scope}
}

func (rc *mockRadiusClient) Environments(scope string) EnvironmentClient {
	return &mockEnvironmentClient{mock: rc, scope: scope}
}

func (rc *mockRadiusClient) Groups(scope string) ResourceGroupClient {
	return &mockResourceGroupClient{mock: rc, scope: scope}
}

func (rc *mockRadiusClient) Resources(scope string, resourceType string) ResourceClient {
	return &mockResourceClient{mock: rc, scope: scope, resourceType: resourceType}
}

func (rc *mockRadiusClient) CompleteOperation(operationID string, update func(state *operationState)) {
	rc.lock.Lock()
	defer rc.lock.Unlock()

	state, ok := rc.operations[operationID]
	if !ok {
		panic("operation not found: " + operationID)
	}

	if update != nil {
		update(state)
	}

	state.complete = true

	if state.kind == http.MethodDelete {
		delete(rc.environments, state.resourceID)
		delete(rc.applications, state.resourceID)
		delete(rc.containers, state.resourceID)
		delete(rc.groups, state.resourceID)
		delete(rc.resources, state.resourceID)
	}
}

var _ ApplicationClient = (*mockApplicationClient)(nil)

type mockApplicationClient struct {
	mock  *mockRadiusClient
	scope string
}

func (ac *mockApplicationClient) id(applicationName string) string {
	return ac.scope + "/providers/Applications.Core/applications/" + applicationName
}

func (ac *mockApplicationClient) CreateOrUpdate(ctx context.Context, applicationName string, resource corerpv20231001preview.ApplicationResource, options *corerpv20231001preview.ApplicationsClientCreateOrUpdateOptions) (corerpv20231001preview.ApplicationsClientCreateOrUpdateResponse, error) {
	id := ac.id(applicationName)

	ac.mock.lock.Lock()
	defer ac.mock.lock.Unlock()

	ac.mock.applications[id] = resource
	return corerpv20231001preview.ApplicationsClientCreateOrUpdateResponse{ApplicationResource: resource}, nil
}

func (ac *mockApplicationClient) Delete(ctx context.Context, applicationName string, options *corerpv20231001preview.ApplicationsClientDeleteOptions) (corerpv20231001preview.ApplicationsClientDeleteResponse, error) {
	id := ac.id(applicationName)

	ac.mock.lock.Lock()
	defer ac.mock.lock.Unlock()

	delete(ac.mock.applications, id)
	return corerpv20231001preview.ApplicationsClientDeleteResponse{}, nil
}

func (ac *mockApplicationClient) Get(ctx context.Context, applicationName string, options *corerpv20231001preview.ApplicationsClientGetOptions) (corerpv20231001preview.ApplicationsClientGetResponse, error) {
	id := ac.id(applicationName)

	ac.mock.lock.Lock()
	defer ac.mock.lock.Unlock()

	application, ok := ac.mock.applications[id]
	if !ok {
		err := &azcore.ResponseError{ErrorCode: v1.CodeNotFound, StatusCode: http.StatusNotFound}
		return corerpv20231001preview.ApplicationsClientGetResponse{}, err
	}

	return corerpv20231001preview.ApplicationsClientGetResponse{ApplicationResource: application}, nil
}

var _ ContainerClient = (*mockContainerClient)(nil)

type mockContainerClient struct {
	mock  *mockRadiusClient
	scope string
}

func (cc *mockContainerClient) id(containerName string) string {
	return cc.scope + "/providers/Applications.Core/containers/" + containerName
}

func (cc *mockContainerClient) BeginCreateOrUpdate(ctx context.Context, containerName string, resource corerpv20231001preview.ContainerResource, options *corerpv20231001preview.ContainersClientBeginCreateOrUpdateOptions) (Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse], error) {
	id := cc.id(containerName)

	cc.mock.lock.Lock()
	defer cc.mock.lock.Unlock()

	value := corerpv20231001preview.ContainersClientCreateOrUpdateResponse{ContainerResource: resource}
	state := &operationState{kind: http.MethodPut, value: value, resourceID: id}

	operationID := uuid.New().String()
	cc.mock.containers[id] = resource
	cc.mock.operations[operationID] = state

	return &mockPoller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse]{mock: cc.mock, operationID: operationID, state: state}, nil
}

func (cc *mockContainerClient) BeginDelete(ctx context.Context, containerName string, options *corerpv20231001preview.ContainersClientBeginDeleteOptions) (Poller[corerpv20231001preview.ContainersClientDeleteResponse], error) {
	id := cc.id(containerName)

	cc.mock.lock.Lock()
	defer cc.mock.lock.Unlock()

	value := corerpv20231001preview.ContainersClientDeleteResponse{}
	state := &operationState{kind: http.MethodDelete, value: value, resourceID: id}

	operationID := uuid.New().String()
	cc.mock.operations[operationID] = state

	return &mockPoller[corerpv20231001preview.ContainersClientDeleteResponse]{mock: cc.mock, operationID: operationID, state: state}, nil
}

func (cc *mockContainerClient) ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse], error) {
	cc.mock.lock.Lock()
	defer cc.mock.lock.Unlock()

	state, ok := cc.mock.operations[resumeToken]
	if !ok {
		panic("operation not found: " + resumeToken)
	}

	return &mockPoller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse]{mock: cc.mock, operationID: resumeToken, state: state}, nil
}

func (cc *mockContainerClient) ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[corerpv20231001preview.ContainersClientDeleteResponse], error) {
	cc.mock.lock.Lock()
	defer cc.mock.lock.Unlock()

	state, ok := cc.mock.operations[resumeToken]
	if !ok {
		panic("operation not found: " + resumeToken)
	}

	return &mockPoller[corerpv20231001preview.ContainersClientDeleteResponse]{mock: cc.mock, operationID: resumeToken, state: state}, nil
}

func (cc *mockContainerClient) Get(ctx context.Context, containerName string, options *corerpv20231001preview.ContainersClientGetOptions) (corerpv20231001preview.ContainersClientGetResponse, error) {
	id := cc.id(containerName)

	cc.mock.lock.Lock()
	defer cc.mock.lock.Unlock()

	container, ok := cc.mock.containers[id]
	if !ok {
		err := &azcore.ResponseError{ErrorCode: v1.CodeNotFound, StatusCode: http.StatusNotFound}
		return corerpv20231001preview.ContainersClientGetResponse{}, err
	}

	return corerpv20231001preview.ContainersClientGetResponse{ContainerResource: container}, nil
}

var _ EnvironmentClient = (*mockEnvironmentClient)(nil)

type mockEnvironmentClient struct {
	mock  *mockRadiusClient
	scope string
}

func (ec *mockEnvironmentClient) List(ctx context.Context, options *corerpv20231001preview.EnvironmentsClientListByScopeOptions) (corerpv20231001preview.EnvironmentsClientListByScopeResponse, error) {
	ec.mock.lock.Lock()
	defer ec.mock.lock.Unlock()

	environments := []*corerpv20231001preview.EnvironmentResource{}
	for _, env := range ec.mock.environments {

		if strings.HasPrefix(strings.ToLower(*env.ID), strings.ToLower(ec.scope)+"/") {
			copy := env
			environments = append(environments, &copy)
		}
	}

	return corerpv20231001preview.EnvironmentsClientListByScopeResponse{EnvironmentResourceListResult: corerpv20231001preview.EnvironmentResourceListResult{Value: environments}}, nil
}

var _ ResourceGroupClient = (*mockResourceGroupClient)(nil)

type mockResourceGroupClient struct {
	mock  *mockRadiusClient
	scope string
}

func (rgc *mockResourceGroupClient) CreateOrUpdate(ctx context.Context, resourceGroupName string, resource ucpv20231001preview.ResourceGroupResource, options *ucpv20231001preview.ResourceGroupsClientCreateOrUpdateOptions) (ucpv20231001preview.ResourceGroupsClientCreateOrUpdateResponse, error) {
	rgc.mock.lock.Lock()
	defer rgc.mock.lock.Unlock()

	id := rgc.scope + "/resourceGroups/" + resourceGroupName

	rgc.mock.groups[id] = resource
	return ucpv20231001preview.ResourceGroupsClientCreateOrUpdateResponse{ResourceGroupResource: resource}, nil
}

func (rgc *mockResourceGroupClient) Get(ctx context.Context, resourceGroupName string, options *ucpv20231001preview.ResourceGroupsClientGetOptions) (ucpv20231001preview.ResourceGroupsClientGetResponse, error) {
	rgc.mock.lock.Lock()
	defer rgc.mock.lock.Unlock()

	id := rgc.scope + "/resourceGroups/" + resourceGroupName

	group, ok := rgc.mock.groups[id]
	if !ok {
		err := &azcore.ResponseError{ErrorCode: v1.CodeNotFound, StatusCode: http.StatusNotFound}
		return ucpv20231001preview.ResourceGroupsClientGetResponse{}, err
	}

	return ucpv20231001preview.ResourceGroupsClientGetResponse{ResourceGroupResource: group}, nil
}

var _ ResourceClient = (*mockResourceClient)(nil)

type mockResourceClient struct {
	mock         *mockRadiusClient
	scope        string
	resourceType string
}

func (rc *mockResourceClient) id(resourceName string) string {
	return rc.scope + "/providers/" + rc.resourceType + "/" + resourceName
}

func (rc *mockResourceClient) BeginCreateOrUpdate(ctx context.Context, resourceName string, resource generated.GenericResource, options *generated.GenericResourcesClientBeginCreateOrUpdateOptions) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], error) {
	id := rc.id(resourceName)

	rc.mock.lock.Lock()
	defer rc.mock.lock.Unlock()

	value := generated.GenericResourcesClientCreateOrUpdateResponse{GenericResource: resource}
	state := &operationState{kind: http.MethodPut, value: value, resourceID: id}

	operationID := uuid.New().String()
	rc.mock.resources[id] = resource
	rc.mock.operations[operationID] = state

	return &mockPoller[generated.GenericResourcesClientCreateOrUpdateResponse]{mock: rc.mock, operationID: operationID, state: state}, nil
}

func (rc *mockResourceClient) BeginDelete(ctx context.Context, resourceName string, options *generated.GenericResourcesClientBeginDeleteOptions) (Poller[generated.GenericResourcesClientDeleteResponse], error) {
	id := rc.id(resourceName)

	rc.mock.lock.Lock()
	defer rc.mock.lock.Unlock()

	value := generated.GenericResourcesClientDeleteResponse{}
	state := &operationState{kind: http.MethodDelete, value: value, resourceID: id}

	operationID := uuid.New().String()
	rc.mock.operations[operationID] = state

	return &mockPoller[generated.GenericResourcesClientDeleteResponse]{mock: rc.mock, operationID: operationID, state: state}, nil
}

func (rc *mockResourceClient) ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], error) {
	rc.mock.lock.Lock()
	defer rc.mock.lock.Unlock()

	state, ok := rc.mock.operations[resumeToken]
	if !ok {
		panic("operation not found: " + resumeToken)
	}

	return &mockPoller[generated.GenericResourcesClientCreateOrUpdateResponse]{mock: rc.mock, operationID: resumeToken, state: state}, nil
}

func (rc *mockResourceClient) ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[generated.GenericResourcesClientDeleteResponse], error) {
	rc.mock.lock.Lock()
	defer rc.mock.lock.Unlock()

	state, ok := rc.mock.operations[resumeToken]
	if !ok {
		panic("operation not found: " + resumeToken)
	}

	return &mockPoller[generated.GenericResourcesClientDeleteResponse]{mock: rc.mock, operationID: resumeToken, state: state}, nil
}

func (rc *mockResourceClient) Get(ctx context.Context, resourceName string) (generated.GenericResourcesClientGetResponse, error) {
	id := rc.id(resourceName)

	rc.mock.lock.Lock()
	defer rc.mock.lock.Unlock()

	resource, ok := rc.mock.resources[id]
	if !ok {
		err := &azcore.ResponseError{ErrorCode: v1.CodeNotFound, StatusCode: http.StatusNotFound}
		return generated.GenericResourcesClientGetResponse{}, err
	}

	return generated.GenericResourcesClientGetResponse{GenericResource: resource}, nil
}

func (rc *mockResourceClient) ListSecrets(ctx context.Context, resourceName string) (generated.GenericResourcesClientListSecretsResponse, error) {
	id := rc.id(resourceName)

	rc.mock.lock.Lock()
	defer rc.mock.lock.Unlock()

	resource, ok := rc.mock.resources[id]
	if !ok {
		err := &azcore.ResponseError{ErrorCode: v1.CodeNotFound, StatusCode: http.StatusNotFound}
		return generated.GenericResourcesClientListSecretsResponse{}, err
	}

	obj, ok := resource.Properties["secrets"]
	if !ok {
		err := &azcore.ResponseError{ErrorCode: v1.CodeNotFound, StatusCode: http.StatusNotFound}
		return generated.GenericResourcesClientListSecretsResponse{}, err
	}

	data := obj.(map[string]string)
	secrets := map[string]*string{}
	for k, v := range data {
		secrets[k] = to.Ptr(v)
	}

	return generated.GenericResourcesClientListSecretsResponse{Value: secrets}, nil
}

var _ Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse] = (*mockPoller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse])(nil)

type mockPoller[T any] struct {
	operationID string
	mock        *mockRadiusClient
	state       *operationState
}

func (mp *mockPoller[T]) Done() bool {
	mp.mock.lock.Lock()
	defer mp.mock.lock.Unlock()

	return mp.state.complete // Status updates are delivered via the Poll function.
}

func (mp *mockPoller[T]) Poll(ctx context.Context) (*http.Response, error) {
	mp.mock.lock.Lock()
	defer mp.mock.lock.Unlock()

	// NOTE: this is ok because our code ignores the actual result.
	mp.state = mp.mock.operations[mp.operationID]
	return nil, nil
}

func (mp *mockPoller[T]) Result(ctx context.Context) (T, error) {
	mp.mock.lock.Lock()
	defer mp.mock.lock.Unlock()

	if mp.state.complete && mp.state.err != nil {
		return mp.state.value.(T), mp.state.err
	} else if mp.state.complete {
		return mp.state.value.(T), nil
	}

	panic("operation not done")
}

func (mp *mockPoller[T]) ResumeToken() (string, error) {
	return mp.operationID, nil
}
