// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha3

import (
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
)

func GetDaprStateStoreAzureAny(resource renderers.RendererResource, properties Properties) ([]outputresource.OutputResource, error) {
	properties.Managed = true
	return GetDaprStateStoreAzureStorage(resource, properties)
}

func GetDaprStateStoreKubernetesAny(resource renderers.RendererResource, properties Properties) ([]outputresource.OutputResource, error) {
	properties.Managed = true
	return GetDaprStateStoreKubernetesRedis(resource, properties)
}
