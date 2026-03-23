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

package pgbackup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// DefaultNamespace is the Kubernetes namespace where Radius is installed.
	DefaultNamespace = "radius-system"

	// PodLabelSelector is the label selector for the PostgreSQL pod deployed by the Helm chart.
	PodLabelSelector = "app.kubernetes.io/name=database"

	// PostgresUser is the superuser for backup/restore operations.
	PostgresUser = "radius"
)

// databases is the list of PostgreSQL databases to back up and restore.
var databases = []string{"ucp", "applications_rp", "dynamic_rp"}

// HasBackup checks whether backup SQL files exist in the given state directory.
func HasBackup(stateDir string) bool {
	for _, db := range databases {
		path := filepath.Join(stateDir, db+".sql")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false
		}
	}

	return true
}

// Backup dumps each PostgreSQL database to a plain SQL file in the state directory.
func Backup(ctx context.Context, kubeContext, namespace, stateDir string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory %q: %w", stateDir, err)
	}

	podName, err := getPodName(ctx, kubeContext, namespace)
	if err != nil {
		return err
	}

	for _, db := range databases {
		logger.Info("Backing up database", "database", db, "stateDir", stateDir)

		cmd := exec.CommandContext(ctx, "kubectl",
			"--context", kubeContext,
			"-n", namespace,
			"exec", podName, "--",
			"pg_dump",
			"-U", PostgresUser,
			"--format=plain",
			"--clean",
			"--if-exists",
			db,
		)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to backup database %q: %w: %s", db, err, stderr.String())
		}

		outPath := filepath.Join(stateDir, db+".sql")
		if err := os.WriteFile(outPath, stdout.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write backup file %q: %w", outPath, err)
		}

		logger.Info("Database backup complete", "database", db, "file", outPath)
	}

	return nil
}

// Restore loads SQL backup files from the state directory into the PostgreSQL database.
func Restore(ctx context.Context, kubeContext, namespace, stateDir string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if !HasBackup(stateDir) {
		logger.Info("No backup found in state directory, skipping restore", "stateDir", stateDir)
		return nil
	}

	podName, err := getPodName(ctx, kubeContext, namespace)
	if err != nil {
		return err
	}

	for _, db := range databases {
		sqlPath := filepath.Join(stateDir, db+".sql")
		logger.Info("Restoring database", "database", db, "file", sqlPath)

		sqlData, err := os.ReadFile(sqlPath)
		if err != nil {
			return fmt.Errorf("failed to read backup file %q: %w", sqlPath, err)
		}

		cmd := exec.CommandContext(ctx, "kubectl",
			"--context", kubeContext,
			"-n", namespace,
			"exec", "-i", podName, "--",
			"psql",
			"-U", PostgresUser,
			"-d", db,
		)

		cmd.Stdin = bytes.NewReader(sqlData)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to restore database %q: %w: %s", db, err, stderr.String())
		}

		logger.Info("Database restore complete", "database", db)
	}

	return nil
}

// WaitForReady waits for the PostgreSQL pod to be ready using kubectl wait.
func WaitForReady(ctx context.Context, kubeContext, namespace string) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Waiting for PostgreSQL pod to be ready")

	cmd := exec.CommandContext(ctx, "kubectl",
		"--context", kubeContext,
		"-n", namespace,
		"wait",
		"--for=condition=ready",
		"pod",
		"-l", PodLabelSelector,
		"--timeout=120s",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("timed out waiting for PostgreSQL pod: %w: %s", err, stderr.String())
	}

	logger.Info("PostgreSQL pod is ready")
	return nil
}

func getPodName(ctx context.Context, kubeContext, namespace string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl",
		"--context", kubeContext,
		"-n", namespace,
		"get", "pods",
		"-l", PodLabelSelector,
		"-o", "jsonpath={.items[0].metadata.name}",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to find PostgreSQL pod: %w: %s", err, stderr.String())
	}

	podName := strings.TrimSpace(stdout.String())
	if podName == "" {
		return "", fmt.Errorf("no PostgreSQL pod found with selector %q in namespace %q", PodLabelSelector, namespace)
	}

	return podName, nil
}
