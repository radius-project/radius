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
	"time"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Container(t *testing.T) {
	template := "testdata/corerp-resources-container.bicep"
	name := "corerp-resources-container"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "ctnr-ctnr",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "ctnr-ctnr"),
					},
				},
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, name)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_ContainerDNSSD_TwoContainersDNS(t *testing.T) {
	template := "testdata/corerp-resources-container-two-containers-dns.bicep"
	name := "corerp-resources-container-two-containers-dns"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "containerad",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
					{
						Name: "containeraf",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "containerad"),
						validation.NewK8sPodForResource(name, "containeraf"),
						validation.NewK8sServiceForResource(name, "containeraf"),
					},
				},
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, name)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_Container_PortExposure(t *testing.T) {
	template := "testdata/corerp-resources-container-port-exposure.bicep"
	name := "corerp-resources-container-port-exposure"
	appNamespace := "corerp-resources-container-port-exposure"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "containerqy",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
					{
						Name: "containerqu",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
					{
						Name: "containerqi",
						Type: validation.ComputeContainersResource,
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

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_ContainerReadinessLiveness(t *testing.T) {
	template := "testdata/corerp-resources-container-liveness-readiness.bicep"
	name := "corerp-resources-container-live-ready"
	appNamespace := "corerp-resources-container-live-ready"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "ctnr-live-ready",
						Type: validation.ComputeContainersResource,
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

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_ContainerManualScale(t *testing.T) {
	template := "testdata/corerp-azure-container-manualscale.bicep"
	name := "corerp-resources-container-manualscale"
	appNamespace := "corerp-resources-container-manualscale"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "ctnr-manualscale",
						Type: validation.ComputeContainersResource,
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
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				label := fmt.Sprintf("radapp.io/application=%s", name)
				require.Eventually(t, func() bool {
					pods, err := test.Options.K8sClient.CoreV1().Pods(appNamespace).List(ctx, metav1.ListOptions{
						LabelSelector: label,
					})
					if err != nil {
						return false
					}
					return len(pods.Items) == 3
				}, 2*time.Minute, 5*time.Second, "expected 3 replicas for manually scaled container")
				t.Logf("validated 3 replicas for %s", name)
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_ContainerWithCommandAndArgs(t *testing.T) {
	container := "testdata/corerp-resources-container-cmd-args.bicep"
	name := "corerp-resources-container-cmd-args"
	appNamespace := "corerp-resources-container-cmd-args"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "ctnr-cmd-args",
						Type: validation.ComputeContainersResource,
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
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
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

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(container, fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_Container_FailDueToNonExistentImage(t *testing.T) {
	template := "testdata/corerp-resources-container-nonexistent-container-image.bicep"
	name := "corerp-resources-container-badimage"
	appNamespace := "corerp-resources-container-badimage"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			// The recipe-driven Radius.Compute/containers type provisions the Kubernetes
			// Deployment successfully even when the image cannot be pulled, so the deployment
			// itself succeeds. Skip object validation (the pod never becomes ready) and instead
			// assert the image-pull failure in PostStepVerify.
			SkipObjectValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "ctnr-ctnr-badimage",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				label := fmt.Sprintf("radapp.io/application=%s", name)
				require.Eventually(t, func() bool {
					pods, err := test.Options.K8sClient.CoreV1().Pods(appNamespace).List(ctx, metav1.ListOptions{
						LabelSelector: label,
					})
					if err != nil || len(pods.Items) == 0 {
						return false
					}
					for _, pod := range pods.Items {
						for _, cs := range pod.Status.ContainerStatuses {
							if cs.State.Waiting != nil &&
								(cs.State.Waiting.Reason == "ImagePullBackOff" || cs.State.Waiting.Reason == "ErrImagePull") {
								t.Logf("validated pod %s container %s in state %s", pod.Name, cs.Name, cs.State.Waiting.Reason)
								return true
							}
						}
					}
					return false
				}, 2*time.Minute, 5*time.Second, "expected pod to report ImagePullBackOff or ErrImagePull")
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, "magpieimage=non-existent-image", fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_Container_FailDueToBadHealthProbe(t *testing.T) {
	template := "testdata/corerp-resources-container-bad-healthprobe.bicep"
	name := "corerp-resources-container-bad-healthprobe"
	appNamespace := "corerp-resources-container-bad-healthprobe"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			// The recipe-driven Radius.Compute/containers type provisions the Kubernetes
			// Deployment successfully even when the health probes never pass, so the deployment
			// itself succeeds. Skip object validation (the pod never becomes ready) and instead
			// assert the failing liveness probe drives the container into CrashLoopBackOff.
			SkipObjectValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "ctnr-bad-healthprobe",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				label := fmt.Sprintf("radapp.io/application=%s", name)
				require.Eventually(t, func() bool {
					pods, err := test.Options.K8sClient.CoreV1().Pods(appNamespace).List(ctx, metav1.ListOptions{
						LabelSelector: label,
					})
					if err != nil || len(pods.Items) == 0 {
						return false
					}
					for _, pod := range pods.Items {
						for _, cs := range pod.Status.ContainerStatuses {
							if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
								t.Logf("validated pod %s container %s in state CrashLoopBackOff", pod.Name, cs.Name)
								return true
							}
							if cs.RestartCount > 0 {
								t.Logf("validated pod %s container %s restarted %d times", pod.Name, cs.Name, cs.RestartCount)
								return true
							}
						}
					}
					return false
				}, 3*time.Minute, 5*time.Second, "expected container to enter CrashLoopBackOff due to failing health probe")
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_Container_Secrets(t *testing.T) {
	template := "testdata/corerp-resources-container-secrets.bicep"
	name := "corerp-resources-container-secrets"
	appNamespace := "corerp-resources-container-secrets"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "cntr-cntr-secrets",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
					{
						Name: "saltysecret",
						Type: validation.SecuritySecretsResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "cntr-cntr-secrets"),
					},
				},
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}
