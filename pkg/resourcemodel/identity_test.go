/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package resourcemodel

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

var values = []struct {
	Description string
	ExpectedID  string
	Identity    ResourceIdentity
}{
	{
		// Ensure that default-initialized identity does not panic.
		Description: "empty",
		ExpectedID:  "",
		Identity:    ResourceIdentity{},
	},
	{
		Description: "Azure",
		ExpectedID:  "/subscriptions/0000/resourceGroups/mygroup/providers/Microsoft.DocumentDB/accounts/someaccount",
		Identity: ResourceIdentity{
			ResourceType: &ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: ProviderAzure,
			},
			Data: ARMIdentity{
				ID:         "/subscriptions/0000/resourceGroups/mygroup/providers/Microsoft.DocumentDB/accounts/someaccount",
				APIVersion: "2020-01-01",
			},
		},
	},
	{
		// "core" group
		Description: "Kubernetes (core)",
		ExpectedID:  "/planes/kubernetes/local/namespaces/test-namespace/providers/core/Secret/test-name",
		Identity: ResourceIdentity{
			ResourceType: &ResourceType{
				Type:     resourcekinds.Secret,
				Provider: ProviderKubernetes,
			},
			Data: KubernetesIdentity{
				Kind:       "Secret",
				APIVersion: "v1",
				Name:       "test-name",
				Namespace:  "test-namespace",
			},
		},
	},
	{
		// Cluster-scoped resource
		Description: "Kubernetes (cluster-scoped)",
		ExpectedID:  "/planes/kubernetes/local/providers/secrets/SecretProviderClass/test-name",
		Identity: ResourceIdentity{
			ResourceType: &ResourceType{
				Type:     resourcekinds.SecretProviderClass,
				Provider: ProviderKubernetes,
			},
			Data: KubernetesIdentity{
				Kind:       "SecretProviderClass",
				APIVersion: "secrets/v1",
				Name:       "test-name",
			},
		},
	},
	{
		// Namespaced non-core group
		Description: "Kubernetes (non-core)",
		ExpectedID:  "/planes/kubernetes/local/namespaces/test-namespace/providers/apps/Deployment/test-name",
		Identity: ResourceIdentity{
			ResourceType: &ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: ProviderKubernetes,
			},
			Data: KubernetesIdentity{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
				Name:       "test-name",
				Namespace:  "test-namespace",
			},
		},
	},
	{
		Description: "AWS",
		ExpectedID:  "/planes/aws/aws/accounts/0000/regions/us-west-2/providers/AWS.Kinesis/Stream/mystream",
		Identity: ResourceIdentity{
			ResourceType: &ResourceType{
				Type:     "AWS.Kinesis/Stream",
				Provider: ProviderAWS,
			},
			Data: UCPIdentity{
				ID: "/planes/aws/aws/accounts/0000/regions/us-west-2/providers/AWS.Kinesis/Stream/mystream",
			},
		},
	},
}

// Test that all formats of ResourceIdentifier round-trip with BSON
func Test_ResourceIdentifier_BSONRoundTrip(t *testing.T) {
	for _, input := range values {
		t.Run(input.Description, func(t *testing.T) {
			b, err := bson.Marshal(&input.Identity)
			require.NoError(t, err)

			output := ResourceIdentity{}
			err = bson.Unmarshal(b, &output)
			require.NoError(t, err)

			require.Equal(t, input.Identity, output)
		})
	}
}

// Test that all formats of ResourceIdentifier round-trip with JSON
func Test_ResourceIdentifier_JSONRoundTrip(t *testing.T) {
	for _, input := range values {
		t.Run(input.Description, func(t *testing.T) {
			b, err := json.Marshal(&input.Identity)
			require.NoError(t, err)

			output := ResourceIdentity{}
			err = json.Unmarshal(b, &output)
			require.NoError(t, err)

			require.Equal(t, input.Identity, output)
		})
	}
}

func Test_GetID(t *testing.T) {
	for _, input := range values {
		t.Run(input.Description, func(t *testing.T) {
			id := input.Identity.GetID()
			require.Equal(t, input.ExpectedID, id)
		})
	}
}
