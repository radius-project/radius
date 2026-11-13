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

package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	recipesutil "github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"uuid"
)

const (
	delegationTargetID = "/planes/radius/local/resourceGroups/rg/providers/Test.Compute/containers/target"
	delegationSourceID = "/planes/radius/local/resourceGroups/rg/providers/Test.Compute/routes/route"
	delegationEnvID    = "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/environments/env"
	delegationAppID    = "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/applications/app"
	delegationVersion  = "2025-08-01-preview"
	delegationNativeID = "/planes/kubernetes/local/namespaces/test-ns/providers/core/Service/test-svc"
)

type delegationTransport func(*http.Request) (*http.Response, error)

func (f delegationTransport) Do(request *http.Request) (*http.Response, error) { return f(request) }

type delegationTestEngine struct {
	engine.Engine
	calls   atomic.Int32
	deletes atomic.Int32
	run     func(engine.ExecuteOptions) (*recipes.RecipeOutput, error)
	recover func(engine.ExecuteOptions) (*recipes.RecipeOutput, error)
}

func (e *delegationTestEngine) Execute(_ context.Context, options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
	if options.Recipe.RecoverExecution {
		if e.recover != nil {
			return e.recover(options)
		}
		return nil, fmt.Errorf("automatic replay is unsafe without a native completion proof")
	}
	e.calls.Add(1)
	return e.run(options)
}

func (e *delegationTestEngine) Delete(_ context.Context, options engine.DeleteOptions) error {
	if options.Recipe.ResourceID != delegationTargetID {
		return fmt.Errorf("a source must never own native deletion")
	}
	e.deletes.Add(1)
	return nil
}

type delegationFixture struct {
	runtime *delegationRuntime
	store   *inmemory.Client
	engine  *delegationTestEngine
	handler *encryption.SensitiveDataHandler
	target  *datamodel.DynamicResource
	member  datamodel.DynamicResource
	request *ctrl.Request
	schema  map[string]any
}

