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

	var kubeMetadataExt *datamodel.BaseKubernetesMetadataExtension
	for _, e := range resource.Properties.Extensions {
		switch e.Kind {
		case datamodel.KubernetesMetadata:
			kubeMetadataExt = e.KubernetesMetadata
		}
	}

	if kubeMetadataExt == nil {
		return output, nil
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

		// Cascade cumulative values from Env->App->Container kubernetes metadata. In case of collisions, rightmost entity wins
		// Merge env & application annotations and label maps
		mergeAnnotations, mergeLabels := mergeEnvAppKubernetesMetadataMaps(options)

		if kubeMetadataExt.Annotations != nil {
			metaAnnotations, specAnnotations, err := getAnnotations(o)
			if err != nil {
				return renderers.RendererOutput{}, err
			}

			// Merge cascaded annotations with current map
			metaAnnotations, specAnnotations = mergeKubernetesMetadataMaps(mergeAnnotations, kubeMetadataExt.Annotations, metaAnnotations, specAnnotations)
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

			// Merge cascaded labels with current map
			metaLabels, specLabels = mergeKubernetesMetadataMaps(mergeLabels, kubeMetadataExt.Labels, metaLabels, specLabels)
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

// mergeEnvAppKubernetesMetadataMaps merges environment, application annotations/labels
func mergeEnvAppKubernetesMetadataMaps(options renderers.RenderOptions) (map[string]string, map[string]string) {
	envOpts := &options.Environment
	appOpts := &options.Application
	mergeAnnotations := map[string]string{}
	mergeLabels := map[string]string{}

	if envOpts != nil && envOpts.KubernetesMetadata.Annotations != nil {
		mergeAnnotations = envOpts.KubernetesMetadata.Annotations
	}
	if envOpts != nil && envOpts.KubernetesMetadata.Labels != nil {
		mergeLabels = envOpts.KubernetesMetadata.Labels
	}
	if appOpts != nil && appOpts.KubernetesMetadata.Annotations != nil {
		mergeAnnotations = labels.Merge(mergeAnnotations, appOpts.KubernetesMetadata.Annotations)
	}
	if appOpts != nil && appOpts.KubernetesMetadata.Labels != nil {
		mergeLabels = labels.Merge(mergeLabels, appOpts.KubernetesMetadata.Labels)
	}

	return mergeAnnotations, mergeLabels
}

// mergeMaps merges meta, spec annotations/labels in the sequence env+app map->current map
func mergeKubernetesMetadataMaps(cascadeMap map[string]string, currMap map[string]string, metaMap map[string]string, specMap map[string]string) (map[string]string, map[string]string) {
	currMap = labels.Merge(cascadeMap, currMap)
	metaMap = labels.Merge(metaMap, currMap)
	specMap = labels.Merge(specMap, currMap)

	return metaMap, specMap
}
