// Package skills provides composable discovery tasks.
package skills

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/radius-project/radius/pkg/discovery"
)

// ValidateAppDefinitionSkill validates generated application definitions.
type ValidateAppDefinitionSkill struct{}

// NewValidateAppDefinitionSkill creates a new validate_app_definition skill.
func NewValidateAppDefinitionSkill() *ValidateAppDefinitionSkill {
	return &ValidateAppDefinitionSkill{}
}

// Name returns the skill name.
func (s *ValidateAppDefinitionSkill) Name() string {
	return "validate_app_definition"
}

// Description returns a description of the skill.
func (s *ValidateAppDefinitionSkill) Description() string {
	return "Validate generated Radius application definition (app.bicep) for correctness"
}

// ValidateAppDefinitionInput contains input for validation.
type ValidateAppDefinitionInput struct {
	// BicepContent is the Bicep content to validate (optional if FilePath provided)
	BicepContent string

	// FilePath is the path to app.bicep to validate (optional if BicepContent provided)
	FilePath string

	// DiscoveryResult is the discovery result for cross-reference validation
	DiscoveryResult *discovery.DiscoveryResult

	// StrictMode enables additional validation checks
	StrictMode bool
}

// ValidationIssue represents a validation problem.
type ValidationIssue struct {
	// Severity is the issue severity (error, warning, info)
	Severity string

	// Message describes the issue
	Message string

	// Line is the line number in the Bicep file (if applicable)
	Line int

	// Resource is the resource name with the issue (if applicable)
	Resource string
}

// ValidateAppDefinitionOutput contains validation results.
type ValidateAppDefinitionOutput struct {
	// Valid indicates whether the Bicep is valid
	Valid bool

	// Issues contains any validation issues found
	Issues []ValidationIssue

	// BicepCompileSuccess indicates if Bicep compilation succeeded
	BicepCompileSuccess bool

	// BicepCompileOutput contains Bicep compiler output (if available)
	BicepCompileOutput string
}

// Execute validates an application definition.
func (s *ValidateAppDefinitionSkill) Execute(input *ValidateAppDefinitionInput) (*ValidateAppDefinitionOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.BicepContent == "" && input.FilePath == "" {
		return nil, fmt.Errorf("either BicepContent or FilePath is required")
	}

	out := &ValidateAppDefinitionOutput{
		Valid:  true,
		Issues: make([]ValidationIssue, 0),
	}

	content := input.BicepContent
	if content == "" && input.FilePath != "" {
		data, err := os.ReadFile(input.FilePath)
		if err != nil {
			return nil, fmt.Errorf("reading file: %w", err)
		}
		content = string(data)
	}

	// Perform structural validation
	s.validateStructure(content, out)

	// Perform cross-reference validation with discovery results
	if input.DiscoveryResult != nil {
		s.validateCrossReferences(content, input.DiscoveryResult, out)
	}

	// Additional strict mode checks
	if input.StrictMode {
		s.validateStrict(content, out)
	}

	// Try Bicep compilation if file path provided
	if input.FilePath != "" {
		s.tryBicepCompile(input.FilePath, out)
	}

	// Determine overall validity
	for _, issue := range out.Issues {
		if issue.Severity == "error" {
			out.Valid = false
			break
		}
	}

	return out, nil
}

func (s *ValidateAppDefinitionSkill) validateStructure(content string, out *ValidateAppDefinitionOutput) {
	// Check for required elements
	if !strings.Contains(content, "extension radius") {
		out.Issues = append(out.Issues, ValidationIssue{
			Severity: "error",
			Message:  "missing 'extension radius' declaration",
		})
	}

	if !strings.Contains(content, "Applications.Core/applications") {
		out.Issues = append(out.Issues, ValidationIssue{
			Severity: "error",
			Message:  "missing application resource definition",
		})
	}

	// Check for environment parameter
	if !strings.Contains(content, "param environment") {
		out.Issues = append(out.Issues, ValidationIssue{
			Severity: "error",
			Message:  "missing 'environment' parameter",
		})
	}

	// Check for TODO comments
	todoCount := strings.Count(content, "// TODO:")
	if todoCount > 0 {
		out.Issues = append(out.Issues, ValidationIssue{
			Severity: "warning",
			Message:  fmt.Sprintf("found %d TODO comments that need attention", todoCount),
		})
	}
}

func (s *ValidateAppDefinitionSkill) validateCrossReferences(content string, result *discovery.DiscoveryResult, out *ValidateAppDefinitionOutput) {
	// Check that all services have container definitions
	for _, svc := range result.Services {
		if !strings.Contains(content, fmt.Sprintf("name: '%s'", svc.Name)) {
			out.Issues = append(out.Issues, ValidationIssue{
				Severity: "warning",
				Message:  fmt.Sprintf("service '%s' not found in generated Bicep", svc.Name),
				Resource: svc.Name,
			})
		}
	}

	// Check that all mapped resource types have resource definitions
	for _, rt := range result.ResourceTypes {
		safeName := strings.ReplaceAll(rt.DependencyID, "-", "_")
		if !strings.Contains(content, fmt.Sprintf("resource %s", safeName)) {
			out.Issues = append(out.Issues, ValidationIssue{
				Severity: "warning",
				Message:  fmt.Sprintf("resource type '%s' not found in generated Bicep", rt.DependencyID),
				Resource: rt.DependencyID,
			})
		}
	}
}

func (s *ValidateAppDefinitionSkill) validateStrict(content string, out *ValidateAppDefinitionOutput) {
	// Check for placeholder images
	if strings.Contains(content, ":latest") {
		out.Issues = append(out.Issues, ValidationIssue{
			Severity: "warning",
			Message:  "using ':latest' tag for container images; consider using specific version tags",
		})
	}

	// Check for empty connections
	if strings.Contains(content, "connections: {\n    }") || strings.Contains(content, "connections: {}") {
		out.Issues = append(out.Issues, ValidationIssue{
			Severity: "info",
			Message:  "some containers have no connections defined",
		})
	}
}

func (s *ValidateAppDefinitionSkill) tryBicepCompile(filePath string, out *ValidateAppDefinitionOutput) {
	// Check if Bicep CLI is available
	bicepPath, err := exec.LookPath("bicep")
	if err != nil {
		// Try rad bicep
		radPath, err := exec.LookPath("rad")
		if err != nil {
			out.BicepCompileOutput = "Bicep CLI not available for compilation check"
			return
		}
		bicepPath = radPath
	}

	// Run Bicep build to validate
	var cmd *exec.Cmd
	if strings.HasSuffix(bicepPath, "rad") {
		cmd = exec.Command(bicepPath, "bicep", "build", filePath)
	} else {
		cmd = exec.Command(bicepPath, "build", filePath)
	}

	output, err := cmd.CombinedOutput()
	out.BicepCompileOutput = string(output)
	out.BicepCompileSuccess = err == nil

	if err != nil {
		out.Issues = append(out.Issues, ValidationIssue{
			Severity: "error",
			Message:  fmt.Sprintf("Bicep compilation failed: %s", strings.TrimSpace(string(output))),
		})
	}
}
