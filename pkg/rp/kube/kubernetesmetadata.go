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

// KubernetesMetadata represents KubernetesMetadata data which will be subsequently cascaded to the current resource.
// Note: We have RenderOptions both in CoreRp and LinkRp. If/When we would want to consolidate them, we can then update this struct to use RenderOptions instead of env and app specific data.
type KubernetesMetadataMap struct {
	EnvMap      map[string]string
	AppMap      map[string]string
	InputMap    map[string]string
	CurrMetaMap map[string]string
	CurrSpecMap map[string]string
}

// MergeMaps merges environment, application maps with current values and returns updated metaMap and specMap
func (km *KubernetesMetadataMap) Merge(ctx context.Context) (map[string]string, map[string]string) {
	mergeMap := map[string]string{}

	if km.EnvMap != nil {
		mergeMap = km.EnvMap
	}
	if km.AppMap != nil {
		// mergeMap is now updated with merged map of env+app data.
		mergeMap = labels.Merge(mergeMap, km.AppMap)
	}

	// Reject custom user entries that may affect Radius reserved keys.
	mergeMap = rejectReservedEntries(ctx, mergeMap)
	km.InputMap = rejectReservedEntries(ctx, km.InputMap)

	// Cumulative Env+App Labels (mergeMap) is now merged with new input map. Existing metaLabels and specLabels are subsequently merged with the result map.
	mergeMap = labels.Merge(mergeMap, km.InputMap)

	updMetaMap := labels.Merge(km.CurrMetaMap, mergeMap)
	updSpecMap := labels.Merge(km.CurrSpecMap, mergeMap)

	return updMetaMap, updSpecMap
}

// Reject custom user entries that would affect Radius reserved keys
func rejectReservedEntries(ctx context.Context, InputMap map[string]string) map[string]string {
	logger := ucplog.FromContextOrDiscard(ctx)

	for k := range InputMap {
		if strings.HasPrefix(k, kubernetes.RadiusDevPrefix) {
			logger.Info("User provided label/annotation key starts with 'radius.dev/' and is not being applied", "key", k)
			delete(InputMap, k)
		}
	}

	return InputMap
}
