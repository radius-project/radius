// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azurefilesharev1alpha3

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
)

const (
	VolumeKindEphemeral  = "ephemeral"
	VolumeKindPersistent = "persistent"
)

var storageAccountDependency outputresource.Dependency = outputresource.Dependency{
	LocalID: outputresource.LocalIDAzureFileShareStorageAccount,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	Arm armauth.ArmConfig
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	// properties, err := r.convert(resource)
	// if err != nil {
	// 	return nil, err
	// }
	// fmt.Println(properties)
	// deps := []azresources.ResourceID{}
	// resourceID, err := azresources.Parse(properties.Resource)
	// if err != nil {
	// 	return nil, err
	// }
	// deps = append(deps, resourceID)
	// return deps, nil
	return nil, nil
}

func (r Renderer) convert(resource renderers.RendererResource) (*AzureFileShare, error) {
	properties := &AzureFileShare{}
	err := resource.ConvertDefinition(properties)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := VolumeProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resources := []outputresource.OutputResource{}
	if properties.Managed {
		//TODO
	} else {
		results, err := RenderUnmanaged(resource.ResourceName, properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		resources = append(resources, results...)
	}

	fileshareID, err := renderers.ValidateResourceID(properties.Resource, AzureFileShareResourceType, "Azure File Share")
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Truncate the fileservices/shares part of the ID to make an ID for the account
	storageAccountID := fileshareID.Truncate().Truncate()

	computedValues, secretValues := MakeSecretsAndValues(storageAccountID.Types[0].Name)

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func RenderUnmanaged(name string, properties VolumeProperties) ([]outputresource.OutputResource, error) {
	if properties.Resource == "" {
		return nil, renderers.ErrResourceMissingForUnmanagedResource
	}

	fileshareID, err := renderers.ValidateResourceID(properties.Resource, AzureFileShareResourceType, "Azure File Share")
	if err != nil {
		return nil, err
	}

	// Truncate the fileservices/shares part of the ID to make an ID for the account
	storageAccountID := fileshareID.Truncate().Truncate()

	storageAccountResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureFileShareStorageAccount,
		ResourceKind: resourcekinds.AzureFileShareStorageAccount,
		Resource: map[string]string{
			handlers.ManagedKey:                     "false",
			handlers.FileShareStorageAccountIDKey:   storageAccountID.ID,
			handlers.FileShareStorageAccountNameKey: storageAccountID.Types[0].Name,
		},
	}

	fileshareResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureFileShare,
		ResourceKind: resourcekinds.AzureFileShare,
		Resource: map[string]string{
			handlers.ManagedKey:                     "false",
			handlers.FileShareStorageAccountIDKey:   storageAccountID.ID,
			handlers.FileShareStorageAccountNameKey: storageAccountID.Types[0].Name,
			handlers.FileShareIDKey:                 fileshareID.ID,
			handlers.FileShareNameKey:               fileshareID.Types[2].Name,
		},
		Dependencies: []outputresource.Dependency{storageAccountDependency},
	}
	return []outputresource.OutputResource{storageAccountResource, fileshareResource}, nil
}

func MakeSecretsAndValues(name string) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	computedValues := map[string]renderers.ComputedValueReference{
		StorageAccountName: {
			LocalID: outputresource.LocalIDAzureFileShareStorageAccount,
			Value:   name,
		},
	}
	secretValues := map[string]renderers.SecretValueReference{
		StorageKeyValue: {
			LocalID: storageAccountDependency.LocalID,
			// https://docs.microsoft.com/en-us/rest/api/storagerp/storage-accounts/list-keys
			Action:        "listKeys",
			ValueSelector: "/keys/0/value",
		},
	}

	return computedValues, secretValues
}

func (r Renderer) makeSecret(ctx context.Context, resource renderers.RendererResource, secrets map[string][]byte) outputresource.OutputResource {
	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resource.ResourceName,
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Type: corev1.SecretTypeOpaque,
		Data: secrets,
	}

	output := outputresource.NewKubernetesOutputResource(outputresource.LocalIDSecret, &secret, secret.ObjectMeta)
	return output
}
