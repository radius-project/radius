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

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
				if err == nil {
					t.Logf("Deployment: %v", deploy)
				}

				srv, err := test.Options.K8sClient.CoreV1().Services(appNamespace).Get(ctx, "ctnr-manifest", metav1.GetOptions{})
				if err == nil {
					t.Logf("Service: %v", srv)
				}

				sa, err := test.Options.K8sClient.CoreV1().ServiceAccounts(appNamespace).Get(ctx, "ctnr-manifest", metav1.GetOptions{})
				if err == nil {
					t.Logf("Service account: %v", sa)
				}

				for _, name := range []string{"ctnr-manifest-secret0", "ctnr-manifest-secret1"} {
					secret, err := test.Options.K8sClient.CoreV1().Secrets(appNamespace).Get(ctx, name, metav1.GetOptions{})
					if err == nil {
						t.Logf("Secret: %v", secret)
					}
				}

				cm, err := test.Options.K8sClient.CoreV1().ConfigMaps(appNamespace).Get(ctx, "ctnr-manifest-config", metav1.GetOptions{})
				if err == nil {
					t.Logf("Config map: %v", cm)
				}

			},
		},
	})

	test.Test(t)
}
