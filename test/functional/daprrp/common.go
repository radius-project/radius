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

package daprrp

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/test/functional/shared"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func commonPostDeleteVerify(ctx context.Context, t *testing.T, test shared.RPTest, resourceType, resourceName, namespace string) {
	resource, err := test.Options.ManagementClient.ShowResource(ctx, resourceType, resourceName)
	require.Error(t, err)
	require.Equal(t, generated.GenericResource{}, resource)

	dynamicClient, err := dynamic.NewForConfig(test.Options.K8sConfig)
	require.NoError(t, err)

	gvr := schema.GroupVersionResource{
		Group:    "dapr.io",
		Version:  "v1alpha1",
		Resource: "components",
	}

	resourceList, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "radius.dev/resource=" + resourceName,
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(resourceList.Items))
}
