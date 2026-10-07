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
	"testing"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/version"
	"github.com/stretchr/testify/require"
)

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
		{name: "floating channel", pin: "0.60", cli: "0.60.0", cp: "0.60.0", expected: []string{"Could not check the Radius extension release", "tag(s): 0.60"}},
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
			require.Contains(t, warning, "Deployment will continue.")
			require.Contains(t, warning, "do not identify cached artifact contents")
		})
	}
}

func Test_formatCompatibilityWarning_UnknownProvenance(t *testing.T) {
	warning := formatCompatibilityWarning("0.60.0", "0.60.0", nil, []bicep.RadiusExtensionReference{
		{Reason: "the template has no Radius extension pin metadata"},
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
	factory := &connections.MockFactory{ControlPlaneVersion: version.VersionInfo{Release: "0.60.0"}}
	warning, err := CheckCompatibility(t.Context(), factory, workspaces.Workspace{Name: "target"}, template)
	require.NoError(t, err)
	require.Contains(t, warning, "Target control-plane release: 0.60.0")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = CheckCompatibility(ctx, factory, workspaces.Workspace{}, template)
	require.ErrorIs(t, err, context.Canceled)

	warning, err = CheckCompatibility(t.Context(), nil, workspaces.Workspace{}, map[string]any{})
	require.NoError(t, err)
	require.Empty(t, warning)

	warning, err = CheckCompatibility(t.Context(), nil, workspaces.Workspace{}, template)
	require.NoError(t, err)
	require.Contains(t, warning, "no workspace connection factory")
}

func Test_DeployWithProgress_Compatibility(t *testing.T) {
	template := map[string]any{"resources": map[string]any{
		"app": map[string]any{"type": "Radius.Core/applications"},
	}}
	for _, checked := range []bool{false, true} {
		factory := &connections.MockFactory{
			ControlPlaneVersionError: errors.New("version unavailable"),
			DeploymentClientError:    errors.New("original deployment connection error"),
		}
		_, err := DeployWithProgress(t.Context(), Options{
			ConnectionFactory: factory, Template: template, CompatibilityChecked: checked,
		})
		require.ErrorContains(t, err, "original deployment connection error")
	}
	_, err := DeployWithProgress(t.Context(), Options{
		ConnectionFactory: &connections.MockFactory{DeploymentClientError: errors.New("connection error")},
		Template:          map[string]any{},
	})
	require.ErrorContains(t, err, "connection error")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = DeployWithProgress(ctx, Options{
		ConnectionFactory: &connections.MockFactory{}, Template: template,
	})
	require.ErrorIs(t, err, context.Canceled)
}
