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

package preflight

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/cli/output"
)

// Registry manages and executes a collection of preflight checks.
type Registry struct {
	checks []PreflightCheck
	output output.Interface
}

// NewRegistry creates a new preflight check registry.
func NewRegistry(output output.Interface) *Registry {
	return &Registry{
		checks: []PreflightCheck{},
		output: output,
	}
}

// AddCheck adds a preflight check to the registry.
func (r *Registry) AddCheck(check PreflightCheck) {
	r.checks = append(r.checks, check)
}

// GetOutput returns the output interface used by this registry.
func (r *Registry) GetOutput() output.Interface {
	return r.output
}

// RunChecks executes all registered preflight checks and returns the results.
// If any Error severity check fails, the function returns an error immediately.
func (r *Registry) RunChecks(ctx context.Context) ([]CheckResult, error) {
	if len(r.checks) == 0 {
		return []CheckResult{}, nil
	}

	r.output.LogInfo("Running pre-flight checks...")

	results := make([]CheckResult, 0, len(r.checks))

	for _, check := range r.checks {
		r.output.LogInfo("  Running %s...", check.Name())

		success, message, err := check.Run(ctx)
		result := CheckResult{
			Check:    check,
			Success:  success,
			Message:  message,
			Error:    err,
			Severity: check.Severity(),
		}

		results = append(results, result)

		r.logCheckResult(result)

		// If this is an error severity check and it failed, stop immediately
		if result.Severity == SeverityError && (!success || err != nil) {
			return results, fmt.Errorf("pre-flight check '%s' failed: %s", check.Name(), r.getFailureReason(result))
		}
	}

	r.output.LogInfo("Pre-flight checks completed successfully")
	return results, nil
}

// logCheckResult logs the result of a preflight check with appropriate formatting.
func (r *Registry) logCheckResult(result CheckResult) {
	status := "✓"
	if !result.Success || result.Error != nil {
		status = "✗"
	}

	message := result.Message
	if result.Error != nil {
		if message == "" {
			message = result.Error.Error()
		} else {
			message = fmt.Sprintf("%s (%s)", message, result.Error.Error())
		}
	}

	// Use LogInfo for all messages with severity prefix
	switch result.Severity {
	case SeverityError:
		if result.Success && result.Error == nil {
			r.output.LogInfo("    %s %s", status, message)
		} else {
			r.output.LogInfo("    %s [ERROR] %s", status, message)
		}
	case SeverityWarning:
		if result.Success && result.Error == nil {
			r.output.LogInfo("    %s %s", status, message)
		} else {
			r.output.LogInfo("    %s [WARNING] %s", status, message)
		}
	case SeverityInfo:
		r.output.LogInfo("    %s %s", status, message)
	}
}

// getFailureReason returns a descriptive reason for why a check failed.
func (r *Registry) getFailureReason(result CheckResult) string {
	if result.Error != nil {
		return result.Error.Error()
	}
	if result.Message != "" {
		return result.Message
	}
	return "check failed"
}
