// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package builders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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

	// Get the image digest so we can be precise about the version
	//
	// NOTE: this isn't correct for multi-platform images since they have multiple manifests
	// however, we don't produce multi-platform images on this code path so it's not an issue
	// right now.
	args = []string{
		"inspect",
		"--format={{index .RepoDigests 0}}",
		input.Image,
	}
	cmd = exec.CommandContext(ctx, "docker", args...)
	buffer := bytes.Buffer{}
	cmd.Stdout = &buffer
	writer = options.Output.Writer()
	cmd.Stderr = writer

	err = cmd.Run()
	_ = writer.Close()
	if err != nil {
		return Output{}, err
	}

	output := Output{
		Result: map[string]interface{}{
			"image": strings.TrimSpace(buffer.String()),
		},
	}

	return output, nil
}
