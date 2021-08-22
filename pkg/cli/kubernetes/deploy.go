// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"

	"k8s.io/client-go/dynamic"
)

type KubernetesDeploymentClient struct {
	Client    dynamic.Interface
	Namespace string
}

func (c KubernetesDeploymentClient) Deploy(ctx context.Context, content string) error {

	return nil
}