func newDelegationFixture(t *testing.T) *delegationFixture {
	t.Helper()
	ctx := t.Context()
	sourceSchema := map[string]any{"type": "object", "properties": map[string]any{
		"application": map[string]any{"type": "string"},
		"environment": map[string]any{"type": "string"},
		"target":      map[string]any{"type": "string", schema.ResourceReferenceAnnotation: "Test.Compute/containers"},
		"path":        map[string]any{"type": "string"},
		"password":    map[string]any{"type": "string", "x-radius-sensitive": true},
		"endpoint":    map[string]any{"type": "string", "readOnly": true},
		"status":      map[string]any{"type": "object", "readOnly": true},
	}}
	targetSchema := map[string]any{"type": "object", "properties": map[string]any{
		"application": map[string]any{"type": "string"},
		"environment": map[string]any{"type": "string"},
		"image":       map[string]any{"type": "string"},
		"endpoint":    map[string]any{"type": "string", "readOnly": true},
		"status":      map[string]any{"type": "object", "readOnly": true},
	}}
	ucp, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		Transport: delegationTransport(func(request *http.Request) (*http.Response, error) {
			resourceSchema := sourceSchema
			if strings.Contains(strings.ToLower(request.URL.Path), "/containers/") {
				resourceSchema = targetSchema
			}
			data, err := json.Marshal(map[string]any{"properties": map[string]any{"schema": resourceSchema}})
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(string(data))), Request: request}, nil
		}),
	})
	require.NoError(t, err)
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	keyData, err := json.Marshal(encryption.KeyStore{CurrentVersion: 1, Keys: map[string]encryption.KeyData{
		"1": {Key: base64.StdEncoding.EncodeToString(key), Version: 1},
	}})
	require.NoError(t, err)
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: encryption.DefaultEncryptionKeySecretName, Namespace: encryption.RadiusNamespace},
		Data:       map[string][]byte{encryption.DefaultEncryptionKeySecretKey: keyData},
	}).Build()
	handler, err := encryption.NewSensitiveDataHandlerFromProvider(ctx, encryption.NewKubernetesKeyProvider(kube, nil))
	require.NoError(t, err)
	loader := configloader.NewMockConfigurationLoader(gomock.NewController(t))
	loader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, metadata *recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, error) {
		if strings.EqualFold(metadata.ResourceID, delegationTargetID) {
			return &recipes.EnvironmentDefinition{Driver: "bicep", AcceptsDelegationFrom: []string{"Test.Compute/routes"}}, nil
		}
		return &recipes.EnvironmentDefinition{Driver: "delegated", DelegatesTo: "Test.Compute/containers"}, nil
	}).AnyTimes()
	loader.EXPECT().LoadConfiguration(gomock.Any(), gomock.Any()).Return(&recipes.Configuration{}, nil).AnyTimes()
	store := inmemory.NewClient()
	eng := &delegationTestEngine{}
	eng.run = func(options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		result := &recipes.RecipeOutput{Resources: []string{delegationNativeID}, Values: map[string]any{"endpoint": "target-endpoint"}}
		for _, incoming := range options.Recipe.IncomingReferences {
			result.DelegatedOutputs = append(result.DelegatedOutputs, recipes.DelegatedOutput{
				ID: strings.ToUpper(incoming.ID), Revision: incoming.Revision, Values: map[string]any{"endpoint": "http://target" + incoming.Properties["path"].(string)},
			})
		}
		return result, nil
	}
	request := &ctrl.Request{ResourceID: delegationTargetID, APIVersion: delegationVersion, OperationID: uuid.New(),
		OperationType: v1.OperationType{Type: "Test.Compute/containers", Method: v1.OperationPut}.String()}
	target := &datamodel.DynamicResource{Properties: map[string]any{"application": delegationAppID, "environment": delegationEnvID, "image": "saved-image"}}
	target.ID, target.Type, target.Name = delegationTargetID, "Test.Compute/containers", "target"
	target.TenantID, target.UpdatedAPIVersion = "tenant", delegationVersion
	target.DelegationOperationID = request.OperationID.String()
	target.SetProvisioningState(v1.ProvisioningStateAccepted)
	target.DesiredState, err = datamodel.NewDesiredState(ctx, recipes.IncomingReference{ID: target.ID, Type: target.Type, APIVersion: delegationVersion,
		Revision: request.OperationID.String(), Properties: target.Properties}, targetSchema, handler)
	require.NoError(t, err)
	member := datamodel.DynamicResource{Properties: map[string]any{}, DelegationTargetID: target.ID}
	member.ID, member.Type, member.Name = delegationSourceID, "Test.Compute/routes", "route"
	member.TenantID, member.UpdatedAPIVersion = target.TenantID, delegationVersion
	member.DesiredState, err = datamodel.NewDesiredState(ctx, recipes.IncomingReference{ID: member.ID, Type: member.Type, APIVersion: delegationVersion,
		Revision: "member-revision", Properties: map[string]any{"application": delegationAppID, "environment": delegationEnvID,
			"target": delegationTargetID, "path": "/initial", "password": "private-password"}}, sourceSchema, handler)
	require.NoError(t, err)
	members := []datamodel.DynamicResource{member}
	target.DelegatingDesiredStates = &members
	target.DelegationResourceTypes = []string{member.Type}
	require.NoError(t, store.Save(ctx, &database.Object{ID: delegationAppID, Data: &v1.BaseResource{}}))
	require.NoError(t, store.Save(ctx, &database.Object{ID: target.ID, Data: target}))
	return &delegationFixture{runtime: &delegationRuntime{opts: ctrl.Options{DatabaseClient: store, KubeClient: kube, UcpClient: ucp}, loader: loader, engine: eng},
		store: store, engine: eng, handler: handler, target: target, member: member, request: request, schema: sourceSchema}
}

