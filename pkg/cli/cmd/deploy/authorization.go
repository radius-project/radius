// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/ucp/authorization"
)

func (r *Runner) confirmAuthorizationChanges() error {
	changes := bicep.InspectTemplateResources(r.Template).AuthorizationResources
	if len(changes) == 0 {
		return nil
	}
	parameters := make(map[string]any, len(r.Parameters))
	for key, value := range r.Parameters {
		parameters[key] = value
	}
	resolved, err := authorization.ResolveDeploymentTemplate(r.Template, parameters, r.Workspace.Scope)
	if err != nil {
		return fmt.Errorf("resolve authorization changes before confirmation: %w", err)
	}
	changes = bicep.InspectTemplateResources(resolved).AuthorizationResources
	summary := make([]string, 0, len(changes))
	for _, change := range changes {
		id, err := authorization.DeploymentResourceID(r.Workspace.Scope, change)
		if err != nil {
			return err
		}
		scope := id[:strings.LastIndex(strings.ToLower(id), "/providers/radius.core/")]
		raw, err := json.Marshal(map[string]any{
			"id": id, "scope": scope, "properties": change["properties"],
		})
		if err != nil {
			return fmt.Errorf("summarize authorization changes: %w", err)
		}
		summary = append(summary, string(raw))
	}
	slices.Sort(summary)
	r.Output.LogInfo("Authorization changes (deployment scope %s):", r.Workspace.Scope)
	for _, line := range summary {
		r.Output.LogInfo("%s", line)
	}
	r.Output.LogInfo("These changes can grant access or lock out administrators. Assignments inherit to descendants. Removing a declaration does not revoke access.")
	if r.Yes {
		return nil
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("deployment contains authorization changes; rerun with --yes to confirm non-interactively")
	}
	confirmed, err := prompt.YesOrNoPrompt("Apply this deployment, including its authorization changes?", prompt.ConfirmNo, r.Prompter)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("deployment cancelled before applying resources")
	}
	return nil
}
