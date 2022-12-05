// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetesnamespace

import (
	"context"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	appsv1 "k8s.io/api/apps/v1"
)

// Renderer is the renderes.Renderer implementation for the kubernetesNamespaceOverride extension.
type Renderer struct {
	Inner renderers.Renderer
}

// GetDependencyIDs returns dependencies for the datamodel passed in
func (r *Renderer) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return r.Inner.GetDependencyIDs(ctx, resource)
}

// Render augments the applications's kubernetes output resource with value for kubernetesnamespaceoverride replica if applicable.
func (r *Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Let the inner renderer do its work
	output, err := r.Inner.Render(ctx, dm, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resource, ok := dm.(*datamodel.Application)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	var kubeNamespaceExt *datamodel.KubeNamespaceOverrideExtension
	for _, e := range resource.Properties.Extensions {
		switch e.Kind {
		case datamodel.KubernetesNamespaceOverride:
			kubeNamespaceExt = e.KubernetesNamespaceOverride
		default:
			continue
		}
		break
	}

	for _, ores := range output.Resources {
		if ores.ResourceType.Provider != resourcemodel.ProviderKubernetes {
			// Not a Kubernetes resource
			continue
		}

		dep, ok := ores.Resource.(*appsv1.Deployment)
		if !ok {
			continue
		}

		var namespace string
		if kubeNamespaceExt != nil && kubeNamespaceExt.Namespace != "" {
			namespace = kubeNamespaceExt.Namespace
		}

		dep.Spec.Template.Namespace = namespace
	}

	return output, nil
}
