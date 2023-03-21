// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetesmetadata

import (
	"context"

	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/rp/kube"
)

// GetLabels returns the labels to be applied to the resource
func GetLabels(ctx context.Context, options renderers.RenderOptions, applicationName string, resourceName string, resourceTypeName string) map[string]string {
	//Create KubernetesMetadata structs to merge labels
	lblMap := &kube.Metadata{
		ObjectMetadata: kubernetes.MakeDescriptiveLabels(applicationName, resourceName, resourceTypeName),
	}

	envOpts := &options.Environment
	appOpts := &options.Application
	envKmeExists := envOpts != nil && envOpts.KubernetesMetadata != nil
	appKmeExists := appOpts != nil && appOpts.KubernetesMetadata != nil

	if envKmeExists && envOpts.KubernetesMetadata.Labels != nil {
		lblMap.EnvData = envOpts.KubernetesMetadata.Labels
	}
	if appKmeExists && appOpts.KubernetesMetadata.Labels != nil {
		lblMap.AppData = appOpts.KubernetesMetadata.Labels
	}

	// Merge cumulative label values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, Env->App->Container->InputExt
	// values are merged in that order. Spec labels are not updated.
	if metaLabels, _ := lblMap.Merge(ctx); len(metaLabels) > 0 {
		return metaLabels
	}

	return nil
}

// GetAnnotations returns the annotations to be applied to the resource
func GetAnnotations(ctx context.Context, options renderers.RenderOptions) map[string]string {
	//Create KubernetesMetadata structs to merge annotations
	annMap := &kube.Metadata{}
	envOpts := &options.Environment
	appOpts := &options.Application
	envKmeExists := envOpts != nil && envOpts.KubernetesMetadata != nil
	appKmeExists := appOpts != nil && appOpts.KubernetesMetadata != nil

	if envKmeExists && envOpts.KubernetesMetadata.Annotations != nil {
		annMap.EnvData = envOpts.KubernetesMetadata.Annotations
	}
	if appKmeExists && appOpts.KubernetesMetadata.Annotations != nil {
		annMap.AppData = appOpts.KubernetesMetadata.Annotations
	}

	// Merge cumulative annotations values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, rightmost entity wins
	// Spec annotations are not updated.
	if metaAnnotations, _ := annMap.Merge(ctx); len(metaAnnotations) > 0 {
		return metaAnnotations
	}

	return nil
}
