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
				if e.KubernetesMetadata != nil && e.KubernetesMetadata.Annotations != nil {
					annotations, err := getAnnotations(o)
					if err != nil {
						return renderers.RendererOutput{}, err
					}
					annMap := labels.Merge(annotations, e.KubernetesMetadata.Annotations)
					err = setAnnotations(o, annMap)
					if err != nil {
						return renderers.RendererOutput{}, err
					}
				}

				if e.KubernetesMetadata != nil && e.KubernetesMetadata.Labels != nil {
					lbls, err := getLabels(o)
					if err != nil {
						return renderers.RendererOutput{}, err
					}
					lblMap := labels.Merge(lbls, e.KubernetesMetadata.Labels)
					err = setLabels(o, lblMap)
					if err != nil {
						return renderers.RendererOutput{}, err
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

func getAnnotations(o runtime.Object) (map[string]string, error) {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return nil, errors.New("cannot cast runtime.Object to v1/Deployment")
	}

	var retann map[string]string
	if dep.Spec.Template.Annotations != nil {
		retann = dep.Spec.Template.Annotations
	}

	return retann, nil
}

func getLabels(o runtime.Object) (map[string]string, error) {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return nil, errors.New("getcannot cast runtime.Object to v1/Deployment")
	}

	var retlbl map[string]string
	if dep.Spec.Template.Labels != nil {
		retlbl = dep.Spec.Template.Labels
	}

	return retlbl, nil
}

/* Discuss with Justin
func convertToUnstructured(ro runtime.Object) (unstructured.Unstructured, error) {
	c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ro)
	if err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("could not convert object %v to unstructured: %w", ro.GetObjectKind(), err)
	}

	return unstructured.Unstructured{Object: c}, nil
}

// setAnnotations sets the value of annotations/labels
func (r *Renderer) setLabelsAnnotations(o runtime.Object, keyvalue map[string]string, isLabel bool) (*unstructured.Unstructured, error) {
	un, err := convertToUnstructured(o)
	if err != nil {
		return nil, err
	}

	if isLabel {
		un.SetLabels(keyvalue)
	} else {
		un.SetAnnotations(keyvalue)
	}

	// After returning &un, how does one cast it back to the runtime/relevant object
	return &un, nil
}
*/

// setLabels sets the value of labels
func setLabels(o runtime.Object, lbl map[string]string) error {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return errors.New("setting labels-cannot cast runtime.Object to v1/Deployment")
	}

	dep.Spec.Template.Labels = lbl
	return nil
}

// setAnnotations sets the value of annotations/labels
func setAnnotations(o runtime.Object, ann map[string]string) error {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return errors.New("setting annotations-cannot cast runtime.Object to v1/Deployment")
	}

	dep.Spec.Template.Annotations = ann
	return nil
}
