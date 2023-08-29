/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package azure

import (
	"context"
	"errors"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	csiv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	errInvalidKeyVaultResourceID = errors.New("failed to parse KeyVault ResourceID. Unable to create secret provider class")
	errUnsupportedIdentityKind   = errors.New("unsupported identity kind")
)

// MakeKeyVaultVolumeSpec creates a Volume and VolumeMount spec for a secret store volume using the given volumeName,
// mountPath and spcName and returns them along with a nil error.
func MakeKeyVaultVolumeSpec(volumeName string, mountPath, spcName string) (corev1.Volume, corev1.VolumeMount, error) {
	// Make Volume Spec which uses the SecretProvider created above
	volumeSpec := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver: "secrets-store.csi.k8s.io",
				// We will support only Read operations
				ReadOnly: to.Ptr(true),
				VolumeAttributes: map[string]string{
					"secretProviderClass": spcName,
				},
			},
		},
	}

	// Make Volume mount spec
	volumeMountSpec := corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		// We will support only reads to the secret store volume
		ReadOnly: true,
	}

	return volumeSpec, volumeMountSpec, nil
}

// TransformSecretProviderClass updates the clientID and tenantID for azure workload identity.
func TransformSecretProviderClass(ctx context.Context, options *handlers.PutOptions) error {
	spc, ok := options.Resource.CreateResource.Data.(*csiv1.SecretProviderClass)
	if !ok {
		return errors.New("cannot transform service account")
	}

	// Update the clientID and tenantID only for azure workload identity.
	if spc.Annotations != nil && spc.Annotations[kubernetes.AnnotationIdentityType] == string(rpv1.AzureIdentityWorkload) {
		clientID, tenantID, err := extractIdentityInfo(options)
		if err != nil {
			return err
		}

		spc.Spec.Parameters["clientID"] = clientID
		spc.Spec.Parameters["tenantID"] = tenantID
	}

	return nil
}

// MakeKeyVaultSecretProviderClass creates a SecretProviderClass object for an Azure KeyVault resource and returns an
// OutputResource with the ServiceAccount as a dependency.
func MakeKeyVaultSecretProviderClass(appName, name string, res *datamodel.VolumeResource, objSpec string, envOpt *renderers.EnvironmentOptions) (*rpv1.OutputResource, error) {
	prop := res.Properties.AzureKeyVault

	kvResourceID, err := resources.ParseResource(prop.Resource)
	if err != nil {
		return nil, errInvalidKeyVaultResourceID
	}

	params := map[string]string{
		"usePodIdentity": "false",
		"keyvaultName":   kvResourceID.Name(),
		"objects":        objSpec,
	}

	switch envOpt.Identity.Kind {
	case rpv1.AzureIdentityWorkload:
		break

	default:
		return nil, errUnsupportedIdentityKind
	}

	secretProvider := &csiv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecretProviderClass",
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.NormalizeResourceName(name),
			Namespace: envOpt.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(appName, res.Name, res.Type),
			Annotations: map[string]string{
				kubernetes.AnnotationIdentityType: string(envOpt.Identity.Kind),
			},
		},
		Spec: csiv1.SecretProviderClassSpec{
			Provider:   "azure",
			Parameters: params,
		},
	}

	or := rpv1.NewKubernetesOutputResource(rpv1.LocalIDSecretProviderClass, secretProvider, secretProvider.ObjectMeta)
	or.CreateResource.Dependencies = []string{rpv1.LocalIDServiceAccount}

	return &or, nil

}
