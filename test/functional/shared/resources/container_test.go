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
	"fmt"
	"testing"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Container(t *testing.T) {
	template := "testdata/corerp-resources-container.bicep"
	name := "corerp-resources-container"
	appNamespace := "corerp-resources-container-app"

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
						Name: "ctnr-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_ContainerDNSSD_TwoContainersDNS(t *testing.T) {
	template := "testdata/corerp-resources-container-two-containers-dns.bicep"
	name := "corerp-resources-container-two-containers-dns"
	appNamespace := "corerp-resources-container-two-containers-dns"

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
						Name: "containerad",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "containeraf",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "containerad"),
						validation.NewK8sPodForResource(name, "containeraf"),
						validation.NewK8sServiceForResource(name, "containeraf"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_ContainerDNSSD_OptionalPortScheme(t *testing.T) {
	template := "testdata/corerp-resources-container-optional-port-scheme.bicep"
	name := "corerp-resources-container-optional-port-scheme"
	appNamespace := "corerp-resources-container-optional-port-scheme"

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
						Name: "containerqy",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "containerqu",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "containerqi",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "containerqy"),
						validation.NewK8sPodForResource(name, "containerqu"),
						validation.NewK8sPodForResource(name, "containerqi"),
						validation.NewK8sServiceForResource(name, "containerqy"),
						validation.NewK8sServiceForResource(name, "containerqu"),
						validation.NewK8sServiceForResource(name, "containerqi"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_ContainerReadinessLiveness(t *testing.T) {
	template := "testdata/corerp-resources-container-liveness-readiness.bicep"
	name := "corerp-resources-container-live-ready"
	appNamespace := "corerp-resources-container-live-ready-app"

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
						Name: "ctnr-live-ready",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-live-ready"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_ContainerManualScale(t *testing.T) {
	template := "testdata/corerp-azure-container-manualscale.bicep"
	name := "corerp-resources-container-manualscale"
	appNamespace := "corerp-resources-container-manualscale-app"

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
						Name: "ctnr-manualscale",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-manualscale"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_ContainerWithCommandAndArgs(t *testing.T) {
	container := "testdata/corerp-resources-container-cmd-args.bicep"
	name := "corerp-resources-container-cmd-args"
	appNamespace := "corerp-resources-container-cmd-args-app"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(container),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
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
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-cmd-args"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				label := fmt.Sprintf("radapp.io/application=%s", name)
				pods, err := test.Options.K8sClient.CoreV1().Pods(appNamespace).List(ctx, metav1.ListOptions{
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
	})

	test.Test(t)
}

func Test_Container_FailDueToNonExistentImage(t *testing.T) {
	template := "testdata/corerp-resources-container-nonexistent-container-image.bicep"
	name := "corerp-resources-container-badimage"
	appNamespace := "corerp-resources-container-badimage-app"

	// We might see either of these states depending on the timing.
	validate := step.ValidateAnyDetails("DeploymentFailed", []step.DeploymentErrorDetail{
		{
			Code: "ResourceDeploymentFailure",
			Details: []step.DeploymentErrorDetail{
				{
					Code:            "Internal",
					MessageContains: "ErrImagePull",
				},
			},
		},
		{
			Code: "ResourceDeploymentFailure",
			Details: []step.DeploymentErrorDetail{
				{
					Code:            "Internal",
					MessageContains: "ImagePullBackOff",
				},
			},
		},
	})

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor:                               step.NewDeployErrorExecutor(template, validate, "magpieimage=non-existent-image"),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-cntr-badimage"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_Container_FailDueToBadHealthProbe(t *testing.T) {
	template := "testdata/corerp-resources-container-bad-healthprobe.bicep"
	name := "corerp-resources-container-bad-healthprobe"
	appNamespace := "corerp-resources-container-bad-healthprobe-app"
	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code:            "Internal",
				MessageContains: "CrashLoopBackOff",
			},
		},
	})

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor:                               step.NewDeployErrorExecutor(template, validate, functional.GetMagpieImage()),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-cntr-bad-healthprobe"),
					},
				},
			},
		},
	})

	test.Test(t)
}
