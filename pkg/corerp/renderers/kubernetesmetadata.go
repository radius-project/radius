/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package renderers

import (
	"context"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/rp/kube"
)

// # Function Explanation
//
// GetLabels merges cumulative label values from Environment, Application, Container and InputExt kubernetes metadata and
// returns a map of labels.
func GetLabels(ctx context.Context, options RenderOptions, applicationName string, resourceName string, resourceTypeName string) map[string]string {
	// Create KubernetesMetadata struct to merge labels
	lblMap := kube.Metadata{
		ObjectMetadata: kubernetes.MakeDescriptiveLabels(applicationName, resourceName, resourceTypeName),
	}
	envOpts := &options.Environment
	appOpts := &options.Application

	if envOpts.KubernetesMetadata != nil && envOpts.KubernetesMetadata.Labels != nil {
		lblMap.EnvData = envOpts.KubernetesMetadata.Labels
	}
	if appOpts.KubernetesMetadata != nil && appOpts.KubernetesMetadata.Labels != nil {
		lblMap.AppData = appOpts.KubernetesMetadata.Labels
	}

	// Merge cumulative label values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, Env->App->Container->InputExt
	// values are merged in that order. Spec labels are not updated.
	if metaLabels, _ := lblMap.Merge(ctx); len(metaLabels) > 0 {
		return metaLabels
	}

	return nil
}

// # Function Explanation
//
// GetAnnotations returns the merged annotations from Environment and Application KubernetesMetadata.
func GetAnnotations(ctx context.Context, options RenderOptions) map[string]string {
	// Create KubernetesMetadata struct to merge annotations
	annMap := kube.Metadata{}
	envOpts := &options.Environment
	appOpts := &options.Application

	if envOpts.KubernetesMetadata != nil && envOpts.KubernetesMetadata.Annotations != nil {
		annMap.EnvData = envOpts.KubernetesMetadata.Annotations
	}
	if appOpts.KubernetesMetadata != nil && appOpts.KubernetesMetadata.Annotations != nil {
		annMap.AppData = appOpts.KubernetesMetadata.Annotations
	}

	// Merge cumulative annotations values from Env->App->Container->InputExt kubernetes metadata. In case of collisions, rightmost entity wins
	// Spec annotations are not updated.
	if metaAnnotations, _ := annMap.Merge(ctx); len(metaAnnotations) > 0 {
		return metaAnnotations
	}

	return nil
}
