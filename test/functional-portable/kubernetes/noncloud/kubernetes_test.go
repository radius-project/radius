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

package kubernetes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/controller/reconciler"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_TutorialApplication_KubernetesManifests(t *testing.T) {
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

	namespace := "kubernetes-interop-tutorial"
	environmentName := namespace + "-env"
	applicationName := namespace

	// Create the namespace, if it already exists we can ignore the error.
	_, err := opts.K8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	require.NoError(t, controller_runtime.IgnoreAlreadyExists(err))

	cli := radcli.NewCLI(t, "")

	params := []string{
		testutil.GetBicepRecipeRegistry(),
		testutil.GetBicepRecipeVersion(),

		// Avoid a conflict between app namespace and env namespace.
		fmt.Sprintf("name=%s", environmentName),
		fmt.Sprintf("namespace=%s", environmentName),
	}

	err = cli.Deploy(ctx, "testdata/tutorial-environment.bicep", "", "", params...)
	require.NoError(t, err)

	deployment := makeDeployment(types.NamespacedName{Name: "demo", Namespace: namespace}, environmentName, applicationName)
	recipe := makeRecipe(types.NamespacedName{Name: "db", Namespace: namespace}, environmentName, applicationName)

	t.Run("Deploy", func(t *testing.T) {
		t.Log("Creating recipe")
		err = opts.Client.Create(ctx, recipe)
		require.NoError(t, err)

		t.Log("Creating deployment")
		err = opts.Client.Create(ctx, deployment)
		require.NoError(t, err)
	})

	t.Run("Check Recipe status", func(t *testing.T) {
		ctx, cancel := testcontext.NewWithCancel(t)
		defer cancel()

		// Get resource version
		err = opts.Client.Get(ctx, types.NamespacedName{Name: "db", Namespace: namespace}, recipe)
		require.NoError(t, err)

		t.Log("Waiting for recipe ready")
		recipe, err = waitForRecipeReady(t, ctx, types.NamespacedName{Name: "db", Namespace: namespace}, opts.Client, recipe.ResourceVersion)
		require.NoError(t, err)

		// Doing a basic check that the recipe has a resource provisioned.
		require.NotEmpty(t, recipe.Status.Resource)

		client, err := generated.NewGenericResourcesClient(recipe.Status.Scope, recipe.Spec.Type, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(opts.Connection))
		require.NoError(t, err)

		_, err = client.Get(ctx, recipe.Name, nil)
		require.NoError(t, err)
	})

	t.Run("Check Deployment status", func(t *testing.T) {
		ctx, cancel := testcontext.NewWithCancel(t)
		defer cancel()

		// Get resource version
		err = opts.Client.Get(ctx, types.NamespacedName{Name: "demo", Namespace: namespace}, deployment)
		require.NoError(t, err)

		t.Log("Waiting for deployment ready")
		deployment, err = waitForDeploymentReady(t, types.NamespacedName{Name: "demo", Namespace: namespace}, opts.K8sClient, deployment.ResourceVersion)
		require.NoError(t, err)

		// Doing a basic check that the Deployment has environment variables set.
		require.NotEmpty(t, deployment.Spec.Template.Spec.Containers[0].EnvFrom)

		// Doing a basic check that the deployment has a resource provisioned.
		client, err := generated.NewGenericResourcesClient(recipe.Status.Scope, "Applications.Core/containers", &aztoken.AnonymousCredential{}, sdk.NewClientOptions(opts.Connection))
		require.NoError(t, err)

		_, err = client.Get(ctx, deployment.Name, nil)
		require.NoError(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Log("Deleting recipe")
		err = opts.Client.Delete(ctx, recipe)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			err = opts.Client.Get(ctx, types.NamespacedName{Name: "db", Namespace: namespace}, recipe)
			return apierrors.IsNotFound(err)
		}, time.Second*60, time.Second*5, "waiting for recipe to be deleted")

		t.Log("Deleting deployment")
		err = opts.Client.Delete(ctx, deployment)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			err = opts.Client.Get(ctx, types.NamespacedName{Name: "demo", Namespace: namespace}, deployment)
			return apierrors.IsNotFound(err)
		}, time.Second*60, time.Second*5, "waiting for deployment to be deleted")
	})
}

