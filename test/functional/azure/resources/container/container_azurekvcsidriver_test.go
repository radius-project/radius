// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/keys"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/volumev1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func Test_ContainerAzureKeyVaultCSIDriver(t *testing.T) {
	application := "azure-resources-container-azurekvcsidriver"
	template := "testdata/azure-resources-container-azurekvcsidriver.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: &azuretest.DeployStepExecutor{
				Description: fmt.Sprintf("deploy %s --parameters secretValue=%s", template, "abcd1234"),
				Template:    template,
				Parameters:  []string{"secretValue=abcd1234"},
			},
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.ManagedIdentityUserAssignedIdentities,
						Tags: map[string]string{
							keys.TagRadiusApplication: application,
							keys.TagRadiusResource:    "backend",
						},
					},
				},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment:                  validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDUserAssignedManagedIdentity: validation.NewOutputResource(outputresource.LocalIDUserAssignedManagedIdentity, outputresource.TypeARM, resourcekinds.AzureUserAssignedManagedIdentity, true, false, rest.OutputResourceStatus{}),
							// Since only secrets (no keys/certs) are mounted, only one role assignment is expected
							"role-assignment-1": {
								SkipLocalIDWhenMatching: true,
								OutputResourceType:      outputresource.TypeARM,
								ResourceKind:            resourcekinds.AzureRoleAssignment,
								Managed:                 true,
								VerifyStatus:            false,
							},
							outputresource.LocalIDAADPodIdentity: validation.NewOutputResource(outputresource.LocalIDAADPodIdentity, outputresource.TypeAADPodIdentity, resourcekinds.AzurePodIdentity, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "myshare",
						ResourceType:    volumev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDSecretProviderClass: validation.NewOutputResource(outputresource.LocalIDSecretProviderClass, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForResource(application, "backend"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, at azuretest.ApplicationTest) {
				labelset := kubernetes.MakeSelectorLabels(application, "backend")

				matches, err := at.Options.K8sClient.CoreV1().Pods(application).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err, "failed to list pods")

				found := false
				var volIndex int

				// Verify persistent volume
				found = false
				for index, vol := range matches.Items[0].Spec.Volumes {
					if vol.Name == "my-kv" {
						found = true
						volIndex = index
					}
				}
				require.True(t, found, "persistent volume did not get mounted")
				volume := matches.Items[0].Spec.Volumes[volIndex]
				require.Equal(t, "secrets-store.csi.k8s.io", volume.CSI.Driver)
				require.Equal(t, map[string]string{"secretProviderClass": "myshare-sp"}, volume.CSI.VolumeAttributes)
			},
		},
	})

	test.Test(t)
}
