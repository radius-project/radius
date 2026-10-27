// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package deploy

import (
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/stretchr/testify/require"
)

func TestAuthorizationChangeConfirmation(t *testing.T) {
	t.Parallel()
	resource := func(typ string) map[string]any {
		return map[string]any{"type": typ, "apiVersion": "2025-08-01-preview", "name": "read-only", "properties": map[string]any{}}
	}
	for _, spec := range []struct {
		name     string
		template map[string]any
		yes      bool
		denied   bool
	}{
		{"ordinary deployment", map[string]any{"resources": map[string]any{"app": map[string]any{"type": "Radius.Core/applications@2025-08-01-preview"}}}, false, false},
		{"role requires confirmation", map[string]any{"resources": []any{resource("Radius.Core/roleDefinitions")}}, false, true},
		{"explicit yes", map[string]any{"resources": []any{resource("Radius.Core/roleAssignments")}}, true, false},
		{"ARM array requires confirmation", map[string]any{"resources": []any{resource("Radius.Core/roleAssignments")}}, false, true},
		{"existing role does not change policy", map[string]any{"resources": map[string]any{"role": map[string]any{"type": "Radius.Core/roleDefinitions@2025-08-01-preview", "existing": true}}}, false, false},
	} {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			writer := &strings.Builder{}
			runner := Runner{Template: spec.template, Yes: spec.yes, Workspace: &workspaces.Workspace{Scope: "/planes/radius/local/resourceGroups/team"}, Output: &output.OutputWriter{Writer: writer}}
			err := runner.confirmAuthorizationChanges()
			if spec.denied {
				require.ErrorContains(t, err, "--yes")
				require.Contains(t, writer.String(), "lock out administrators")
			} else {
				require.NoError(t, err)
			}

		})
	}
}

func TestAuthorizationSummaryResolvesParametersAndScope(t *testing.T) {
	t.Parallel()
	writer := &strings.Builder{}
	runner := Runner{
		Yes: true, Workspace: &workspaces.Workspace{Scope: "/planes/radius/local/resourceGroups/team"},
		Output:     &output.OutputWriter{Writer: writer},
		Parameters: map[string]map[string]any{"subject": {"value": "test-user"}},
		Template: map[string]any{
			"parameters": map[string]any{"subject": map[string]any{"type": "string"}},
			"resources": []any{map[string]any{
				"type": "Radius.Core/roleAssignments", "apiVersion": "2025-08-01-preview", "name": "test-grant",
				"scope": "[format('{0}/providers/Radius.Core/applications/app',resourceGroup().id)]",
				"properties": map[string]any{
					"roleDefinitionId": "/providers/Radius.Core/roleDefinitions/reader",
					"principal":        map[string]any{"type": "user", "issuer": "kubernetes:test", "subject": "[parameters('subject')]"},
				},
			}},
		},
	}
	require.NoError(t, runner.confirmAuthorizationChanges())
	require.Contains(t, writer.String(), "test-user")
	require.Contains(t, writer.String(), "/providers/radius.core/applications/app")
	require.NotContains(t, writer.String(), "[parameters")
	require.NotContains(t, writer.String(), "[format")
}