func TestDelegationGroupedInitialAndIndependentLifecycle(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	original := f.engine.run
	f.engine.run = func(options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		require.Equal(t, delegationTargetID, options.Recipe.ResourceID)
		require.Equal(t, "saved-image", options.Recipe.Properties["image"])
		require.Len(t, options.Recipe.IncomingReferences, 1)
		object, member, err := readDynamicResource(t.Context(), f.store, delegationSourceID)
		require.NoError(t, err)
		require.Equal(t, v1.ProvisioningStateUpdating, member.ProvisioningState())
		require.Empty(t, member.Properties)
		data, err := json.Marshal(object.Data)
		require.NoError(t, err)
		require.NotContains(t, string(data), "private-password")
		require.NotContains(t, string(data), "/initial")
		return original(options)
	}
	result, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	require.EqualValues(t, 1, f.engine.calls.Load())
	_, member, err := readDynamicResource(t.Context(), f.store, delegationSourceID)
	require.NoError(t, err)
	require.Equal(t, "http://target/initial", member.Properties["endpoint"])
	require.Nil(t, member.Properties["password"])
	require.Empty(t, member.OutputResources())
	_, target, err := readDynamicResource(t.Context(), f.store, delegationTargetID)
	require.NoError(t, err)
	require.Len(t, target.OutputResources(), 1)
	require.Equal(t, delegationNativeID, target.OutputResources()[0].ID.String())
	require.Len(t, target.Status()["delegatedResources"], 1)
	require.Empty(t, target.DelegationOperationID)
	f.engine.run = original

	// A target-only update retains the saved member inputs.
	f.claim(t, target)
	result, err = f.runtime.put(t.Context(), f.request, target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	require.EqualValues(t, 2, f.engine.calls.Load())

	// A standalone member PUT changes only that snapshot and invokes only the target.
	incoming, sourceSchema, err := member.DesiredState.Materialize(t.Context(), f.handler)
	require.NoError(t, err)
	incoming.Revision, incoming.Properties["path"] = "member-update", "/updated"
	member.DesiredState, err = datamodel.NewDesiredState(t.Context(), incoming, sourceSchema, f.handler)
	require.NoError(t, err)
	require.NoError(t, f.store.Save(t.Context(), &database.Object{ID: member.ID, Data: member}))
	f.claim(t, member)
	result, err = f.runtime.put(t.Context(), f.request, member, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	_, member, err = readDynamicResource(t.Context(), f.store, delegationSourceID)
	require.NoError(t, err)
	require.Equal(t, "http://target/updated", member.Properties["endpoint"])
	require.EqualValues(t, 3, f.engine.calls.Load())

	// Target deletion with a live binding must not touch physical infrastructure.
	f.claim(t, target)
	result, err = f.runtime.delete(t.Context(), f.request, target)
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateFailed, result.ProvisioningState())
	require.Zero(t, f.engine.deletes.Load())

	f.claim(t, member)
	f.engine.run = func(options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		require.NotNil(t, options.Recipe.IncomingReferences)
		require.Empty(t, options.Recipe.IncomingReferences)
		_, _, err := readDynamicResource(t.Context(), f.store, member.ID)
		require.NoError(t, err, "source removal must wait for target recomputation")
		return original(options)
	}
	result, err = f.runtime.delete(t.Context(), f.request, member)
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	_, err = f.store.Get(t.Context(), member.ID)
	require.Error(t, err)
	require.EqualValues(t, 4, f.engine.calls.Load())
	require.Zero(t, f.engine.deletes.Load())
}

func (f *delegationFixture) claim(t *testing.T, source *datamodel.DynamicResource) {
	t.Helper()
	object, target, err := readDynamicResource(t.Context(), f.store, delegationTargetID)
	require.NoError(t, err)
	f.request.OperationID = uuid.New()
	f.request.ResourceID = source.ID
	target.DelegationOperationID = f.request.OperationID.String()
	object.Data = target
	require.NoError(t, f.store.Save(t.Context(), object, database.WithETag(object.ETag)))
}

func TestDelegationInvalidProjectionPreservesPhysicalInventory(t *testing.T) {
	t.Parallel()
	for _, values := range []map[string]any{
		{"endpoint": 42},
		{"password": "must-not-be-published"},
		{"application": "another-app"},
	} {
		t.Run(fmt.Sprint(values), func(t *testing.T) {
			t.Parallel()
			f := newDelegationFixture(t)
			f.engine.run = func(options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
				return &recipes.RecipeOutput{Resources: []string{delegationNativeID}, DelegatedOutputs: []recipes.DelegatedOutput{
					{ID: delegationSourceID, Revision: "member-revision", Values: values},
				}}, nil
			}
			result, err := f.runtime.put(t.Context(), f.request, f.target, "")
			require.NoError(t, err)
			require.Equal(t, v1.ProvisioningStateFailed, result.ProvisioningState())
			_, target, err := readDynamicResource(t.Context(), f.store, delegationTargetID)
			require.NoError(t, err)
			require.Len(t, target.OutputResources(), 1)
			require.NotContains(t, target.Status(), "delegatedResources")
			_, member, err := readDynamicResource(t.Context(), f.store, delegationSourceID)
			require.NoError(t, err)
			require.Equal(t, v1.ProvisioningStateFailed, member.ProvisioningState())
			require.Equal(t, delegationTargetID, member.DelegationTargetID)
			require.Empty(t, member.Properties)
		})
	}
}

func TestDelegationAmbiguousExecutionCannotReplay(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	f.engine.run = func(engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		return nil, fmt.Errorf("native response lost")
	}
	result, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateFailed, result.ProvisioningState())
	_, err = f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.EqualValues(t, 1, f.engine.calls.Load())
}

