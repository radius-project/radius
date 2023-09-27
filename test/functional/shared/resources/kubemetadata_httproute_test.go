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

func Test_KubeMetadataHTTPRoute(t *testing.T) {
	template := "testdata/corerp-resources-httproute-kubernetesmetadata.bicep"
	name := "corerp-app-rte-kme"
	appNamespace := "corerp-ns-rte-kme-app"

	expectedAnnotations := map[string]string{
		"user.ann.1": "user.ann.val.1",
		"user.ann.2": "user.ann.val.2",
	}

	expectedLabels := map[string]string{
		"app.kubernetes.io/managed-by": "radius-rp",
		"app.kubernetes.io/name":       "ctnr-rte-kme",
		"app.kubernetes.io/part-of":    "corerp-app-rte-kme",
		"radapp.io/application":        "corerp-app-rte-kme",
		"radapp.io/resource":           "ctnr-rte-kme",
		"radapp.io/resource-type":      "applications.core-httproutes",
		"user.lbl.1":                   "user.lbl.val.1",
		"user.lbl.2":                   "user.lbl.val.2",
	}

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
						Name: "ctnr-rte-kme-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "ctnr-rte-kme",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-rte-kme-ctnr"),
						validation.NewK8sServiceForResource(name, "ctnr-rte-kme"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {

				// Verify service labels and annotations
				service, err := test.Options.K8sClient.CoreV1().Services(appNamespace).Get(ctx, "ctnr-rte-kme", metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, service)

				require.Truef(t, functional.IsMapSubSet(expectedAnnotations, service.Annotations), "Annotations do not match. expected: %v, actual: %v", expectedAnnotations, service.Annotations)
				require.Truef(t, functional.IsMapSubSet(expectedLabels, service.Labels), "Labels do not match. expected: %v, actual: %v", expectedLabels, service.Labels)
			},
		},
	})

	test.Test(t)
}
