// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"encoding/json"
	"testing"

	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ConvertComponentToInternal(t *testing.T) {
	uses := map[string]interface{}{
		"binding": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]",
		"env": map[string]interface{}{
			"SERVICE__BACKEND__HOST": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default.host]",
		},
		"secrets": map[string]interface{}{
			"store": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]",
			"keys": map[string]interface{}{
				"secret": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]",
			},
		},
	}
	usesJson, err := json.Marshal(uses)
	require.NoError(t, err, "failed to marshal uses json")

	trait := map[string]interface{}{
		"kind":    "radius.dev/InboundRoute@v1alpha1",
		"binding": "web",
	}

	traitJson, err := json.Marshal(trait)
	require.NoError(t, err, "failed to marshal traits json")

	config := map[string]interface{}{
		"managed": "true",
	}

	configJson, err := json.Marshal(config)
	require.NoError(t, err, "failed to marshal config json")

	bindings := map[string]interface{}{
		"default": map[string]interface{}{
			"kind": "http",
		},
	}

	bindingJson, err := json.Marshal(bindings)
	require.NoError(t, err, "failed to marshal bindings json")

	run := map[string]interface{}{
		"container": map[string]interface{}{
			"image": "rynowak/frontend:0.5.0-dev",
		},
	}

	runJson, err := json.Marshal(run)
	require.NoError(t, err, "failed to marshal run json")

	original := radiusv1alpha1.Component{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "radius.dev/v1alpha1",
			Kind:       "Component",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "frontend",
			Namespace: "default",
			Annotations: map[string]string{
				keys.AnnotationsApplication: "frontend-backend",
				keys.AnnotationsComponent:   "frontend",
			},
		},
		Spec: v1alpha1.ComponentSpec{
			Kind:      "Component",
			Run:       &runtime.RawExtension{Raw: runJson},
			Bindings:  runtime.RawExtension{Raw: bindingJson},
			Hierarchy: []string{"frontend-backend", "frontend"},
			Uses: &[]runtime.RawExtension{
				{
					Raw: usesJson,
				},
			},
			Traits: &[]runtime.RawExtension{
				{
					Raw: traitJson,
				},
			},
			Config: &runtime.RawExtension{Raw: configJson},
		},
	}

	actual := components.GenericComponent{}
	expected := components.GenericComponent{
		Name: "frontend",
		Kind: "Component",
		Config: map[string]interface{}{
			"managed": "true",
		},
		Run: map[string]interface{}{
			"container": map[string]interface{}{
				"image": "rynowak/frontend:0.5.0-dev",
			},
		},
		Bindings: map[string]components.GenericBinding{
			"default": {
				Kind:                 "http",
				AdditionalProperties: map[string]interface{}{},
			},
		},
		Uses: []components.GenericDependency{
			{
				Binding: components.BindingExpression{
					Kind: "component",
					Value: &components.ComponentBindingValue{
						Application: "frontend-backend",
						Component:   "frontend",
						Binding:     "default",
					},
				},
				Env: map[string]components.BindingExpression{
					"SERVICE__BACKEND__HOST": {
						Kind: "component",
						Value: &components.ComponentBindingValue{
							Application: "frontend-backend",
							Component:   "frontend",
							Binding:     "default",
							Property:    "host",
						},
					},
				},
				Secrets: &components.GenericDependencySecrets{
					Store: components.BindingExpression{
						Kind: "component",
						Value: &components.ComponentBindingValue{
							Application: "frontend-backend",
							Component:   "frontend",
							Binding:     "default",
						},
					},
					Keys: map[string]components.BindingExpression{
						"secret": {
							Kind: "component",
							Value: &components.ComponentBindingValue{
								Application: "frontend-backend",
								Component:   "frontend",
								Binding:     "default",
							},
						},
					},
				},
			},
		},
		Traits: []components.GenericTrait{
			{
				Kind: "radius.dev/InboundRoute@v1alpha1",
				AdditionalProperties: map[string]interface{}{
					"binding": "web",
				},
			},
		},
	}

	err = ConvertComponentToInternal(&original, &actual, nil)
	require.NoError(t, err, "failed to convert component")

	require.Equal(t, expected, actual)
}
