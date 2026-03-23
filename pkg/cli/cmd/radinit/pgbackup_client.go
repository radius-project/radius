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

package radinit

import (
	"context"

	"github.com/radius-project/radius/pkg/cli/pgbackup"
)

//go:generate mockgen -typed -destination=./mock_pgbackupclient.go -package=radinit -self_package github.com/radius-project/radius/pkg/cli/cmd/radinit github.com/radius-project/radius/pkg/cli/cmd/radinit PGBackupClient

// PGBackupClient is the interface for PostgreSQL backup and restore operations.
// It wraps the pgbackup package so that it can be mocked in tests.
type PGBackupClient interface {
	WaitForReady(ctx context.Context, kubeContext, namespace string) error
	HasBackup(stateDir string) bool
	Restore(ctx context.Context, kubeContext, namespace, stateDir string) error
}

// defaultPGBackupClient is the production implementation of PGBackupClient.
type defaultPGBackupClient struct{}

func (defaultPGBackupClient) WaitForReady(ctx context.Context, kubeContext, namespace string) error {
	return pgbackup.WaitForReady(ctx, kubeContext, namespace)
}

func (defaultPGBackupClient) HasBackup(stateDir string) bool {
	return pgbackup.HasBackup(stateDir)
}

func (defaultPGBackupClient) Restore(ctx context.Context, kubeContext, namespace, stateDir string) error {
	return pgbackup.Restore(ctx, kubeContext, namespace, stateDir)
}

// NewPGBackupClient returns the production PGBackupClient.
func NewPGBackupClient() PGBackupClient {
	return defaultPGBackupClient{}
}
