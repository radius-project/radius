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
)

//go:generate mockgen -typed -destination=./mock_pgbackupclient.go -package=shutdown -self_package github.com/radius-project/radius/pkg/cli/cmd/shutdown github.com/radius-project/radius/pkg/cli/cmd/shutdown PGBackupClient

// PGBackupClient is the interface for PostgreSQL backup operations.
// It wraps pgbackup so that it can be mocked in tests.
type PGBackupClient interface {
	Backup(ctx context.Context, kubeContext, namespace, stateDir string) error
}

// defaultPGBackupClient is the production implementation.
type defaultPGBackupClient struct{}

func (defaultPGBackupClient) Backup(ctx context.Context, kubeContext, namespace, stateDir string) error {
	return pgbackup.Backup(ctx, kubeContext, namespace, stateDir)
}

// NewPGBackupClient returns the production PGBackupClient.
func NewPGBackupClient() PGBackupClient {
	return defaultPGBackupClient{}
}
