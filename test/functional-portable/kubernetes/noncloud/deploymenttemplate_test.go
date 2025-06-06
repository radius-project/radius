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
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/controller/reconciler"
	"github.com/radius-project/radius/pkg/sdk"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/radius-project/radius/test/testutil"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_DeploymentTemplate_Env(t *testing.T) {
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

	name := "dt-env"
	namespace := "dt-env-ns"
	templateFilePath := path.Join("testdata", "env", "env.json")
	parameters := []string{
		fmt.Sprintf("name=%s", name),
		fmt.Sprintf("namespace=%s", namespace),
	}

	providerConfig, err := sdkclients.NewDefaultProviderConfig(name).String()
	require.NoError(t, err)

	parametersMap := createParametersMap(parameters)

	template, err := os.ReadFile(templateFilePath)
	require.NoError(t, err)

	// Create the namespace, if it already exists we can ignore the error.
	_, err = opts.K8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	require.NoError(t, controller_runtime.IgnoreAlreadyExists(err))

	deploymentTemplate := makeDeploymentTemplate(types.NamespacedName{Name: name, Namespace: namespace}, string(template), providerConfig, parametersMap)

	t.Run("Create DeploymentTemplate", func(t *testing.T) {
		t.Log("Creating DeploymentTemplate")
		err = opts.Client.Create(ctx, deploymentTemplate)
		require.NoError(t, err)
	})

	t.Run("Check DeploymentTemplate status", func(t *testing.T) {
		ctx, cancel := testcontext.NewWithCancel(t)
		defer cancel()

		// Get resource version
		err = opts.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploymentTemplate)
		require.NoError(t, err)

		t.Log("Waiting for DeploymentTemplate ready")
		deploymentTemplate, err := waitForDeploymentTemplateReady(t, ctx, types.NamespacedName{Name: name, Namespace: namespace}, opts.Client, deploymentTemplate.ResourceVersion)
		require.NoError(t, err)

		scope, err := reconciler.ParseDeploymentScopeFromProviderConfig(deploymentTemplate.Spec.ProviderConfig)
		require.NoError(t, err)

		expectedResources := [][]string{
			{"Applications.Core/environments", fmt.Sprintf("%s-env", name)},
		}

		err = assertExpectedResourcesExist(ctx, scope, expectedResources, opts.Connection)
		require.NoError(t, err)
	})

	t.Run("Delete DeploymentTemplate", func(t *testing.T) {
		t.Log("Deleting DeploymentTemplate")
		err = opts.Client.Delete(ctx, deploymentTemplate)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			err = opts.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploymentTemplate)
			return apierrors.IsNotFound(err)
		}, time.Second*60, time.Second*5, "waiting for deploymentTemplate to be deleted")
	})
}

