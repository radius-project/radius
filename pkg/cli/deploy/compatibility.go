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
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/version"
)

// CheckCompatibility reports version skew without blocking deployment. Cancellation still stops it.
func CheckCompatibility(ctx context.Context, factory connections.Factory, workspace workspaces.Workspace, template map[string]any) (string, error) {
	references := bicep.RadiusExtensionReferences(template)
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

func releaseVersion(value string) (*semver.Version, error) {
	return semver.StrictNewVersion(strings.TrimPrefix(value, "v"))
}

func formatCompatibilityWarning(cliRelease, controlPlaneRelease string, versionErr error, references []bicep.RadiusExtensionReference) string {
	cliVersion, cliErr := releaseVersion(cliRelease)
	controlPlaneVersion, controlPlaneErr := releaseVersion(controlPlaneRelease)
	var reasons []string
	if cliErr != nil {
		reasons = append(reasons, "The CLI does not report a full release version.")
	}
	if versionErr != nil {
		reasons = append(reasons, fmt.Sprintf("Could not read the target control-plane release: %v.", versionErr))
	} else if controlPlaneErr != nil {
		reasons = append(reasons, "The target control plane does not report a full release version.")
	} else if cliErr == nil && version.Compare(cliVersion, controlPlaneVersion) != 0 {
		reasons = append(reasons, "The CLI and target control-plane releases differ.")
	}

	tags := make([]string, 0, len(references))
	for _, reference := range references {
		tag := extensionTag(reference.Reference)
		extensionVersion, err := releaseVersion(tag)
		if reference.Reason != "" {
			reasons = append(reasons, "Could not verify the resolved Radius extension release: "+reference.Reason+".")
		}
		if err != nil {
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
		} else {
			if cliErr == nil && version.Compare(extensionVersion, cliVersion) != 0 {
				reasons = append(reasons, fmt.Sprintf("Configured Radius extension %s differs from the CLI release.", extensionVersion))
			}
			if controlPlaneErr == nil && versionErr == nil && version.Compare(extensionVersion, controlPlaneVersion) != 0 {
				reasons = append(reasons, fmt.Sprintf("Configured Radius extension %s differs from the target control-plane release.", extensionVersion))
			}
		}
		tags = append(tags, tag)
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
		"  Compilation can succeed with types or properties that the target does not support.\n"+
		"  Use a published exact-version extension pin for the target release, or upgrade the target and CLI together.\n"+
		"  Configured pins do not identify cached artifact contents or prove schema compatibility. Deployment will continue.\n",
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
