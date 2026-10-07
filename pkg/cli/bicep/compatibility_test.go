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

func Test_PrepareTemplate_RecordsRadiusPin(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "app.bicep")
	require.NoError(t, os.WriteFile(source, []byte("extension radius\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"),
		[]byte(`{"extensions":{"radius":"br:example.io/radius:0.60.2"}}`), 0600))
	compiler, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("BICEP", compiler)
	t.Setenv("RADIUS_UNIT_BICEP_COMPILER", "true")
	template, err := newTestImpl().PrepareTemplate(t.Context(), source)
	require.NoError(t, err)
	require.Equal(t, "br:example.io/radius:0.60.2", RadiusExtensionReferences(template)[0].Reference)
}

func compatibilityTemplate(t *testing.T, text string) map[string]any {
	t.Helper()
	var template map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &template))
	return template
}

func Test_recordRadiusExtensionPin(t *testing.T) {
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
			recordRadiusExtensionPin(filesystem.NewOSFS(), source, template)
			references := RadiusExtensionReferences(template)
			require.Len(t, references, 1)
			require.Equal(t, tt.ref, references[0].Reference)
			require.Contains(t, references[0].Reason, tt.reason)
			if tt.ref != "" {
				require.Contains(t, references[0].Reason, "not compiler provenance")
			}
			metadata := template["metadata"].(map[string]any)
			require.Contains(t, metadata, "_generator")
		})
	}
}

func Test_RadiusExtensionReferences(t *testing.T) {
	template := compatibilityTemplate(t, `{
		"metadata":{"_rad":{"radiusExtension":"br:example.io/radius:0.60.2"}},
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
	references := RadiusExtensionReferences(template)
	require.Equal(t, []RadiusExtensionReference{
		{Reason: "the template has no Radius extension pin metadata"},
		{Reference: "br:example.io/radius:0.60.2"},
	}, references)
}

func Test_RadiusExtensionReferences_NoRadius(t *testing.T) {
	template := compatibilityTemplate(t, `{"resources":[{"type":"Microsoft.Storage/storageAccounts"}]}`)
	recordRadiusExtensionPin(filesystem.NewOSFS(), "app.bicep", template)
	require.NotContains(t, template, "metadata")
	require.Empty(t, RadiusExtensionReferences(template))
}

func Test_recordRadiusExtensionPin_ReadError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"), []byte("{}"), 0600))
	template := compatibilityTemplate(t, `{"resources":{"app":{"type":"Radius.Core/applications"}}}`)
	fs := flakyFS{FileSystem: filesystem.NewOSFS(), failReadSubstr: "bicepconfig.json"}
	recordRadiusExtensionPin(fs, filepath.Join(root, "app.bicep"), template)
	require.Contains(t, RadiusExtensionReferences(template)[0].Reason, "readfile failed")
}

func Test_readRadiusExtensionPin_InvalidPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows rejects a NUL in an absolute path")
	}
	_, err := readRadiusExtensionPin(filesystem.NewOSFS(), "\x00")
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
		require.Empty(t, RadiusExtensionReferences(template))
	}
	template := compatibilityTemplate(t, `{
		"metadata":{"_rad":{"radiusExtensionError":"root error"}},
		"resources":[
			{"type":"Radius.Core/applications"},
			{"type":"Microsoft.Resources/deployments","properties":{"template":{
				"metadata":{"_rad":{"radiusExtensionError":"nested error"}},
				"resources":[{"type":"Radius.Core/applications"}]
			}}}
		]
	}`)
	require.Equal(t, []RadiusExtensionReference{{Reason: "nested error"}, {Reason: "root error"}}, RadiusExtensionReferences(template))
}
