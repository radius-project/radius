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

	dockerparser "github.com/novln/docker-parser"
	"github.com/project-radius/radius/pkg/cli/environments"
)

type ImageOperation int

const (
	ImagePull ImageOperation = 0
	ImagePush ImageOperation = 1
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

	// NOTE: in addition to normalizing the user-provided string into a fully-formed reference
	// we also apply overrides here. The override works in both directions.
	//
	// Ex: we might push 'localhost:60063/todo:latest' but output 'localhost:5000/todo@sha256....'
	//
	// Registries running on the user's computer have inherent asymmetry because the networking
	// environment is asymmetric when comparing the host to the runtime.
	pushReference, err := NormalizeImage(options.Registry, input.Image, ImagePush)
	if err != nil {
		return Output{}, err
	}

	input.Context = NormalizePath(options.BaseDirectory, input.Context)
	input.DockerFile = NormalizePath(input.Context, input.DockerFile)

	args := []string{
		"build",
		input.Context,
		"-f", input.DockerFile,
		"-t", pushReference,
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
		pushReference,
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
		pushReference,
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

	pullReference, err := NormalizeImage(options.Registry, strings.TrimSpace(buffer.String()), ImagePull)
	if err != nil {
		return Output{}, err
	}

	output := Output{
		Result: map[string]interface{}{
			"image": pullReference,
		},
	}

	return output, nil
}

func NormalizeImage(registry *environments.Registry, image string, operation ImageOperation) (string, error) {
	reference, err := dockerparser.Parse(image)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	if registry == nil {
		return reference.Remote(), nil
	}

	if operation == ImagePush {
		return fmt.Sprintf("%s/%s", registry.PushEndpoint, reference.Name()), nil
	}

	return fmt.Sprintf("%s/%s", registry.PullEndpoint, reference.Name()), nil
}
