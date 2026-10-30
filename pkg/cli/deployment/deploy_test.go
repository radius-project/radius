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

package deployment

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/sdk"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
)

func Test_GetProviderConfigs(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
	}
	options := clients.DeploymentOptions{
		Providers: &clients.Providers{},
	}

	var expectedConfig sdkclients.ProviderConfig

	expectedConfig.Radius = &sdkclients.Radius{
		Type: "Radius",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &sdkclients.Deployments{
		Type: "Microsoft.Resources",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs(options)
	require.Equal(t, providerConfig, expectedConfig)
}

func Test_GetProviderConfigsWithAzProvider(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
	}

	options := clients.DeploymentOptions{
		Providers: &clients.Providers{
			Azure: &clients.AzureProvider{
				Scope: "/subscriptions/dummy/resourceGroups/azrg",
			},
		},
	}

	var expectedConfig sdkclients.ProviderConfig

	expectedConfig.Az = &sdkclients.Az{
		Type: "AzureResourceManager",
		Value: sdkclients.Value{
			Scope: "/subscriptions/dummy/resourceGroups/" + "azrg",
		},
	}

	expectedConfig.Radius = &sdkclients.Radius{
		Type: "Radius",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &sdkclients.Deployments{
		Type: "Microsoft.Resources",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs(options)
	require.Equal(t, providerConfig, expectedConfig)
}

func Test_GetProviderConfigsWithAWSProvider_SetsOwner(t *testing.T) {
	const awsScope = "/planes/aws/aws/accounts/123456789012/regions/us-west-2"
	const envID = "/planes/radius/local/resourceGroups/testrg/providers/Applications.Core/environments/env"
	const appID = "/planes/radius/local/resourceGroups/testrg/providers/Applications.Core/applications/app"

	tests := []struct {
		name      string
		radius    *clients.RadiusProvider
		wantOwner string
	}{
		{
			name:      "with an application",
			radius:    &clients.RadiusProvider{EnvironmentID: envID, ApplicationID: appID},
			wantOwner: "deploy;/planes/radius/local/resourceGroups/testrg;" + appID + ";" + envID,
		},
		{
			name:      "without an application",
			radius:    &clients.RadiusProvider{EnvironmentID: envID},
			wantOwner: "deploy;/planes/radius/local/resourceGroups/testrg;;" + envID,
		},
		{
			name:      "without a Radius provider",
			wantOwner: "deploy;/planes/radius/local/resourceGroups/testrg;;",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceDeploymentClient := ResourceDeploymentClient{RadiusResourceGroup: "testrg"}
			options := clients.DeploymentOptions{
				Providers: &clients.Providers{
					AWS:    &clients.AWSProvider{Scope: awsScope},
					Radius: tt.radius,
				},
			}

			providerConfig := resourceDeploymentClient.GetProviderConfigs(options)
			require.Equal(t, &sdkclients.AWS{
				Type:  sdkclients.ProviderTypeAWS,
				Value: sdkclients.Value{Scope: awsScope, Owner: tt.wantOwner},
			}, providerConfig.AWS)
			require.Empty(t, providerConfig.Radius.Value.Owner, "only the AWS provider has an owner")
		})
	}
}

func Test_GetProviderConfigsWithoutAWSScope_HasNoAWSProvider(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{RadiusResourceGroup: "testrg"}
	options := clients.DeploymentOptions{
		Providers: &clients.Providers{
			AWS:    &clients.AWSProvider{},
			Radius: &clients.RadiusProvider{EnvironmentID: "/planes/radius/local/resourceGroups/testrg/providers/Applications.Core/environments/env"},
		},
	}

	require.Nil(t, resourceDeploymentClient.GetProviderConfigs(options).AWS)
}

func Test_monitorProgress_StopsWhenContextCanceled(t *testing.T) {
	requests := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"value":[]}`)
	}))
	defer server.Close()

	connection, err := sdk.NewDirectConnection(server.URL)
	require.NoError(t, err)
	operationsClient, err := sdkclients.NewResourceDeploymentOperationsClient(&sdkclients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          connection.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(connection),
	})
	require.NoError(t, err)
	dc := &ResourceDeploymentClient{RadiusResourceGroup: "testrg", OperationsClient: operationsClient}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- dc.monitorProgress(ctx, "test-deployment", make(chan clients.ResourceProgress, 1), &sync.WaitGroup{})
	}()

	// Let monitorProgress start waiting for its first poll, then cancel as Deploy does
	// when the deployment completes.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(OperationPollInterval / 2):
		t.Fatal("monitorProgress did not stop when its context was canceled")
	}
	require.Zero(t, requests.Load())
}
