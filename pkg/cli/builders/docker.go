// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package builders

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

var _ Builder = (*dockerBuilder)(nil)

type dockerBuilder struct {
}

type dockerInput struct {
	Context    string `json:"context"`
	DockerFile string `json:"dockerFile"`
	Image      string `json:"image"`
}

func (builder *dockerBuilder) Build(ctx context.Context, options Options) (Output, error) {
	b, err := json.Marshal(&options.Values)
	if err != nil {
		return Output{}, err
	}

	input := dockerInput{}
	err = json.Unmarshal(b, &input)
	if err != nil {
		return Output{}, err
	}

	if input.Context == "" {
		return Output{}, fmt.Errorf("%s is required", "context")
	}
	if input.Image == "" {
		return Output{}, fmt.Errorf("%s is required", "image")
	}
	if input.DockerFile == "" {
		input.DockerFile = "Dockerfile"
	}

	input.Context = NormalizePath(options.BaseDirectory, input.Context)
	input.DockerFile = NormalizePath(input.Context, input.DockerFile)

	args := []string{
		"build",
		input.Context,
		"-f", input.DockerFile,
		"-t", input.Image,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr

	fmt.Printf("running: %s\n", cmd.String())
	err = cmd.Run()
	if err != nil {
		return Output{}, err
	}

	args = []string{
		"push",
		input.Image,
	}
	cmd = exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr

	err = cmd.Run()
	if err != nil {
		return Output{}, err
	}

	output := Output{
		Result: map[string]interface{}{
			"image": input.Image,
		},
	}

	return output, nil
}
