// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetesmetadata

import (
	"context"
	"errors"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

// Renderer is the renderers.Renderer implementation for the kubernetesmetadata extension.
type Renderer struct {
	Inner renderers.Renderer
}

// GetDependencyIDs returns dependencies for the container/other datamodel passed in
func (r *Renderer) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	// Let the inner renderer do its work
	return r.Inner.GetDependencyIDs(ctx, resource)
}

// Render augments the container's kubernetes output resource with value for kubernetesmetadata replica if applicable.
func (r *Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Let the inner renderer do its work
	output, err := r.Inner.Render(ctx, dm, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	var kubeMetadataExt = &datamodel.BaseKubernetesMetadataExtension{}
	for _, e := range resource.Properties.Extensions {
		switch e.Kind {
		case datamodel.KubernetesMetadata:
			kubeMetadataExt = e.KubernetesMetadata
			break
		}
	}

	if kubeMetadataExt == nil {
		return renderers.RendererOutput{}, nil
	}

	for _, ores := range output.Resources {
		if ores.ResourceType.Provider != resourcemodel.ProviderKubernetes {
			// Not a Kubernetes resource
			continue
		}
		o, ok := ores.Resource.(runtime.Object)
		if !ok {
			return renderers.RendererOutput{}, errors.New("found Kubernetes resource with non-Kubernetes payload")
		}

		// Here we will update to reading from Render.Options, potentially retrieving from Env and App Annotations
		if kubeMetadataExt.Annotations != nil {
			metaAnnotations, specAnnotations, err := getAnnotations(o)
			if err != nil {
				return renderers.RendererOutput{}, err
			}
			metaAnnotations = labels.Merge(metaAnnotations, kubeMetadataExt.Annotations)
			specAnnotations = labels.Merge(specAnnotations, kubeMetadataExt.Annotations)
			err = setAnnotations(o, metaAnnotations, specAnnotations)
			if err != nil {
				return renderers.RendererOutput{}, err
			}
		}

		if kubeMetadataExt.Labels != nil {
			metaLabels, specLabels, err := getLabels(o)
			if err != nil {
				return renderers.RendererOutput{}, err
			}
			metaLabels = labels.Merge(metaLabels, kubeMetadataExt.Labels)
			specLabels = labels.Merge(specLabels, kubeMetadataExt.Labels)
			err = setLabels(o, metaLabels, specLabels)
			if err != nil {
				return renderers.RendererOutput{}, err
			}
		}

	}

	return output, nil
}

func getAnnotations(o runtime.Object) (map[string]string, map[string]string, error) {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return nil, nil, errors.New("getting annotations-cannot cast runtime.Object to v1/Deployment")
	}

	var (
		depMetaAnnotations map[string]string
		depSpecAnnotations map[string]string
	)

	if dep.Annotations != nil {
		depMetaAnnotations = dep.Annotations
	}
	if dep.Spec.Template.Annotations != nil {
		depSpecAnnotations = dep.Spec.Template.Annotations
	}

	return depMetaAnnotations, depSpecAnnotations, nil
}

func getLabels(o runtime.Object) (map[string]string, map[string]string, error) {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return nil, nil, errors.New("getting labels-cannot cast runtime.Object to v1/Deployment")
	}

	var (
		depMetaLabels map[string]string
		depSpecLabels map[string]string
	)

	if dep.Labels != nil {
		depMetaLabels = dep.Labels
	}
	if dep.Spec.Template.Labels != nil {
		depSpecLabels = dep.Spec.Template.Labels
	}

	return depMetaLabels, depSpecLabels, nil
}

// setLabels sets the value of labels
func setLabels(o runtime.Object, metaLabels map[string]string, specLabels map[string]string) error {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return errors.New("setting labels-cannot cast runtime.Object to v1/Deployment")
	}

	dep.SetLabels(metaLabels)
	dep.Spec.Template.Labels = specLabels
	return nil
}

// setAnnotations sets the value of annotations/labels
func setAnnotations(o runtime.Object, metaAnnotations map[string]string, specAnnotations map[string]string) error {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return errors.New("setting annotations-cannot cast runtime.Object to v1/Deployment")
	}

	dep.SetAnnotations(metaAnnotations)
	dep.Spec.Template.Annotations = specAnnotations
	return nil
}