func TestDelegationNativeCompletionRecoveryUsesOriginalIdentityAndInputs(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	original := f.engine.run
	f.engine.run = func(options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		require.Equal(t, f.request.OperationID.String(), options.Recipe.ExecutionID)
		return nil, fmt.Errorf("native response lost")
	}
	result, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateFailed, result.ProvisioningState())
	operationID := f.request.OperationID.String()
	f.engine.recover = func(options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		require.Equal(t, operationID, options.Recipe.ExecutionID)
		require.Equal(t, "/initial", options.Recipe.IncomingReferences[0].Properties["path"])
		return original(options)
	}
	f.request.OperationID = uuid.New()
	object, target, err := readDynamicResource(t.Context(), f.store, delegationTargetID)
	require.NoError(t, err)
	target.SetProvisioningState(v1.ProvisioningStateUpdating)
	object.Data = target
	require.NoError(t, f.store.Save(t.Context(), object, database.WithETag(object.ETag)))
	result, err = f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	require.EqualValues(t, 1, f.engine.calls.Load())
}

func TestDelegationKnownPreNativeFailureReleasesClaim(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	f.engine.run = func(engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		return nil, recipes.NewRecipeError(recipes.RecipeDownloadFailed, "download unavailable", recipesutil.RecipeSetupError)
	}
	result, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateFailed, result.ProvisioningState())
	_, target, err := readDynamicResource(t.Context(), f.store, delegationTargetID)
	require.NoError(t, err)
	require.Empty(t, target.DelegationOperationID)
	require.False(t, target.DelegationExecutionStarted)
	require.NotNil(t, target.DesiredState)
}

func TestDelegationReturnedResultResumesWithoutNativeReplay(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	incoming, _, err := f.member.DesiredState.Materialize(t.Context(), f.handler)
	require.NoError(t, err)
	output, err := f.engine.run(engine.ExecuteOptions{Recipe: recipes.ResourceMetadata{IncomingReferences: []recipes.IncomingReference{incoming}}})
	require.NoError(t, err)
	output.ProviderResourceIDs = map[string]string{delegationNativeID: "provider-native-id"}
	encoded, err := json.Marshal(map[string]any{"output": output, "providerResourceIDs": output.ProviderResourceIDs})
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(encoded, &result))
	envelope := map[string]any{"value": result}
	require.NoError(t, f.handler.EncryptSensitiveFields(envelope, []string{"value"}, delegationResultContext(f.target)))
	f.target.DelegationExecutionStarted = true
	f.target.DelegationResult = envelope["value"].(map[string]any)
	require.NoError(t, f.store.Save(t.Context(), &database.Object{ID: f.target.ID, Data: f.target}))
	restored, err := restoreDelegationResult(t.Context(), f.target, f.handler)
	require.NoError(t, err)
	require.Equal(t, output.ProviderResourceIDs, restored.ProviderResourceIDs)
	resumed, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, resumed.ProvisioningState())
	require.Zero(t, f.engine.calls.Load())
	_, target, err := readDynamicResource(t.Context(), f.store, f.target.ID)
	require.NoError(t, err)
	require.Nil(t, target.DelegationResult)
}
func TestDelegationApplicationCleanupDoesNotReprovision(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	result, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	application := &v1.BaseResource{}
	application.SetProvisioningState(v1.ProvisioningStateUpdating)
	application.DeletionRequested = true
	require.NoError(t, f.store.Save(t.Context(), &database.Object{ID: delegationAppID, Data: application}))
	f.claim(t, &f.member)
	result, err = f.runtime.delete(t.Context(), f.request, &f.member)
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	require.EqualValues(t, 1, f.engine.calls.Load())
	require.Zero(t, f.engine.deletes.Load())
}

func TestDelegationTargetApplicationCleanupOwnsOnePhysicalDelete(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	_, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	application := &v1.BaseResource{}
	application.SetProvisioningState(v1.ProvisioningStateDeleting)
	require.NoError(t, f.store.Save(t.Context(), &database.Object{ID: delegationAppID, Data: application}))
	f.claim(t, f.target)
	result, err := f.runtime.delete(t.Context(), f.request, f.target)
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	require.EqualValues(t, 1, f.engine.deletes.Load())
	require.EqualValues(t, 1, f.engine.calls.Load())
	_, err = f.store.Get(t.Context(), delegationSourceID)
	require.Error(t, err)
	_, err = f.store.Get(t.Context(), delegationTargetID)
	require.Error(t, err)
}

func TestDelegationCompetingWorkersCannotRunTargetTwice(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	original := f.engine.run
	f.engine.run = func(options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		close(started)
		<-release
		return original(options)
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.runtime.put(t.Context(), f.request, f.target, "")
		done <- err
	}()
	<-started
	_, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.ErrorContains(t, err, "automatic replay is unsafe")
	close(release)
	require.NoError(t, <-done)
	require.EqualValues(t, 1, f.engine.calls.Load())
}

