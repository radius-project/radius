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

func Process(ctx context.Context, env environments.Environment, app radyaml.Manifest, layersToProcess []radyaml.Stage, all bool) error {
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
		if layer.Deploy != nil {
			for _, param := range layer.Deploy.Params {
				processor.ProcessBuild(ctx, param)
			}

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
	Application radyaml.Manifest
	Env         environments.Environment
	Parameters  clients.DeploymentParameters
}

func (p *processor) ProcessBuild(ctx context.Context, param radyaml.DeployStageParameter) error {
	if param.Container == nil {
		return errors.New("no builder was specified")
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	builder := builders.GetBuilders()["container"]
	values, err := builder.Build(ctx, *param.Container, builders.BuilderOptions{BaseDirectory: wd})
	if err != nil {
		return err
	}

	p.Parameters[param.Name] = map[string]interface{}{
		"value": values,
	}

	return nil
}

func (p *processor) ProcessDeploy(ctx context.Context, name string, stage radyaml.DeployStage, force bool) error {
	deployFile := *stage.Bicep
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
