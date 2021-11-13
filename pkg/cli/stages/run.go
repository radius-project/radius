// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package stages

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/output"
)

// Run processes the stages of a rad.yaml. This is expected to be used from the CLI and thus writes
// output to the console.
func Run(ctx context.Context, options Options) ([]StageResult, error) {
	output.LogInfo("Using environment %s", options.Environment.GetName())

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
		stage := options.Manifest.Stages[i]

		result := StageResult{
			Stage: &stage,
			Input: clients.ShallowCopy(processor.Parameters),
		}

		output.LogInfo("")
		step := output.BeginStep("Processing stage %s: %d of %d", stage.Name, i+1, length)

		if stage.Bicep != nil {
			err := processor.ProcessDeploy(ctx, *stage.Bicep)
			if err != nil {
				return nil, fmt.Errorf("stage %s failed: %w", stage.Name, err)
			}
		} else {
			output.LogInfo("Nothing to do for stage %s...", stage.Name)
		}

		output.CompleteStep(step)

		// Record results for testability
		result.Output = clients.ShallowCopy(processor.Parameters)
		processor.Results = append(processor.Results, result)
	}

	return processor.Results, nil
}
