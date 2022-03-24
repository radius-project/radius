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
	"github.com/project-radius/radius/pkg/renderers/daprsecretstorev1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprSecretStoreGeneric(t *testing.T) {
	application := "azure-resources-dapr-secretstore-generic"
	template := "testdata/azure-resources-dapr-secretstore-generic.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor:           azuretest.NewDeployStepExecutor(template),
			AzureResources:     &validation.AzureResourceSet{},
			SkipAzureResources: true,
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "myapp",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "secretstore-generic",
						ResourceType:    daprsecretstorev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDaprSecretStoreGeneric: validation.NewOutputResource(outputresource.LocalIDDaprSecretStoreGeneric, outputresource.TypeKubernetes, resourcekinds.DaprSecretStoreGeneric, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Objects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "myapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
