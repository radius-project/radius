package resources_test

import (
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

func Test_AzureConnectionCognitiveService(t *testing.T) {
	applicationName := "azure-connection-cognitive-service"
	containerResourceName := "translation-service"
	template := "testdata/azure-connection-cognitive-service.bicep"
	test := azuretest.NewApplicationTest(t, applicationName, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: "Microsoft.CognitiveServices/accounts",
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
							outputresource.LocalIDDeployment:                  validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAADPodIdentity:              validation.NewOutputResource(outputresource.LocalIDAADPodIdentity, outputresource.TypeAADPodIdentity, resourcekinds.AzurePodIdentity, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDUserAssignedManagedIdentity: validation.NewOutputResource(outputresource.LocalIDUserAssignedManagedIdentity, outputresource.TypeARM, resourcekinds.AzureUserAssignedManagedIdentity, true, false, rest.OutputResourceStatus{}),
							// role assignment for connected azure resource
							"role-assignment-1": {
								SkipLocalIDWhenMatching: true,
								OutputResourceType:      outputresource.TypeARM,
								ResourceKind:            resourcekinds.AzureRoleAssignment,
								Managed:                 true,
								VerifyStatus:            false,
							},
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					applicationName: {
						validation.NewK8sObjectForResource(applicationName, containerResourceName),
					},
				},
			},
		},
	})

	test.Test(t)
}
