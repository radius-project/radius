/*
Copyright 2026 The Radius Authors.

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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/recipepack"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_ContainerImages_DefaultRecipePack verifies that the default recipe pack can build an image
// with Radius.Compute/containerImages and that a Radius.Compute/containers resource can run it with
// no registry configuration. It relies on the Helm chart defaults: the BuildKit sidecar, the
// in-cluster registry exposed on NodePort 31500, and the dynamic-rp loopback proxy.
func Test_ContainerImages_DefaultRecipePack(t *testing.T) {
	if os.Getenv("RADIUS_TEST_EXTERNAL_KUBECONFIG") != "" {
		t.Skip("the in-cluster registry is only reachable from control-plane cluster nodes")
	}

	template := "testdata/containerimages-default-pack.bicep"
	name := "containerimages-default"
	appName := "containerimages-default-app"
	imageName := "defaultpackimage"
	containerName := "defaultpackcntr"

	preSetup, envID := rp.NewPreviewEnvPreSetup(name, rp.NewRPTestOptions(t).Workspace.Scope, name)

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, fmt.Sprintf("environment=%s", envID)),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: appName,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: imageName,
						Type: "radius.compute/containerimages",
						App:  appName,
					},
					{
						Name: containerName,
						Type: validation.ComputeContainersResource,
						App:  appName,
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				resource, err := ct.Options.ManagementClient.GetResource(ctx, "Radius.Compute/containerImages", imageName)
				require.NoError(t, err)
				imageReference, _ := resource.Properties["imageReference"].(string)
				require.Equal(t, recipepack.DefaultContainerImagesRegistry+"/"+imageName+":functest", imageReference)

				// The image exits immediately, so wait for kubelet to pull it rather than for readiness.
				require.EventuallyWithT(t, func(c *assert.CollectT) {
					pods, err := ct.Options.K8sClient.CoreV1().Pods(name).List(ctx, metav1.ListOptions{})
					if !assert.NoError(c, err) {
						return
					}
					pulled := false
					for _, pod := range pods.Items {
						for _, status := range pod.Status.ContainerStatuses {
							if status.Image == imageReference && status.ImageID != "" {
								pulled = true
							}
							if waiting := status.State.Waiting; waiting != nil {
								assert.False(c, strings.Contains(waiting.Reason, "ImagePull") || waiting.Reason == "ErrImageNeverPull", "image pull failed: %s", waiting.Message)
							}
						}
					}
					assert.True(c, pulled, "kubelet has not pulled %s yet", imageReference)
				}, 3*time.Minute, 5*time.Second)
			},
		},
	})

	test.PreSetup = preSetup
	test.Test(t)
}
