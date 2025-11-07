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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

// TerraformVariable represents a Terraform input variable
type TerraformVariable struct {
	Name        string
	Type        string
	Description string
	Default     interface{}
	Required    bool
}

// TerraformModule represents a parsed Terraform module
type TerraformModule struct {
	Name      string
	Variables []TerraformVariable
}

// CloneGitRepository clones a git repository and returns the local path
func CloneGitRepository(gitURL, tempDir string) (string, error) {
	repoName := filepath.Base(gitURL)
	repoName = strings.TrimSuffix(repoName, ".git")
	localPath := filepath.Join(tempDir, repoName)

	// Remove existing directory if it exists
	if _, err := os.Stat(localPath); err == nil {
		if err := os.RemoveAll(localPath); err != nil {
			return "", fmt.Errorf("failed to remove existing directory: %w", err)
		}
	}

	cmd := exec.Command("git", "clone", gitURL, localPath)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to clone repository: %w", err)
	}

	return localPath, nil
}

// ParseTerraformModule parses Terraform files in the given directory using terraform-config-inspect
func ParseTerraformModule(modulePath string) (*TerraformModule, error) {
	mod, diags := tfconfig.LoadModule(modulePath)
	if diags.HasErrors() {
		return nil, fmt.Errorf("error loading terraform module: %w", diags.Err())
	}

	module := &TerraformModule{
		Name:      filepath.Base(modulePath),
		Variables: []TerraformVariable{},
	}

	sortedVariables := make([]*tfconfig.Variable, 0, len(mod.Variables))
	for _, variable := range mod.Variables {
		sortedVariables = append(sortedVariables, variable)
	}

	sort.SliceStable(sortedVariables, func(i, j int) bool {
		if sortedVariables[i].Pos.Filename == sortedVariables[j].Pos.Filename {
			if sortedVariables[i].Pos.Line == sortedVariables[j].Pos.Line {
				return sortedVariables[i].Name < sortedVariables[j].Name
			}
			return sortedVariables[i].Pos.Line < sortedVariables[j].Pos.Line
		}
		return sortedVariables[i].Pos.Filename < sortedVariables[j].Pos.Filename
	})

	for _, variable := range sortedVariables {
		tfVar := TerraformVariable{
			Name:        variable.Name,
			Type:        convertTfConfigTypeToString(variable.Type),
			Description: variable.Description,
			Required:    variable.Required,
		}

		if variable.Default != nil {
			tfVar.Default = variable.Default
		}

		module.Variables = append(module.Variables, tfVar)
	}

	return module, nil
}

// convertTfConfigTypeToString converts tfconfig type to string representation
func convertTfConfigTypeToString(tfType string) string {
	return tfType
}

// ConvertTerraformTypeToJSONSchema converts Terraform types to JSON Schema types
func ConvertTerraformTypeToJSONSchema(tfType string) string {
	tfType = strings.TrimSpace(tfType)
	tfType = strings.ToLower(tfType)

	switch {
	case tfType == "string":
		return "string"
	case tfType == "number":
		return "number"
	case tfType == "bool" || tfType == "boolean":
		return "boolean"
	case strings.HasPrefix(tfType, "list("):
		return "array"
	case strings.HasPrefix(tfType, "set("):
		return "array"
	case strings.HasPrefix(tfType, "map("):
		return "object"
	case strings.HasPrefix(tfType, "object("):
		return "object"
	default:
		return "string"
	}
}
