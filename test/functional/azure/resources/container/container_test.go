// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container_test

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func Test_ContainerHttpBinding(t *testing.T) {
	application := "azure-resources-container-httpbinding"
	template := "testdata/azure-resources-container-httpbinding.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "frontend",
						OutputResources: map[string]validation.OutputResourceSet{
							"Deployment": validation.NewOutputResource("Deployment", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
							"Service":    validation.NewOutputResource("Service", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "backend",
						OutputResources: map[string]validation.OutputResourceSet{
							"Deployment": validation.NewOutputResource("Deployment", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
							"Service":    validation.NewOutputResource("Service", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "frontend"),
						validation.NewK8sObjectForComponent(application, "backend"),
					},
				},
			},
			SkipARMResources: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, at azuretest.ApplicationTest) {
				appclient := radclient.NewApplicationClient(at.Options.ARMConnection, at.Options.Environment.SubscriptionID)

				// get application and verify name
				response, err := appclient.Get(ctx, at.Options.Environment.ResourceGroup, application, nil)
				require.NoError(t, err)
				assert.Equal(t, application, *response.ApplicationResource.Name)
			},
		},
	})

	test.Test(t)
}

func Test_ContainerInboundRoute(t *testing.T) {
	application := "azure-resources-container-inboundroute"
	template := "testdata/azure-resources-container-inboundroute.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "frontend"),
						validation.NewK8sObjectForComponent(application, "backend"),
					},
				},
			},
			SkipARMResources: true,
			SkipComponents:   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, at azuretest.ApplicationTest) {
				// Verify that we've created an ingress resource. We don't verify reachability because allocating
				// a public IP can take a few minutes.
				labelset := map[string]string{
					keys.LabelRadiusApplication: application,
					keys.LabelRadiusComponent:   "frontend",
				}
				matches, err := at.Options.K8sClient.NetworkingV1().Ingresses(application).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err, "failed to list ingresses")
				require.Lenf(t, matches.Items, 1, "items should contain one match, instead it had: %+v", matches.Items)
			},
		},
	})

	test.Test(t)
}
