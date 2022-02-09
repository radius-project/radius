// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/project-radius/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/project-radius/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	Namespace = "mynamespace"
	BaseURL   = "/apis/api.radius.dev/v1alpha3/fake"
)

func Test_ListApplication(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	app1Name := "my-app"
	// Simulate a *rendered* resource
	resource1 := radiusv1alpha3.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      app1Name,
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Template: rawOrPanic(map[string]interface{}{
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	id, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application"}))
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&resource1).Build()

	rp := NewResourceProvider(c, BaseURL, "http")

	response, err := rp.ListApplications(context.Background(), id)
	require.NoError(t, err)

	expectedID, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: app1Name}))
	require.NoError(t, err)

	expected := resourceprovider.ApplicationResourceList{
		Value: []resourceprovider.ApplicationResource{
			{
				ID:   expectedID.ID,
				Type: expectedID.Type(),
				Name: app1Name,
				Properties: map[string]interface{}{
					"definition-property": "definition-value",
					"status": rest.ApplicationStatus{
						ProvisioningState: "Provisioned",
						HealthState:       "Healthy",
					},
				},
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_GetApplication(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	app1Name := "my-app"
	// Simulate a *rendered* resource
	resource1 := radiusv1alpha3.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      app1Name,
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Template: rawOrPanic(map[string]interface{}{
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	id, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: app1Name}))
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&resource1).Build()

	rp := NewResourceProvider(c, BaseURL, "http")

	response, err := rp.GetApplication(context.Background(), id)
	require.NoError(t, err)

	require.NoError(t, err)

	expected := resourceprovider.ApplicationResource{
		ID:   id.ID,
		Type: id.Type(),
		Name: app1Name,
		Properties: map[string]interface{}{
			"definition-property": "definition-value",
			"status": rest.ApplicationStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_DeleteApplication(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	app1Name := "my-app"
	// Simulate a *rendered* resource
	resource1 := radiusv1alpha3.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      app1Name,
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Template: rawOrPanic(map[string]interface{}{
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	id, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: app1Name}))
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&resource1).Build()

	rp := NewResourceProvider(c, BaseURL, "http")

	response, err := rp.DeleteApplication(context.Background(), id)
	require.NoError(t, err)

	require.Equal(t, rest.NewNoContentResponse(), response)
}

func Test_ListResources(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	appName := "my-app"
	// Simulate a *rendered* resource
	app := radiusv1alpha3.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      appName,
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Template: rawOrPanic(map[string]interface{}{
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	resource1Name := "my-resource"
	expectedID1, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "Container", Name: resource1Name}))
	require.NoError(t, err)
	// Simulate a *rendered* resource
	resource1 := radiusv1alpha3.Container{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Container",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      resource1Name,
			Namespace: Namespace,
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: appName,
			},
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Template: rawOrPanic(map[string]interface{}{
				"name": resource1Name,
				"id":   expectedID1.ID,
				"type": expectedID1.Type(),
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	resource2Name := "my-resource2"
	expectedID2, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "Container", Name: resource2Name}))
	require.NoError(t, err)

	resource2 := radiusv1alpha3.Container{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Container",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      resource2Name,
			Namespace: Namespace,
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: appName,
			},
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Template: rawOrPanic(map[string]interface{}{
				"name": resource2Name,
				"id":   expectedID2.ID,
				"type": expectedID2.Type(),
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	id, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "Container"}))
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&app, &resource1, &resource2).Build()

	rp := NewResourceProvider(c, BaseURL, "http")

	response, err := rp.ListResources(context.Background(), id)
	require.NoError(t, err)

	expected := resourceprovider.RadiusResourceList{
		Value: []resourceprovider.RadiusResource{
			{
				ID:   expectedID1.ID,
				Type: expectedID1.Type(),
				Name: resource1Name,
				Properties: map[string]interface{}{
					"definition-property": "definition-value",
					"status":              map[string]interface{}{},
				},
			},
			{
				ID:   expectedID2.ID,
				Type: expectedID2.Type(),
				Name: resource2Name,
				Properties: map[string]interface{}{
					"definition-property": "definition-value",
					"status":              map[string]interface{}{},
				},
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_ListAllV3ResourcesByApplication(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	appName := "my-app"
	// Simulate a *rendered* resource
	app := radiusv1alpha3.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      appName,
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Template: rawOrPanic(map[string]interface{}{
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	resource1Name := "my-resource"
	expectedID1, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "HttpRoute", Name: resource1Name}))
	require.NoError(t, err)
	// Simulate a *rendered* resource
	resource1 := radiusv1alpha3.HttpRoute{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "HttpRoute",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      resource1Name,
			Namespace: Namespace,
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: appName,
			},
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Template: rawOrPanic(map[string]interface{}{
				"name": resource1Name,
				"id":   expectedID1.ID,
				"type": expectedID1.Type(),
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	id, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "RadiusResource"}))
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&app, &resource1).Build()

	rp := NewResourceProvider(c, BaseURL, "http")

	response, err := rp.ListAllV3ResourcesByApplication(context.Background(), id)
	require.NoError(t, err)

	expected := resourceprovider.RadiusResourceList{
		Value: []resourceprovider.RadiusResource{
			{
				ID:   expectedID1.ID,
				Type: expectedID1.Type(),
				Name: resource1Name,
				Properties: map[string]interface{}{
					"definition-property": "definition-value",
					"status":              map[string]interface{}{},
				},
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_GetResource(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	appName := "my-app"
	// Simulate a *rendered* resource
	app := radiusv1alpha3.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      appName,
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Template: rawOrPanic(map[string]interface{}{
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	resource1Name := "my-resource"
	expectedID1, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "HttpRoute", Name: resource1Name}))
	require.NoError(t, err)
	// Simulate a *rendered* resource
	resource1 := radiusv1alpha3.HttpRoute{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "HttpRoute",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(appName, resource1Name),
			Namespace: Namespace,
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: appName,
			},
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Template: rawOrPanic(map[string]interface{}{
				"name": resource1Name,
				"id":   expectedID1.ID,
				"type": expectedID1.Type(),
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	resource2Name := "my-resource2"
	expectedID2, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "Container", Name: resource2Name}))
	require.NoError(t, err)

	resource2 := radiusv1alpha3.Container{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Container",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(appName, resource2Name),
			Namespace: Namespace,
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: appName,
			},
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Template: rawOrPanic(map[string]interface{}{
				"name": resource2Name,
				"id":   expectedID2.ID,
				"type": expectedID2.Type(),
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	id, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "Container", Name: resource2Name}))
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&app, &resource1, &resource2).Build()

	rp := NewResourceProvider(c, BaseURL, "http")

	response, err := rp.GetResource(context.Background(), id, radclient.AzureConnectionResourceProperties{})
	require.NoError(t, err)

	expected := resourceprovider.RadiusResource{
		ID:   expectedID2.ID,
		Type: expectedID2.Type(),
		Name: resource2Name,
		Properties: map[string]interface{}{
			"definition-property": "definition-value",
			"status":              map[string]interface{}{},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_DeleteResource(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	appName := "my-app"
	// Simulate a *rendered* resource
	app := radiusv1alpha3.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      appName,
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Template: rawOrPanic(map[string]interface{}{
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	resource1Name := "my-resource"
	expectedID1, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "HttpRoute", Name: resource1Name}))
	require.NoError(t, err)
	// Simulate a *rendered* resource
	resource1 := radiusv1alpha3.HttpRoute{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "HttpRoute",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(appName, resource1Name),
			Namespace: Namespace,
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: appName,
			},
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Template: rawOrPanic(map[string]interface{}{
				"name": resource1Name,
				"id":   expectedID1.ID,
				"type": expectedID1.Type(),
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}
	id, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "HttpRoute", Name: resource1Name}))
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&app, &resource1).Build()

	rp := NewResourceProvider(c, BaseURL, "http")

	response, err := rp.DeleteResource(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, rest.NewNoContentResponse(), response)
}

