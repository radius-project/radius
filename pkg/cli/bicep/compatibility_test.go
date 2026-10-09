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

package bicep

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	if os.Getenv("RADIUS_UNIT_BICEP_COMPILER") == "true" && len(os.Args) > 1 && os.Args[1] == "build" {
		fmt.Print(`{"resources":{"app":{"type":"Radius.Core/applications"}}}`)
		return
	}
	m.Run()
}

// Test_PrepareTemplate_ExactPinMatch exercises the real PrepareTemplate -> resolveRadiusExtensionPin
// path with a bicepconfig.json pinning an exact release, confirming the resolved pin resolves to the
// configured reference and reports no error -- the only input PrepareTemplate ever produces for an
// exact, successfully-read pin.
func Test_PrepareTemplate_ExactPinMatch(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "app.bicep")
	require.NoError(t, os.WriteFile(source, []byte("extension radius\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"),
		[]byte(`{"extensions":{"radius":"br:example.io/radius:0.60.2"}}`), 0600))
	compiler, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("BICEP", compiler)
	t.Setenv("RADIUS_UNIT_BICEP_COMPILER", "true")
	_, pin, err := newTestImpl().PrepareTemplate(t.Context(), source)
	require.NoError(t, err)
	require.Equal(t, "br:example.io/radius:0.60.2", pin.Reference)
	require.NoError(t, pin.Err)
}

func compatibilityTemplate(t *testing.T, text string) map[string]any {
	t.Helper()
	var template map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &template))
	return template
}

func Test_resolveRadiusExtensionPin(t *testing.T) {
	tests := []struct {
		name   string
		config string
		ref    string
		reason string
	}{
		{name: "exact pin", config: `{"extensions":{"radius":"br:example.io/radius:0.60.2"}}`, ref: "br:example.io/radius:0.60.2"},
		{name: "channel pin", config: `{"extensions":{"radius":"br:example.io/radius:0.60"}}`, ref: "br:example.io/radius:0.60"},
		{name: "no configuration", reason: "no bicepconfig.json"},
		{name: "no radius alias", config: `{}`, reason: "no radius extension"},
		{name: "invalid configuration", config: `{"extensions":`, reason: "could not read the Radius pin"},
		{name: "non-string alias", config: `{"extensions":{"radius":true}}`, reason: "not a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.config != "" {
				require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"), []byte(tt.config), 0600))
			}
			source := filepath.Join(root, "nested", "app.bicep")
			require.NoError(t, os.MkdirAll(filepath.Dir(source), 0700))
			template := compatibilityTemplate(t, `{
				"metadata":{"_generator":{"name":"bicep","version":"0.32.4"}},
				"imports":{"Radius":{"provider":"Radius","version":"latest"}},
				"resources":{"app":{"type":"Radius.Core/applications@2025-08-01-preview"}}
			}`)
			pin := resolveRadiusExtensionPin(filesystem.NewOSFS(), source, template)
			require.Equal(t, tt.ref, pin.Reference)
			if tt.reason == "" {
				// An exact, successfully-read pin must never carry an error -- that is the signal
				// CheckCompatibility uses to decide whether a warning is warranted at all.
				require.NoError(t, pin.Err)
			} else {
				require.ErrorContains(t, pin.Err, tt.reason)
			}
			// PrepareTemplate must leave the template itself untouched; the pin is returned
			// out-of-band so generated manifests and published artifacts are not altered.
			require.NotContains(t, template, "_rad")
			metadata := template["metadata"].(map[string]any)
			require.Contains(t, metadata, "_generator")
		})
	}
}

func Test_RadiusExtensionReferences(t *testing.T) {
	template := compatibilityTemplate(t, `{
		"resources":{
			"app":{"type":"Radius.Core/applications@2025-08-01-preview"},
			"module":{
				"type":"Microsoft.Resources/deployments",
				"properties":{"template":{"resources":[
					{"type":"Applications.Core/containers","apiVersion":"2023-10-01-preview"},
					{"type":"Applications.Core/containers","apiVersion":"2023-10-01-preview"}
				]}}
			},
			"data":{
				"type":"Microsoft.Storage/storageAccounts",
				"properties":{"template":{"resources":[{"type":"Radius.Core/environments"}]}}
			}
		}
	}`)
	pin := RadiusExtensionPin{Reference: "br:example.io/radius:0.60.2"}
	references := RadiusExtensionReferences(template, pin)
	require.Equal(t, []RadiusExtensionReference{
		{Reason: UnknownPinReason},
		{Reference: "br:example.io/radius:0.60.2"},
	}, references)
}

