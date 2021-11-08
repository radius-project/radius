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
)

var _ Builder = (*dockerBuilder)(nil)

type dockerBuilder struct {
}

type dockerInput struct {
	Context    string `json:"context"`
	DockerFile string `json:"dockerFile"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

func (builder *dockerBuilder) Build(ctx context.Context, values interface{}, options BuilderOptions) (map[string]interface{}, error) {
	b, err := json.Marshal(&values)
	if err != nil {
		return nil, err
	}

	input := dockerInput{}
	err = json.Unmarshal(b, &input)
	if err != nil {
		return nil, err
	}

	if input.Context == "" {
		return nil, fmt.Errorf("%s is required", "context")
	}
	if input.Repository == "" {
		return nil, fmt.Errorf("%s is required", "repository")
	}
	if input.Tag == "" {
		input.Tag = "latest"
	}
	if input.DockerFile == "" {
		input.DockerFile = "Dockerfile"
	}

	input.Context = normalize(options.BaseDirectory, input.Context)
	input.DockerFile = normalize(input.Context, input.DockerFile)

	args := []string{
		"build",
		input.Context,
		"-f", input.DockerFile,
		"-t", fmt.Sprintf("%s:%s", input.Repository, input.Tag),
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	fmt.Printf("running: %s\n", cmd.String())
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	args = []string{
		"push",
		fmt.Sprintf("%s:%s", input.Repository, input.Tag),
	}
	cmd = exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	output := map[string]interface{}{
		"kind":       "container",
		"repository": input.Repository,
		"image":      fmt.Sprintf("%s:%s", input.Repository, input.Tag),
		"tag":        input.Tag,

		// TODO digest
	}

	return output, nil
}
