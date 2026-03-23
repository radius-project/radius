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

package k3d

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// DefaultClusterName is the default name for the k3d cluster used by GitHub workspaces.
	DefaultClusterName = "radius-github"

	// ContextPrefix is the prefix k3d adds to cluster names to form kubeconfig context names.
	ContextPrefix = "k3d-"
)

// ContextName returns the kubeconfig context name for a k3d cluster.
func ContextName(clusterName string) string {
	return ContextPrefix + clusterName
}

// EnsureInstalled checks that the k3d CLI is available on the PATH.
func EnsureInstalled(ctx context.Context) error {
	_, err := exec.LookPath("k3d")
	if err != nil {
		return fmt.Errorf("k3d is not installed or not on PATH: %w", err)
	}

	return nil
}

// ClusterExists checks if a k3d cluster with the given name already exists.
func ClusterExists(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "k3d", "cluster", "list", "--no-headers")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{}

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("failed to list k3d clusters: %w", err)
	}

	for _, line := range strings.Split(stdout.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return true, nil
		}
	}

	return false, nil
}

// CreateCluster creates a new k3d cluster with the given name and returns the kubeconfig context name.
// If a cluster with the same name already exists, it returns the context name without creating a new one.
func CreateCluster(ctx context.Context, name string) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	exists, err := ClusterExists(ctx, name)
	if err != nil {
		return "", err
	}

	if exists {
		logger.Info("k3d cluster already exists", "name", name)
		return ContextName(name), nil
	}

	logger.Info("Creating k3d cluster", "name", name)
	cmd := exec.CommandContext(ctx, "k3d", "cluster", "create", name,
		"--wait",
		"--timeout", "120s",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create k3d cluster %q: %w: %s", name, err, stderr.String())
	}

	return ContextName(name), nil
}

// DeleteCluster deletes a k3d cluster with the given name.
func DeleteCluster(ctx context.Context, name string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	exists, err := ClusterExists(ctx, name)
	if err != nil {
		return err
	}

	if !exists {
		logger.Info("k3d cluster does not exist, nothing to delete", "name", name)
		return nil
	}

	logger.Info("Deleting k3d cluster", "name", name)
	cmd := exec.CommandContext(ctx, "k3d", "cluster", "delete", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete k3d cluster %q: %w: %s", name, err, stderr.String())
	}

	return nil
}
