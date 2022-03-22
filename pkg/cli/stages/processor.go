// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sync"

	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/builders"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/deploy"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/radyaml"
	"golang.org/x/sync/errgroup"
)

func (p *processor) ProcessBuild(ctx context.Context, stage radyaml.BuildStage) error {
	registry := p.Options.Environment.GetContainerRegistry()

	// We'll run the build in parallel - each output gets its own output stream.
	group, ctx := errgroup.WithContext(ctx)
	streams := output.NewStreamGroup(p.Stdout)

	// Synchronize access to add results
	mtx := sync.Mutex{}

	for name, target := range stage {
		stream := streams.NewStream(name)

		// Copy induction variables since they are closed-over.
		name := name
		target := target

		group.Go(func() error {
			stream.Print(fmt.Sprintf("Processing build %s\n", name))

			builder := p.Options.Builders[target.Builder]
			if builder == nil {
				return fmt.Errorf("no builder named %s was found", target.Builder)
			}

			result, err := builder.Build(ctx, builders.Options{
				BaseDirectory: p.BaseDirectory,
				Registry:      registry,
				Output:        stream,
				Values:        target.Values,
			})
			if err != nil {
				return fmt.Errorf("build of %s failed: %w", target.Builder, err)
			}

			mtx.Lock()
			defer mtx.Unlock()
			p.Parameters[name] = map[string]interface{}{
				"value": result.Result,
			}

			stream.Print(fmt.Sprintf("Done processing build %s\n", name))
			return nil
		})
	}

	err := group.Wait()
	if err != nil {
		return err
	}

	return nil
}

func (p *processor) BuildBicep(ctx context.Context, deployFile string) (string, error) {
	err := deploy.ValidateBicepFile(deployFile)
	if err != nil {
		return "", err
	}

	step := output.BeginStep("Building %s...", deployFile)
	template, err := bicep.Build(deployFile)
	if err != nil {
		return "", err
	}
	output.CompleteStep(step)
	return template, nil
}

func (p *processor) ProcessDeploy(ctx context.Context, stage radyaml.BicepStage) error {
	deployFile := path.Join(p.BaseDirectory, *stage.Template)
	template, err := p.BicepBuildFunc(ctx, deployFile)
	if err != nil {
		return err
	}

	parser := bicep.ParameterParser{FileSystem: bicep.OSFileSystem{}}

	// Get parameters from stage definition
	stageParameters, err := json.Marshal(stage.Parameters)
	if err != nil {
		return err
	}

	// Convert stage definition parameters to map[string]interface{}
	var stageTemplateParameters map[string]interface{}
	if err = json.Unmarshal([]byte(stageParameters), &stageTemplateParameters); err != nil {
		return err
	}

	templateParameters := make(map[string]map[string]interface{})
	templateParameters[p.CurrentStage.Name] = stageTemplateParameters

	// Get parameters from parsed stage template
	// This will overwrite defined stage parameters from above
	if template != "" {
		templateParameters, err = parser.ParseFileContents([]byte(template))
		if err != nil {
			return err
		}
	}

	// We might have additional parameters that are specific to this stage, so make a copy
	// that way the parameters don't leak outside the stage.
	parameters := clients.ShallowCopy(p.Parameters)
	if stage.ParameterFile != nil {
		filePath := path.Join(p.BaseDirectory, *stage.ParameterFile)

		parsedFile, err := parser.Parse(filePath)
		if err != nil {
			return err
		}

		for key, value := range parsedFile {
			if _, ok := templateParameters[key]; !ok {
				delete(parameters, key)
				parameters[key] = value
			}
		}
	}

	for key := range p.Parameters {
		if _, ok := templateParameters[key]; !ok {
			delete(parameters, key)
		}
	}

	progressText := fmt.Sprintf("Deploying %s...", deployFile)
	completionText := fmt.Sprintf("Deployed stage %s: %d of %d", p.CurrentStage.Name, p.CurrentStage.DisplayIndex, p.CurrentStage.TotalCount)

	result, err := deploy.DeployWithProgress(ctx, deploy.Options{
		Environment:    p.Environment,
		Template:       template,
		Parameters:     parameters,
		ProgressText:   progressText,
		CompletionText: completionText,
	})
	if err != nil {
		return err
	}

	if result.Outputs != nil {
		for key, output := range result.Outputs {
			p.Parameters[key] = map[string]interface{}{
				"value": output.Value,
			}
		}
	}

	return nil
}
