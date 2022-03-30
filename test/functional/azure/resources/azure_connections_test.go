// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/keys"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/validation"
)

func Test_AzureConnections(t *testing.T) {
	applicationName := "azure-connection-database-service"
	containerResourceName := "db-service"
	template := "testdata/azure-connection-database-service.bicep"
	magpieImage := "magpieimage=" + os.Getenv("MAGPIE_IMAGE")
	fmt.Println("magpieImage:", magpieImage)
	test := azuretest.NewApplicationTest(t, applicationName, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template, magpieImage),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type:            "Microsoft.DocumentDB/databaseAccounts",
						AzureConnection: true,
					},
					{
						Type: azresources.ManagedIdentityUserAssignedIdentities,
						Tags: map[string]string{
							keys.TagRadiusApplication: applicationName,
							keys.TagRadiusResource:    containerResourceName,
						},
					},
				},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: applicationName,
						ResourceName:    containerResourceName,
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment:                  validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAADPodIdentity:              validation.NewOutputResource(outputresource.LocalIDAADPodIdentity, outputresource.TypeAADPodIdentity, resourcekinds.AzurePodIdentity, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDUserAssignedManagedIdentity: validation.NewOutputResource(outputresource.LocalIDUserAssignedManagedIdentity, outputresource.TypeARM, resourcekinds.AzureUserAssignedManagedIdentity, false, rest.OutputResourceStatus{}),
							// role assignment for connected azure resource
							"role-assignment-1": {
								SkipLocalIDWhenMatching: true,
								OutputResourceType:      outputresource.TypeARM,
								ResourceKind:            resourcekinds.AzureRoleAssignment,
								VerifyStatus:            false,
							},
							"role-assignment-2": {
								SkipLocalIDWhenMatching: true,
								OutputResourceType:      outputresource.TypeARM,
								ResourceKind:            resourcekinds.AzureRoleAssignment,
								VerifyStatus:            false,
							},
						},
					},
				},
			},
			Objects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					applicationName: {
						validation.NewK8sPodForResource(applicationName, containerResourceName),
					},
				},
			},
		},
	})

	test.Test(t)
}
