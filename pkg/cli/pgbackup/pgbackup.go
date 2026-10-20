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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/radius-project/radius/pkg/process"
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

const (
	// DefaultStateArchive is the default archive name where durable Radius state is committed:
	// the PostgreSQL dumps produced by this package and the Terraform state Secrets exported by
	// pkg/cli/tfstate. It is shared by the "rad shutdown" and "rad startup" commands.
	DefaultStateArchive = "radius-state"

	// StateArchiveEnvVar overrides DefaultStateArchive. It lets parallel tests use isolated
	// archives without colliding.
	StateArchiveEnvVar = "RADIUS_STATE_ARCHIVE"

	// DefaultStateBranch is deprecated. Use DefaultStateArchive.
	DefaultStateBranch = DefaultStateArchive

	// StateBranchEnvVar is deprecated. Use StateArchiveEnvVar.
	StateBranchEnvVar = "RADIUS_STATE_BRANCH"
)

// Databases is the list of PostgreSQL databases that hold control-plane state.
var Databases = []string{"ucp", "applications_rp", "dynamic_rp"}

// StateArchiveName returns the state archive name, honoring StateArchiveEnvVar, then the
// deprecated StateBranchEnvVar, and falling back to DefaultStateArchive.
func StateArchiveName() string {
	if v := os.Getenv(StateArchiveEnvVar); v != "" {
		return v
	}
	if v := os.Getenv(StateBranchEnvVar); v != "" {
		return v
	}
	return DefaultStateArchive
}

// StateBranchName is deprecated. Use StateArchiveName.
func StateBranchName() string {
	return StateArchiveName()
}

// copyResourcesHeader matches the pg_dump COPY header for the "resources" table -- the table that
// holds every UCP resource, including resource groups -- in plain-format output, and captures its
// column list, e.g.:
//
//	COPY public.resources (id, original_id, resource_type, ...) FROM stdin;
var copyResourcesHeader = regexp.MustCompile(`^COPY\s+(?:[\w"]+\.)?"?resources"?\s*\((.*)\)\s+FROM\s+stdin;\s*$`)

// seedResourceTypes are the normalized resource_type values UCP writes for itself rather than on a
// user's behalf: the resource provider metadata the initializer registers on every boot
// (pkg/ucp/initializer, registerResourceProviderDirect) and the planes. A control plane that was
// reset or lost its data still contains these rows, so they say nothing about user state. Every
// other type -- a resource group, for example -- is created by a user ("rad init" creates the
// default group).
var seedResourceTypes = map[string]bool{
	"/system.resources/resourceproviders/":                           true,
	"/system.resources/resourceproviders/resourcetypes/":             true,
	"/system.resources/resourceproviders/resourcetypes/apiversions/": true,
	"/system.resources/resourceproviders/locations/":                 true,
	"/system.resources/resourceprovidersummaries/":                   true,
	"/system.radius/planes/":                                         true,
	"/system.aws/planes/":                                            true,
	"/system.azure/planes/":                                          true,
}

// IsControlPlaneEmpty reports whether the backed-up ucp database dump in stateDir contains no
// user-created resources: no rows in any "resources" table other than the provider metadata and
// planes UCP seeds for itself (seedResourceTypes). A database in this state (e.g. after a postgres
// pod crash-loop, or "rad install" re-run mid-session) is not safe to persist: committing it would
// overwrite the durable archive with unrecoverable data loss, even though `rad startup` restored
// the previous snapshot successfully at the start of the run.
func IsControlPlaneEmpty(stateDir string) (bool, error) {
	path := filepath.Join(stateDir, "ucp.sql")
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed to read backup file %q: %w", path, err)
	}
	defer f.Close()

	// Stream the dump line by line, so a large snapshot is never held in memory. bufio.Reader rather
	// than bufio.Scanner, because data rows carry resource JSON and can exceed Scanner's 64KB limit.
	r := bufio.NewReader(f)
	inResources := false
	typeColumn := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("failed to read backup file %q: %w", path, err)
		}
		line = strings.TrimRight(line, "\r\n")

		switch {
		case !inResources:
			if m := copyResourcesHeader.FindStringSubmatch(line); m != nil {
				inResources = true
				typeColumn = columnIndex(m[1], "resource_type")
			}
		case line == "":
			// Skip blank lines inside the COPY block.
		case strings.HasPrefix(line, `\.`):
			// End of this resources block without a user-created row. Keep scanning: a dump can hold
			// more than one table named resources (other schemas), and any of them may have user data.
			inResources = false
		default:
			if typeColumn < 0 {
				// The dump does not name a resource_type column, so seed rows cannot be told apart;
				// treat any row as user data rather than refuse to persist a real snapshot.
				return false, nil
			}
			fields := strings.Split(line, "\t")
			if typeColumn >= len(fields) || !seedResourceTypes[strings.ToLower(fields[typeColumn])] {
				return false, nil
			}
		}

		if errors.Is(err, io.EOF) {
			// No user-created row anywhere. That includes a dump with no resources table at all, and
			// one cut off before its terminator: both are at least as degenerate as an empty table.
			return true, nil
		}
	}
}

// columnIndex returns the position of column in a pg_dump COPY column list, or -1.
func columnIndex(columns, column string) int {
	for i, c := range strings.Split(columns, ",") {
		if strings.Trim(strings.TrimSpace(c), `"`) == column {
			return i
		}
	}
	return -1
}

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

		cmd := process.CommandContext(ctx, "kubectl",
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

		cmd := process.CommandContext(ctx, "kubectl",
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

	cmd := process.CommandContext(ctx, "kubectl",
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
	cmd := process.CommandContext(ctx, "kubectl",
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
