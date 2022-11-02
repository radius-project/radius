// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/ucp/resources"

	corev1 "k8s.io/api/core/v1"
)

// MakeAzureFileShareVolumeSpec builds volume spec for Azure file share.
// TODO: This is unused code now. We will enable file share later.
func MakeAzureFileShareVolumeSpec(volumeName string, persistentVolume *datamodel.PersistentVolume, applicationName string, options renderers.RenderOptions) (corev1.Volume, corev1.VolumeMount, error) { //nolint:all
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
