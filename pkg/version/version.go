// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package version

// Values for these are injected by the build.
var (
	channel = "edge"
	release = "edge"
	version = "edge"
	commit  = "unknown"
)

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
