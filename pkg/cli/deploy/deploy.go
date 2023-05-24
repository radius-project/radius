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

package deploy

import (
	"context"
	"sync"

	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/output"
)

// DeployWithProgress runs a deployment and displays progress to the user. This is intended to be used
// from the CLI and thus logs to the console.
func DeployWithProgress(ctx context.Context, options Options) (clients.DeploymentResult, error) {
	deploymentClient, err := options.ConnectionFactory.CreateDeploymentClient(ctx, options.Workspace)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	err = bicep.InjectEnvironmentParam(options.Template, options.Parameters, options.Providers.Radius.EnvironmentID)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	err = bicep.InjectApplicationParam(options.Template, options.Parameters, options.Providers.Radius.ApplicationID)
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
		Providers:    options.Providers,
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
