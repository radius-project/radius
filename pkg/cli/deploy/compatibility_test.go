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
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/version"
	"github.com/stretchr/testify/require"
)

// TestMain intercepts the fake "bicep build" invocation used by Test_CheckCompatibility_ExactPinMatch
// so the test can exercise the real PrepareTemplate -> resolveRadiusExtensionPin path without
// depending on a real Bicep compiler being installed.
func TestMain(m *testing.M) {
	if os.Getenv("RADIUS_UNIT_BICEP_COMPILER") == "true" && len(os.Args) > 1 && os.Args[1] == "build" {
		fmt.Print(`{"resources":{"app":{"type":"Radius.Core/applications"}}}`)
		return
	}
	m.Run()
}

func Test_formatCompatibilityWarning(t *testing.T) {
	tests := []struct {
		name       string
		pin        string
		cli        string
		cp         string
		versionErr error
		expected   []string
	}{
		{name: "matching releases", pin: "0.60.2", cli: "v0.60.2", cp: "0.60.2"},
		{name: "newer extension", pin: "0.60.2", cli: "0.60.0", cp: "0.60.0", expected: []string{
			"Configured Radius extension 0.60.2 differs from the CLI",
			"Configured Radius extension 0.60.2 differs from the target",
		}},
		{name: "older extension", pin: "0.60.0", cli: "0.60.2", cp: "0.60.2", expected: []string{"extension 0.60.0 differs"}},
		{name: "CLI and server skew", pin: "0.60.2", cli: "0.60.2", cp: "0.60.0", expected: []string{"CLI and target control-plane releases differ"}},
		// A channel pin matching a channel the CLI and control plane both already run cannot be
		// made any more precise (an exact-version tag for it may not even be published yet), so it
		// is not worth flagging -- this is the common case for a default `rad init` project.
		{name: "floating channel matches CLI and control plane", pin: "0.60", cli: "0.60.1", cp: "0.60.3"},
		{name: "floating channel does not match control plane", pin: "0.60", cli: "0.60.0", cp: "0.61.0", expected: []string{"Could not check the Radius extension release", "tag(s): 0.60"}},
		{name: "latest", pin: "latest", cli: "0.60.0", cp: "0.60.0", expected: []string{"Could not check", "tag(s): latest"}},
		{name: "custom tag", pin: "custom", cli: "0.60.0", cp: "0.60.0", expected: []string{"Could not check", "tag(s): unknown"}},
		{name: "development CLI", pin: "0.60.0", cli: "edge", cp: "0.60.0", expected: []string{"CLI does not report a full release"}},
		{name: "development server", pin: "0.60.0", cli: "0.60.0", cp: "edge", expected: []string{"control plane does not report a full release"}},
		{name: "unreachable server", pin: "0.60.0", cli: "0.60.0", versionErr: errors.New("HTTP 404"), expected: []string{"Could not read", "HTTP 404", "control-plane release: unknown"}},
		{name: "matching prerelease", pin: "0.61.0-rc1", cli: "v0.61.0-rc1", cp: "0.61.0-rc1"},
		{name: "different prerelease", pin: "0.61.0-rc2", cli: "0.61.0-rc10", cp: "0.61.0-rc10", expected: []string{"extension 0.61.0-rc2 differs"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			warning := formatCompatibilityWarning(tt.cli, tt.cp, tt.versionErr, []bicep.RadiusExtensionReference{
				{Reference: "br:example.io/radius:" + tt.pin},
			})
			if len(tt.expected) == 0 {
				require.Empty(t, warning)
				return
			}
			for _, expected := range tt.expected {
				require.Contains(t, warning, expected)
			}
			require.Contains(t, warning, "Use a published exact-version extension pin")
		})
	}
}

// Test_channelMatches covers channelMatches directly, including the tag shapes that
// formatCompatibilityWarning's table-driven cases cannot reach on their own: a non-channel tag, and
// a channel tag whose major or minor component overflows uint64 (channelPattern only guarantees
// digits, not that they fit in a uint64).
func Test_channelMatches(t *testing.T) {
	cliVersion, err := releaseVersion("0.60.1")
	require.NoError(t, err)
	controlPlaneVersion, err := releaseVersion("0.60.3")
	require.NoError(t, err)

	require.False(t, channelMatches("not-a-channel", cliVersion, controlPlaneVersion), "non-channel tag")
	require.False(t, channelMatches("0.61", cliVersion, controlPlaneVersion), "channel not matching either release")
	require.False(t, channelMatches("99999999999999999999.60", cliVersion, controlPlaneVersion), "major overflows uint64")
	require.False(t, channelMatches("0.99999999999999999999", cliVersion, controlPlaneVersion), "minor overflows uint64")
	require.True(t, channelMatches("0.60", cliVersion, controlPlaneVersion), "channel matching both releases")
}

func Test_formatCompatibilityWarning_UnknownProvenance(t *testing.T) {
	warning := formatCompatibilityWarning("0.60.0", "0.60.0", nil, []bicep.RadiusExtensionReference{
		{Reason: bicep.UnknownPinReason},
	})
	require.Contains(t, warning, "no Radius extension pin metadata")
	require.Contains(t, warning, "tag(s): unknown")
}

