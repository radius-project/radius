// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetesmetadata

import (
	"context"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Renderer is the renderers.Renderer implementation for the kubernetesmetadata extension.
type Renderer struct {
	Inner renderers.Renderer
}

// GetDependencyIDs returns dependencies for the container/other datamodel passed in
func (r *Renderer) GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	// Let the inner renderer do its work
	return r.Inner.GetDependencyIDs(ctx, resource)
}

// Render augments the container's kubernetes output resource with value for kubernetesmetadata replica if applicable.
func (r *Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {

	// Let the inner renderer do its work
	output, err := r.Inner.Render(ctx, dm, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	var kubeMetadataExt *datamodel.KubeMetadataExtension
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

		dep, ok := ores.Resource.(*appsv1.Deployment)
		if !ok {
			continue
		}

		inputAnnotations := map[string]string{}
		inputLabels := map[string]string{}

		if kubeMetadataExt != nil && kubeMetadataExt.Annotations != nil {
			inputAnnotations = kubeMetadataExt.Annotations
		}

		existingMetaAnnotations, existingSpecAnnotations := getAnnotations(dep)

		// Merge cumulative annotation values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, rightmost entity wins
		existingMetaAnnotations, existingSpecAnnotations = mergeAnnotations(ctx, options, inputAnnotations, existingMetaAnnotations, existingSpecAnnotations)
		setAnnotations(dep, existingMetaAnnotations, existingSpecAnnotations)

		if kubeMetadataExt != nil && kubeMetadataExt.Labels != nil {
			inputLabels = kubeMetadataExt.Labels
		}

		existingMetaLabels, existingSpecLabels := getLabels(dep)

		// Merge cumulative label values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, rightmost entity wins
		existingMetaLabels, existingSpecLabels = mergeLabels(ctx, options, inputLabels, existingMetaLabels, existingSpecLabels)
		setLabels(dep, existingMetaLabels, existingSpecLabels)

	}

	return output, nil
}

func getAnnotations(dep *appsv1.Deployment) (map[string]string, map[string]string) {
	depMetaAnnotations := map[string]string{}
	depSpecAnnotations := map[string]string{}

	if dep.Annotations != nil {
		depMetaAnnotations = dep.Annotations
	}
	if dep.Spec.Template.Annotations != nil {
		depSpecAnnotations = dep.Spec.Template.Annotations
	}

	return depMetaAnnotations, depSpecAnnotations
}

func getLabels(dep *appsv1.Deployment) (map[string]string, map[string]string) {
	depMetaLabels := map[string]string{}
	depSpecLabels := map[string]string{}

	if dep.Labels != nil {
		depMetaLabels = dep.Labels
	}
	if dep.Spec.Template.Labels != nil {
		depSpecLabels = dep.Spec.Template.Labels
	}

	return depMetaLabels, depSpecLabels
}

// setLabels sets the value of labels
func setLabels(dep *appsv1.Deployment, metaLabels map[string]string, specLabels map[string]string) {
	if len(metaLabels) > 0 {
		dep.SetLabels(metaLabels)
	}

	if len(specLabels) > 0 {
		dep.Spec.Template.Labels = specLabels
	}
}

// setAnnotations sets the value of annotations/labels
func setAnnotations(dep *appsv1.Deployment, metaAnnotations map[string]string, specAnnotations map[string]string) {
	if len(metaAnnotations) > 0 {
		dep.SetAnnotations(metaAnnotations)
	}

	if len(specAnnotations) > 0 {
		dep.Spec.Template.Annotations = specAnnotations
	}
}

// mergeAnnotations merges environment, application annotations with current values
func mergeAnnotations(ctx context.Context, options renderers.RenderOptions, currAnnotations map[string]string, existingMetaAnnotations map[string]string, existingSpecAnnotations map[string]string) (map[string]string, map[string]string) {
	envOpts := &options.Environment
	appOpts := &options.Application
	mergeAnnotations := map[string]string{}

	if envOpts != nil && envOpts.KubernetesMetadata != nil && envOpts.KubernetesMetadata.Annotations != nil {
		mergeAnnotations = envOpts.KubernetesMetadata.Annotations
	}
	if appOpts != nil && appOpts.KubernetesMetadata != nil && appOpts.KubernetesMetadata.Annotations != nil {
		// mergeAnnotations is now updated with merged map.
		mergeAnnotations = labels.Merge(mergeAnnotations, appOpts.KubernetesMetadata.Annotations)
	}

	// Cumulative Env+App Annotations is now merged with input annotations. Existing metaAnnotations and specAnnotations are subsequently merged with the result map.
	existingMetaAnnotations, existingSpecAnnotations = mergeMaps(ctx, mergeAnnotations, currAnnotations, existingMetaAnnotations, existingSpecAnnotations)

	return existingMetaAnnotations, existingSpecAnnotations
}

// mergeLabels merges environment, application labels with current values
func mergeLabels(ctx context.Context, options renderers.RenderOptions, currLabels map[string]string, existingMetaLabels map[string]string, existingSpecLabels map[string]string) (map[string]string, map[string]string) {
	envOpts := &options.Environment
	appOpts := &options.Application
	mergeLabels := map[string]string{}

	if envOpts != nil && envOpts.KubernetesMetadata != nil && envOpts.KubernetesMetadata.Labels != nil {
		mergeLabels = envOpts.KubernetesMetadata.Labels
	}
	if appOpts != nil && appOpts.KubernetesMetadata != nil && appOpts.KubernetesMetadata.Labels != nil {
		// mergeLabels is now updated with merged map.
		mergeLabels = labels.Merge(mergeLabels, appOpts.KubernetesMetadata.Labels)
	}

	// Cumulative Env+App Labels is now merged with input labels. Existing metaLabels and specLabels are subsequently merged with the result map.
	existingMetaLabels, existingSpecLabels = mergeMaps(ctx, mergeLabels, currLabels, existingMetaLabels, existingSpecLabels)

	return existingMetaLabels, existingSpecLabels
}

// mergeMaps merges four maps
func mergeMaps(ctx context.Context, mergeMap map[string]string, newInputMap map[string]string, existingMetaMap map[string]string, existingSpecMap map[string]string) (map[string]string, map[string]string) {

	// Reject custom user entries that may affect Radius reserved keys.
	mergeMap = rejectReservedEntries(ctx, mergeMap)
	newInputMap = rejectReservedEntries(ctx, newInputMap)

	// Cumulative Env+App Labels (mergeMap) is now merged with new input map. Existing metaLabels and specLabels are subsequently merged with the result map.
	mergeMap = labels.Merge(mergeMap, newInputMap)
	existingMetaMap = labels.Merge(existingMetaMap, mergeMap)
	existingSpecMap = labels.Merge(existingSpecMap, mergeMap)

	return existingMetaMap, existingSpecMap
}

// Reject custom user entries that would affect Radius reserved keys
func rejectReservedEntries(ctx context.Context, inputMap map[string]string) map[string]string {
	logger := ucplog.FromContextOrDiscard(ctx)

	for k := range inputMap {
		if strings.HasPrefix(k, kubernetes.RadiusDevPrefix) {
			logger.Info("User provided label/annotation key starts with 'radius.dev/' and is not being applied", "key", k)
			delete(inputMap, k)
		}
	}

	return inputMap
}
