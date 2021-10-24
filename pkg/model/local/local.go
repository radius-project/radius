// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package local

import (
	"github.com/Azure/radius/pkg/handlers"
	model "github.com/Azure/radius/pkg/model/typesv1alpha3"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/websitev1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewLocalModel(k8s *client.Client) model.ApplicationModel {
	renderers := map[string]renderers.Renderer{
		websitev1alpha3.ResourceType: &websitev1alpha3.LocalRenderer{},
	}

	handlers := map[string]model.Handlers{
		resourcekinds.Kubernetes: {ResourceHandler: handlers.NewKubernetesHandler(*k8s), HealthHandler: nil},
	}
	return model.NewModel(renderers, handlers)
}
