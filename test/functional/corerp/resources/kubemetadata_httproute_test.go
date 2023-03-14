// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
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
		"radius.dev/application":       "corerp-app-rte-kme",
		"radius.dev/resource":          "ctnr-rte-kme",
		"radius.dev/resource-type":     "applications.core-httproutes",
		"user.lbl.1":                   "user.lbl.val.1",
		"user.lbl.2":                   "user.lbl.val.2",
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
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {

				// Verify service labels and annotations
				service, err := test.Options.K8sClient.CoreV1().Services(appNamespace).Get(ctx, "ctnr-rte-kme", metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, service)

<<<<<<< HEAD
				require.Truef(t, functional.IsMapSubSet(expectedAnnotations, service.Annotations), "Annotations do not match. expected: %v, actual: %v", expectedAnnotations, service.Annotations)
				require.Truef(t, functional.IsMapSubSet(expectedLabels, service.Labels), "Labels do not match. expected: %v, actual: %v", expectedLabels, service.Labels)
=======
				require.True(t, isMapSubSet(expectedAnnotations, service.Annotations))
				require.True(t, isMapSubSet(expectedLabels, service.Labels))

>>>>>>> 58990ec8d (added functional test)
			},
		},
	})

	test.Test(t)
}
