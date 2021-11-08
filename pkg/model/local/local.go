// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package local

import (
	model "github.com/Azure/radius/pkg/model/typesv1alpha3"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/httproutev1alpha3"
	"github.com/Azure/radius/pkg/renderers/mongodbv1alpha3"
	"github.com/Azure/radius/pkg/renderers/servicev1alpha3"
)

func NewLocalModel() model.ApplicationModel {
	r := map[string]renderers.Renderer{
		servicev1alpha3.ResourceType:   &servicev1alpha3.LocalRenderer{},
		mongodbv1alpha3.ResourceType:   &mongodbv1alpha3.AzureRenderer{},
		httproutev1alpha3.ResourceType: &httproutev1alpha3.LocalRenderer{},
	}

	handlers := map[string]model.Handlers{}
	transformers := map[string]renderers.SecretValueTransformer{
		mongodbv1alpha3.CosmosMongoResourceType.Type(): &mongodbv1alpha3.AzureTransformer{},
	}
	return model.NewModel(r, handlers, transformers)
}
