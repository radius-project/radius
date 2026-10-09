/*
Copyright 2026 The Radius Authors.

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

package deploy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/version"
)

// CheckCompatibility reports version skew without blocking deployment. Cancellation still stops it.
// pin is the Radius extension pin resolved by bicep.Impl.PrepareTemplate for the root template.
func CheckCompatibility(ctx context.Context, factory connections.Factory, workspace workspaces.Workspace, template map[string]any, pin bicep.RadiusExtensionPin) (string, error) {
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	references := knownReferences(ctx, bicep.RadiusExtensionReferences(template, pin))
	if len(references) == 0 {
		return "", nil
	}
	info := version.VersionInfo{}
	var err error
	if factory == nil {
		err = errors.New("no workspace connection factory is available")
	} else {
		info, err = factory.GetControlPlaneVersion(ctx, workspace)
	}
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	return formatCompatibilityWarning(version.Release(), info.Release, err, references), nil
}

// knownReferences drops references whose pin is simply unknown -- a JSON template or a resource
// declared inside a nested Bicep module, neither of which can carry pin metadata -- logging them at
// debug level instead. Following the resulting warning's own advice (adding or fixing a pin) could
// never clear a warning caused by an unknown pin, so it is not surfaced as one.
func knownReferences(ctx context.Context, references []bicep.RadiusExtensionReference) []bicep.RadiusExtensionReference {
	logger := ucplog.FromContextOrDiscard(ctx)
	known := make([]bicep.RadiusExtensionReference, 0, len(references))
	for _, reference := range references {
		if reference.Reference == "" && reference.Reason == bicep.UnknownPinReason {
			logger.V(ucplog.LevelDebug).Info("Skipping Radius type compatibility check for a resource with no determinable extension pin")
			continue
		}
		known = append(known, reference)
	}
	return known
}

func releaseVersion(value string) (*semver.Version, error) {
	return semver.StrictNewVersion(strings.TrimPrefix(value, "v"))
}

func formatCompatibilityWarning(cliRelease, controlPlaneRelease string, versionErr error, references []bicep.RadiusExtensionReference) string {
	cliVersion, cliErr := releaseVersion(cliRelease)
	controlPlaneVersion, controlPlaneErr := releaseVersion(controlPlaneRelease)

	tags := make([]string, len(references))
	for i, reference := range references {
		tags[i] = extensionTag(reference.Reference)
	}

	// A channel pin (e.g. "0.60", as written by `rad init`) never identifies a full release, but
	// when the CLI and control plane already run that very channel there is nothing actionable to
	// flag: an exact-version tag for the channel may not even be published yet, so the warning's own
	// advice could never clear it.
	channelMatch := cliErr == nil && controlPlaneErr == nil && matchesChannel(tags, cliVersion, controlPlaneVersion)

	var reasons []string
	if cliErr != nil {
		reasons = append(reasons, "The CLI does not report a full release version.")
	}
	if versionErr != nil {
		reasons = append(reasons, fmt.Sprintf("Could not read the target control-plane release: %v.", versionErr))
	} else if controlPlaneErr != nil {
		reasons = append(reasons, "The target control plane does not report a full release version.")
	} else if cliErr == nil && !channelMatch && version.Compare(cliVersion, controlPlaneVersion) != 0 {
		reasons = append(reasons, "The CLI and target control-plane releases differ.")
	}

	for i, reference := range references {
		tag := tags[i]
		extensionVersion, err := releaseVersion(tag)
		if reference.Reason != "" {
			reasons = append(reasons, "Could not verify the resolved Radius extension release: "+reference.Reason+".")
		}
		if err != nil {
			if channelMatch && channelMatches(tag, cliVersion, controlPlaneVersion) {
				continue
			}
			reason := reference.Reason
			if reason == "" {
				reason = "the configured reference is floating, local, custom, or digest-pinned and does not identify a full release"
			}
			if reference.Reason == "" {
				reasons = append(reasons, "Could not check the Radius extension release: "+reason+".")
			}
			if tag == "" {
				tag = "unknown"
			}
			tags[i] = tag
		} else {
			if cliErr == nil && version.Compare(extensionVersion, cliVersion) != 0 {
				reasons = append(reasons, fmt.Sprintf("Configured Radius extension %s differs from the CLI release.", extensionVersion))
			}
			if controlPlaneErr == nil && versionErr == nil && version.Compare(extensionVersion, controlPlaneVersion) != 0 {
				reasons = append(reasons, fmt.Sprintf("Configured Radius extension %s differs from the target control-plane release.", extensionVersion))
			}
		}
	}
	if len(reasons) == 0 {
		return ""
	}
	if controlPlaneRelease == "" {
		controlPlaneRelease = "unknown"
	}
	return fmt.Sprintf("WARNING: Radius type compatibility is not verified.\n"+
		"  Configured Radius extension tag(s): %s\n  CLI release: %s\n  Target control-plane release: %s\n"+
		"  %s\n"+
		"  Use a published exact-version extension pin for the target release, or upgrade the target and CLI together.\n",
		strings.Join(tags, ", "), cliRelease, controlPlaneRelease, strings.Join(reasons, "\n  "))
}

func extensionTag(reference string) string {
	if !strings.HasPrefix(reference, "br:") || strings.Contains(reference, "@") {
		return ""
	}
	// The tag follows the last colon in the final path segment, so a registry port is not a tag.
	repository := strings.TrimPrefix(reference, "br:")
	slash := strings.LastIndex(repository, "/")
	if slash < 0 {
		return ""
	}
	_, tag, ok := strings.Cut(repository[slash+1:], ":")
	if !ok {
		return ""
	}
	// Do not print arbitrary reference contents, including connection credentials.
	if _, err := releaseVersion(tag); err == nil {
		return tag
	}
	if tag == "latest" {
		return tag
	}
	if _, err := semver.NewVersion(tag); err == nil && !strings.ContainsAny(tag, "\r\n\t ") {
		return tag
	}
	return ""
}

// channelPattern matches a bare "major.minor" channel tag such as "0.60", as `rad init` writes to
// bicepconfig.json, distinguishing it from an exact-version tag (which has a patch component too)
// or a non-numeric tag such as "latest" or "custom".
var channelPattern = regexp.MustCompile(`^(\d+)\.(\d+)$`)

// channelMatches reports whether tag names the channel that the CLI and control plane both
// currently run, in which case the pin cannot be made any more precise and is not worth flagging.
func channelMatches(tag string, cliVersion, controlPlaneVersion *semver.Version) bool {
	parts := channelPattern.FindStringSubmatch(tag)
	if parts == nil {
		return false
	}
	major, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return false
	}
	minor, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return false
	}
	return cliVersion.Major() == major && cliVersion.Minor() == minor &&
		controlPlaneVersion.Major() == major && controlPlaneVersion.Minor() == minor
}

// matchesChannel reports whether any tag names a channel the CLI and control plane both run.
func matchesChannel(tags []string, cliVersion, controlPlaneVersion *semver.Version) bool {
	for _, tag := range tags {
		if channelMatches(tag, cliVersion, controlPlaneVersion) {
			return true
		}
	}
	return false
}
