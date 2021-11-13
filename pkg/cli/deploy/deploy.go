// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/output"
)

func ValidateBicepFile(filePath string) error {
	_, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("could not find file: %w", err)
	}

	if path.Ext(filePath) != ".bicep" {
		return errors.New("file must be a .bicep file")
	}

	return nil
}

type Options struct {
	Environment  environments.Environment
	Template     string
	Parameters   clients.DeploymentParameters
	ProgressText string
}

// DeployWithProgress run a deployment and displays progress to the user. This is intended to be used
// from the CLI and thus logs to the console.
func DeployWithProgress(ctx context.Context, options Options) (clients.DeploymentResult, error) {
	deploymentClient, err := environments.CreateDeploymentClient(ctx, options.Environment)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	step := output.BeginStep(options.ProgressText)
	output.LogInfo("")
	result, err := deploymentClient.Deploy(ctx, clients.DeploymentOptions{
		Template:   options.Template,
		Parameters: options.Parameters,
	})
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	output.LogInfo("")
	output.CompleteStep(step)

	output.LogInfo("Deployment Complete")
	output.LogInfo("")

	if len(result.Resources) > 0 {
		output.LogInfo("Resources:")

		for _, resource := range result.Resources {
			if output.ShowResource(resource) {
				output.LogInfo("    " + output.FormatResourceForDisplay(resource))
			}
		}

		diagnosticsClient, err := environments.CreateDiagnosticsClient(ctx, options.Environment)
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
