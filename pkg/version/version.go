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
	"encoding/json"
	"fmt"

	"github.com/project-radius/radius/pkg/cli/output"
)

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

// # Function Explanation
//
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

// # Function Explanation
//
// IsEdgeChannel returns true if the channel is equal to "edge" and false otherwise.
func IsEdgeChannel() bool {
	return channel == "edge"
}

// # Function Explanation
//
// Channel returns the designated channel for downloads of assets.
//
// For a real release this will be the major.minor - for any other build it's the same
// as Release().
func Channel() string {
	return channel
}

// # Function Explanation
//
// Commit returns the full git SHA of the build.
//
// This should only be used for informational purposes.
func Commit() string {
	return commit
}

// # Function Explanation
//
// Release returns the semver release version of the build.
//
// This should only be used for informational purposes.
func Release() string {
	return release
}

// # Function Explanation
//
// Version returns the 'git describe' output of the build.
//
// This should only be used for informational purposes.
func Version() string {
	return version
}

// # Function Explanation
//
// ChartVersion returns the version of the Helm Chart to be used by
// Kubernetes, Dev, and potentially Azure environments
func ChartVersion() string {
	return chartVersion
}

// # Function Explanation
//
// VersionString formats and returns a string containing version information in either JSON or plain text format.
func VersionString(format string, v VersionInfo, bicepVersion string) string {
	var displayVersion = struct {
		Release string `json:"release"`
		Version string `json:"version"`
		Bicep   string `json:"bicep"`
		Commit  string `json:"commit"`
	}{
		v.Release,
		v.Version,
		bicepVersion,
		v.Commit,
	}

	switch format {
	case output.FormatJson:
		jsonStr, _ := json.MarshalIndent(displayVersion, "", "  ")
		return fmt.Sprintln(string(jsonStr))
	default:
		formatStr := "Release: %s \nVersion: %s\nBicep version: %s\nCommit: %s\n"
		return fmt.Sprintf(formatStr, displayVersion.Release, displayVersion.Version, displayVersion.Bicep, displayVersion.Commit)
	}
}