func makeDeployment(name types.NamespacedName, environmentName string, applicationName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			Annotations: map[string]string{
				"radapp.io/enabled":          "true",
				"radapp.io/connection-redis": "db",
				"radapp.io/environment":      environmentName,
				"radapp.io/application":      applicationName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "demo"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "demo"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "demo",
							Image: "ghcr.io/radius-project/tutorial/webapp:edge",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 3000,
								},
							},
						},
					},
				},
			},
		},
	}
}

func makeRecipe(name types.NamespacedName, environmentName string, applicationName string) *radappiov1alpha3.Recipe {
	return &radappiov1alpha3.Recipe{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			Annotations: map[string]string{
				"radapp.io/enabled":          "true",
				"radapp.io/connection-redis": "db",
			},
		},
		Spec: radappiov1alpha3.RecipeSpec{
			Type:        "Applications.Datastores/redisCaches",
			Environment: environmentName,
			Application: applicationName,
		},
	}
}

func waitForRecipeReady(t *testing.T, ctx context.Context, name types.NamespacedName, client controller_runtime.WithWatch, initialVersion string) (*radappiov1alpha3.Recipe, error) {
	// Based on https://gist.github.com/PrasadG193/52faed6499d2ec739f9630b9d044ffdc
	lister := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			listOptions := &controller_runtime.ListOptions{Raw: &options, Namespace: name.Namespace, FieldSelector: fields.ParseSelectorOrDie("metadata.name=" + name.Name)}
			recipes := &radappiov1alpha3.RecipeList{}
			err := client.List(ctx, recipes, listOptions)
			if err != nil {
				return nil, err
			}

			return recipes, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			listOptions := &controller_runtime.ListOptions{Raw: &options, Namespace: name.Namespace, FieldSelector: fields.ParseSelectorOrDie("metadata.name=" + name.Name)}
			recipes := &radappiov1alpha3.RecipeList{}
			return client.Watch(ctx, recipes, listOptions)
		},
	}
	watcher, err := watchtools.NewRetryWatcher(initialVersion, lister)
	require.NoError(t, err)
	defer watcher.Stop()

	for {
		event := <-watcher.ResultChan()
		r, ok := event.Object.(*radappiov1alpha3.Recipe)
		if !ok {
			// Not a recipe, likely an event.
			t.Logf("Received event: %+v", event)
			continue
		}

		t.Logf("Received recipe. Status: %+v", r.Status)
		if r.Status.Phrase == radappiov1alpha3.PhraseReady {
			return r, nil
		}
	}
}

func waitForDeploymentReady(t *testing.T, name types.NamespacedName, client *kubernetes.Clientset, initialVersion string) (*appsv1.Deployment, error) {
	// Based on https://gist.github.com/PrasadG193/52faed6499d2ec739f9630b9d044ffdc
	watcher, err := watchtools.NewRetryWatcher(initialVersion, cache.NewFilteredListWatchFromClient(client.AppsV1().RESTClient(), "deployments", name.Namespace, func(options *metav1.ListOptions) {
		options.FieldSelector = "metadata.name=" + name.Name
	}))
	require.NoError(t, err)
	defer watcher.Stop()

	for {
		event := <-watcher.ResultChan()
		d, ok := event.Object.(*appsv1.Deployment)
		if !ok {
			// Not a deployment, likely an event.
			t.Logf("Received event: %+v", event)
			continue
		}

		t.Logf("Received deployment. Annotations: %+v", d.Annotations)

		data, ok := d.Annotations[reconciler.AnnotationRadiusStatus]
		if !ok || data == "" {
			continue
		}

		status := map[string]any{}
		err := json.Unmarshal([]byte(data), &status)
		require.NoError(t, err)

		if d.Status.ObservedGeneration == d.Generation && d.Status.ReadyReplicas == 1 && status["phrase"] == "Ready" {
			return d, nil
		}
	}
}
