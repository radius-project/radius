// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha1"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha1"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ConvertK8sApplicationToARM(t *testing.T) {
	original := radiusv1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "radius.dev/v1alpha1",
			Kind:       "Application",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "frontend-backend",
			Namespace: "default",
			Annotations: map[string]string{
				kubernetes.AnnotationsApplication: "frontend-backend",
			},
		},
		Spec: v1alpha1.ApplicationSpec{},
	}

	expected := &radclient.ApplicationResource{
		TrackedResource: radclient.TrackedResource{
			Resource: radclient.Resource{
				Name: to.StringPtr("frontend-backend"),
			},
		},
		Properties: &radclient.ApplicationProperties{},
	}

	actual, err := ConvertK8sApplicationToARM(original)
	require.NoError(t, err, "failed to convert application")

	require.Equal(t, expected, actual)
}

func Test_ConvertK8sComponentToARM(t *testing.T) {
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
				kubernetes.AnnotationsApplication: "frontend-backend",
				kubernetes.AnnotationsComponent:   "frontend",
			},
		},
		Spec: v1alpha1.ComponentSpec{
			Kind:      "Component",
			Run:       &runtime.RawExtension{Raw: runJson},
			Bindings:  &runtime.RawExtension{Raw: bindingJson},
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

	expected := &radclient.ComponentResource{
		TrackedResource: radclient.TrackedResource{
			Resource: radclient.Resource{
				Name: to.StringPtr("frontend"),
			},
		},
		Kind: to.StringPtr("Component"),
		Properties: &radclient.ComponentProperties{
			Config: map[string]interface{}{
				"managed": "true",
			},
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "rynowak/frontend:0.5.0-dev",
				},
			},
			Bindings: map[string]interface{}{
				"default": map[string]interface{}{
					"kind": "http",
				},
			},
			Uses: []*radclient.ComponentDependency{
				{
					Binding: to.StringPtr("[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]"),
					Env: map[string]*string{
						"SERVICE__BACKEND__HOST": to.StringPtr("[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default.host]"),
					},
					Secrets: &radclient.ComponentDependencySecrets{
						Store: to.StringPtr("[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]"),
						Keys: map[string]*string{
							"secret": to.StringPtr("[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]"),
						},
					},
				},
			},
			Traits: []radclient.ComponentTraitClassification{
				&radclient.InboundRouteTrait{
					ComponentTrait: radclient.ComponentTrait{
						Kind: to.StringPtr("radius.dev/InboundRoute@v1alpha1"),
					},
					Binding: to.StringPtr("web"),
				},
			},
		},
	}

	actual, err := ConvertK8sComponentToARM(original)
	require.NoError(t, err, "failed to convert component")

	require.Equal(t, expected, actual)
}

func Test_ConvertK8sDeploymentToARM(t *testing.T) {
	original := radiusv1alpha1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "radius.dev/v1alpha1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "frontend-backend-default",
			Namespace: "default",
			Annotations: map[string]string{
				kubernetes.AnnotationsApplication: "frontend-backend",
				kubernetes.AnnotationsDeployment:  "default",
			},
		},
		Spec: radiusv1alpha1.DeploymentSpec{
			Components: []radiusv1alpha1.DeploymentComponent{
				{
					ComponentName: "frontend",
				},
				{
					ComponentName: "backend",
				},
			},
		},
	}

	expected := &radclient.DeploymentResource{
		TrackedResource: radclient.TrackedResource{
			Resource: radclient.Resource{
				Name: to.StringPtr("default"),
			},
		},
		Properties: &radclient.DeploymentProperties{
			Components: []*radclient.DeploymentComponent{
				{
					ComponentName: to.StringPtr("frontend"),
				},
				{
					ComponentName: to.StringPtr("backend"),
				},
			},
		},
	}

	actual, err := ConvertK8sDeploymentToARM(original)
	require.NoError(t, err, "failed to convert deployment")

	require.Equal(t, expected, actual)
}
