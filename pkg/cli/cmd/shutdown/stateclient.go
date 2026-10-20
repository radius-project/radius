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

package shutdown

import (
	"context"

	"github.com/radius-project/radius/pkg/cli/pgbackup"
	"github.com/radius-project/radius/pkg/cli/tfstate"
)

// StateBackupClient backs up the durable Radius state for a Kubernetes context. It wraps the
// pgbackup and tfstate packages so the command can be unit tested without a cluster.
type StateBackupClient interface {
	// BackupDatabases dumps the control-plane PostgreSQL databases into stateDir.
	BackupDatabases(ctx context.Context, kubeContext, namespace, stateDir string) error

	// BackupTerraform exports the Terraform state Secrets into stateDir.
	BackupTerraform(ctx context.Context, kubeContext, namespace, stateDir string) error
}

// defaultStateBackupClient is the production implementation.
type defaultStateBackupClient struct{}

// NewStateBackupClient returns the production StateBackupClient.
func NewStateBackupClient() StateBackupClient {
	return defaultStateBackupClient{}
}

func (defaultStateBackupClient) BackupDatabases(ctx context.Context, kubeContext, namespace, stateDir string) error {
	return pgbackup.Backup(ctx, kubeContext, namespace, stateDir)
}

func (defaultStateBackupClient) BackupTerraform(ctx context.Context, kubeContext, namespace, stateDir string) error {
	client, err := tfstate.NewClientForContext(kubeContext, namespace)
	if err != nil {
		return err
	}
	return client.Backup(ctx, stateDir)
}
