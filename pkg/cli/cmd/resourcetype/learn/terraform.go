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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

// GitRepository clones a git repository and returns the local path
func CloneGitRepository(gitURL, tempDir string) (string, error) {
	repoName := filepath.Base(gitURL)
	if strings.HasSuffix(repoName, ".git") {
		repoName = strings.TrimSuffix(repoName, ".git")
	}

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

// ParseTerraformModule parses Terraform files in the given directory
func ParseTerraformModule(modulePath string) (*TerraformModule, error) {
	module := &TerraformModule{
		Name:      filepath.Base(modulePath),
		Variables: []TerraformVariable{},
	}

	// Find variable files only at the root level (not in subdirectories)
	variableFiles := []string{"variables.tf", "vars.tf"}

	for _, filename := range variableFiles {
		filePath := filepath.Join(modulePath, filename)
		if _, err := os.Stat(filePath); err == nil {
			variables, err := parseVariablesFromFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse file %s: %w", filePath, err)
			}
			module.Variables = append(module.Variables, variables...)
		}
	}

	return module, nil
}

// parseVariablesFromFile parses Terraform variables from a single .tf file
func parseVariablesFromFile(filePath string) ([]TerraformVariable, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return parseVariablesFromContent(string(content))
}

// parseVariablesFromContent parses Terraform variables from file content
func parseVariablesFromContent(content string) ([]TerraformVariable, error) {
	variables := []TerraformVariable{}

	// Regular expression to match variable blocks - handles nested braces
	variableRegex := regexp.MustCompile(`(?s)variable\s+"([^"]+)"\s*\{(.*?)\n\}`)
	matches := variableRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) != 3 {
			continue
		}

		varName := match[1]
		varBlock := match[2]

		variable := TerraformVariable{
			Name:     varName,
			Required: true, // Default to required unless default is provided
		}

		// Parse type - handle multiline object types and other complex types
		variable.Type = parseTypeFromBlock(varBlock)

		// Parse description
		if descMatch := regexp.MustCompile(`description\s*=\s*"([^"]*)"`).FindStringSubmatch(varBlock); len(descMatch) == 2 {
			variable.Description = descMatch[1]
		}

		// Parse default value
		defaultValue := parseDefaultValue(varBlock)
		if defaultValue != "" {
			variable.Default = defaultValue
			variable.Required = false // Has default, so not required
		}

		variables = append(variables, variable)
	}

	return variables, nil
}

// parseTypeFromBlock extracts the type value from a variable block
func parseTypeFromBlock(varBlock string) string {
	// Find the type assignment
	typeRegex := regexp.MustCompile(`(?s)type\s*=\s*(.*)`)
	typeMatch := typeRegex.FindStringSubmatch(varBlock)
	if len(typeMatch) < 2 {
		return "string" // Default type
	}

	typeContent := strings.TrimSpace(typeMatch[1])

	// If it starts with object(, we need to find the matching closing parenthesis
	if strings.HasPrefix(typeContent, "object(") {
		return extractCompleteObjectType(typeContent)
	}

	// For simple types, take everything until newline or next field
	lines := strings.Split(typeContent, "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}

	return "string"
}

// extractCompleteObjectType extracts the complete object type definition
func extractCompleteObjectType(content string) string {
	parenCount := 0
	var result strings.Builder

	for i, char := range content {
		result.WriteRune(char)

		if char == '(' {
			parenCount++
		} else if char == ')' {
			parenCount--
			if parenCount == 0 {
				// We've found the matching closing parenthesis
				return strings.TrimSpace(result.String())
			}
		}

		// Stop if we hit a newline followed by a field that's not part of the object
		if char == '\n' && i+1 < len(content) {
			remaining := content[i+1:]
			if regexp.MustCompile(`^\s*(description|default)\s*=`).MatchString(remaining) && parenCount == 0 {
				break
			}
		}
	}

	return strings.TrimSpace(result.String())
}

// ConvertTerraformTypeToJSONSchema converts Terraform types to JSON Schema types
func ConvertTerraformTypeToJSONSchema(tfType string) string {
	// Clean up the type string
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
		return "string" // Default fallback
	}
}

// parseDefaultValue extracts the complete default value including multi-line objects
func parseDefaultValue(varBlock string) string {
	// Find the start of the default value
	defaultRegex := regexp.MustCompile(`default\s*=\s*`)
	match := defaultRegex.FindStringIndex(varBlock)
	if match == nil {
		return ""
	}

	// Start parsing from after "default = "
	start := match[1]
	content := varBlock[start:]

	// Check if it's an object (starts with {)
	trimmedContent := strings.TrimSpace(content)
	if strings.HasPrefix(trimmedContent, "{") {
		return parseCompleteObject(content)
	}

	// For non-objects, take content until next field or end of block
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])

		// Check if this line contains the complete value
		// Look for patterns like: value followed by a new terraform field
		for i, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			if i > 0 && (strings.HasPrefix(trimmedLine, "description") ||
				strings.HasPrefix(trimmedLine, "validation") ||
				strings.HasPrefix(trimmedLine, "type") ||
				trimmedLine == "}" ||
				strings.HasPrefix(trimmedLine, "variable")) {
				// Combine all lines up to this point
				return strings.TrimSpace(strings.Join(lines[:i], "\n"))
			}
		}

		return firstLine
	}

	return ""
}

// parseCompleteObject extracts a complete object value with balanced braces
func parseCompleteObject(content string) string {
	braceCount := 0
	inString := false
	escaped := false
	var result strings.Builder

	for _, char := range content {
		if escaped {
			escaped = false
			result.WriteRune(char)
			continue
		}

		if char == '\\' && inString {
			escaped = true
			result.WriteRune(char)
			continue
		}

		if char == '"' {
			inString = !inString
		}

		if !inString {
			if char == '{' {
				braceCount++
			} else if char == '}' {
				braceCount--
			}
		}

		result.WriteRune(char)

		// If we've closed all braces, we're done
		if braceCount == 0 && char == '}' {
			break
		}
	}

	return strings.TrimSpace(result.String())
}