func Test_ListSecrets(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))

	secret := corev1.Secret{
		TypeMeta: v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "some-secret",
			Namespace: Namespace,
		},
		Data: map[string][]byte{
			"secret-key": []byte("secret-value"),
		},
	}

	appName := "my-app"
	// Simulate a *rendered* resource
	app := radiusv1alpha3.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      appName,
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Template: rawOrPanic(map[string]interface{}{
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	resource1Name := "my-resource"
	expectedID1, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "HttpRoute", Name: resource1Name}))
	require.NoError(t, err)

	resource1 := radiusv1alpha3.HttpRoute{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "HttpRoute",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(appName, resource1Name),
			Namespace: Namespace,
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: appName,
			},
			Generation: 1,
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Template: rawOrPanic(map[string]interface{}{
				"name": resource1Name,
				"id":   expectedID1.ID,
				"type": expectedID1.Type(),
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
		Status: radiusv1alpha3.ResourceStatus{
			ObservedGeneration: 1,
			Phrase:             "Deployed",
			Resources: map[string]*radiusv1alpha3.OutputResource{
				"SecretLocalID": {
					Resource: corev1.ObjectReference{
						Namespace:  Namespace,
						Name:       "some-secret",
						Kind:       "Secret",
						APIVersion: "v1",
					},
				},
			},
			ComputedValues: rawOrPanic(map[string]renderers.ComputedValueReference{
				"computed-property": {
					Value: "computed-value",
				},
			}),
			SecretValues: rawOrPanic(map[string]renderers.SecretValueReference{
				"secret-property": {
					LocalID:       "SecretLocalID",
					ValueSelector: "secret-key",
				},
			}),
		},
	}

	id, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "HttpRoute", Name: resource1Name}))
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&app, &resource1, &secret).Build()

	rp := NewResourceProvider(c, BaseURL, "http")

	response, err := rp.ListSecrets(context.Background(), resourceprovider.ListSecretsInput{TargetID: id.ID})
	require.NoError(t, err)

	expected := map[string]interface{}{
		"secret-property": "secret-value",
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_GetOperation(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	appName := "my-app"
	// Simulate a *rendered* resource
	app := radiusv1alpha3.Application{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "Application",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      appName,
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Template: rawOrPanic(map[string]interface{}{
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
	}

	resource1Name := "my-resource"
	expectedID1, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "HttpRoute", Name: resource1Name}))
	require.NoError(t, err)
	// Simulate a *rendered* resource
	resource1 := radiusv1alpha3.HttpRoute{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "HttpRoute",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(appName, resource1Name),
			Namespace: Namespace,
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: appName,
			},
			Generation: 1,
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Template: rawOrPanic(map[string]interface{}{
				"name": resource1Name,
				"id":   expectedID1.ID,
				"type": expectedID1.Type(),
				"body": map[string]interface{}{
					"properties": map[string]interface{}{
						"definition-property": "definition-value",
					},
				},
			}),
		},
		Status: radiusv1alpha3.ResourceStatus{
			Phrase:             "Deployed",
			ObservedGeneration: 1,
			Resources: map[string]*radiusv1alpha3.OutputResource{
				"LocalID": {
					Status: radiusv1alpha3.OutputResourceStatus{
						ProvisioningState: "Provisioned",
						HealthState:       "Healthy",
					},
				},
			},
		},
	}

	id, err := azresources.Parse(azresources.MakeID(
		"kubernetes",
		Namespace,
		azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
		azresources.ResourceType{Type: "Application", Name: appName},
		azresources.ResourceType{Type: "HttpRoute", Name: resource1Name},
		azresources.ResourceType{Type: azresources.OperationResourceType}))
	require.NoError(t, err)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&app, &resource1).Build()

	rp := NewResourceProvider(c, BaseURL, "http")

	response, err := rp.GetOperation(context.Background(), id)
	require.NoError(t, err)

	expected := resourceprovider.RadiusResource{
		ID:   id.Truncate().ID,
		Type: id.Truncate().Type(),
		Name: resource1Name,
		Properties: map[string]interface{}{
			"definition-property": "definition-value",
			"status": rest.ResourceStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
				OutputResources: []rest.OutputResource{
					{
						LocalID:            "LocalID",
						OutputResourceType: "kubernetes",
						Status: rest.OutputResourceStatus{
							ProvisioningState: "Provisioned",
							HealthState:       "Healthy",
						},
					},
				},
			},
		},
	}

	require.Equal(t, rest.NewOKResponse(expected), response)
}

func rawOrPanic(obj interface{}) *runtime.RawExtension {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return &runtime.RawExtension{Raw: b}
}
