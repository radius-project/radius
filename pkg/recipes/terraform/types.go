/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package terraform

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

//go:generate mockgen -destination=./mock_executor.go -package=terraform -self_package github.com/project-radius/radius/pkg/recipes/terraform github.com/project-radius/radius/pkg/recipes/terraform TerraformExecutor

type TerraformExecutor interface {
	// Deploy installs terraform and runs terraform init and apply on the terraform module referenced by the recipe using terraform-exec.
	Deploy(ctx context.Context, options Options) (*recipes.RecipeOutput, error)
}

// Options represents the options required to build inputs to interact with Terraform.
type Options struct {
	// RootDir is the root directory of where Terraform is installed and executed for a specific recipe deployment/deletion request.
	RootDir string

	// EnvConfig is the kubernetes runtime and cloud provider configuration for the Radius environment in which the application consuming the terraform recipe will be deployed.
	EnvConfig *recipes.Configuration

	// EnvRecipe is the recipe metadata associated with the Radius environment in which the application consuming the terraform recipe will be deployed.
	EnvRecipe *recipes.EnvironmentDefinition

	// ResourceRecipe is recipe metadata associated with the Radius resource deploying the Terraform recipe.
	ResourceRecipe *recipes.ResourceMetadata
}

// NewTerraform creates a new Terraform executor with Terraform logs enabled.
func NewTerraform(ctx context.Context, workingDir, execPath string) (*tfexec.Terraform, error) {
	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		return nil, err
	}

	configureTerraformLogs(ctx, tf)

	return tf, err
}

// StreamingWriter is a writer that processes data in a streaming manner.
type StreamingWriter struct {
	logger logr.Logger
}

// Write processes the data in a streaming manner.
func (w *StreamingWriter) Write(p []byte) (n int, err error) {
	w.logger.Info(string(p))
	return len(p), nil
}

type StreamingErrorWriter struct {
	logger logr.Logger
}

func (w *StreamingErrorWriter) Write(p []byte) (n int, err error) {
	w.logger.Error(fmt.Errorf(string(p)), string(p))
	return len(p), nil
}

// configureTerraformLogs configures the Terraform logs to be streamed to the Radius logs.
func configureTerraformLogs(ctx context.Context, tf *tfexec.Terraform) {
	logger := ucplog.FromContextOrDiscard(ctx)

	err := tf.SetLog("TRACE")
	if err != nil {
		logger.Error(err, "Failed to set log level for Terraform")
		return
	}

	streamingWriter := &StreamingWriter{
		logger: logger,
	}
	tf.SetStdout(streamingWriter)

	streamingErrorWriter := &StreamingErrorWriter{
		logger: logger,
	}
	tf.SetStderr(streamingErrorWriter)
}