func Test_DeploymentTemplate_Module(t *testing.T) {
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

	name := "dt-module"
	namespace := "dt-module-ns"
	templateFilePath := path.Join("testdata", "module", "module.json")
	parameters := []string{
		fmt.Sprintf("name=%s", name),
		fmt.Sprintf("namespace=%s", namespace),
	}

	providerConfig, err := sdkclients.NewDefaultProviderConfig(name).String()
	require.NoError(t, err)

	parametersMap := createParametersMap(parameters)

	template, err := os.ReadFile(templateFilePath)
	require.NoError(t, err)

	// Create the namespace, if it already exists we can ignore the error.
	_, err = opts.K8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	require.NoError(t, controller_runtime.IgnoreAlreadyExists(err))

	deploymentTemplate := makeDeploymentTemplate(types.NamespacedName{Name: name, Namespace: namespace}, string(template), providerConfig, parametersMap)

	t.Run("Create DeploymentTemplate", func(t *testing.T) {
		t.Log("Creating DeploymentTemplate")
		err = opts.Client.Create(ctx, deploymentTemplate)
		require.NoError(t, err)
	})

	t.Run("Check DeploymentTemplate status", func(t *testing.T) {
		ctx, cancel := testcontext.NewWithCancel(t)
		defer cancel()

		// Get resource version
		err = opts.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploymentTemplate)
		require.NoError(t, err)

		t.Log("Waiting for DeploymentTemplate ready")
		deploymentTemplate, err := waitForDeploymentTemplateReady(t, ctx, types.NamespacedName{Name: name, Namespace: namespace}, opts.Client, deploymentTemplate.ResourceVersion)
		require.NoError(t, err)

		scope, err := reconciler.ParseDeploymentScopeFromProviderConfig(deploymentTemplate.Spec.ProviderConfig)
		require.NoError(t, err)

		expectedResources := [][]string{
			{"Applications.Core/environments", fmt.Sprintf("%s-env", name)},
			{"Applications.Core/applications", fmt.Sprintf("%s-app", name)},
		}

		err = assertExpectedResourcesExist(ctx, scope, expectedResources, opts.Connection)
		require.NoError(t, err)
	})

	t.Run("Delete DeploymentTemplate", func(t *testing.T) {
		t.Log("Deleting DeploymentTemplate")
		err = opts.Client.Delete(ctx, deploymentTemplate)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			err = opts.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploymentTemplate)
			return apierrors.IsNotFound(err)
		}, time.Second*60, time.Second*5, "waiting for deploymentTemplate to be deleted")
	})
}

func Test_DeploymentTemplate_Recipe(t *testing.T) {
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

	name := "dt-recipe"
	namespace := "dt-recipe-ns"
	templateFilePath := path.Join("testdata", "recipe", "recipe.json")
	parameters := []string{
		testutil.GetBicepRecipeRegistry(),
		testutil.GetBicepRecipeVersion(),
		fmt.Sprintf("name=%s", name),
		fmt.Sprintf("namespace=%s", namespace),
	}

	providerConfig, err := sdkclients.NewDefaultProviderConfig(name).String()
	require.NoError(t, err)

	parametersMap := createParametersMap(parameters)

	template, err := os.ReadFile(templateFilePath)
	require.NoError(t, err)

	// Create the namespace, if it already exists we can ignore the error.
	_, err = opts.K8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	require.NoError(t, controller_runtime.IgnoreAlreadyExists(err))

	deploymentTemplate := makeDeploymentTemplate(types.NamespacedName{Name: name, Namespace: namespace}, string(template), providerConfig, parametersMap)

	t.Run("Create DeploymentTemplate", func(t *testing.T) {
		t.Log("Creating DeploymentTemplate")
		err = opts.Client.Create(ctx, deploymentTemplate)
		require.NoError(t, err)
	})

	t.Run("Check DeploymentTemplate status", func(t *testing.T) {
		ctx, cancel := testcontext.NewWithCancel(t)
		defer cancel()

		// Get resource version
		err = opts.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploymentTemplate)
		require.NoError(t, err)

		t.Log("Waiting for DeploymentTemplate ready")
		deploymentTemplate, err := waitForDeploymentTemplateReady(t, ctx, types.NamespacedName{Name: name, Namespace: namespace}, opts.Client, deploymentTemplate.ResourceVersion)
		require.NoError(t, err)

		scope, err := reconciler.ParseDeploymentScopeFromProviderConfig(deploymentTemplate.Spec.ProviderConfig)
		require.NoError(t, err)

		expectedResources := [][]string{
			{"Applications.Core/environments", fmt.Sprintf("%s-env", name)},
			{"Applications.Core/applications", fmt.Sprintf("%s-app", name)},
			{"Applications.Datastores/redisCaches", fmt.Sprintf("%s-recipe", name)},
		}

		err = assertExpectedResourcesExist(ctx, scope, expectedResources, opts.Connection)
		require.NoError(t, err)
	})

	t.Run("Delete DeploymentTemplate", func(t *testing.T) {
		t.Log("Deleting DeploymentTemplate")
		err = opts.Client.Delete(ctx, deploymentTemplate)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			err = opts.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploymentTemplate)
			return apierrors.IsNotFound(err)
		}, time.Minute*10, time.Second*10, "waiting for deploymentTemplate to be deleted")
	})
}

