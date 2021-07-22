// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mechanics_test

import (
	"fmt"
	"testing"

	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

// Tests that we can add a component to a deployed application
// by redeploying with more components.
func Test_RedeployWithAnotherComponent(t *testing.T) {
	application := "azure-mechanics-redeploy-withanothercomponent"
	templateFmt := "testdata/azure-mechanics-redeploy-withanothercomponent.step%d.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 1)),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					// None
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "a",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "a"),
						validation.NewK8sObjectForComponent(application, "b"),
					},
				},
			},
		},
		{
			Executor: azuretest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 2)),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					// None
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "a",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "b",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "a"),
						validation.NewK8sObjectForComponent(application, "b"),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Tests that we can delete a component from a deployed application
// by redeploying with fewer components.
func Test_RedeployWithoutComponent(t *testing.T) {
	application := "azure-mechanics-redeploy-withoutcomponent"
	templateFmt := "testdata/azure-mechanics-redeploy-withoutcomponent.step%d.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 1)),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					// None
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "a",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "b",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "a"),
						validation.NewK8sObjectForComponent(application, "b"),
					},
				},
			},
		},
		{
			Executor: azuretest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 2)),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					// None
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "a",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "a"),
					},
				},
			},
		},
	})

	test.Test(t)
}
