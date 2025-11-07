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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

const (
	learnLongDescription = `Learn and generate resource type definitions from Terraform modules.

This command analyzes a Terraform module from a Git repository and generates a Radius resource type definition YAML file. The generated definition includes:

- Resource type schema based on Terraform input variables
- Property types converted from Terraform types to JSON schema types  
- Required properties based on variables without default values
- Standard Radius application and environment properties
- Auto-inferred namespace based on module name and provider patterns (e.g., AWS.Network, Azure.Storage)

The generated YAML can be used with 'rad resource-type create' to register the new resource type.
`

	learnExample = `
# Generate resource type from a Terraform module (namespace auto-inferred)
rad resource-type learn --git-url https://github.com/example/terraform-aws-vpc

# Override auto-inferred namespace and type name
rad resource-type learn --git-url https://github.com/example/terraform-aws-vpc --namespace "MyCompany.AWS" --type-name "vpc"

# Generate and save to specific file
rad resource-type learn --git-url https://github.com/example/terraform-aws-vpc --output vpc-resource-type.yaml
`
)

// NewCommand creates an instance of the `rad resource-type learn` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "learn",
		Short:   "Generate resource type definitions from Terraform modules",
		Long:    learnLongDescription,
		Example: learnExample,
		Args:    cobra.NoArgs,
		RunE:    framework.RunCommand(runner),
	}

	cmd.Flags().StringVar(&runner.GitURL, "git-url", "", "Git repository URL containing the Terraform module (required)")
	cmd.Flags().StringVar(&runner.Namespace, "namespace", "", "Namespace for the generated resource type (auto-inferred if not provided)")
	cmd.Flags().StringVar(&runner.TypeName, "type-name", "", "Name for the resource type (auto-generated if not provided)")
	cmd.Flags().StringVar(&runner.OutputFile, "output", "", "Output file path (prints to stdout if not provided)")

	_ = cmd.MarkFlagRequired("git-url")

	return cmd, runner
}

const (
	defaultPlaneName            = "local"
	defaultResourceProviderName = "Custom.Resources"
	learnEndpointTemplate       = "/providers/System.Resources/resourceproviders/%s/resourcetypes/learn"
	apiVersionParam             = "api-version=2023-10-01-preview"
)

// Runner is the runner implementation for the `rad resource-type learn` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	GitURL       string
	Namespace    string
	TypeName     string
	OutputFile   string
	Workspace    *workspaces.Workspace
}

// NewRunner creates a new instance of the `rad resource-type learn` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resource-type learn` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	if r.GitURL == "" {
		return fmt.Errorf("git-url is required")
	}

	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	return nil
}

type resourceTypeLearnRequest struct {
	GitURL    string `json:"gitUrl"`
	Namespace string `json:"namespace,omitempty"`
	TypeName  string `json:"typeName,omitempty"`
}

type resourceTypeLearnResponse struct {
	Namespace         string `json:"namespace"`
	TypeName          string `json:"typeName"`
	YAML              string `json:"yaml"`
	VariableCount     int    `json:"variableCount"`
	GeneratedTypeName bool   `json:"generatedTypeName"`
	InferredNamespace bool   `json:"inferredNamespace"`
}

// Run runs the `rad resource-type learn` command.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Learning resource type from Terraform module...")
	r.Output.LogInfo("Repository: %s", r.GitURL)

	connection, err := r.Workspace.Connect(ctx)
	if err != nil {
		return err
	}

	planeName := extractPlaneName(r.Workspace.Scope)

	requestPayload := resourceTypeLearnRequest{
		GitURL:    r.GitURL,
		Namespace: r.Namespace,
		TypeName:  r.TypeName,
	}

	resourceProviderName := r.Namespace
	if resourceProviderName == "" {
		resourceProviderName = defaultResourceProviderName
	}

	body, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("failed to encode request payload: %w", err)
	}

	endpoint := strings.TrimSuffix(connection.Endpoint(), "/")
	escapedProviderName := url.PathEscape(resourceProviderName)
	learnPath := fmt.Sprintf(learnEndpointTemplate, escapedProviderName)
	url := fmt.Sprintf("%s/planes/radius/%s%s?%s", endpoint, planeName, learnPath, apiVersionParam)

	r.Output.LogInfo("Requesting Radius control plane to generate resource type schema...")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := connection.Client().Do(req)
	if err != nil {
		return fmt.Errorf("failed to call resource type learn endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resource type learn request failed: status %d, body: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	var response resourceTypeLearnResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if response.VariableCount == 0 {
		r.Output.LogInfo("Warning: No Terraform variables found in the module")
	} else {
		r.Output.LogInfo("Found %d Terraform variables", response.VariableCount)
	}

	if response.GeneratedTypeName {
		r.Output.LogInfo("Generated resource type name: %s", response.TypeName)
	}

	if response.InferredNamespace {
		r.Output.LogInfo("Inferred namespace: %s", response.Namespace)
	}

	if r.OutputFile != "" {
		outputDir := filepath.Dir(r.OutputFile)
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		if err := os.WriteFile(r.OutputFile, []byte(response.YAML), 0o644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		r.Output.LogInfo("Resource type definition written to: %s", r.OutputFile)
	} else {
		fmt.Print(response.YAML)
	}

	r.Output.LogInfo("Resource type learning completed successfully!")
	r.Output.LogInfo("You can now use 'rad resource-type create --from-file %s' to register this resource type",
		func() string {
			if r.OutputFile != "" {
				return r.OutputFile
			}
			return "<saved-file>"
		}())

	return nil
}

func extractPlaneName(scope string) string {
	if scope == "" {
		return defaultPlaneName
	}

	segments := strings.Split(scope, "/")
	for i := 0; i < len(segments); i++ {
		if strings.EqualFold(segments[i], "planes") && i+2 < len(segments) {
			return segments[i+2]
		}
	}

	return defaultPlaneName
}
