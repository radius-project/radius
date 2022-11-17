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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	extensions := resource.Properties.Extensions
	for _, e := range extensions {
		switch e.Kind {
		case datamodel.KubernetesMetadata:
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
				//
				if e.KubernetesMetadata != nil {
					if e.KubernetesMetadata.Annotations != nil {
						if annotations, ok := r.getAnnotations(o); ok {
							var ann map[string]string
							ann = labels.Merge(annotations, e.KubernetesMetadata.Annotations)
							r.setLabelsAnnotations(o, ann, false)
						}
					}

					if e.KubernetesMetadata.Labels != nil {
						if lbls, ok := r.getLabels(o); ok {
							var lbl map[string]string
							lbl = labels.Merge(lbls, e.KubernetesMetadata.Labels)
							// r.setLabelsAnnotations(o, ann, true) -- real call
							r.setLabels(o, lbl, true)
						}
					}

				}
			}
		default:
			continue
		}
		break
	}

	return output, nil
}

func (r *Renderer) getAnnotations(o runtime.Object) (map[string]string, bool) {
	if dep, ok := o.(*appsv1.Deployment); ok {
		if dep.Spec.Template.Annotations == nil {
			dep.Spec.Template.Annotations = map[string]string{}
		}

		return dep.Spec.Template.Annotations, true
	}

	if un, ok := o.(*unstructured.Unstructured); ok {
		if a := un.GetAnnotations(); a != nil {
			return a, true
		}

		return map[string]string{}, true
	}

	return nil, false
}

// setAnnotations sets the value of annotations/labels
func (r *Renderer) setLabelsAnnotations(o runtime.Object, keyvalue map[string]string, isLabel bool) {
	// this is unable to be cast.. why why why?7
	un, ok := o.(*unstructured.Unstructured)
	if ok {
		if isLabel {
			un.SetLabels(keyvalue)
		} else {
			un.SetAnnotations(keyvalue)
		}
	}
}

// setAnnotations sets the value of annotations/labels
// this works but we should try and make unstructured.Unstructured (setLabelsAnnotations above) work
func (r *Renderer) setLabels(o runtime.Object, keyvalue map[string]string, isLabel bool) {
	if dep, ok := o.(*appsv1.Deployment); ok {
		dep.Spec.Template.Labels = keyvalue
	}
}

func (r *Renderer) getLabels(o runtime.Object) (map[string]string, bool) {
	if dep, ok := o.(*appsv1.Deployment); ok {
		if dep.Spec.Template.Labels == nil {
			dep.Spec.Template.Labels = map[string]string{}
		}

		return dep.Spec.Template.Labels, true
	}

	if un, ok := o.(*unstructured.Unstructured); ok {
		if a := un.GetLabels(); a != nil {
			return a, true
		}

		return map[string]string{}, true
	}

	return nil, false
}
