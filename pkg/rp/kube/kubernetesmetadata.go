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

// Metadata represents KubernetesMetadata data.
type Metadata struct {
	EnvData        map[string]string
	AppData        map[string]string
	Input          map[string]string
	CurrObjectMeta map[string]string
	CurrSpec       map[string]string
}

// MergeMaps merges environment, application maps with current values and returns updated metaMap and specMap
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
	mergeMap = labels.Merge(mergeMap, km.Input)

	updMetaMap := labels.Merge(km.CurrObjectMeta, mergeMap)
	updSpecMap := labels.Merge(km.CurrSpec, mergeMap)

	return updMetaMap, updSpecMap
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
