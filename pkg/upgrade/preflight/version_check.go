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

	"github.com/Masterminds/semver/v3"
)

// Ensure VersionCompatibilityCheck implements PreflightCheck interface
var _ PreflightCheck = (*VersionCompatibilityCheck)(nil)

// VersionCompatibilityCheck validates that the target version is a valid upgrade
// from the current version. It prevents downgrades and enforces incremental upgrade policies.
type VersionCompatibilityCheck struct {
	currentVersion string
	targetVersion  string
}

// NewVersionCompatibilityCheck creates a new version compatibility check.
// Both currentVersion and targetVersion must be specific semantic versions (e.g., "v0.43.0").
// If targetVersion is "latest", it must be resolved to a specific version before creating this check.
func NewVersionCompatibilityCheck(currentVersion, targetVersion string) *VersionCompatibilityCheck {
	return &VersionCompatibilityCheck{
		currentVersion: currentVersion,
		targetVersion:  targetVersion,
	}
}

// Name returns the name of this check.
func (v *VersionCompatibilityCheck) Name() string {
	return "Version Compatibility"
}

// Severity returns the severity level of this check.
func (v *VersionCompatibilityCheck) Severity() CheckSeverity {
	return SeverityError
}

// Run executes the version compatibility check.
func (v *VersionCompatibilityCheck) Run(ctx context.Context) (bool, string, error) {
	valid, message, err := v.isValidUpgradeVersion(v.currentVersion, v.targetVersion)
	if err != nil {
		return false, "", fmt.Errorf("failed to validate version compatibility: %w", err)
	}

	if !valid {
		return false, message, nil
	}

	if v.targetVersion == "latest" {
		return false, "Target version 'latest' must be resolved to a specific version before validation", nil
	}

	return true, fmt.Sprintf("Upgrade from %s to %s is valid", v.currentVersion, v.targetVersion), nil
}

// isValidUpgradeVersion checks if the target version is a valid upgrade from the current version.
// This logic is extracted from the original kubernetes upgrade command.
func (v *VersionCompatibilityCheck) isValidUpgradeVersion(currentVersion, targetVersion string) (bool, string, error) {
	// "latest" should be resolved to an actual version before calling this check
	if targetVersion == "latest" {
		return false, "Target version 'latest' must be resolved to a specific version before validation", nil
	}

	// Ensure both versions have 'v' prefix for semver parsing
	if len(currentVersion) > 0 && currentVersion[0] != 'v' {
		currentVersion = "v" + currentVersion
	}
	if len(targetVersion) > 0 && targetVersion[0] != 'v' {
		targetVersion = "v" + targetVersion
	}

	// Parse versions using semver library
	current, err := semver.NewVersion(currentVersion)
	if err != nil {
		return false, "", fmt.Errorf("invalid current version format: %w", err)
	}

	target, err := semver.NewVersion(targetVersion)
	if err != nil {
		return false, "", fmt.Errorf("invalid target version format: %w", err)
	}

	// Check if versions are the same
	if current.Equal(target) {
		return false, "Target version is the same as current version", nil
	}

	// Check if downgrade attempt
	if target.LessThan(current) {
		return false, "Downgrading is not supported", nil
	}

	// Get the next expected version (increment minor version)
	expectedNextMinor := semver.New(current.Major(), current.Minor()+1, 0, "", "")

	// Special case: major version increment (e.g., 0.x -> 1.0)
	if target.Major() > current.Major() {
		if target.Major() == current.Major()+1 && target.Minor() == 0 && target.Patch() == 0 {
			return true, "", nil
		}
		return false, fmt.Sprintf("Skipping multiple major versions not supported. Expected next major version: %d.0.0", current.Major()+1), nil
	}

	// Allow increment of minor version by exactly 1
	if target.Major() == current.Major() && target.Minor() == current.Minor()+1 {
		return true, "", nil
	}

	return false, fmt.Sprintf("Only incremental version upgrades are supported. Expected next version: %s", expectedNextMinor), nil
}

// ValidateVersionJump checks if a version jump is safe for upgrade.
// This is a utility function that can be used when resolving "latest" versions.
// It returns true if the jump is safe (incremental), false if it requires special handling.
func ValidateVersionJump(currentVersion, targetVersion string) (bool, string, error) {
	check := NewVersionCompatibilityCheck(currentVersion, targetVersion)
	return check.isValidUpgradeVersion(currentVersion, targetVersion)
}
