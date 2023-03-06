// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetesmetadata

import (
	"context"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/kube"
	"github.com/project-radius/radius/pkg/ucp/resources"
	appsv1 "k8s.io/api/apps/v1"
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

		//Create KubernetesMetadata struct to merge annotations
		annMap := &kube.KubernetesMetadataMap{
			InputMap:    inputAnnotations,
			CurrMetaMap: existingMetaAnnotations,
			CurrSpecMap: existingSpecAnnotations,
		}

		envOpts := &options.Environment
		appOpts := &options.Application
		envKmeExists := envOpts != nil && envOpts.KubernetesMetadata != nil
		appKmeExists := appOpts != nil && appOpts.KubernetesMetadata != nil

		if envKmeExists && envOpts.KubernetesMetadata.Annotations != nil {
			annMap.EnvMap = envOpts.KubernetesMetadata.Annotations
		}
		if appKmeExists && appOpts.KubernetesMetadata.Annotations != nil {
			annMap.AppMap = envOpts.KubernetesMetadata.Annotations
		}

		// Merge cumulative annotation values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, rightmost entity wins
		updMetaAnnotations, updSpecAnnotations := annMap.Merge(ctx)
		setAnnotations(dep, updMetaAnnotations, updSpecAnnotations)

		if kubeMetadataExt != nil && kubeMetadataExt.Labels != nil {
			inputLabels = kubeMetadataExt.Labels
		}

		existingMetaLabels, existingSpecLabels := getLabels(dep)

		//Create KubernetesMetadata struct to merge labels
		lblMap := &kube.KubernetesMetadataMap{
			InputMap:    inputLabels,
			CurrMetaMap: existingMetaLabels,
			CurrSpecMap: existingSpecLabels,
		}

		if envKmeExists && envOpts.KubernetesMetadata.Labels != nil {
			annMap.EnvMap = envOpts.KubernetesMetadata.Labels
		}
		if appKmeExists && appOpts.KubernetesMetadata.Labels != nil {
			annMap.AppMap = envOpts.KubernetesMetadata.Labels
		}

		// Merge cumulative label values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, rightmost entity wins
		updMetaLabels, updSpecLabels := lblMap.Merge(ctx)
		setLabels(dep, updMetaLabels, updSpecLabels)
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