// makeDeploymentTemplate returns a DeploymentTemplate object with the given name, template, providerConfig, and parameters.
func makeDeploymentTemplate(name types.NamespacedName, template, providerConfig string, parameters map[string]string) *radappiov1alpha3.DeploymentTemplate {
	deploymentTemplate := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}

	return deploymentTemplate
}

// waitForDeploymentTemplateReady watches the creation of the DeploymentTemplate object
// and waits for it to be in the "Ready" state.
func waitForDeploymentTemplateReady(t *testing.T, ctx context.Context, name types.NamespacedName, client controller_runtime.WithWatch, initialVersion string) (*radappiov1alpha3.DeploymentTemplate, error) {
	// Based on https://gist.github.com/PrasadG193/52faed6499d2ec739f9630b9d044ffdc
	lister := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			listOptions := &controller_runtime.ListOptions{Raw: &options, Namespace: name.Namespace, FieldSelector: fields.ParseSelectorOrDie("metadata.name=" + name.Name)}
			deploymentTemplates := &radappiov1alpha3.DeploymentTemplateList{}
			err := client.List(ctx, deploymentTemplates, listOptions)
			if err != nil {
				return nil, err
			}

			return deploymentTemplates, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			listOptions := &controller_runtime.ListOptions{Raw: &options, Namespace: name.Namespace, FieldSelector: fields.ParseSelectorOrDie("metadata.name=" + name.Name)}
			deploymentTemplates := &radappiov1alpha3.DeploymentTemplateList{}
			return client.Watch(ctx, deploymentTemplates, listOptions)
		},
	}
	watcher, err := watchtools.NewRetryWatcherWithContext(ctx, initialVersion, lister)
	require.NoError(t, err)
	defer watcher.Stop()

	for {
		event := <-watcher.ResultChan()
		r, ok := event.Object.(*radappiov1alpha3.DeploymentTemplate)
		if !ok {
			// Not a deploymentTemplate, likely an event.
			t.Logf("Received event: %+v", event)
			continue
		}

		t.Logf("Received deploymentTemplate. Status: %+v", r.Status)
		if r.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseReady {
			return r, nil
		}
	}
}

// createParametersMap creates a map of parameters from a list of parameters
// in the form of key=value.
func createParametersMap(parameters []string) map[string]string {
	parametersMap := make(map[string]string)
	for _, param := range parameters {
		kv := strings.Split(param, "=")
		key := kv[0]
		value := kv[1]
		parametersMap[key] = value
	}

	return parametersMap
}

// assertExpectedResourcesExist asserts that the expected resources exist
// in Radius for the given scope.
func assertExpectedResourcesExist(ctx context.Context, scope string, expectedResources [][]string, connection sdk.Connection) error {
	for _, resource := range expectedResources {
		resourceType := resource[0]
		resourceName := resource[1]

		client, err := generated.NewGenericResourcesClient(scope, resourceType, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(connection))
		if err != nil {
			return err
		}

		_, err = client.Get(ctx, resourceName, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

// assertExpectedResourcesToNotExist asserts that the expected resources do not exist
// in Radius for the given scope. This is useful for testing cleanup after deletion.
func assertExpectedResourcesToNotExist(ctx context.Context, scope string, expectedResources [][]string, connection sdk.Connection) error {
	for _, resource := range expectedResources {
		resourceType := resource[0]
		resourceName := resource[1]

		client, err := generated.NewGenericResourcesClient(scope, resourceType, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(connection))
		if err != nil {
			return err
		}

		_, err = client.Get(ctx, resourceName, nil)
		if err == nil {
			return fmt.Errorf("expected resource %s/%s to be not found, but was found", resourceType, resourceName)
		}

		if !clients.Is404Error(err) {
			return fmt.Errorf("Expected resource %s/%s to be not found, but instead got error: %v", resourceType, resourceName, err)
		}
	}

	return nil
}
