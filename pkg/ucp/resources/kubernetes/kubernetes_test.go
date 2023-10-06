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

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ToUCPResourceID(t *testing.T) {
	t.Run("kubernetes resource type : deployment", func(t *testing.T) {
		namespace := "default"
		resourceType := "deployment"
		resourceName := "test-deployment"
		expectedID := "/planes/kubernetes/local/namespaces/default/providers/apps/Deployment/test-deployment"
		ucpID, err := ToUCPResourceID(namespace, resourceType, resourceName, "")
		require.NoError(t, err)
		require.Equal(t, expectedID, ucpID)
	})

	t.Run("kubernetes resource type: dapr component", func(t *testing.T) {
		namespace := "test-dapr"
		resourceType := "Component"
		resourceName := "test-dapr-pubsub"
		provider := "dapr.io"
		expectedID := "/planes/kubernetes/local/namespaces/test-dapr/providers/dapr.io/Component/test-dapr-pubsub"
		ucpID, err := ToUCPResourceID(namespace, resourceType, resourceName, provider)
		require.NoError(t, err)
		require.Equal(t, expectedID, ucpID)
	})

	t.Run("cluster scoped resource", func(t *testing.T) {
		resourceType := "deployment"
		resourceName := "test-deployment"
		expectedID := "/planes/kubernetes/local/providers/apps/Deployment/test-deployment"
		ucpID, err := ToUCPResourceID("", resourceType, resourceName, "")
		require.NoError(t, err)
		require.Equal(t, expectedID, ucpID)
	})

	t.Run("empty resource type", func(t *testing.T) {
		namespace := "default"
		resourceName := "test-deployment"
		_, err := ToUCPResourceID(namespace, "", resourceName, "")
		require.EqualError(t, err, "resourceType or resourceName is empty")
	})
}
