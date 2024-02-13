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

package namespace

import (
	"context"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

//go:generate mockgen -destination=./mock_namespace.go -package=namespace -self_package github.com/radius-project/radius/pkg/cli/cmd/env/namespace github.com/radius-project/radius/pkg/cli/cmd/env/namespace Interface
type Interface interface {
	ValidateNamespace(ctx context.Context, namespace string, workspace workspaces.Workspace) error
}

type Impl struct {
}

// Ensure sure namespace is available
//

// ValidateNamespace creates a Kubernetes client and checks if the given namespace exists. If it does not exist, creates it.
// If unsuccessful, returns an error.
func (i *Impl) ValidateNamespace(ctx context.Context, namespace string, workspace workspaces.Workspace) error {
	// get the current kubernetes context from the workspace
	kubernetesContext, hasContext := workspace.KubernetesContext()
	if !hasContext {
		return clierrors.Message("no kubernetes context found in the current workspace")
	}
	client, _, err := kubernetes.NewClientset(kubernetesContext)
	if err != nil {
		return err
	}

	return kubernetes.EnsureNamespace(ctx, client, namespace)
}
