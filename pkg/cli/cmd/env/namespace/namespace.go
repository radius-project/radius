// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package namespace

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
)

//go:generate mockgen -destination=./mock_namespace.go -package=namespace -self_package github.com/project-radius/radius/pkg/cli/cmd/env/namespace github.com/project-radius/radius/pkg/cli/cmd/env/namespace Interface
type Interface interface {
	ValidateNamespace(ctx context.Context, namespace string) error
}

type Impl struct {
}

// Ensure sure namespace is available
func (i *Impl) ValidateNamespace(ctx context.Context, namespace string) error {
	client, _, err := kubernetes.NewClientset("")
	if err != nil {
		return err
	}

	return kubernetes.EnsureNamespace(ctx, client, namespace)
}
