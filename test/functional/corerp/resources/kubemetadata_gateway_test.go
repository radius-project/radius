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
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_Gateway_KubernetesMetadata(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-gateway-kubernetesmetadata.bicep"
	name := "corerp-resources-gateway-kme"
	appNamespace := "default-corerp-resources-gateway-kme"
	expectedAnnotations := map[string]string{
		"user.ann.1": "user.ann.val.1",
		"user.ann.2": "user.ann.val.2",
	}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "http-gtwy-kme",
						Type: validation.GatewaysResource,
						App:  name,
					},
					{
						Name: "http-gtwy-front-rte-kme",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "http-gtwy-front-ctnr-kme",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "http-gtwy-back-rte-kme",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "http-gtwy-back-ctnr-kme",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "http-gtwy-front-ctnr-kme"),
						validation.NewK8sPodForResource(name, "http-gtwy-back-ctnr-kme"),
						validation.NewK8sHTTPProxyForResource(name, "http-gtwy-kme"),
						validation.NewK8sHTTPProxyForResource(name, "http-gtwy-front-rte-kme"),
						validation.NewK8sServiceForResource(name, "http-gtwy-front-rte-kme"),
						validation.NewK8sHTTPProxyForResource(name, "http-gtwy-back-rte-kme"),
						validation.NewK8sServiceForResource(name, "http-gtwy-back-rte-kme"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct corerp.CoreRPTest) {
				// Check labels and annotations
				t.Logf("Checking label, annotation values in HTTPProxy resources")
				httpproxies, err := functional.GetHTTPProxyList(ctx, ct.Options.Client, appNamespace, name)
				require.NoError(t, err)
				for _, httpproxy := range httpproxies.Items {
					expectedLabels := getExpectedLabels(t, httpproxy.Name)
					require.Truef(t, functional.IsMapSubSet(expectedLabels, httpproxy.Labels), "labels in httpproxy %v do not match expected values : ", httpproxy.Name)
					require.Truef(t, functional.IsMapSubSet(expectedAnnotations, httpproxy.Annotations), "annotations in httpproxy %v do not match expected values", httpproxy.Name)
				}
			},
		},
	})

	test.Test(t)
}

// getExpectedLabels returns the expected labels for the given resource name
func getExpectedLabels(t *testing.T, resourceName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "radius-rp",
		"app.kubernetes.io/name":       resourceName,
		"app.kubernetes.io/part-of":    "corerp-resources-gateway-kme",
		"radius.dev/application":       "corerp-resources-gateway-kme",
		"radius.dev/resource":          resourceName,
		"radius.dev/resource-type":     "applications.core-gateways",
		"user.lbl.1":                   "user.lbl.val.1",
		"user.lbl.2":                   "user.lbl.val.2",
	}
}
