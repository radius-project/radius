/*
Copyright 2026 The Radius Authors.

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

package frontend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	sm "github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucpmodel "github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"uuid"
)

func TestGroupedAdmissionProtectsAndValidatesDeclarations(t *testing.T) {
	t.Parallel()
	const sourceID = "/planes/radius/local/resourceGroups/test-group/providers/Test.Compute/routes/route"
	const envID = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/env"
	const appID = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/applications/app"
	for _, name := range []string{"valid", "wrong-target", "cross-scope", "cross-environment", "retarget", "manual",
		"parameters", "computed-input", "duplicate", "two-targets", "nondelegated", "chaining"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sourceSchema := map[string]any{"type": "object", "properties": map[string]any{
				"application": map[string]any{"type": "string"},
				"environment": map[string]any{"type": "string"},
				"target":      map[string]any{"type": "string", schema.ResourceReferenceAnnotation: "Applications.Test/testResources"},
				"other":       map[string]any{"type": "string", schema.ResourceReferenceAnnotation: "Applications.Test/testResources"},
				"password":    map[string]any{"type": "string", "x-radius-sensitive": true},
				"path":        map[string]any{"type": "string", "default": "/default"},
				"endpoint":    map[string]any{"type": "string", "readOnly": true},
				"recipe":      map[string]any{"type": "object"},
			}}
			ucp, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
				Transport: desiredStateTestTransport(func(request *http.Request) (*http.Response, error) {
					properties := map[string]any{"schema": sourceSchema}
					if name == "manual" && !strings.Contains(strings.ToLower(request.URL.Path), "/apiversions/") {
						properties["capabilities"] = []string{ucpmodel.CapabilityManualResourceProvisioning}
					}
					data, err := json.Marshal(map[string]any{"properties": properties})
					require.NoError(t, err)
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
						Body: io.NopCloser(strings.NewReader(string(data))), Request: request}, nil
				}),
			})
			require.NoError(t, err)
			target := &datamodel.DynamicResource{Properties: map[string]any{"application": appID, "environment": envID}}
			target.ID, target.Type, target.TenantID = testResourceID, "Applications.Test/testResources", "tenant"
			metadata := recipes.ResourceMetadata{ResourceID: target.ID, ApplicationID: appID, EnvironmentID: envID}
			declaration := datamodel.DelegatingResource{ID: sourceID, Type: "Test.Compute/routes", APIVersion: testAPIVersion,
				Properties: map[string]any{"application": appID, "environment": envID, "target": strings.ToUpper(target.ID), "password": "private-input"}}
			definition := &recipes.EnvironmentDefinition{Driver: "delegated", DelegatesTo: target.Type}
			switch name {
			case "wrong-target":
				declaration.Properties["target"] = target.ID + "-other"
			case "cross-scope":
				declaration.ID = strings.Replace(sourceID, "test-group", "different-group", 1)
			case "cross-environment":
				declaration.Properties["environment"] = envID + "-other"
			case "parameters":
				declaration.Properties["recipe"] = map[string]any{"parameters": map[string]any{"override": "invalid"}}
			case "computed-input":
				declaration.Properties["endpoint"] = "injected-output"
			case "two-targets":
				declaration.Properties["other"] = target.ID + "-other"
			case "nondelegated":
				definition.Driver = "bicep"
			case "chaining":
				definition.AcceptsDelegationFrom = []string{"Another.Type/resources"}
			}
			target.DelegatingResources = []datamodel.DelegatingResource{declaration}
			if name == "duplicate" {
				duplicate := declaration
				duplicate.ID = strings.ToUpper(duplicate.ID)
				target.DelegatingResources = append(target.DelegatingResources, duplicate)
			}
			store := inmemory.NewClient()
			if name == "retarget" {
				existing := &datamodel.DynamicResource{DelegationTargetID: target.ID + "-previous"}
				existing.ID, existing.TenantID = declaration.ID, target.TenantID
				require.NoError(t, store.Save(t.Context(), &database.Object{ID: existing.ID, Data: existing}))
			}
			loader := configloader.NewMockConfigurationLoader(gomock.NewController(t))
			loader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).Return(definition, nil).AnyTimes()
			handler := createTestHandler(t)
			response, err := admitDelegatingResources(createTestContext(t), target, nil,
				&recipes.EnvironmentDefinition{Driver: "bicep", AcceptsDelegationFrom: []string{declaration.Type}},
				metadata, loader, ucp, handler, &controller.Options{DatabaseClient: store})
			require.NoError(t, err)
			if name != "valid" {
				require.NotNil(t, response)
				require.Nil(t, target.DelegatingDesiredStates)
				return
			}
			if response != nil {
				t.Fatalf("valid admission rejected: %s", response.(*rest.BadRequestResponse).Body.Error.Message)
			}
			require.NotNil(t, target.DelegatingDesiredStates)
			require.Len(t, *target.DelegatingDesiredStates, 1)
			member := (*target.DelegatingDesiredStates)[0]
			require.Empty(t, member.Properties)
			require.Equal(t, target.ID, member.DelegationTargetID)
			materialized, _, err := member.DesiredState.Materialize(t.Context(), handler)
			require.NoError(t, err)
			require.Equal(t, "private-input", materialized.Properties["password"])
			require.Equal(t, "/default", materialized.Properties["path"])
			encoded, err := json.Marshal(target)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "private-input")
			require.NotContains(t, string(encoded), "/default")
		})
	}
}

func TestDelegationAdmissionSerializesTargetAndDelegateArrivals(t *testing.T) {
	t.Parallel()
	store := inmemory.NewClient()
	target := &datamodel.DynamicResource{DesiredState: &datamodel.DesiredState{}}
	target.ID, target.Type = testResourceID, "Applications.Test/testResources"
	object := &database.Object{ID: target.ID, Data: target}
	require.NoError(t, store.Save(t.Context(), object))
	originalETag := object.ETag
	options := &controller.Options{DatabaseClient: store}
	var wg sync.WaitGroup
	responses := make(chan rest.Response, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := v1.WithARMRequestContext(t.Context(), &v1.ARMRequestContext{OperationID: uuid.New()})
			source := &datamodel.DynamicResource{DesiredState: &datamodel.DesiredState{}, DelegationTargetID: target.ID}
			source.Type = "Test.Compute/routes"
			response, err := makeDelegationUpdateFilter()(ctx, source, nil, options)
			require.NoError(t, err)
			responses <- response
		}()
	}

	wg.Wait()
	close(responses)
	accepted := 0
	for response := range responses {
		if response == nil {
			accepted++
		}
	}
	require.Equal(t, 1, accepted)
	// A target PUT that read before a delegate's claim cannot overwrite that claim.
	target.DelegationOperationID = uuid.New().String()
	err := store.Save(t.Context(), &database.Object{ID: target.ID, Data: target}, database.WithETag(originalETag))
	require.ErrorIs(t, err, &database.ErrConcurrency{})
}

func TestFailedSourceRetryKeepsOriginalProtectedAdmission(t *testing.T) {
	t.Parallel()
	store := inmemory.NewClient()
	operationID := uuid.New().String()
	target := &datamodel.DynamicResource{DelegationOperationID: operationID, DelegationExecutionStarted: true}
	target.ID = testResourceID
	target.SetProvisioningState(v1.ProvisioningStateFailed)
	require.NoError(t, store.Save(t.Context(), &database.Object{ID: target.ID, Data: target}))
	previous := &datamodel.DynamicResource{
		DesiredState:       &datamodel.DesiredState{Revision: operationID, Payload: map[string]any{"protected": "original"}},
		DelegationTargetID: target.ID,
	}
	previous.ID, previous.Type = target.ID+"-source", "Test.Compute/routes"
	require.NoError(t, store.Save(t.Context(), &database.Object{ID: previous.ID, Data: previous}))
	incoming := &datamodel.DynamicResource{DesiredState: &datamodel.DesiredState{Revision: uuid.New().String()},
		DelegationTargetID: target.ID}
	incoming.ID, incoming.Type = previous.ID, previous.Type
	response, err := makeDelegationUpdateFilter()(createTestContext(t), incoming, previous, &controller.Options{DatabaseClient: store})
	require.NoError(t, err)
	require.Nil(t, response)
	require.Equal(t, previous.DesiredState, incoming.DesiredState)
	saved, err := store.Get(t.Context(), target.ID)
	require.NoError(t, err)
	var unchanged datamodel.DynamicResource
	require.NoError(t, saved.As(&unchanged))
	require.Equal(t, operationID, unchanged.DelegationOperationID)
	response, err = makeDelegationDeleteFilter()(createTestContext(t), previous, &controller.Options{DatabaseClient: store})
	require.NoError(t, err)
	require.NotNil(t, response)
	other := &datamodel.DynamicResource{DesiredState: &datamodel.DesiredState{}, DelegationTargetID: target.ID}
	other.ID = previous.ID + "-other"
	require.NoError(t, store.Save(t.Context(), &database.Object{ID: other.ID, Data: other}))
	response, err = makeDelegationUpdateFilter()(createTestContext(t), other, nil, &controller.Options{DatabaseClient: store})
	require.NoError(t, err)
	require.NotNil(t, response)
}

type failingDelegationSourceStore struct {
	database.Client
	sourceID string
}

func (s failingDelegationSourceStore) Save(ctx context.Context, object *database.Object, options ...database.SaveOptions) error {
	if object.ID == s.sourceID {
		return &database.ErrConcurrency{}
	}
	return s.Client.Save(ctx, object, options...)
}

func TestDelegationAdmissionFailureReleasesTargetClaim(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"source save", "queue"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			ctx := createTestContext(t)
			sourceID := v1.ARMRequestContextFromContext(ctx).ResourceID.String()
			store := inmemory.NewClient()
			target := &datamodel.DynamicResource{DesiredState: &datamodel.DesiredState{Revision: "target-input"}}
			target.ID = sourceID + "-target"
			require.NoError(t, store.Save(ctx, &database.Object{ID: target.ID, Data: target}))
			var databaseClient database.Client = store
			status := sm.NewMockStatusManager(gomock.NewController(t))
			if failure == "source save" {
				databaseClient = failingDelegationSourceStore{Client: store, sourceID: sourceID}
			} else {
				status.EXPECT().QueueAsyncOperation(gomock.Any(), gomock.Any(), gomock.Any()).Return(&database.ErrConcurrency{})
			}
			options := controller.Options{DatabaseClient: databaseClient, StatusManager: status}
			resourceOptions := controller.ResourceOptions[datamodel.DynamicResource]{
				RequestConverter: func([]byte, string) (*datamodel.DynamicResource, error) {
					return &datamodel.DynamicResource{
						Properties: map[string]any{}, DesiredState: &datamodel.DesiredState{Revision: "source-input"},
						DelegationTargetID: target.ID,
					}, nil
				},
				UpdateFilters: []controller.UpdateFilter[datamodel.DynamicResource]{makeDelegationUpdateFilter()},
			}
			operation, err := newDelegationAdmissionController(options, resourceOptions, false)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")
			_, err = operation.Run(ctx, httptest.NewRecorder(), request)
			require.ErrorIs(t, err, &database.ErrConcurrency{})
			object, err := store.Get(ctx, target.ID)
			require.NoError(t, err)
			var current datamodel.DynamicResource
			require.NoError(t, object.As(&current))
			require.Empty(t, current.DelegationOperationID)
			require.Equal(t, "target-input", current.DesiredState.Revision)
		})
	}
}
