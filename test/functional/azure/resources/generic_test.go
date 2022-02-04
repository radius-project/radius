// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/genericv1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/validation"
)

func Test_GenericResource(t *testing.T) {
	application := "azure-resources-generic"
	template := "testdata/azure-resources-generic.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor:       azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "twilio",
						ResourceType:    genericv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDGeneric: validation.NewOutputResource(outputresource.LocalIDGeneric, outputresource.TypeConfig, resourcekinds.Generic, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "myapp",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForResource(application, "myapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
