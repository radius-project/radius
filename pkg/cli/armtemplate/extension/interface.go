// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extension

import "context"

// An extension.Store can provide more resources that we don't deploy ourselves.
//
// For example, K8s resources deployed in a Kubernetes cluster.
type Store interface {
	GetDeployedResource(ctx context.Context, ref interface{}, version string) (map[string]interface{}, error)
}
