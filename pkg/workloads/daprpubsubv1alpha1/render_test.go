// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
)

func Test_Render_Unanaged_Failure(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
				"name":    "cool-servicebus",
				// Topic is required
			},
		},
		ServiceValues: map[string]map[string]interface{}{},
	}

	_, err := renderer.Render(context.Background(), workload)
	require.Error(t, err)
}

func Test_Render_Managed_Success(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
				"name":    "cool-servicebus",
				"topic":   "cool-topic",
			},
		},
		ServiceValues: map[string]map[string]interface{}{},
	}

	resources, err := renderer.Render(context.Background(), workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindDaprPubSubTopicAzureServiceBus, resource.Type)

	expected := map[string]string{
		"name":                 "test-component",
		"namespace":            "test-app",
		"apiVersion":           "dapr.io/v1alpha1",
		"kind":                 "Component",
		"servicebuspubsubname": "cool-servicebus",
		"servicebustopic":      "cool-topic",
	}
	require.Equal(t, expected, resource.Resource)
}
