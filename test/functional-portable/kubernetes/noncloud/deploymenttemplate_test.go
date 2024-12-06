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
	"path"
	"testing"
	"time"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/controller/reconciler"
	"github.com/radius-project/radius/pkg/sdk"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/spf13/afero"
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

func Test_DeploymentTemplate(t *testing.T) {
	defaultProviderConfig, err := generateDefaultProviderConfig()
	require.NoError(t, err)

	testcases := []struct {
		name             string
		namespace        string
		fileName         string
		templateFilePath string
		providerConfig   string
		parameters       map[string]string
	}{
		{
			name:             "dt-env",
			namespace:        "dt-ns-env",
			fileName:         "env.bicep",
			templateFilePath: path.Join("testdata", "env", "env.json"),
			providerConfig:   defaultProviderConfig,
			parameters: map[string]string{
				"name":      "dt-env",
				"namespace": "dt-ns-env",
			},
		},
		{
			name:             "dt-module",
			namespace:        "dt-ns-module",
			fileName:         "module.bicep",
			templateFilePath: path.Join("testdata", "module", "module.json"),
			providerConfig:   defaultProviderConfig,
			parameters: map[string]string{
				"name":      "dt-module",
				"namespace": "dt-ns-module",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			opts := rp.NewRPTestOptions(t)

			name := tc.name
			namespace := tc.namespace

			template, err := afero.ReadFile(afero.NewOsFs(), tc.templateFilePath)
			require.NoError(t, err)

			// Create the namespace, if it already exists we can ignore the error.
			_, err = opts.K8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
			require.NoError(t, controller_runtime.IgnoreAlreadyExists(err))

			deploymentTemplate := makeDeploymentTemplate(types.NamespacedName{Name: name, Namespace: namespace}, string(template), tc.providerConfig, tc.fileName, tc.parameters)

			t.Run("Deploy", func(t *testing.T) {
				t.Log("Creating DeploymentTemplate")
				err = opts.Client.Create(ctx, deploymentTemplate)
				require.NoError(t, err)
			})

			t.Run("Check status", func(t *testing.T) {
				ctx, cancel := testcontext.NewWithCancel(t)
				defer cancel()

				// Get resource version
				err = opts.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploymentTemplate)
				require.NoError(t, err)

				t.Log("Waiting for DeploymentTemplate ready")
				deploymentTemplate, err := waitForDeploymentTemplateReady(t, ctx, types.NamespacedName{Name: name, Namespace: namespace}, opts.Client, deploymentTemplate.ResourceVersion)
				require.NoError(t, err)

				// Doing a basic check that the deploymentTemplate has a resource provisioned.
				require.NotEmpty(t, deploymentTemplate.Status.Resource)

				scope, err := reconciler.ParseDeploymentScopeFromProviderConfig(deploymentTemplate.Spec.ProviderConfig)
				require.NoError(t, err)

				client, err := generated.NewGenericResourcesClient(scope, "Applications.Core/environments", &aztoken.AnonymousCredential{}, sdk.NewClientOptions(opts.Connection))
				require.NoError(t, err)

				_, err = client.Get(ctx, deploymentTemplate.Name, nil)
				require.NoError(t, err)
			})

			t.Run("Delete", func(t *testing.T) {
				t.Log("Deleting DeploymentTemplate")
				err = opts.Client.Delete(ctx, deploymentTemplate)
				require.NoError(t, err)

				require.Eventually(t, func() bool {
					err = opts.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploymentTemplate)
					return apierrors.IsNotFound(err)
				}, time.Second*60, time.Second*5, "waiting for deploymentTemplate to be deleted")
			})
		})
	}
}

func makeDeploymentTemplate(name types.NamespacedName, template, providerConfig, rootFileName string, parameters map[string]string) *radappiov1alpha3.DeploymentTemplate {
	deploymentTemplate := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
			RootFileName:   rootFileName,
		},
	}

	return deploymentTemplate
}

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
	watcher, err := watchtools.NewRetryWatcher(initialVersion, lister)
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

func generateDefaultProviderConfig() (string, error) {
	providerConfig := sdkclients.ProviderConfig{}

	providerConfig.Radius = &sdkclients.Radius{
		Type: "radius",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/default",
		},
	}
	providerConfig.Deployments = &sdkclients.Deployments{
		Type: "Microsoft.Resources",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/default",
		},
	}

	marshalledProviderConfig, err := json.MarshalIndent(providerConfig, "", "  ")
	if err != nil {
		return "", err
	}
	return string(marshalledProviderConfig), nil
}
