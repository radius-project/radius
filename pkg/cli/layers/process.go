// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package layers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/cli/bicep"
	"github.com/Azure/radius/pkg/cli/builders"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/deploy"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/radyaml"
)

func Process(ctx context.Context, env environments.Environment, baseDir string, app radyaml.Manifest, layersToProcess []radyaml.Stage, all bool) error {
	if len(layersToProcess) == 0 {
		output.LogInfo("Nothing to do...")
	}

	processor := &processor{
		Application: app,
		BaseDir:     baseDir,
		CacheDir:    path.Join(baseDir, ".cache"),
		Env:         env,
		Build:       map[string]*buildResult{},
		Parameters:  map[string]map[string]interface{}{},
	}

	for _, build := range app.Build {
		processor.Build[build.Name] = &buildResult{Target: build}
	}

	for i, layer := range layersToProcess {
		output.LogInfo("")
		step := output.BeginStep("Processing stage %s: %d of %d", layer.Name, i+1, len(layersToProcess))

		// Always perform an action on the last layer
		force := all || len(layersToProcess)-1 == i
		if layer.Deploy != nil {
			for _, param := range layer.Deploy.Params {
				result, err := processor.ProcessBuild(ctx, param)
				if err != nil {
					return err
				}

				processor.Parameters[param.Name] = map[string]interface{}{
					"value": result.Result,
				}
			}

			// Cache results that don't accept a build as input.
			err := processor.ProcessDeploy(ctx, layer.Name, *layer.Deploy, force, len(layer.Deploy.Params) == 0)
			if err != nil {
				return err
			}
		}

		output.CompleteStep(step)
	}

	return nil
}

type processor struct {
	Application radyaml.Manifest
	BaseDir     string
	CacheDir    string
	Env         environments.Environment
	Build       map[string]*buildResult
	Parameters  clients.DeploymentParameters
}

type buildResult struct {
	Target radyaml.BuildTarget
	Result map[string]interface{}
}

func (p *processor) ProcessBuild(ctx context.Context, param radyaml.DeployStageParameter) (*buildResult, error) {
	br, ok := p.Build[param.Name]
	if !ok {
		return nil, fmt.Errorf("no build is defined matching name %s", param.Name)
	}

	if br.Result != nil {
		return br, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if br.Target.Container != nil {
		builder, ok := builders.GetBuilders()["container"]
		if !ok {
			return nil, fmt.Errorf("builder %q is not supported", "container")
		}

		values, err := builder.Build(ctx, *br.Target.Container, builders.BuilderOptions{BaseDirectory: wd})
		if err != nil {
			return nil, err
		}

		br.Result = values
	} else if br.Target.NPM != nil {
		builder, ok := builders.GetBuilders()["npm"]
		if !ok {
			return nil, fmt.Errorf("builder %q is not supported", "npm")
		}

		values, err := builder.Build(ctx, *br.Target.NPM, builders.BuilderOptions{BaseDirectory: wd})
		if err != nil {
			return nil, err
		}

		br.Result = values
	} else {
		return nil, errors.New("no builder was specified")
	}

	return br, nil
}

func (p *processor) ProcessDeploy(ctx context.Context, name string, stage radyaml.DeployStage, force bool, cache bool) error {
	deployFile := *stage.Bicep
	deployFile = path.Join(p.BaseDir, deployFile)

	client, err := environments.CreateDeploymentClient(ctx, p.Env)
	if err != nil {
		return err
	}

	if !force {
		var existing *clients.DeploymentResult

		// Try the cache first
		cacheFile := path.Join(p.CacheDir, fmt.Sprintf("deploy-%s.json", name))
		b, err := ioutil.ReadFile(cacheFile)
		if os.IsNotExist(err) {
			// No cache
		} else if err != nil {
			return err
		} else {
			outputs := map[string]clients.DeploymentOutput{}
			err = json.Unmarshal(b, &outputs)
			if err != nil {
				return err
			}

			existing = &clients.DeploymentResult{Outputs: outputs}
		}

		if existing == nil {
			existing, err = client.GetExistingDeployment(ctx, clients.DeploymentOptions{
				DeploymentName: name,
			})
			if err != nil {
				return err
			}
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

	if len(result.Resources) > 0 {
		output.LogInfo("Resources:")

		for _, resource := range result.Resources {
			if output.ShowResource(resource) {
				output.LogInfo("    " + output.FormatResourceForDisplay(resource))
			}
		}

		endpoints, err := findPublicEndpoints(ctx, p.Env, result)
		if err != nil {
			return err
		}

		if len(endpoints) > 0 {
			output.LogInfo("")
			output.LogInfo("Public Endpoints:")

			for _, entry := range endpoints {
				output.LogInfo("    %s %s", output.FormatResourceForDisplay(entry.Resource), entry.Endpoint)
			}
		}
	}

	if cache {
		// Store outputs in the cache.
		b, err := json.Marshal(result.Outputs)
		if err != nil {
			return err
		}

		err = os.MkdirAll(p.CacheDir, 0755)
		if err != nil {
			return err
		}

		err = ioutil.WriteFile(path.Join(p.CacheDir, fmt.Sprintf("deploy-%s.json", name)), b, 0644)
		if err != nil {
			return err
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

type publicEndpoint struct {
	Resource azresources.ResourceID
	Endpoint string
}

func findPublicEndpoints(ctx context.Context, env environments.Environment, result clients.DeploymentResult) ([]publicEndpoint, error) {
	diag, err := environments.CreateDiagnosticsClient(ctx, env)
	if err != nil {
		return nil, err
	}

	endpoints := []publicEndpoint{}
	for _, resource := range result.Resources {
		endpoint, err := diag.GetPublicEndpoint(ctx, clients.EndpointOptions{ResourceID: resource})
		if err != nil {
			return nil, err
		}

		if endpoint == nil {
			continue
		}

		endpoints = append(endpoints, publicEndpoint{Resource: resource, Endpoint: *endpoint})
	}

	return endpoints, nil
}
