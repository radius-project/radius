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

package sdk

import (
	"testing"

	"reflect"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/require"
)

func Test_NewClientOptions(t *testing.T) {
	t.Run("provided nil config returns default config", func(t *testing.T) {
		connection, err := NewDirectConnection("http://example.com")
		require.NoError(t, err)
		expectedClientOptions := getDefaultClientOptions(connection)

		result := NewClientOptions(connection, nil)

		// have to set ShouldRetry to nil because reflect.DeepEqual doesn't work with functions
		expectedClientOptions.Retry.ShouldRetry = nil
		result.Retry.ShouldRetry = nil

		// require.Equal doesn't work with functions, so we have to do this
		require.True(t, reflect.DeepEqual(expectedClientOptions, expectedClientOptions))
	})

	t.Run("provided empty config returns default config", func(t *testing.T) {
		connection, err := NewDirectConnection("http://example.com")
		require.NoError(t, err)
		expectedClientOptions := getDefaultClientOptions(connection)

		result := NewClientOptions(connection, &arm.ClientOptions{})

		// have to set ShouldRetry to nil because reflect.DeepEqual doesn't work with functions
		expectedClientOptions.Retry.ShouldRetry = nil
		result.Retry.ShouldRetry = nil

		// require.Equal doesn't work with functions, so we have to do this
		require.True(t, reflect.DeepEqual(expectedClientOptions, expectedClientOptions))
	})

	t.Run("merges with provided options", func(t *testing.T) {
		connection, err := NewDirectConnection("http://example.com")
		require.NoError(t, err)
		expectedClientOptions := getDefaultClientOptions(connection)
		expectedClientOptions.Retry.MaxRetries = 100

		providedClientOptions := &arm.ClientOptions{}
		providedClientOptions.Retry.MaxRetries = 100
		result := NewClientOptions(connection, providedClientOptions)

		// have to set ShouldRetry to nil because reflect.DeepEqual doesn't work with functions
		expectedClientOptions.Retry.ShouldRetry = nil
		result.Retry.ShouldRetry = nil

		// require.Equal doesn't work with functions, so we have to do this
		require.True(t, reflect.DeepEqual(expectedClientOptions, expectedClientOptions))
	})
}

func getDefaultClientOptions(connection Connection) *arm.ClientOptions {
	return &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.Configuration{
				Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
					cloud.ResourceManager: {
						Endpoint: connection.Endpoint(),
						Audience: "https://management.core.windows.net",
					},
				},
			},
			PerRetryPolicies: []policy.Policy{
				// Autorest will inject an empty bearer token, which conflicts with bearer auth
				// when its used by Kubernetes. We don't *ever* need Autorest to handle auth for us
				// so we just remove it.
				//
				// We'll solve this problem permanently by writing our own client.
				&removeAuthorizationHeaderPolicy{},
			},
			Transport: connection.Client(),
		},
		DisableRPRegistration: true,
	}
}
