// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container

import (
	"context"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r Renderer) makeServiceAccountForVolume(appName, name, namespace, clientID, tenantID string, resource *datamodel.ContainerResource) (outputresource.OutputResource, error) {
	labels := kubernetes.MakeDescriptiveLabels(appName, resource.Name, resource.Type)
	labels["azure.workload.identity/use"] = "true"

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"azure.workload.identity/client-id": clientID,
				"azure.workload.identity/tenant-id": tenantID,
			},
		},
	}

	or := outputresource.NewKubernetesOutputResource(
		resourcekinds.ServiceAccount,
		outputresource.LocalIDServiceAccount,
		sa,
		sa.ObjectMeta)

	or.Dependencies = []outputresource.Dependency{
		{LocalID: outputresource.LocalIDUserAssignedManagedIdentity},
	}

	return or, nil
}

// Assigns roles/permissions to a specific resource for the managed identity resource.
func (r Renderer) makeRoleAssignmentsForAzureKeyVaultCSIDriver(ctx context.Context, keyVaultID string, roleNames []string) ([]outputresource.OutputResource, error) {
	outputResources := []outputresource.OutputResource{}
	for _, roleName := range roleNames {
		roleAssignment := outputresource.OutputResource{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: resourcemodel.ProviderAzure,
			},
			LocalID:  outputresource.GenerateLocalIDForRoleAssignment(keyVaultID, roleName),
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         roleName,
				handlers.RoleAssignmentScope: keyVaultID,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
				},
			},
		}

		outputResources = append(outputResources, roleAssignment)
	}

	return outputResources, nil
}

// Create the volume specs for Pod.

func (r Renderer) makeEphemeralVolume(volumeName string, volume *datamodel.EphemeralVolume) (corev1.Volume, corev1.VolumeMount, error) {
	// Make volume spec
	volumeSpec := corev1.Volume{}
	volumeSpec.Name = volumeName
	volumeSpec.VolumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{}
	if volume != nil && volume.ManagedStore == datamodel.ManagedStoreMemory {
		volumeSpec.VolumeSource.EmptyDir.Medium = corev1.StorageMediumMemory
	} else {
		volumeSpec.VolumeSource.EmptyDir.Medium = corev1.StorageMediumDefault
	}

	// Make volumeMount spec
	volumeMountSpec := corev1.VolumeMount{}
	volumeMountSpec.MountPath = volume.MountPath
	volumeMountSpec.Name = volumeName

	return volumeSpec, volumeMountSpec, nil
}

func (r Renderer) makeAzureFileSharePersistentVolume(volumeName string, persistentVolume *datamodel.PersistentVolume, applicationName string, options renderers.RenderOptions) (corev1.Volume, corev1.VolumeMount, error) { //nolint:all
	// Make volume spec
	volumeSpec := corev1.Volume{}
	volumeSpec.Name = volumeName
	volumeSpec.VolumeSource.AzureFile = &corev1.AzureFileVolumeSource{}
	volumeSpec.AzureFile.SecretName = applicationName
	resourceID, err := resources.ParseResource(persistentVolume.Source)
	if err != nil {
		return corev1.Volume{}, corev1.VolumeMount{}, err
	}
	shareName := resourceID.TypeSegments()[2].Name
	volumeSpec.AzureFile.ShareName = shareName

	// Make volumeMount spec
	volumeMountSpec := corev1.VolumeMount{}
	volumeMountSpec.Name = volumeName
	if persistentVolume != nil && persistentVolume.Permission == datamodel.VolumePermissionRead {
		volumeMountSpec.MountPath = persistentVolume.MountPath
		volumeMountSpec.ReadOnly = true
	}
	return volumeSpec, volumeMountSpec, nil
}

func (r Renderer) makeAzureKeyVaultPersistentVolume(volumeName string, keyvaultVolume *datamodel.PersistentVolume, secretProviderClassName string, options renderers.RenderOptions) (corev1.Volume, corev1.VolumeMount, error) {
	// Make Volume Spec which uses the SecretProvider created above
	volumeSpec := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver: "secrets-store.csi.k8s.io",
				// We will support only Read operations
				ReadOnly: to.Ptr(true),
				VolumeAttributes: map[string]string{
					"secretProviderClass": secretProviderClassName,
				},
			},
		},
	}

	// Make Volume mount spec
	volumeMountSpec := corev1.VolumeMount{
		Name:      volumeName,
		MountPath: keyvaultVolume.MountPath,
		// We will support only reads to the secret store volume
		ReadOnly: true,
	}

	return volumeSpec, volumeMountSpec, nil
}
