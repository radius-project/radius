// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package providers

import "context"

// A providers.Store can provide more resources that we don't deploy ourselves.
//
// For example, K8s resources deployed in a Kubernetes cluster.
type Store interface {
	GetDeployedResource(ctx context.Context, ref string, version string) (map[string]interface{}, error)
}
