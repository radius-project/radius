// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package k3d

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Azure/radius/pkg/cli/clients"
)

var _ clients.ServerLifecycleClient = (*ServerLifecycleClient)(nil)

type ServerLifecycleClient struct {
	ClusterName string
}

func (c *ServerLifecycleClient) GetStatus(ctx context.Context) (string, error) {
	err := RequireK3dInstalled()
	if err != nil {
		return "", err
	}

	nodes, err := c.getNodeStatus(ctx)
	if err != nil {
		nodes = fmt.Sprintf("error %s", err.Error())
	}

	registry, err := c.getRegistryEndpoint(ctx)
	if err != nil {
		registry = fmt.Sprintf("error %s", err.Error())
	}

	http, https, err := c.getIngressEndpoints(ctx)
	if err != nil {
		http = fmt.Sprintf("error %s", err.Error())
		https = fmt.Sprintf("error %s", err.Error())
	}

	template := `Nodes:            %s
Registry:         %s
Ingress (http):   %s
Ingress (https):  %s`

	return fmt.Sprintf(template, nodes, registry, http, https), nil
}

func (c *ServerLifecycleClient) IsRunning(ctx context.Context) (bool, error) {
	err := RequireK3dInstalled()
	if err != nil {
		return false, err
	}

	_, err = c.getNodeStatus(ctx)
	return err == nil, nil
}

func (c *ServerLifecycleClient) EnsureStarted(ctx context.Context) error {
	err := RequireK3dInstalled()
	if err != nil {
		return err
	}

	// Start/Stop commands provided by k3d are idempotent.
	cmd := exec.CommandContext(ctx, "k3d", "cluster", "start", c.ClusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start cluster: %s", output)
	}

	return nil
}

func (c *ServerLifecycleClient) EnsureStopped(ctx context.Context) error {
	err := RequireK3dInstalled()
	if err != nil {
		return err
	}

	// Start/Stop commands provided by k3d are idempotent.
	cmd := exec.CommandContext(ctx, "k3d", "cluster", "stop", c.ClusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop cluster: %s", output)
	}

	return nil
}

func (c *ServerLifecycleClient) listClusterStatus(ctx context.Context, clusterName string) (*Cluster, error) {
	// To check the status of the cluster we use the list command.
	cmd := exec.CommandContext(ctx, "k3d", "cluster", "list", "--output", "json")

	// Run the command and capture stdout. StdErr will be part of the error returned.
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	clusters := []Cluster{}
	err = json.Unmarshal(output, &clusters)
	if err != nil {
		return nil, fmt.Errorf("k3d command returned invalid JSON. Got: %s", string(output))
	}

	for _, cluster := range clusters {
		if strings.EqualFold(cluster.Name, clusterName) {
			copy := cluster
			return &copy, nil
		}
	}

	return nil, fmt.Errorf("k3d cluster %s was not found. Try recreating the environment with 'rad env init dev'", clusterName)
}

func (c *ServerLifecycleClient) getNodeStatus(ctx context.Context) (string, error) {
	cluster, err := c.listClusterStatus(ctx, c.ClusterName)
	if err != nil {
		return "", err
	}

	for _, node := range cluster.Nodes {
		if !node.State.Running {
			return "", fmt.Errorf("node %s is not running", node.Name)
		}
	}

	return "ready", nil
}

func (c *ServerLifecycleClient) getRegistryEndpoint(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "port", fmt.Sprintf("%s-registry", c.ClusterName))
	b := bytes.Buffer{}
	cmd.Stdout = &b
	err := cmd.Start()
	if err != nil {
		return "", err
	}

	err = cmd.Wait()
	if err != nil {
		return "", err
	}

	ports, err := parsePorts(b.String())
	if err != nil {
		return "", err
	}

	if len(ports) == 0 {
		return "", fmt.Errorf("no ports found")
	}

	return fmt.Sprintf("localhost:%s", ports[0].HostPort), nil
}

func (c *ServerLifecycleClient) getIngressEndpoints(ctx context.Context) (string, string, error) {
	cmd := exec.CommandContext(ctx, "docker", "port", fmt.Sprintf("k3d-%s-serverlb", c.ClusterName))
	b := bytes.Buffer{}
	cmd.Stdout = &b
	err := cmd.Start()
	if err != nil {
		return "", "", err
	}

	err = cmd.Wait()
	if err != nil {
		return "", "", err
	}

	ports, err := parsePorts(b.String())
	if err != nil {
		return "", "", err
	}

	httpEndpoint := ""
	httpsEndpoint := ""

	for _, port := range ports {
		if port.ContainerPort == "80" {
			httpEndpoint = fmt.Sprintf("http://localhost:%s", port.HostPort)
		} else if port.ContainerPort == "443" {
			httpsEndpoint = fmt.Sprintf("https://localhost:%s", port.HostPort)
		}
	}

	return httpEndpoint, httpsEndpoint, nil
}

func parsePorts(input string) ([]DockerPort, error) {
	regex := regexp.MustCompile(`(?m:^(.*)/(tcp|udp) -> 0\.0\.0\.0:(.*)$)`)
	matches := regex.FindAllStringSubmatch(input, -1)

	results := []DockerPort{}
	for _, match := range matches {
		if len(match) != 4 {
			return nil, fmt.Errorf("could not match input %q", input)
		}

		results = append(results, DockerPort{
			ContainerPort: match[1],
			Protocol:      match[2],
			HostPort:      match[3],
		})
	}

	return results, nil
}

type DockerPort struct {
	ContainerPort string
	HostPort      string
	Protocol      string
}

type ClusterStatus struct {
	ClusterName    string
	ServersRunning int
	ServersTotal   int
	Ready          bool
}

// Reverse engineered from `k3d cluster status --output json`
type Cluster struct {
	Name  string        `json:"name"`
	Nodes []ClusterNode `json:"nodes"`
}

type ClusterNode struct {
	Name  string           `json:"name"`
	Role  string           `json:"role"`
	State ClusterNodeState `json:"State"` // YES it's upper case. I don't like it either :-/
}

type ClusterNodeState struct {
	Running bool   `json:"Running"`
	Status  string `json:"Status"`
	Started string `json:"Started"`
}