// Test_RadiusExtensionReferences_ExactPinMatch asserts that an exact, successfully-resolved pin
// produces a reference with no Reason -- the input that must never produce a warning.
func Test_RadiusExtensionReferences_ExactPinMatch(t *testing.T) {
	template := compatibilityTemplate(t, `{"resources":{"app":{"type":"Radius.Core/applications"}}}`)
	pin := RadiusExtensionPin{Reference: "br:example.io/radius:0.60.2"}
	require.Equal(t, []RadiusExtensionReference{{Reference: "br:example.io/radius:0.60.2"}}, RadiusExtensionReferences(template, pin))
}

func Test_RadiusExtensionReferences_PinReadError(t *testing.T) {
	template := compatibilityTemplate(t, `{"resources":{"app":{"type":"Radius.Core/applications"}}}`)
	pin := RadiusExtensionPin{Err: fmt.Errorf("no bicepconfig.json was found for the source")}
	require.Equal(t, []RadiusExtensionReference{{Reason: "no bicepconfig.json was found for the source"}}, RadiusExtensionReferences(template, pin))
}

func Test_RadiusExtensionReferences_NoRadius(t *testing.T) {
	template := compatibilityTemplate(t, `{"resources":[{"type":"Microsoft.Storage/storageAccounts"}]}`)
	pin := resolveRadiusExtensionPin(filesystem.NewOSFS(), "app.bicep", template)
	require.Empty(t, pin.Reference)
	require.NoError(t, pin.Err)
	require.Empty(t, RadiusExtensionReferences(template, pin))
}

func Test_resolveRadiusExtensionPin_ReadError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"), []byte("{}"), 0600))
	template := compatibilityTemplate(t, `{"resources":{"app":{"type":"Radius.Core/applications"}}}`)
	fs := flakyFS{FileSystem: filesystem.NewOSFS(), failReadSubstr: "bicepconfig.json"}
	pin := resolveRadiusExtensionPin(fs, filepath.Join(root, "app.bicep"), template)
	require.Empty(t, pin.Reference)
	require.ErrorContains(t, pin.Err, "readfile failed")
}

func Test_readRadiusExtensionPin_InvalidPath(t *testing.T) {
	path := "app.bicep"
	if runtime.GOOS == "windows" {
		path = "\x00"
	} else {
		if runtime.GOOS == "darwin" {
			// On macOS, filepath.Abs (via os.Getwd) can still resolve the working directory's path
			// after it has been removed, so this technique does not reliably force the error path
			// readRadiusExtensionPin is meant to exercise here.
			t.Skip("removing the working directory does not reliably fail filepath.Abs on darwin")
		}
		// Resolving a relative path fails when the working directory no longer exists.
		dir := t.TempDir()
		t.Chdir(dir)
		require.NoError(t, os.Remove(dir))
	}
	_, err := readRadiusExtensionPin(filesystem.NewOSFS(), path)
	require.ErrorContains(t, err, "could not locate")
}

func Test_RadiusExtensionReferences_InvalidShapes(t *testing.T) {
	for _, template := range []map[string]any{
		nil,
		{},
		{"resources": true},
		{"resources": []any{"not a resource", map[string]any{}}},
		{"resources": map[string]any{
			"invalid": "not a resource",
			"module":  map[string]any{"type": "Microsoft.Resources/deployments"},
			"invalid-template": map[string]any{
				"type":       "Microsoft.Resources/deployments",
				"properties": map[string]any{"template": true},
			},
		}},
	} {
		require.Empty(t, RadiusExtensionReferences(template, RadiusExtensionPin{}))
	}
	template := compatibilityTemplate(t, `{
		"resources":[
			{"type":"Radius.Core/applications"},
			{"type":"Microsoft.Resources/deployments","properties":{"template":{
				"resources":[{"type":"Radius.Core/applications"}]
			}}}
		]
	}`)
	pin := RadiusExtensionPin{Reference: "br:example.io/radius:0.60.2"}
	require.Equal(t, []RadiusExtensionReference{
		{Reason: UnknownPinReason},
		{Reference: "br:example.io/radius:0.60.2"},
	}, RadiusExtensionReferences(template, pin))
}
