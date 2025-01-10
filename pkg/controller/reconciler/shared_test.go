/*
Copyright 2023.

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

package reconciler

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	recipeTestWaitDuration            = time.Second * 10
	recipeTestWaitInterval            = time.Second * 1
	recipeTestControllerDelayInterval = time.Millisecond * 100
)

func createEnvironment(radius *mockRadiusClient, resourceGroup, name string) {
	id := fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Applications.Core/environments/%s", resourceGroup, name)
	radius.Update(func() {
		radius.environments[id] = v20231001preview.EnvironmentResource{
			ID:       to.Ptr(id),
			Name:     to.Ptr(name),
			Location: to.Ptr(v1.LocationGlobal),
		}
	})
}

func deleteEnvironment(radius *mockRadiusClient, resourceGroup, name string) {
	id := fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Applications.Core/environments/%s", resourceGroup, name)
	radius.Delete(func() {
		delete(radius.environments, id)
	})
}

func makeRecipe(name types.NamespacedName, resourceType string) *radappiov1alpha3.Recipe {
	return &radappiov1alpha3.Recipe{
		ObjectMeta: ctrl.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: radappiov1alpha3.RecipeSpec{
			Type: resourceType,
		},
	}
}

func waitForRecipeStateUpdating(t *testing.T, client client.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.RecipeStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.RecipeStatus{}
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		logger.Logf("Fetching Recipe: %+v", name)
		current := &radappiov1alpha3.Recipe{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("Recipe.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.PhraseUpdating, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}

	}, recipeTestWaitDuration, recipeTestWaitInterval, "failed to enter updating state")

	return status
}

func waitForRecipeStateReady(t *testing.T, client client.Client, name types.NamespacedName) *radappiov1alpha3.RecipeStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.RecipeStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching Recipe: %+v", name)
		current := &radappiov1alpha3.Recipe{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("Recipe.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.PhraseReady, current.Status.Phrase) {
			assert.Empty(t, current.Status.Operation)
		}
	}, recipeTestWaitDuration, recipeTestWaitInterval, "failed to enter updating state")

	return status
}

func waitForRecipeStateDeleting(t *testing.T, client client.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.RecipeStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.RecipeStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching Recipe: %+v", name)
		current := &radappiov1alpha3.Recipe{}
		err := client.Get(ctx, name, current)
		assert.NoError(t, err)

		status = &current.Status
		logger.Logf("Recipe.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.PhraseDeleting, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}
	}, recipeTestWaitDuration, recipeTestWaitInterval, "failed to enter deleting state")

	return status
}

func waitForRecipeDeleted(t *testing.T, client client.Client, name types.NamespacedName) {
	ctx := testcontext.New(t)

	logger := t
	require.Eventuallyf(t, func() bool {
		logger.Logf("Fetching Recipe: %+v", name)
		current := &radappiov1alpha3.Recipe{}
		err := client.Get(ctx, name, current)
		if apierrors.IsNotFound(err) {
			return true
		}

		logger.Logf("Recipe.Status: %+v", current.Status)
		return false

	}, recipeTestWaitDuration, recipeTestWaitInterval, "recipe still exists")
}

func makeDeployment(name types.NamespacedName) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.Name,
			Namespace:   name.Namespace,
			Annotations: map[string]string{},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name.Name,
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}
}

func makeDeploymentTemplate(name types.NamespacedName, template, providerConfig string, parameters map[string]string) *radappiov1alpha3.DeploymentTemplate {
	return &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: ctrl.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			ProviderConfig: providerConfig,
			Parameters:     parameters,
		},
	}
}

func makeDeploymentResource(name types.NamespacedName, id string) *radappiov1alpha3.DeploymentResource {
	return &radappiov1alpha3.DeploymentResource{
		ObjectMeta: ctrl.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: radappiov1alpha3.DeploymentResourceSpec{
			Id: id,
		},
	}
}

func generateProviderConfig(radiusScope, azureScope, awsScope string) (string, error) {
	if radiusScope == "" {
		return "", fmt.Errorf("radiusScope is required")
	}

	providerConfig := sdkclients.ProviderConfig{}
	if awsScope != "" {
		providerConfig.AWS = &sdkclients.AWS{
			Type: "aws",
			Value: sdkclients.Value{
				Scope: awsScope,
			},
		}
	}
	if azureScope != "" {
		providerConfig.Az = &sdkclients.Az{
			Type: "azure",
			Value: sdkclients.Value{
				Scope: azureScope,
			},
		}
	}

	providerConfig.Radius = &sdkclients.Radius{
		Type: "Radius",
		Value: sdkclients.Value{
			Scope: radiusScope,
		},
	}
	providerConfig.Deployments = &sdkclients.Deployments{
		Type: "Microsoft.Resources",
		Value: sdkclients.Value{
			Scope: radiusScope,
		},
	}

	b, err := json.Marshal(providerConfig)

	return string(b), err
}
