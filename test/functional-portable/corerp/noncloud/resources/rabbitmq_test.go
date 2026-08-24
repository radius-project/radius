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

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Test_RabbitMQ deploys a Radius.Messaging/rabbitMQ resource using the default
// preview-environment recipe pack and validates the recipe-provisioned broker.
func Test_RabbitMQ(t *testing.T) {
	template := "testdata/corerp-resources-rabbitmq.bicep"
	name := "corerp-resources-rabbitmq"
	resourceName := "rabbitmq"
	appNamespace := "corerp-resources-rabbitmq"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: resourceName,
						Type: validation.MessagingRabbitMQResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, resourceName),
						validation.NewK8sServiceForResource(name, resourceName),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				labelset := kubernetes.MakeSelectorLabels(name, resourceName)
				deployments, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err)
				require.Len(t, deployments.Items, 1, "expected one RabbitMQ Deployment")

				deployment := deployments.Items[0]
				require.Equal(t, int32(1), deployment.Status.AvailableReplicas, "RabbitMQ Deployment should have one available replica")
				require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
				require.Equal(t, "rabbitmq", deployment.Spec.Template.Spec.Containers[0].Name)
				require.Len(t, deployment.Spec.Template.Spec.Containers[0].Ports, 1)
				require.Equal(t, int32(5672), deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}
