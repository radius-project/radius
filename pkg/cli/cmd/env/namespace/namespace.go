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

	"github.com/project-radius/radius/pkg/cli/kubernetes"
)

//go:generate mockgen -destination=./mock_namespace.go -package=namespace -self_package github.com/project-radius/radius/pkg/cli/cmd/env/namespace github.com/project-radius/radius/pkg/cli/cmd/env/namespace Interface
type Interface interface {
	ValidateNamespace(ctx context.Context, namespace string) error
}

type Impl struct {
}

// Ensure sure namespace is available
//

// ValidateNamespace creates a Kubernetes client and checks if the given namespace exists. If it does not exist, creates it.
// If unsuccessful, returns an error.
func (i *Impl) ValidateNamespace(ctx context.Context, namespace string) error {
	client, _, err := kubernetes.NewClientset("")
	if err != nil {
		return err
	}

	return kubernetes.EnsureNamespace(ctx, client, namespace)
}