func Test_extensionTag(t *testing.T) {
	for _, reference := range []string{
		"br:radius",
		"br:radius:0.60.0",
		"br:example.io/radius@sha256:abc",
		"./custom-extension.tgz",
		"br:example.io/radius:secret\ninjected output",
		"br:example.io/radius:credential-secret",
		"br:localhost:5000/radius/types",
		"br:localhost:5000/radius/types@sha256:abc",
	} {
		require.Empty(t, extensionTag(reference))
	}
}

func Test_extensionTag_RegistryPort(t *testing.T) {
	tests := map[string]string{
		"br:localhost:5000/radius/types:0.60.2": "0.60.2",
		"br:localhost:5000/radius/types:0.60":   "0.60",
		"br:localhost:5000/radius/types:latest": "latest",
		"br:example.io/radius:0.60.2":           "0.60.2",
	}
	for reference, expected := range tests {
		require.Equal(t, expected, extensionTag(reference), reference)
	}
}

func Test_CheckCompatibility(t *testing.T) {
	template := map[string]any{"resources": map[string]any{
		"app": map[string]any{"type": "Radius.Core/applications"},
	}}
	pin := bicep.RadiusExtensionPin{Reference: "br:example.io/radius:0.60.2"}
	factory := &connections.MockFactory{ControlPlaneVersion: version.VersionInfo{Release: "0.60.0"}}
	warning, err := CheckCompatibility(t.Context(), factory, workspaces.Workspace{Name: "target"}, template, pin)
	require.NoError(t, err)
	require.Contains(t, warning, "Target control-plane release: 0.60.0")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = CheckCompatibility(ctx, factory, workspaces.Workspace{}, template, pin)
	require.ErrorIs(t, err, context.Canceled)

	warning, err = CheckCompatibility(t.Context(), nil, workspaces.Workspace{}, map[string]any{}, bicep.RadiusExtensionPin{})
	require.NoError(t, err)
	require.Empty(t, warning)

	warning, err = CheckCompatibility(t.Context(), nil, workspaces.Workspace{}, template, pin)
	require.NoError(t, err)
	require.Contains(t, warning, "no workspace connection factory")
}

// Test_CheckCompatibility_UnknownPinLogsNotWarns runs a template with no resolvable pin (a JSON
// template or a nested-module resource) through the real bicep.RadiusExtensionReferences path and
// confirms CheckCompatibility skips the control-plane lookup entirely and produces no warning: an
// unknown pin cannot be cleared by following the warning's own advice, so it must never surface one.
func Test_CheckCompatibility_UnknownPinLogsNotWarns(t *testing.T) {
	template := map[string]any{"resources": map[string]any{
		"app": map[string]any{"type": "Radius.Core/applications"},
	}}
	factory := &connections.MockFactory{ControlPlaneVersionError: errors.New("control plane must not be contacted")}
	warning, err := CheckCompatibility(t.Context(), factory, workspaces.Workspace{}, template, bicep.RadiusExtensionPin{})
	require.NoError(t, err)
	require.Empty(t, warning)
}

// Test_formatCompatibilityWarning_RealExactPinMatch resolves a Radius extension pin through the
// real PrepareTemplate -> resolveRadiusExtensionPin -> RadiusExtensionReferences path, rather than
// hand-constructing a bicep.RadiusExtensionReference with no Reason field, and confirms that an
// exact pin which matches the CLI and control-plane releases produces no warning.
func Test_formatCompatibilityWarning_RealExactPinMatch(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "app.bicep")
	require.NoError(t, os.WriteFile(source, []byte("extension radius\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"),
		[]byte(`{"extensions":{"radius":"br:example.io/radius:0.60.2"}}`), 0600))
	compiler, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("BICEP", compiler)
	t.Setenv("RADIUS_UNIT_BICEP_COMPILER", "true")

	impl := &bicep.Impl{FileSystem: filesystem.NewOSFS(), Output: &output.OutputWriter{Writer: io.Discard}}
	template, pin, err := impl.PrepareTemplate(t.Context(), source)
	require.NoError(t, err)
	require.Equal(t, "br:example.io/radius:0.60.2", pin.Reference)
	require.NoError(t, pin.Err)

	references := bicep.RadiusExtensionReferences(template, pin)
	warning := formatCompatibilityWarning("0.60.2", "0.60.2", nil, references)
	require.Empty(t, warning, "an exact pin matching the CLI and control-plane release must not warn")
}

// Test_DeployWithProgress_ConnectionError confirms DeployWithProgress still propagates a connection
// error. DeployWithProgress no longer runs a compatibility check itself (see
// Test_CheckCompatibility and friends above) -- the sole caller, rad deploy's Runner.Run, always
// performs that check beforehand and never relies on CompatibilityChecked being false.
func Test_DeployWithProgress_ConnectionError(t *testing.T) {
	template := map[string]any{"resources": map[string]any{
		"app": map[string]any{"type": "Radius.Core/applications"},
	}}
	_, err := DeployWithProgress(t.Context(), Options{
		ConnectionFactory: &connections.MockFactory{DeploymentClientError: errors.New("connection error")},
		Template:          template,
	})
	require.ErrorContains(t, err, "connection error")
}
