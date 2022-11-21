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
		o, ok := ores.Resource.(runtime.Object)
		if !ok {
			return renderers.RendererOutput{}, errors.New("found Kubernetes resource with non-Kubernetes payload")
		}

		var (
			inputAnnotations map[string]string
			inputLabels      map[string]string
		)

		if kubeMetadataExt != nil && kubeMetadataExt.Annotations != nil {
			inputAnnotations = kubeMetadataExt.Annotations
		}

		metaAnnotations, specAnnotations := getAnnotations(o)

		// Merge cumulative annotation values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, rightmost entity wins
		metaAnnotations, specAnnotations = mergeKubernetesMetadataAnnotations(options, inputAnnotations, metaAnnotations, specAnnotations)

		if !(len(metaAnnotations) == 0 && len(specAnnotations) == 0) {
			setAnnotations(o, metaAnnotations, specAnnotations)
		}

		if kubeMetadataExt != nil && kubeMetadataExt.Labels != nil {
			inputLabels = kubeMetadataExt.Labels
		}

		metaLabels, specLabels := getLabels(o)

		// Merge cumulative label values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, rightmost entity wins
		metaLabels, specLabels = mergeKubernetesMetadataLabels(options, inputLabels, metaLabels, specLabels)

		if !(len(metaLabels) == 0 && len(specLabels) == 0) {
			setLabels(o, metaLabels, specLabels)
		}

	}

	return output, nil
}

func getAnnotations(o runtime.Object) (map[string]string, map[string]string) {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return nil, nil
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

	return depMetaAnnotations, depSpecAnnotations
}

func getLabels(o runtime.Object) (map[string]string, map[string]string) {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return nil, nil
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

	return depMetaLabels, depSpecLabels
}

// setLabels sets the value of labels
func setLabels(o runtime.Object, metaLabels map[string]string, specLabels map[string]string) {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return
	}

	if len(metaLabels) > 0 {
		dep.SetLabels(metaLabels)
	}

	if len(specLabels) > 0 {
		dep.Spec.Template.Labels = specLabels
	}
}

// setAnnotations sets the value of annotations/labels
func setAnnotations(o runtime.Object, metaAnnotations map[string]string, specAnnotations map[string]string) {
	dep, ok := o.(*appsv1.Deployment)
	if !ok {
		return
	}

	if len(metaAnnotations) > 0 {
		dep.SetAnnotations(metaAnnotations)
	}

	if len(specAnnotations) > 0 {
		dep.Spec.Template.Annotations = specAnnotations
	}
}

// mergeKubernetesMetadataAnnotations merges environment, application annotations with current values
func mergeKubernetesMetadataAnnotations(options renderers.RenderOptions, currAnnotations map[string]string, metaAnnotations map[string]string, specAnnotations map[string]string) (map[string]string, map[string]string) {
	envOpts := &options.Environment
	appOpts := &options.Application
	mergeAnnotations := map[string]string{}

	if envOpts != nil && envOpts.KubernetesMetadata.Annotations != nil {
		mergeAnnotations = envOpts.KubernetesMetadata.Annotations
	}
	if appOpts != nil && appOpts.KubernetesMetadata.Annotations != nil {
		// mergeAnnotations is now updated with merged map.
		mergeAnnotations = labels.Merge(mergeAnnotations, appOpts.KubernetesMetadata.Annotations)
	}

	// Cumulative Env+App Annotations is now merged with input annotations. metaAnnotations and specAnnotations are subsequently merged with the result map.
	mergeAnnotations = labels.Merge(mergeAnnotations, currAnnotations)
	metaAnnotations = labels.Merge(metaAnnotations, mergeAnnotations)
	specAnnotations = labels.Merge(specAnnotations, mergeAnnotations)

	return metaAnnotations, specAnnotations
}

// mergeKubernetesMetadataLabels merges environment, application labels with current values
func mergeKubernetesMetadataLabels(options renderers.RenderOptions, currLabels map[string]string, metaLabels map[string]string, specLabels map[string]string) (map[string]string, map[string]string) {
	envOpts := &options.Environment
	appOpts := &options.Application
	mergeLabels := map[string]string{}

	if envOpts != nil && envOpts.KubernetesMetadata.Labels != nil {
		mergeLabels = envOpts.KubernetesMetadata.Labels
	}
	if appOpts != nil && appOpts.KubernetesMetadata.Labels != nil {
		// mergeLabels is now updated with merged map.
		mergeLabels = labels.Merge(mergeLabels, appOpts.KubernetesMetadata.Labels)
	}

	// Cumulative Env+App Labels is now merged with input labels. metaLabels and specLabels are subsequently merged with the result map.
	mergeLabels = labels.Merge(mergeLabels, currLabels)
	metaLabels = labels.Merge(metaLabels, mergeLabels)
	specLabels = labels.Merge(specLabels, mergeLabels)

	return metaLabels, specLabels
}
