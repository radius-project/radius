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
		ClusterName:  fmt.Sprintf("radius-%s", name),
		ContextName:  fmt.Sprintf("k3d-radius-%s", name),
		RegistryName: fmt.Sprintf("radius-%s-registry", name),
	}

	args := []string{
		"cluster", "create", config.ClusterName,

		// Create a registry for local images to avoid server roundtrips
		"--registry-create", config.RegistryName,

		// Add a new kubernetes context to the config and switch to it
		"--kubeconfig-update-default=true",
		"--kubeconfig-switch-context=true",

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

	// Now we need to get the registry push URL since this will be determined dynamically.
	client := ServerLifecycleClient{ClusterName: config.ClusterName}
	config.RegistryPushEndpoint, err = client.GetRegistryEndpoint(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to determine local registry URL: %w", err)
	}

	config.HTTPEndpoint, config.HTTPSEndpoint, err = client.getIngressEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to determine public endpoints: %w", err)
	}

	// We're using the default port of 5000 here.
	config.RegistryPullEndpoint = config.RegistryName + ":5000"

	return &config, nil
}

type ClusterConfig struct {
	ClusterName          string
	ContextName          string
	RegistryName         string
	RegistryPushEndpoint string
	RegistryPullEndpoint string
	HTTPEndpoint         string
	HTTPSEndpoint        string
}
