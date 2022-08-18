// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
)

func ValidateBicepFile(filePath string) error {
	_, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("could not find file: %w", err)
	}

	if !strings.EqualFold(path.Ext(filePath), ".bicep") {
		return errors.New("file must be a .bicep file")
	}

	return nil
}

func ReadARMJSON(filePath string) (map[string]interface{}, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read json file: %w", err)
	}

	var template map[string]interface{}
	err = json.Unmarshal(bytes, &template)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal json file: %w", err)
	}

	return template, nil
}

type Options struct {
	Workspace         workspaces.Workspace
	ConnectionFactory connections.Factory
	Template          map[string]interface{}
	Parameters        clients.DeploymentParameters
	ProgressText      string
	CompletionText    string
}

// DeployWithProgress run a deployment and displays progress to the user. This is intended to be used
// from the CLI and thus logs to the console.
func DeployWithProgress(ctx context.Context, options Options) (clients.DeploymentResult, error) {
	if options.ConnectionFactory == nil {
		options.ConnectionFactory = connections.DefaultFactory
	}

	var deploymentClient clients.DeploymentClient
	var err error
	deploymentClient, err = options.ConnectionFactory.CreateDeploymentClient(ctx, options.Workspace)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	step := output.BeginStep(options.ProgressText)
	output.LogInfo("")

	// Watch for progress while we're deploying.
	progressChan := make(chan clients.ResourceProgress, 1)
	listener := NewProgressListener(progressChan)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		listener.Run()
		wg.Done()
	}()

	result, err := deploymentClient.Deploy(ctx, clients.DeploymentOptions{
		Template:     options.Template,
		Parameters:   options.Parameters,
		ProgressChan: progressChan,
	})

	// Drain any UI progress updates before we process the results of the deployment.
	wg.Wait()
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	output.LogInfo("")
	output.CompleteStep(step)

	output.LogInfo(options.CompletionText)
	output.LogInfo("")

	if len(result.Resources) > 0 {
		output.LogInfo("Resources:")

		for _, resource := range result.Resources {
			if output.ShowResource(resource) {
				output.LogInfo("    " + output.FormatResourceForDisplay(resource))
			}
		}
		var diagnosticsClient clients.DiagnosticsClient
		diagnosticsClient, err = options.ConnectionFactory.CreateDiagnosticsClient(ctx, options.Workspace)
		if err != nil {
			return clients.DeploymentResult{}, err
		}

		endpoints, err := FindPublicEndpoints(ctx, diagnosticsClient, result)
		if err != nil {
			return clients.DeploymentResult{}, err
		}

		if len(endpoints) > 0 {
			output.LogInfo("")
			output.LogInfo("Public Endpoints:")

			for _, entry := range endpoints {
				output.LogInfo("    %s %s", output.FormatResourceForDisplay(entry.Resource), entry.Endpoint)
			}
		}
	}

	return result, nil
}
