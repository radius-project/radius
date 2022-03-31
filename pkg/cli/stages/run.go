// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package stages

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/radyaml"
)

// Run processes the stages of a rad.yaml. This is expected to be used from the CLI and thus writes
// output to the console.
func Run(ctx context.Context, options Options) ([]StageResult, error) {
	output.LogInfo("Using environment %s", options.Environment.GetName())

	// Unit tests don't have to set these.
	if options.Stderr == nil {
		options.Stderr = io.Discard
	}
	if options.Stdout == nil {
		options.Stdout = io.Discard
	}

	// Validate that the desired stage is found
	length := len(options.Manifest.Stages)
	if options.FinalStage != "" {
		found := false
		for i, stage := range options.Manifest.Stages {
			if strings.EqualFold(options.FinalStage, stage.Name) {
				length = i + 1
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("stage %q not found in rad.yaml", options.FinalStage)
		}
	}

	// Validate and process stages up front so we can report errors eagerly.
	// Note: we process all stages here so we can validate the ones
	// that aren't running.
	stages := []radyaml.Stage{}
	for _, raw := range options.Manifest.Stages {
		stage, err := raw.ApplyProfile(options.Profile)
		if err != nil {
			return nil, err
		}

		stages = append(stages, stage)
	}

	if length == 0 {
		output.LogInfo("Nothing to do...")
		return nil, nil
	}

	processor := &processor{
		Options:    options,
		Parameters: clients.ShallowCopy(options.Parameters),
	}

	if processor.Options.BicepBuildFunc == nil {
		processor.Options.BicepBuildFunc = processor.BuildBicep
	}

	for i := 0; i < length; i++ {
		stage := stages[i]

		processor.CurrentStage = stageInfo{
			Name:         stage.Name,
			DisplayIndex: i + 1,
			TotalCount:   length,
		}

		result := StageResult{
			Stage: &stage,
			Input: clients.ShallowCopy(processor.Parameters),
		}

		output.LogInfo("")
		step := output.BeginStep("Processing stage %s: %d of %d", processor.CurrentStage.Name, processor.CurrentStage.DisplayIndex, processor.CurrentStage.TotalCount)

		if stage.Build != nil {
			err := processor.ProcessBuild(ctx, stage.Build)
			if err != nil {
				return nil, fmt.Errorf("stage %s failed: %w", processor.CurrentStage.Name, err)
			}
		}

		if stage.Bicep != nil {
			err := processor.ProcessDeploy(ctx, *stage.Bicep)
			if err != nil {
				return nil, fmt.Errorf("stage %s failed: %w", processor.CurrentStage.Name, err)
			}
		} else {
			output.LogInfo("No deployment step for stage %s...", processor.CurrentStage.Name)
		}

		output.CompleteStep(step)

		// Record results for testability
		result.Output = clients.ShallowCopy(processor.Parameters)
		processor.Results = append(processor.Results, result)
	}

	return processor.Results, nil
}
