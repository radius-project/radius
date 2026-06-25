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

// Package pgbackup backs up and restores the Radius control-plane PostgreSQL databases.
//
// The Radius control plane stores resource data and deployment history in three logical
// PostgreSQL databases (ucp, applications_rp, dynamic_rp) served by a single in-cluster
// PostgreSQL instance. When the control plane runs on an ephemeral cluster, that data is lost on
// teardown. This package dumps each database to a plain SQL file (via "kubectl exec ... pg_dump")
// and restores them (via "kubectl exec ... psql") so state survives across runs.
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
	// DefaultNamespace is the Kubernetes namespace where Radius (and its PostgreSQL instance)
	// is installed.
	DefaultNamespace = "radius-system"

	// PodLabelSelector is the label selector for the PostgreSQL pod deployed by the Helm chart.
	PodLabelSelector = "app.kubernetes.io/name=database"

	// PostgresUser is the superuser used for backup/restore operations. It can read and write
	// every logical database regardless of which per-RP user owns the data.
	PostgresUser = "radius"
)

// Databases is the list of PostgreSQL databases that hold control-plane state.
var Databases = []string{"ucp", "applications_rp", "dynamic_rp"}

// HasBackup reports whether a SQL dump exists for every database in the state directory.
func HasBackup(stateDir string) bool {
	for _, db := range Databases {
		path := filepath.Join(stateDir, db+".sql")
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}

	return true
}

// Backup dumps each PostgreSQL database to a plain SQL file in the state directory.
func Backup(ctx context.Context, kubeContext, namespace, stateDir string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("failed to create state directory %q: %w", stateDir, err)
	}

	podName, err := getPodName(ctx, kubeContext, namespace)
	if err != nil {
		return err
	}

	for _, db := range Databases {
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
			return fmt.Errorf("failed to back up database %q: %w: %s", db, err, stderr.String())
		}

		outPath := filepath.Join(stateDir, db+".sql")
		if err := os.WriteFile(outPath, stdout.Bytes(), 0o644); err != nil {
			return fmt.Errorf("failed to write backup file %q: %w", outPath, err)
		}

		logger.Info("Database backup complete", "database", db, "file", outPath)
	}

	return nil
}

// Restore loads each SQL dump from the state directory into its PostgreSQL database. The dumps
// are produced with --clean --if-exists, so restore is idempotent.
func Restore(ctx context.Context, kubeContext, namespace, stateDir string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if !HasBackup(stateDir) {
		logger.Info("No database backup found, skipping restore", "stateDir", stateDir)
		return nil
	}

	podName, err := getPodName(ctx, kubeContext, namespace)
	if err != nil {
		return err
	}

	for _, db := range Databases {
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

// WaitForReady blocks until the PostgreSQL pod reports ready, using "kubectl wait".
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

// getPodName resolves the name of the PostgreSQL pod via its label selector.
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
