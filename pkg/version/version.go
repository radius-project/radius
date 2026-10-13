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

package version

import (
	"regexp"

	"github.com/Masterminds/semver/v3"
)

var legacyRCPrereleasePattern = regexp.MustCompile(`^rc([1-9][0-9]*)$`)

// Values for these are injected by the build.
var (
	channel      = "edge"
	release      = "edge"
	version      = "edge"
	commit       = "unknown"
	chartVersion = "0.42.42-dev"
)

// VersionInfo is used for a serializable representation of our versioning info.
type VersionInfo struct {
	Channel      string `json:"channel"`
	Commit       string `json:"commit"`
	Release      string `json:"release"`
	Version      string `json:"version"`
	Bicep        string `json:"bicep"`
	ChartVersion string `json:"chartVersion"`
}

// NewVersionInfo creates a new VersionInfo object with the current version information.
func NewVersionInfo() VersionInfo {
	return VersionInfo{
		Channel:      Channel(),
		Commit:       Commit(),
		Release:      Release(),
		Version:      Version(),
		ChartVersion: ChartVersion(),
	}
}

// IsEdgeChannel returns true if the channel is equal to "edge" and false otherwise.
func IsEdgeChannel() bool {
	return channel == "edge"
}

// Channel returns the designated channel for downloads of assets.
//
// For a real release this will be the major.minor - for any other build it's the same
// as Release().
func Channel() string {
	return channel
}

// Commit returns the full git SHA of the build.
//
// This should only be used for informational purposes.
func Commit() string {
	return commit
}

// Release returns the semver release version of the build.
//
// This should only be used for informational purposes.
func Release() string {
	return release
}

// Version returns the 'git describe' output of the build.
//
// This should only be used for informational purposes.
func Version() string {
	return version
}

// ChartVersion returns the version of the Helm Chart
func ChartVersion() string {
	return chartVersion
}

// Compare returns -1, 0, or 1 for release precedence without modifying either version.
// Historical rcN suffixes are compared numerically only when both versions use that form.
func Compare(left, right *semver.Version) int {
	leftLegacyRC := legacyRCPrereleasePattern.FindStringSubmatch(left.Prerelease())
	rightLegacyRC := legacyRCPrereleasePattern.FindStringSubmatch(right.Prerelease())
	if leftLegacyRC != nil && rightLegacyRC != nil {
		left = semver.New(left.Major(), left.Minor(), left.Patch(), "rc."+leftLegacyRC[1], left.Metadata())
		right = semver.New(right.Major(), right.Minor(), right.Patch(), "rc."+rightLegacyRC[1], right.Metadata())
	}
	return left.Compare(right)
}
