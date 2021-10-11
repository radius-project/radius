// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package layers

import (
	"context"
	"errors"
	"os"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/bicep"
	"github.com/Azure/radius/pkg/cli/builders"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/deploy"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/radyaml"
)

func Process(ctx context.Context, env environments.Environment, app radyaml.RADYaml, layersToProcess []radyaml.RADYamlLayer, all bool) error {
	if len(layersToProcess) == 0 {
		output.LogInfo("Nothing to do...")
	}

	processor := &processor{
		Application: app,
		Env:         env,
		Parameters:  map[string]map[string]interface{}{},
	}

	for i, layer := range layersToProcess {
		step := output.BeginStep("Processing layer %s: %d of %d", layer.Name, i+1, len(layersToProcess))

		// Always perform an action on the last layer
		force := all || len(layersToProcess)-1 == i
		if layer.Build != nil {
			err := processor.ProcessBuild(ctx, *layer.Build, force)
			if err != nil {
				return err
			}

			continue
		}

		if layer.Deploy != nil {
			err := processor.ProcessDeploy(ctx, layer.Name, *layer.Deploy, force)
			if err != nil {
				return err
			}

			continue
		}

		output.CompleteStep(step)
		output.LogInfo("")
		output.LogInfo("")
	}

	return nil
}

type processor struct {
	Application radyaml.RADYaml
	Env         environments.Environment
	Parameters  clients.DeploymentParameters
}

func (p *processor) ProcessBuild(ctx context.Context, targets []radyaml.RADYamlLayerBuildTarget, force bool) error {
	for _, target := range targets {
		if target.Docker == nil {
			return errors.New("no builder was specified")
		}

		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		builder := builders.GetBuilders()["docker"]
		values, err := builder.Build(ctx, *target.Docker, builders.BuilderOptions{BaseDirectory: wd})
		if err != nil {
			return err
		}

		p.Parameters[target.Name] = map[string]interface{}{
			"value": values,
		}
	}

	return nil
}

func (p *processor) ProcessDeploy(ctx context.Context, name string, deployFile string, force bool) error {
	client, err := environments.CreateDeploymentClient(ctx, p.Env)
	if err != nil {
		return err
	}

	if !force {
		existing, err := client.GetExistingDeployment(ctx, clients.DeploymentOptions{
			DeploymentName: name,
		})
		if err != nil {
			return err
		}

		if existing != nil {
			output.LogInfo("Found existing deployment")

			// Outputs already include .value
			for key, output := range existing.Outputs {
				p.Parameters[key] = map[string]interface{}{
					"value": output.Value,
				}
			}

			return nil
		}
	}

	err = deploy.ValidateBicepFile(deployFile)
	if err != nil {
		return err
	}

	step := output.BeginStep("Building %s...", deployFile)
	template, err := bicep.Build(deployFile)
	if err != nil {
		return err
	}
	output.CompleteStep(step)

	step = output.BeginStep("Deploying %s...", deployFile)
	options := clients.DeploymentOptions{
		Template:       template,
		Parameters:     p.Parameters,
		UpdateChannel:  nil,
		DeploymentName: name,
	}

	output.LogInfo("")
	result, err := deploy.PerformDeployment(ctx, client, options)
	if err != nil {
		return err
	}

	output.LogInfo("")
	output.CompleteStep(step)

	output.LogInfo("Deployment Complete")
	output.LogInfo("")

	output.LogInfo("Resources:")
	output.LogInfo("")

	for _, resource := range result.Resources {
		if cli.ShowResource(resource) {
			output.LogInfo("%-30s %-15s", cli.FormatTypeForDisplay(resource), resource.Name())
		}
	}

	diag, err := environments.CreateDiagnosticsClient(ctx, p.Env)
	if err != nil {
		return err
	}

	endpoints := []struct {
		ResourceID azresources.ResourceID
		Endpoint   string
	}{}
	for _, resource := range result.Resources {
		if cli.FormatTypeForDisplay(resource) == "HttpRoute" {
			endpoint, err := diag.GetPublicEndpoint(ctx, clients.EndpointOptions{ResourceID: resource})
			if err != nil {
				return err
			}

			if endpoint != nil {
				endpoints = append(endpoints, struct {
					ResourceID azresources.ResourceID
					Endpoint   string
				}{ResourceID: resource, Endpoint: *endpoint})
			}
		}
	}

	if len(endpoints) > 0 {
		output.LogInfo("")
		output.LogInfo("Public Endpoints:")
		output.LogInfo("")

		for _, entry := range endpoints {
			output.LogInfo("%-30s %-15s %s", cli.FormatTypeForDisplay(entry.ResourceID), entry.ResourceID.Name(), entry.Endpoint)
		}
	}

	// Outputs already include .value
	for key, output := range result.Outputs {
		p.Parameters[key] = map[string]interface{}{
			"value": output.Value,
		}
	}

	return nil
}
