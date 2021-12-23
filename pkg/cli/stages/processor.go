// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package stages

import (
	"context"
	"fmt"
	"path"

	"github.com/Azure/radius/pkg/cli/bicep"
	"github.com/Azure/radius/pkg/cli/builders"
	"github.com/Azure/radius/pkg/cli/deploy"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/radyaml"
)

func (p *processor) ProcessBuild(ctx context.Context, stage radyaml.BuildStage) error {
	for name, target := range stage {
		step := output.BeginStep("Processing build %s", name)

		builder := p.Options.Builders[target.Builder]
		if builder == nil {
			return fmt.Errorf("no builder named %s was found", target.Builder)
		}

		result, err := builder.Build(ctx, builders.Options{
			BaseDirectory: p.BaseDirectory,
			Stderr:        p.Stderr,
			Stdout:        p.Stdout,
			Values:        target.Values,
		})
		if err != nil {
			return fmt.Errorf("build of %s failed: %w", target.Builder, err)
		}

		p.Parameters[name] = map[string]interface{}{
			"value": result.Result,
		}

		output.CompleteStep(step)
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

	progressText := fmt.Sprintf("Deploying %s...", deployFile)
	completionText := fmt.Sprintf("Deployed stage %s: %d of %d", p.CurrrentStage.Name, p.CurrrentStage.DisplayIndex, p.CurrrentStage.TotalCount)

	result, err := deploy.DeployWithProgress(ctx, deploy.Options{
		Environment:    p.Environment,
		Template:       template,
		Parameters:     p.Parameters,
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
