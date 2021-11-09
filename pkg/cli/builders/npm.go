// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package builders

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/Azure/radius/pkg/cli/output"
	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/pkg/project/types"
)

var _ Builder = (*dockerBuilder)(nil)

type npmBuilder struct {
}

type npmInput struct {
	Directory string             `json:"directory"`
	Script    string             `json:"script"`
	Container *npmContainerInput `json:"container,omitempty"`
}

type npmContainerInput struct {
	Image string `json:"image"`
}

func (builder *npmBuilder) Build(ctx context.Context, values interface{}, options BuilderOptions) (map[string]interface{}, error) {
	b, err := json.Marshal(&values)
	if err != nil {
		return nil, err
	}

	input := npmInput{}
	err = json.Unmarshal(b, &input)
	if err != nil {
		return nil, err
	}

	if input.Directory == "" {
		return nil, fmt.Errorf("%s is required", "directory")
	}

	input.Directory = normalize(options.BaseDirectory, input.Directory)

	if builder.ShouldBuildContainer(input, options) {
		return builder.BuildContainer(ctx, input, options)
	}

	return builder.BuildExecutable(ctx, input, options)
}

func (builder *npmBuilder) ShouldBuildContainer(input npmInput, options BuilderOptions) bool {
	return input.Container != nil && options.PreferContainer
}

func (builder *npmBuilder) BuildExecutable(ctx context.Context, input npmInput, options BuilderOptions) (map[string]interface{}, error) {
	output.LogInfo("Building %s for local run...", input.Directory)
	if input.Script != "" {
		cmd := exec.CommandContext(ctx, "npm", "run-script", input.Script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		err := cmd.Run()
		if err != nil {
			return nil, err
		}
	}

	output := map[string]interface{}{
		"kind":             "executable",
		"name":             "npm",
		"workingDirectory": input.Directory,
		"args": []string{
			"start",
			input.Script,
		},
	}

	return output, nil
}

func (builder *npmBuilder) BuildContainer(ctx context.Context, input npmInput, options BuilderOptions) (map[string]interface{}, error) {
	output.LogInfo("Building %s as a container...", input.Directory)
	c, err := pack.NewClient()
	if err != nil {
		return nil, err
	}

	env := map[string]string{
		"NODE_ENV": "development",
	}

	if input.Script != "" {
		env["BP_NODE_RUN_SCRIPTS"] = input.Script
	}

	err = c.Build(ctx, pack.BuildOptions{
		RelativeBaseDir: input.Directory,
		Image:           input.Container.Image,
		Builder:         "paketobuildpacks/builder:base",
		Env:             env,
		ProjectDescriptor: types.Descriptor{
			Build: types.Build{
				Exclude: []string{
					"node_modules",
					"project-lock.json",
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	args := []string{
		"push",
		fmt.Sprintf("%s:%s", input.Container.Image, "latest"),
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	output := map[string]interface{}{
		"kind":  "container",
		"image": input.Container.Image + ":latest",
	}

	return output, nil
}
