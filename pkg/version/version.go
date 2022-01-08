// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package version

import (
	"encoding/json"
	"fmt"

	"github.com/project-radius/radius/pkg/cli/output"
)

// Values for these are injected by the build.
var (
	channel = "edge"
	release = "edge"
	version = "edge"
	commit  = "unknown"
)

// VersionInfo is used for a serializable representation of our versioning info.
type VersionInfo struct {
	Channel string `json:"channel"`
	Commit  string `json:"commit"`
	Release string `json:"release"`
	Version string `json:"version"`
	Bicep   string `json:"bicep"`
}

func NewVersionInfo() VersionInfo {
	return VersionInfo{
		Channel: Channel(),
		Commit:  Commit(),
		Release: Release(),
		Version: Version(),
	}
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

// VersionString returns a formatted string representation of the version from a list of supported
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
