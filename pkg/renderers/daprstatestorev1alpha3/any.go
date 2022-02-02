// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha3

import (
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
)

func GetDaprStateStoreAzureAny(resource renderers.RendererResource) ([]outputresource.OutputResource, error) {
	resource.Definition["managed"] = true
	return GetDaprStateStoreAzureStorage(resource)
}

func GetDaprStateStoreKubernetesAny(resource renderers.RendererResource) ([]outputresource.OutputResource, error) {
	resource.Definition["managed"] = true
	return GetDaprStateStoreKubernetesRedis(resource)
}
