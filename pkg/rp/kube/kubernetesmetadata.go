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

package kube

import (
	"context"
	"strings"

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"k8s.io/apimachinery/pkg/labels"
)

// Metadata represents KubernetesMetadata data. It includes labels/annotations defined as KubernetesMetadataExtension at the
// Environment/Application/Current Resource(Container eg.) level and pre-existing labels/annotations that may be present in the outputresource.
type Metadata struct {
	EnvData        map[string]string // EnvData contains labels/annotations defined as a KubernetesMetadataExtension at the Environment level.
	AppData        map[string]string // AppData contains labels/annotations defined as a KubernetesMetadataExtension at the Application level.
	Input          map[string]string // Input contains labels/annotations defined as a KubernetesMetadataExtension at the Current Resource level.
	ObjectMetadata map[string]string // ObjectMetadata contains labels/annotations that are in the outputresource at the ObjectMeta level.
	SpecData       map[string]string // SpecData contains labels/annotations that are in the outputresource at the Spec level.
}

// Merge merges environment, application maps with current values and returns updated metaMap and specMap
// More info:
// ObjectMeta: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
// Spec: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
func (km *Metadata) Merge(ctx context.Context) (map[string]string, map[string]string) {
	mergedDataMap := map[string]string{}

	if km.EnvData != nil {
		mergedDataMap = km.EnvData
	}
	if km.AppData != nil {
		// mergeMap is now updated with merged map of env+app data.
		mergedDataMap = labels.Merge(mergedDataMap, km.AppData)
	}

	// Reject custom user entries that may affect Radius reserved keys.
	mergedDataMap = rejectReservedEntries(ctx, mergedDataMap)
	km.Input = rejectReservedEntries(ctx, km.Input)

	// Cumulative Env+App Labels (mergeMap) is now merged with new input map. Existing metaLabels and specLabels are subsequently merged with the result map.
	mergedDataMap = labels.Merge(mergedDataMap, km.Input)
	metaMap := labels.Merge(km.ObjectMetadata, mergedDataMap)
	specMap := labels.Merge(km.SpecData, mergedDataMap)

	return metaMap, specMap
}

// rejectReservedEntries rejects custom user entries that would affect Radius reserved keys
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
