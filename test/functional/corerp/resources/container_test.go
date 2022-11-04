// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Container(t *testing.T) {
	template := "testdata/corerp-resources-container.bicep"
	name := "corerp-resources-container"

	requiredSecrets := map[string]map[string]string{}

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
						Name: "ctnr-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "ctnr-ctnr"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_ContainerHttpRoute(t *testing.T) {
	template := "testdata/corerp-resources-container-httproute.bicep"
	name := "corerp-resources-container-httproute"

	requiredSecrets := map[string]map[string]string{}

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
						Name: "ctnr-rte-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "ctnr-rte-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "ctnr-rte-ctnr"),
						validation.NewK8sServiceForResource(name, "ctnr-rte-rte"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_ContainerReadinessLiveness(t *testing.T) {
	template := "testdata/corerp-resources-container-liveness-readiness.bicep"
	name := "corerp-resources-container-live-ready"

	requiredSecrets := map[string]map[string]string{}

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
						Name: "ctnr-live-ready",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "ctnr-live-ready"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_ContainerManualScale(t *testing.T) {
	template := "testdata/corerp-azure-container-manualscale.bicep"
	name := "corerp-resources-container-manualscale"

	requiredSecrets := map[string]map[string]string{}

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
						Name: "ctnr-manualscale",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "ctnr-manualscale"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_ContainerWithCommandAndArgs(t *testing.T) {
	container := "testdata/corerp-resources-container-cmd-args.bicep"
	name := "corerp-resources-container-cmd-args"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(container),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "ctnr-cmd-args",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "ctnr-cmd-args"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {
				label := fmt.Sprintf("radius.dev/application=%s", name)
				pods, err := test.Options.K8sClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				require.Len(t, pods.Items, 1)
				t.Logf("validated number of pods: %d", len(pods.Items))
				pod := pods.Items[0]
				containers := pod.Spec.Containers
				require.Len(t, containers, 1)
				t.Logf("validated number of containers: %d", len(containers))
				container := containers[0]
				require.Equal(t, []string{"/bin/sh"}, container.Command)
				require.Equal(t, []string{"-c", "while true; do echo hello; sleep 10;done"}, container.Args)
				t.Logf("validated command and args of pod: %s", pod.Name)
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
