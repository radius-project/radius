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

import "context"

// PreflightCheck defines the interface for all preflight checks
// that must be performed before an upgrade operation.
type PreflightCheck interface {
	// Run executes the preflight check and returns success status,
	// a descriptive message, and any error encountered.
	Run(ctx context.Context) (bool, string, error)

	// Name returns the human-readable name of this check.
	Name() string

	// Severity returns the severity level of this check.
	Severity() CheckSeverity
}

// CheckSeverity represents the severity level of a preflight check.
type CheckSeverity string

const (
	// SeverityError indicates a check failure that must prevent the upgrade.
	SeverityError CheckSeverity = "Error"

	// SeverityWarning indicates a check failure that should be noted but may not prevent the upgrade.
	SeverityWarning CheckSeverity = "Warning"

	// SeverityInfo indicates an informational check that provides useful context.
	SeverityInfo CheckSeverity = "Info"
)

// CheckResult represents the result of running a preflight check.
type CheckResult struct {
	Check    PreflightCheck
	Success  bool
	Message  string
	Error    error
	Severity CheckSeverity
}
