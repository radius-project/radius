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
	writer := options.Output.Writer()
	cmd.Stdout = writer
	cmd.Stderr = writer

	options.Output.Print(fmt.Sprintf("running: %s\n", cmd.String()))
	err = cmd.Start()
	if err != nil {
		_ = writer.Close()
		return Output{}, err
	}

	err = cmd.Wait()
	_ = writer.Close()
	if err != nil {
		return Output{}, err
	}

	args = []string{
		"push",
		input.Image,
	}
	cmd = exec.CommandContext(ctx, "docker", args...)
	writer = options.Output.Writer()
	cmd.Stdout = writer
	cmd.Stderr = writer

	options.Output.Print(fmt.Sprintf("running: %s\n", cmd.String()))
	err = cmd.Start()
	if err != nil {
		_ = writer.Close()
		return Output{}, err
	}

	err = cmd.Wait()
	if err != nil {
		return Output{}, err
	}
	_ = writer.Close()

	output := Output{
		Result: map[string]interface{}{
			"image": input.Image,
		},
	}

	return output, nil
}
