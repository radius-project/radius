// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_KubernetesMetadataContainer(t *testing.T) {
	template := "testdata/corerp-resources-kubemetadata-cascade.bicep"
	name := "corerp-kubemetadata-app"
	ns := "corerp-kubemetadata-ns"

	requiredSecrets := map[string]map[string]string{}

	expectedAnnotations := map[string]string{
		"user.env.ann.1":  "user.env.ann.val.1",
		"user.env.ann.2":  "user.env.ann.val.2",
		"user.app.ann.1":  "user.app.ann.val.1",
		"user.app.ann.2":  "user.app.ann.val.2",
		"user.cntr.ann.1": "user.cntr.ann.val.1",
		"user.cntr.ann.2": "user.cntr.ann.val.2",
		"collision.ann.1": "collision.cntr.ann.val.1",
	}

	expectedLabels := map[string]string{
		"user.env.lbl.1":  "user.env.lbl.val.1",
		"user.env.lbl.2":  "user.env.lbl.val.2",
		"user.app.lbl.1":  "user.app.lbl.val.1",
		"user.app.lbl.2":  "user.app.lbl.val.2",
		"user.cntr.lbl.1": "user.cntr.lbl.val.1",
		"user.cntr.lbl.2": "user.cntr.lbl.val.2",
		"collision.lbl.1": "collision.cntr.lbl.val.1",
	}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-kubemetadata-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-kubemetadata-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"corerp-kubemetadata-ns": {
						validation.NewK8sPodForResource(name, "corerp-kubemetadata-ctnr"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {
				// Verify pod labels and annotations
				label := fmt.Sprintf("radius.dev/application=%s", name)
				pods, err := test.Options.K8sClient.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				require.Len(t, pods.Items, 1)
				t.Logf("validated number of pods: %d", len(pods.Items))
				pod := pods.Items[0]
				require.Equal(t, isMapSubSet(expectedAnnotations, pod.Annotations), true)
				require.Equal(t, isMapSubSet(expectedLabels, pod.Labels), true)

				// Verify deployment labels and annotations
				deployments, err := test.Options.K8sClient.AppsV1().Deployments(ns).List(context.Background(), metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				require.Len(t, deployments.Items, 1)
				deployment := deployments.Items[0]
				require.Equal(t, isMapSubSet(expectedAnnotations, deployment.Annotations), true)
				require.Equal(t, isMapSubSet(expectedLabels, deployment.Labels), true)
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func isMapSubSet(expectedMap map[string]string, actualMap map[string]string) bool {
	if len(expectedMap) > len(actualMap) {
		return false
	}

	for k1, v1 := range expectedMap {
		v2, ok := actualMap[k1]
		if !(ok && strings.EqualFold(v1, v2)) {
			return false
		}

	}

	return true
}
