// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package namespace

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
	k8s "k8s.io/client-go/kubernetes"
)

//go:generate mockgen -destination=./mock_namespace.go -package=namespace -self_package github.com/project-radius/radius/pkg/cli/cmd/env/namespace github.com/project-radius/radius/pkg/cli/cmd/env/namespace Interface
type Interface interface {
	ValidateNamespace(ctx context.Context, client k8s.Interface, namespace string) error
}

type Impl struct {
}

// Ensure sure namespace is available
func (*Impl) ValidateNamespace(ctx context.Context, client k8s.Interface, namespace string) error {
	return kubernetes.EnsureNamespace(ctx, client, namespace)
}
