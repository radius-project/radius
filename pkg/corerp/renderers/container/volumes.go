// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container

import (
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/ucp/resources"

	corev1 "k8s.io/api/core/v1"
)

// Create the volume specs for Pod.
func makeEphemeralVolume(volumeName string, volume *datamodel.EphemeralVolume) (corev1.Volume, corev1.VolumeMount, error) {
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

func makeAzureFileSharePersistentVolume(volumeName string, persistentVolume *datamodel.PersistentVolume, applicationName string, options renderers.RenderOptions) (corev1.Volume, corev1.VolumeMount, error) { //nolint:all
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
