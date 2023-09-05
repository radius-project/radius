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

package resource_test

import (
	"context"
	"testing"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/*
Test_Container_YAMLManifest tests the scenario where the base manifest yaml (./testdata/manifest/basemanifest.yaml)
has Deployment, Service, ServiceAccount, and multiple secrets and configmaps. The deployment resource in the manifest
uses environment varibles from secret and configmap and volume from secret, which are unsupported by
Applications.Core/containers resource. This enables Radius to render kubernetes resources unsupported by containers
resource.
*/
func Test_Container_YAMLManifest(t *testing.T) {
	template := "testdata/corerp-resources-container-manifest.bicep"
	name := "corerp-resources-container-manifest"
	appNamespace := "corerp-resources-container-manifest"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "ctnr-manifest",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-manifest"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				deploy, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).Get(ctx, "ctnr-manifest", metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, "base-manifest-test", deploy.ObjectMeta.Annotations["source"])
				require.ElementsMatch(t,
					[]string{"TEST_SECRET_KEY", "TEST_CONFIGMAP_KEY"},
					[]string{
						deploy.Spec.Template.Spec.Containers[0].Env[0].Name,
						deploy.Spec.Template.Spec.Containers[0].Env[1].Name,
					})

				srv, err := test.Options.K8sClient.CoreV1().Services(appNamespace).Get(ctx, "ctnr-manifest", metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, "base-manifest-test", srv.ObjectMeta.Annotations["source"])

				sa, err := test.Options.K8sClient.CoreV1().ServiceAccounts(appNamespace).Get(ctx, "ctnr-manifest", metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, "base-manifest-test", sa.ObjectMeta.Annotations["source"])

				for _, name := range []string{"ctnr-manifest-secret0", "ctnr-manifest-secret1"} {
					_, err := test.Options.K8sClient.CoreV1().Secrets(appNamespace).Get(ctx, name, metav1.GetOptions{})
					require.NoError(t, err)
				}

				_, err = test.Options.K8sClient.CoreV1().ConfigMaps(appNamespace).Get(ctx, "ctnr-manifest-config", metav1.GetOptions{})
				require.NoError(t, err)
			},
		},
	})

	test.Test(t)
}

/*
Test_Container_YAMLManifest_SideCar tests the scenario where the base manifest yaml (./testdata/manifest/sidecar.yaml)
has the fluentbit sidecar. Radius injects the application container described in container resource into the given
base deployment. With this, user can add multiple sidecars to their final deployment with application container.
*/
func Test_Container_YAMLManifest_SideCar(t *testing.T) {
	template := "testdata/corerp-resources-container-manifest-sidecar.bicep"
	name := "corerp-resources-container-sidecar"
	appNamespace := "corerp-resources-container-sidecar"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "ctnr-sidecar",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-sidecar"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				deploy, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).Get(ctx, "ctnr-sidecar", metav1.GetOptions{})
				require.NoError(t, err)

				require.Len(t, deploy.Spec.Template.Spec.Containers, 2)

				// Ensure that Pod includes sidecar.
				require.ElementsMatch(t, []string{"ctnr-sidecar", "log-collector"}, []string{
					deploy.Spec.Template.Spec.Containers[0].Name,
					deploy.Spec.Template.Spec.Containers[1].Name,
				})
			},
		},
	})

	test.Test(t)
}

func Test_Container_pod_patching(t *testing.T) {
	template := "testdata/corerp-resources-container-pod-patching.bicep"
	name := "corerp-resources-container-podpatch"
	appNamespace := "corerp-resources-container-podpatch"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "ctnr-podpatch",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-podpatch"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				deploy, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).Get(ctx, "ctnr-podpatch", metav1.GetOptions{})
				require.NoError(t, err)

				t.Logf("deploy: %+v", deploy)

				require.Len(t, deploy.Spec.Template.Spec.Containers, 2)

				// Ensure that Pod includes sidecar.
				require.ElementsMatch(t, []string{"ctnr-podpatch", "log-collector"}, []string{
					deploy.Spec.Template.Spec.Containers[0].Name,
					deploy.Spec.Template.Spec.Containers[1].Name,
				})

				require.True(t, deploy.Spec.Template.Spec.HostNetwork)
			},
		},
	})

	test.Test(t)
}
