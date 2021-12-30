// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package k3d

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func CreateCluster(ctx context.Context, name string) (*ClusterConfig, error) {
	err := RequireK3dInstalled()
	if err != nil {
		return nil, err
	}

	config := ClusterConfig{
		ClusterName: fmt.Sprintf("radius-%s", name),
		ContextName: fmt.Sprintf("k3d-radius-%s", name),
		Registry:    fmt.Sprintf("radius-%s-registry", name),
	}

	args := []string{
		"cluster", "create", config.ClusterName,

		// Create a registry for local images to avoid server roundtrips
		"--registry-create", config.Registry,

		// Add a new kubernetes context to the config, but don't switch to it
		"--kubeconfig-update-default=true",
		"--kubeconfig-switch-context=false",

		// Skip the built-in ingress since we're providing our own
		"--k3s-arg", "--disable=traefik@server:*",

		// Map HTTP & HTTPS ports to a dynamic port on the host
		"--port", "80@loadbalancer:*", "--port", "443@loadbalancer:*",

		// Wait for nodes to start
		"--wait",
	}

	cmd := exec.CommandContext(ctx, "k3d", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	// We don't need to show anything interactive here, the k3d binary outputs progress
	// messages.

	err = cmd.Wait()
	if err != nil {
		return nil, err
	}

	return &config, nil
}

type ClusterConfig struct {
	ClusterName string
	ContextName string
	Registry    string
}
