/*
Copyright 2023 The Radius Authors.

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

package secret

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	genfake "github.com/radius-project/radius/pkg/cli/clients_new/generated/fake"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

const testOwnerID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/kafkas/kafka"

func Test_ManagedSecretName(t *testing.T) {
	ownerID, err := resources.ParseResource(testOwnerID)
	require.NoError(t, err)

	name := ManagedSecretName(ownerID)

	// The name is <ownerName>-<shortHash>-secrets: readable prefix, disambiguating hash, reserved suffix.
	require.True(t, strings.HasPrefix(name, "kafka-"), "name should start with the owner name: %q", name)
	require.True(t, strings.HasSuffix(name, managedSecretNameSuffix), "name should end with the reserved suffix: %q", name)

	// Deterministic: the same owner ID always yields the same managed secret name (Materialize/Delete rely
	// on this).
	require.Equal(t, name, ManagedSecretName(ownerID))

	// Uniqueness: a different resource type with the SAME name in the SAME resource group must not collide.
	other, err := resources.ParseResource("/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/queues/kafka")
	require.NoError(t, err)
	require.NotEqual(t, name, ManagedSecretName(other), "same name but different type must not collide")
}

// rfc1123Label matches a valid Kubernetes object name (lowercase alphanumeric and '-', start/end
// alphanumeric, 1-63 chars).
var rfc1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func Test_ManagedSecretName_ValidKubernetesName(t *testing.T) {
	cases := map[string]string{
		"simple lowercase":      "/planes/radius/local/resourceGroups/rg/providers/Applications.Test/kafkas/kafka",
		"uppercase in name":     "/planes/radius/local/resourceGroups/rg/providers/Applications.Test/kafkas/MyKafka",
		"underscores/dots":      "/planes/radius/local/resourceGroups/rg/providers/Applications.Test/kafkas/my_kafka.instance",
		"very long owner name":  "/planes/radius/local/resourceGroups/rg/providers/Applications.Test/kafkas/" + strings.Repeat("a", 120),
		"long name upper mixed": "/planes/radius/local/resourceGroups/rg/providers/Applications.Test/kafkas/" + strings.Repeat("AbC-", 30),
	}
	for label, id := range cases {
		t.Run(label, func(t *testing.T) {
			ownerID, err := resources.ParseResource(id)
			require.NoError(t, err)

			name := ManagedSecretName(ownerID)

			require.LessOrEqual(t, len(name), managedSecretNameMaxLength, "name exceeds the 63-char limit: %q", name)
			require.Regexp(t, rfc1123Label, name, "name is not a valid RFC 1123 label: %q", name)
			require.Equal(t, name, ManagedSecretName(ownerID), "name must be deterministic")
		})
	}
}

func Test_Materialize(t *testing.T) {
	var capturedName string
	var captured generated.GenericResource

	transport := genfake.NewServerFactoryTransport(&genfake.ServerFactory{
		GenericResourcesServer: genfake.GenericResourcesServer{
			BeginCreateOrUpdate: func(ctx context.Context, resourceName string, params generated.GenericResource, options *generated.GenericResourcesClientBeginCreateOrUpdateOptions) (resp azfake.PollerResponder[generated.GenericResourcesClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
				capturedName = resourceName
				captured = params
				resp.SetTerminalResponse(http.StatusOK, generated.GenericResourcesClientCreateOrUpdateResponse{}, nil)
				return
			},
		},
	})

	m := NewMaterializer(&arm.ClientOptions{ClientOptions: policy.ClientOptions{Transport: transport}})

	result, err := m.Materialize(context.Background(), Request{
		OwnerResourceID: testOwnerID,
		EnvironmentID:   "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/env",
		ApplicationID:   "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/applications/app",
		Data:            map[string]string{"connectionString": "abc"},
	})
	require.NoError(t, err)

	ownerID, err := resources.ParseResource(testOwnerID)
	require.NoError(t, err)
	expectedName := ManagedSecretName(ownerID)

	require.Equal(t, expectedName, capturedName)
	require.Equal(t, expectedName, result.Name)
	require.Equal(t, "/planes/radius/local/resourceGroups/test-group/providers/Radius.Security/secrets/"+expectedName, result.ID)

	require.Equal(t, "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/env", captured.Properties["environment"])
	require.Equal(t, "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/applications/app", captured.Properties["application"])

	data, ok := captured.Properties["data"].(map[string]any)
	require.True(t, ok)
	connectionString, ok := data["connectionString"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "abc", connectionString["value"])
}

func Test_Materialize_OmitsEmptyApplication(t *testing.T) {
	var captured generated.GenericResource

	transport := genfake.NewServerFactoryTransport(&genfake.ServerFactory{
		GenericResourcesServer: genfake.GenericResourcesServer{
			BeginCreateOrUpdate: func(ctx context.Context, resourceName string, params generated.GenericResource, options *generated.GenericResourcesClientBeginCreateOrUpdateOptions) (resp azfake.PollerResponder[generated.GenericResourcesClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
				captured = params
				resp.SetTerminalResponse(http.StatusOK, generated.GenericResourcesClientCreateOrUpdateResponse{}, nil)
				return
			},
		},
	})

	m := NewMaterializer(&arm.ClientOptions{ClientOptions: policy.ClientOptions{Transport: transport}})

	_, err := m.Materialize(context.Background(), Request{
		OwnerResourceID: testOwnerID,
		EnvironmentID:   "env",
		Data:            map[string]string{"connectionString": "abc"},
	})
	require.NoError(t, err)

	_, hasApp := captured.Properties["application"]
	require.False(t, hasApp)
}

func Test_Materialize_InvalidOwnerID(t *testing.T) {
	m := NewMaterializer(&arm.ClientOptions{})
	_, err := m.Materialize(context.Background(), Request{OwnerResourceID: "not-an-id", Data: map[string]string{"k": "v"}})
	require.Error(t, err)
}

func Test_clientOptions_PreservesArmFields(t *testing.T) {
	// Sibling arm.ClientOptions fields (e.g. DisableRPRegistration, which sdk.NewClientOptions sets to
	// true) must survive the per-request copy; only APIVersion is overridden.
	m := &clientMaterializer{armClientOptions: &arm.ClientOptions{
		DisableRPRegistration: true,
		AuxiliaryTenants:      []string{"tenant-a"},
	}}

	opts := m.clientOptions()

	require.True(t, opts.DisableRPRegistration, "DisableRPRegistration must be preserved")
	require.Equal(t, []string{"tenant-a"}, opts.AuxiliaryTenants, "AuxiliaryTenants must be preserved")
	require.Equal(t, securitySecretsAPIVersion, opts.APIVersion, "APIVersion must be overridden")

	// The shared options must not be mutated by the copy.
	require.Empty(t, m.armClientOptions.APIVersion, "source options must not be mutated")
}

func Test_clientOptions_NilSource(t *testing.T) {
	m := &clientMaterializer{}
	opts := m.clientOptions()
	require.Equal(t, securitySecretsAPIVersion, opts.APIVersion)
}

func Test_Delete(t *testing.T) {
	var capturedName string

	transport := genfake.NewServerFactoryTransport(&genfake.ServerFactory{
		GenericResourcesServer: genfake.GenericResourcesServer{
			BeginDelete: func(ctx context.Context, resourceName string, options *generated.GenericResourcesClientBeginDeleteOptions) (resp azfake.PollerResponder[generated.GenericResourcesClientDeleteResponse], errResp azfake.ErrorResponder) {
				capturedName = resourceName
				resp.SetTerminalResponse(http.StatusOK, generated.GenericResourcesClientDeleteResponse{}, nil)
				return
			},
		},
	})

	m := NewMaterializer(&arm.ClientOptions{ClientOptions: policy.ClientOptions{Transport: transport}})

	require.NoError(t, m.Delete(context.Background(), testOwnerID))
	ownerID, err := resources.ParseResource(testOwnerID)
	require.NoError(t, err)
	require.Equal(t, ManagedSecretName(ownerID), capturedName)
}

func Test_Delete_NotFoundIsIgnored(t *testing.T) {
	transport := genfake.NewServerFactoryTransport(&genfake.ServerFactory{
		GenericResourcesServer: genfake.GenericResourcesServer{
			BeginDelete: func(ctx context.Context, resourceName string, options *generated.GenericResourcesClientBeginDeleteOptions) (resp azfake.PollerResponder[generated.GenericResourcesClientDeleteResponse], errResp azfake.ErrorResponder) {
				errResp.SetResponseError(http.StatusNotFound, "NotFound")
				return
			},
		},
	})

	m := NewMaterializer(&arm.ClientOptions{ClientOptions: policy.ClientOptions{Transport: transport}})

	require.NoError(t, m.Delete(context.Background(), testOwnerID))
}