func TestDelegationInitialEmptyGroupIsAuthoritative(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	members := []datamodel.DynamicResource{}
	f.target.DelegatingDesiredStates = &members
	require.NoError(t, f.store.Save(t.Context(), &database.Object{ID: f.target.ID, Data: f.target}))
	original := f.engine.run
	f.engine.run = func(options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		require.NotNil(t, options.Recipe.IncomingReferences)
		require.Empty(t, options.Recipe.IncomingReferences)
		return original(options)
	}
	result, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	require.EqualValues(t, 1, f.engine.calls.Load())
}

func TestDelegationInitialRunReceivesEverySuppliedMember(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	second := f.member
	second.ID, second.Name = delegationSourceID+"-second", "route-second"
	incoming, sourceSchema, err := f.member.DesiredState.Materialize(t.Context(), f.handler)
	require.NoError(t, err)
	incoming.ID, incoming.Revision = second.ID, "second-revision"
	second.DesiredState, err = datamodel.NewDesiredState(t.Context(), incoming, sourceSchema, f.handler)
	require.NoError(t, err)
	members := []datamodel.DynamicResource{f.member, second}
	f.target.DelegatingDesiredStates = &members
	require.NoError(t, f.store.Save(t.Context(), &database.Object{ID: f.target.ID, Data: f.target}))
	original := f.engine.run
	f.engine.run = func(options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
		require.Len(t, options.Recipe.IncomingReferences, 2)
		for _, reference := range options.Recipe.IncomingReferences {
			_, member, err := readDynamicResource(t.Context(), f.store, reference.ID)
			require.NoError(t, err)
			require.Equal(t, v1.ProvisioningStateUpdating, member.ProvisioningState())
			require.Empty(t, member.OutputResources())
		}
		return original(options)
	}
	result, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	require.EqualValues(t, 1, f.engine.calls.Load())
	_, target, err := readDynamicResource(t.Context(), f.store, delegationTargetID)
	require.NoError(t, err)
	require.Len(t, target.Status()["delegatedResources"], 2)
}

func TestDelegationActiveApplicationCannotBeRecreated(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	application := &v1.BaseResource{}
	application.SetProvisioningState(v1.ProvisioningStateUpdating)
	require.NoError(t, f.store.Save(t.Context(), &database.Object{ID: delegationAppID, Data: application}))
	_, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.ErrorContains(t, err, "unambiguous application deletion state")
	require.Zero(t, f.engine.calls.Load())
	require.Zero(t, f.engine.deletes.Load())
}

func TestDelegationSourceDeletionAfterTargetGoneHasNoNativeWork(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	require.NoError(t, f.runtime.persistSuppliedMembers(t.Context(), f.target))
	require.NoError(t, f.store.Delete(t.Context(), delegationTargetID))
	result, err := f.runtime.delete(t.Context(), f.request, &f.member)
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
	require.Zero(t, f.engine.calls.Load())
	require.Zero(t, f.engine.deletes.Load())
	_, err = f.store.Get(t.Context(), delegationSourceID)
	require.Error(t, err)
}

func TestDelegationMemberPreparationFailureReleasesClaim(t *testing.T) {
	t.Parallel()
	f := newDelegationFixture(t)
	loader := configloader.NewMockConfigurationLoader(gomock.NewController(t))
	loader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, metadata *recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, error) {
		if metadata.ResourceID == delegationTargetID {
			return &recipes.EnvironmentDefinition{Driver: "bicep", AcceptsDelegationFrom: []string{"Test.Compute/routes"}}, nil
		}
		return nil, fmt.Errorf("member registration unavailable")
	}).AnyTimes()
	f.runtime.loader = loader
	_, err := f.runtime.put(t.Context(), f.request, f.target, "")
	require.ErrorContains(t, err, "member registration unavailable")
	_, target, err := readDynamicResource(t.Context(), f.store, delegationTargetID)
	require.NoError(t, err)
	require.Empty(t, target.DelegationOperationID)
	require.False(t, target.DelegationExecutionStarted)
	require.NotNil(t, target.DesiredState)
	_, member, err := readDynamicResource(t.Context(), f.store, delegationSourceID)
	require.NoError(t, err)
	require.Equal(t, delegationTargetID, member.DelegationTargetID)
	require.NotNil(t, member.DesiredState)
	require.Equal(t, v1.ProvisioningStateFailed, member.ProvisioningState())
	require.Zero(t, f.engine.calls.Load())
}
