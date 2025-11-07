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

package learn

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Options configures the learn operation.
type Options struct {
	GitURL    string
	Namespace string
	TypeName  string
}

// Result captures the outcome of a learn operation.
type Result struct {
	Namespace         string
	TypeName          string
	YAML              []byte
	VariableCount     int
	GeneratedTypeName bool
	InferredNamespace bool
}

// Run executes the learn flow and returns the generated resource type definition in YAML form.
func Run(_ context.Context, opts Options) (Result, error) {
	if opts.GitURL == "" {
		return Result{}, fmt.Errorf("git URL is required")
	}

	tempDir, err := os.MkdirTemp("", "radius-terraform-learn-")
	if err != nil {
		return Result{}, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	modulePath, err := CloneGitRepository(opts.GitURL, tempDir)
	if err != nil {
		return Result{}, fmt.Errorf("failed to clone repository: %w", err)
	}

	module, err := ParseTerraformModule(modulePath)
	if err != nil {
		return Result{}, fmt.Errorf("failed to parse Terraform module: %w", err)
	}

	typeName := opts.TypeName
	generatedTypeName := false
	if typeName == "" {
		typeName = GenerateModuleName(opts.GitURL)
		typeName = generateResourceTypeName(typeName)
		generatedTypeName = true
	}

	namespace := opts.Namespace
	inferredNamespace := false
	if namespace == "" {
		namespace = InferNamespaceFromModule(module, opts.GitURL)
		inferredNamespace = true
	}

	schema, err := GenerateResourceTypeSchema(module, namespace, typeName)
	if err != nil {
		return Result{}, fmt.Errorf("failed to generate resource type schema: %w", err)
	}

	yamlData, err := yaml.Marshal(schema)
	if err != nil {
		return Result{}, fmt.Errorf("failed to marshal YAML: %w", err)
	}

	return Result{
		Namespace:         namespace,
		TypeName:          typeName,
		YAML:              yamlData,
		VariableCount:     len(module.Variables),
		GeneratedTypeName: generatedTypeName,
		InferredNamespace: inferredNamespace,
	}, nil
}
