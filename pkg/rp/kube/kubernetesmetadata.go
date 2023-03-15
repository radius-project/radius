// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kube

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"k8s.io/apimachinery/pkg/labels"
)

// Metadata represents KubernetesMetadata data. It includes labels/annotations defined as KubernetesMetadataExtension at the
// Environment/Application/Current Resource(Container eg.) level and pre-existing labels/annotations that may be present in the outputresource.
type Metadata struct {
	EnvData        map[string]string // Contains labels/annotations defined as a KubernetesMetadataExtension at the Environment level.
	AppData        map[string]string // Contains labels/annotations defined as a KubernetesMetadataExtension at the Application level.
	Input          map[string]string // Contains labels/annotations defined as a KubernetesMetadataExtension at the Current Resource level.
	ObjectMetadata map[string]string // Contains labels/annotations that are in the outputresource at the ObjectMeta level.
	SpecData       map[string]string // Contains labels/annotations that are in the outputresource at the Spec level.
}

// More info:
// ObjectMeta: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
// Spec: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status

// Merge merges environment, application maps with current values and returns updated metaMap and specMap
func (km *Metadata) Merge(ctx context.Context) (map[string]string, map[string]string) {
	mergeMap := map[string]string{}

	if km.EnvData != nil {
		mergeMap = km.EnvData
	}
	if km.AppData != nil {
		// mergeMap is now updated with merged map of env+app data.
		mergeMap = labels.Merge(mergeMap, km.AppData)
	}

	// Reject custom user entries that may affect Radius reserved keys.
	mergeMap = rejectReservedEntries(ctx, mergeMap)
	km.Input = rejectReservedEntries(ctx, km.Input)

	// Cumulative Env+App Labels (mergeMap) is now merged with new input map. Existing metaLabels and specLabels are subsequently merged with the result map.
	// In case of collisions, rightmost entity wins
	mergeMap = labels.Merge(mergeMap, km.Input)
	metaMap := labels.Merge(km.ObjectMetadata, mergeMap)
	specMap := labels.Merge(km.SpecData, mergeMap)

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
