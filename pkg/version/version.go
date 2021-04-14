// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package version

// Values for these are injected by the build.
var (
	release = "edge"
	version = "edge"
	commit  = "unknown"
)

// Commit returns the full git SHA of the build.
func Commit() string {
	return commit
}

// Release returns the semver release version of the build.
func Release() string {
	return release
}

// Version returns the 'git describe' output of the build.
func Version() string {
	return version
}
