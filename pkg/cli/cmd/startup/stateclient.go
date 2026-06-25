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

package startup

import (
	"context"

	"github.com/radius-project/radius/pkg/cli/controlplane"
	"github.com/radius-project/radius/pkg/cli/pgbackup"
	"github.com/radius-project/radius/pkg/cli/tfstate"
)

// ControlPlaneScaler scales the database-backed control-plane deployments to zero and back, so
// state can be restored while no resource provider holds a live PostgreSQL connection.
type ControlPlaneScaler interface {
	// ScaleDown scales the control-plane deployments to zero and returns their previous replica
	// counts so they can be restored by ScaleUp.
	ScaleDown(ctx context.Context) (map[string]int32, error)

	// ScaleUp restores the deployments to the replica counts captured by ScaleDown and waits until
	// they are available again.
	ScaleUp(ctx context.Context, saved map[string]int32) error
}

// newScalerForContext is the production factory for a ControlPlaneScaler. It is a package variable
// so tests can replace it without a cluster.
var newScalerForContext = func(kubeContext, namespace string) (ControlPlaneScaler, error) {
	return controlplane.NewScalerForContext(kubeContext, namespace)
}

// StateRestoreClient restores the durable Radius state for a Kubernetes context. It wraps the
// pgbackup and tfstate packages so the command can be unit tested without a cluster.
type StateRestoreClient interface {
	// WaitForDatabaseReady blocks until the control-plane PostgreSQL instance is ready.
	WaitForDatabaseReady(ctx context.Context, kubeContext, namespace string) error

	// RestoreDatabases loads the control-plane PostgreSQL dumps from stateDir.
	RestoreDatabases(ctx context.Context, kubeContext, namespace, stateDir string) error

	// RestoreTerraform re-creates the Terraform state Secrets from stateDir.
	RestoreTerraform(ctx context.Context, kubeContext, namespace, stateDir string) error
}

// defaultStateRestoreClient is the production implementation.
type defaultStateRestoreClient struct{}

// NewStateRestoreClient returns the production StateRestoreClient.
func NewStateRestoreClient() StateRestoreClient {
	return defaultStateRestoreClient{}
}

func (defaultStateRestoreClient) WaitForDatabaseReady(ctx context.Context, kubeContext, namespace string) error {
	return pgbackup.WaitForReady(ctx, kubeContext, namespace)
}

func (defaultStateRestoreClient) RestoreDatabases(ctx context.Context, kubeContext, namespace, stateDir string) error {
	return pgbackup.Restore(ctx, kubeContext, namespace, stateDir)
}

func (defaultStateRestoreClient) RestoreTerraform(ctx context.Context, kubeContext, namespace, stateDir string) error {
	client, err := tfstate.NewClientForContext(kubeContext, namespace)
	if err != nil {
		return err
	}
	return client.Restore(ctx, stateDir)
}
