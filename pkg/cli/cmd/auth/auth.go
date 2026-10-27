// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

// Package auth implements user-facing authorization management commands.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/mattn/go-isatty"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/sdk"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
)

type connector func(context.Context, *workspaces.Workspace) (sdk.Connection, error)

// NewCommand creates the authorization command group.
func NewCommand(factory framework.Factory) *cobra.Command {
	return newCommand(factory, func(ctx context.Context, workspace *workspaces.Workspace) (sdk.Connection, error) {
		return workspace.Connect(ctx)
	})
}

func newCommand(factory framework.Factory, connect connector) *cobra.Command {
	root := &cobra.Command{Use: "auth", Short: "Inspect identity, access, roles, assignments, and user authorization enforcement"}
	add := func(parent *cobra.Command, kind, verb, use, short string, mutate bool) *cobra.Command {
		runner := &runner{factory: factory, connect: connect, kind: kind, verb: verb, mutate: mutate}
		cmd := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: framework.RunCommand(runner)}
		runner.cmd = cmd
		commonflags.AddWorkspaceFlag(cmd)
		commonflags.AddOutputFlag(cmd)
		if mutate {
			cmd.Flags().BoolVarP(&runner.yes, "yes", "y", false, "Confirm the policy change without prompting (does not bypass authorization)")
		}
		if kind == "role" || kind == "assignment" || kind == "access" {
			cmd.Flags().StringVar(&runner.scope, "scope", "", "Exact installation (/), plane, resource group, or assignable resource ID")
		}
		if kind == "assignment" && (verb == "create" || verb == "list") || kind == "access" {
			cmd.Flags().StringVar(&runner.principal, "principal", "", "Principal as user:<subject>, group:<subject>, or workload:<subject>")
			cmd.Flags().StringVar(&runner.issuer, "issuer", "", "Trusted issuer of the principal; required with --principal")
		}
		if strings.Contains(use, "<") {
			cmd.Args = cobra.ExactArgs(1)
		}
		if kind == "role" && (verb == "create" || verb == "update") {
			cmd.Flags().StringArrayVar(&runner.permissions, "permission", nil, "Exact catalog permission (repeat for multiple permissions)")
			cmd.Flags().StringArrayVar(&runner.assignableScopes, "assignable-scope", nil, "Scope where the role can be assigned (repeat; defaults to --scope)")
		}
		if (kind == "role" || kind == "assignment") && (verb == "create" || verb == "update") {
			cmd.Flags().StringVar(&runner.description, "description", "", "Description of the policy resource")
			cmd.Flags().StringVar(&runner.displayName, "display-name", "", "Display label (never used to match a principal)")
		}
		if kind == "assignment" && verb == "create" {
			cmd.Args = cobra.MaximumNArgs(1)
			cmd.Flags().StringVar(&runner.role, "role", "", "Unambiguous role name at this scope or its ancestors, or a full role definition ID")
		}
		if kind == "assignment" && verb == "list" {
			cmd.Flags().BoolVar(&runner.inherited, "include-inherited", false, "Include assignments at each ancestor scope (requires read access there)")
		}
		if kind == "access" && verb != "list" {
			cmd.Flags().StringArrayVar(&runner.actions, "action", nil, "Exact permission to evaluate (repeat for multiple checks)")
		}
		if kind == "enforcement" && verb == "set" {
			cmd.Flags().StringVar(&runner.mode, "mode", "", "User authorization mode: Disabled, DryRun, or Enforce")
		}
		parent.AddCommand(cmd)
		return cmd
	}
	add(root, "whoami", "show", "whoami", "Show the authenticated principal, trusted groups, and authorization mode", false)
	add(root, "whoami", "show", "status", "Show user authorization status for the current identity", false)
	for _, group := range []struct{ name, short string }{
		{"permission", "List and explain exact catalog permissions"},
		{"role", "Manage role definitions; built-in roles are immutable"},
		{"assignment", "Manage direct and inherited role assignments"},
		{"access", "Check, explain, and list effective access"},
		{"enforcement", "Inspect or change the user authorization mode"},
		{"history", "Inspect access-change history"},
	} {
		cmd := &cobra.Command{Use: group.name, Short: group.short}
		root.AddCommand(cmd)
		switch group.name {
		case "permission":
			add(cmd, group.name, "list", "list", "List catalog permissions", false)
			show := add(cmd, group.name, "show", "show <permission>", "Explain a catalog permission", false)
			show.Aliases = []string{"explain"}
		case "role":
			add(cmd, group.name, "list", "list", "List roles at --scope (defaults to /)", false)
			for _, verb := range []string{"show", "create", "update", "delete"} {
				add(cmd, group.name, verb, verb+" <name-or-id>", strings.ToUpper(verb[:1])+verb[1:]+" a role definition", verb != "show")
			}
		case "assignment":
			add(cmd, group.name, "list", "list", "List assignments at an explicit scope", false)
			add(cmd, group.name, "create", "create [name]", "Assign a role to a principal at an explicit scope", true)
			for _, verb := range []string{"show", "update", "delete"} {
				add(cmd, group.name, verb, verb+" <name-or-id>", strings.ToUpper(verb[:1])+verb[1:]+" a role assignment", verb != "show")
			}
		case "access":
			for _, verb := range []string{"list", "check", "explain"} {
				add(cmd, group.name, verb, verb, strings.ToUpper(verb[:1])+verb[1:]+" access at --scope", false)
			}
		case "enforcement":
			add(cmd, group.name, "show", "show", "Show user authorization settings", false).Aliases = []string{"inspect"}
			add(cmd, group.name, "set", "set", "Change mode; the server enforces supported migration transitions", true)
			add(cmd, group.name, "preview", "preview", "Use DryRun to preview user authorization enforcement", true).Aliases = []string{"rollback"}
			add(cmd, group.name, "enable", "enable", "Enable user authorization enforcement", true)
		case "history":
			add(cmd, group.name, "list", "list", "List retained access changes (the POC retains only the last 100 generations)", false)
			add(cmd, group.name, "show", "show <name>", "Show a retained access change", false)
		}
	}
	return root
}

type runner struct {
	factory                                                 framework.Factory
	connect                                                 connector
	cmd                                                     *cobra.Command
	workspace                                               *workspaces.Workspace
	kind, verb, name, scope, format                         string
	principal, issuer, role, description, displayName, mode string
	permissions, assignableScopes, actions                  []string
	yes, mutate, inherited                                  bool
	resolveRole                                             bool
	subject                                                 *api.Principal
}

func (r *runner) Validate(cmd *cobra.Command, args []string) error {
	var err error
	r.workspace, err = cli.RequireWorkspace(cmd, r.factory.GetConfigHolder().Config)
	if err != nil {
		return err
	}
	r.format, err = cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		r.name = args[0]
	}
	if r.principal != "" || r.issuer != "" {
		kind, subject, found := strings.Cut(r.principal, ":")
		if !found || subject == "" || strings.TrimSpace(r.issuer) == "" ||
			(kind != "user" && kind != "group" && kind != "workload") {
			return fmt.Errorf("--principal must be user:<subject>, group:<subject>, or workload:<subject>, with a nonempty --issuer")
		}
		r.subject = &api.Principal{Type: to.Ptr(api.PrincipalType(kind)), Issuer: &r.issuer, Subject: &subject}
	}
	if r.kind == "role" || r.kind == "assignment" {
		kind := "roleDefinitions"
		if r.kind == "assignment" {
			kind = "roleAssignments"
		}
		if r.name != "" {
			scope, name, err := resourceName(r.name, kind)
			if err != nil {
				return err
			}
			if scope != "" {
				if r.scope != "" && !strings.EqualFold(r.scope, scope) {
					return fmt.Errorf("--scope conflicts with the resource ID scope %q", scope)
				}
				r.scope = scope
			}
			r.name = name
		}
		if r.kind == "role" && !r.mutate && r.scope == "" {
			r.scope = "/"
		}
	}
	if r.kind == "role" || r.kind == "assignment" || r.kind == "access" {
		if err := validateScope(r.scope); err != nil {
			return fmt.Errorf("--scope: %w", err)
		}
	}
	for _, scope := range r.assignableScopes {
		if err := validateScope(scope); err != nil {
			return fmt.Errorf("--assignable-scope: %w", err)
		}
	}
	if r.kind == "role" && r.verb == "create" && len(r.permissions) == 0 {
		return fmt.Errorf("at least one --permission is required")
	}
	if r.kind == "assignment" && r.verb == "create" {
		if r.subject == nil || r.role == "" {
			return fmt.Errorf("--principal, --issuer, --role and --scope are required")
		}
		roleScope, roleName, err := resourceName(r.role, "roleDefinitions")
		if err != nil {
			return err
		}
		if roleScope != "" {
			r.role = resourceID(roleScope, "roleDefinitions", roleName)
		} else {
			r.role = roleName
			r.resolveRole = true
		}
	}
	if r.kind == "access" && r.verb != "list" && len(r.actions) == 0 {
		return fmt.Errorf("at least one --action is required")
	}
	if r.kind == "enforcement" && r.mutate {
		if r.verb == "preview" {
			r.mode = "DryRun"
		}
		if r.verb == "enable" {
			r.mode = "Enforce"
		}
		if r.mode != "Disabled" && r.mode != "DryRun" && r.mode != "Enforce" {
			return fmt.Errorf("--mode must be Disabled, DryRun or Enforce")
		}
	}
	if r.kind == "history" && r.verb == "show" {
		return validateName(r.name)
	}
	if r.verb == "update" && !cmd.Flags().Changed("description") && !cmd.Flags().Changed("display-name") &&
		!cmd.Flags().Changed("permission") && !cmd.Flags().Changed("assignable-scope") {
		return fmt.Errorf("specify at least one field to update")
	}
	return nil
}

func validateScope(scope string) error {
	if scope == "/" {
		return nil
	}
	if !strings.HasPrefix(scope, "/") || strings.HasSuffix(scope, "/") ||
		strings.ContainsAny(scope, "%\\?#*{}") || strings.ContainsFunc(scope, unicode.IsSpace) {
		return fmt.Errorf("an explicit, concrete Radius scope ID is required")
	}
	for _, part := range strings.Split(scope[1:], "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsFunc(part, unicode.IsControl) {
			return fmt.Errorf("invalid scope ID %q", scope)
		}
	}
	id, err := resources.Parse(scope)
	if err != nil {
		return err
	}
	if !id.IsUCPQualified() || (!id.IsScope() && !id.IsResource()) {
		return fmt.Errorf("invalid Radius scope ID %q", scope)
	}
	return nil
}

func (r *runner) confirm(message string) error {
	if _, err := fmt.Fprintln(r.cmd.ErrOrStderr(), message); err != nil {
		return err
	}
	if r.yes {
		return nil
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("policy change requires confirmation; rerun with --yes to proceed non-interactively")
	}
	yes, err := prompt.YesOrNoPrompt("Apply this policy change?", prompt.ConfirmNo, r.factory.GetPrompter())
	if err != nil {
		return err
	}
	if !yes {
		return fmt.Errorf("policy change cancelled")
	}
	return nil
}

func (r *runner) Run(ctx context.Context) error {
	if r.mutate && !r.yes && !isatty.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("policy change requires confirmation; rerun with --yes to proceed non-interactively")
	}
	connection, err := r.connect(ctx, r.workspace)
	if err != nil {
		return err
	}
	factory, err := api.NewClientFactory(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(connection))
	if err != nil {
		return err
	}
	client := policyClient{factory: factory}
	switch r.kind {
	case "whoami":
		value, err := factory.NewInstallationAccessClient().WhoAmI(ctx, nil)
		if err != nil {
			return err
		}
		if r.cmd.Name() == "status" {
			if _, err := fmt.Fprintln(r.cmd.ErrOrStderr(), "POC limitation: production recovery and complete adoption history are not implemented."); err != nil {
				return err
			}
		}
		return r.write(value.WhoAmIResult, columns("MODE", ".mode", "TYPE", ".principal.type", "PRINCIPAL", ".principal.subject",
			"ISSUER", ".issuer", "GROUPS", ".groups[*].subject", "ADMINISTRATOR", ".isRadiusAdministrator"))
	case "permission":
		values, err := collectPages(ctx, factory.NewPermissionsClient().NewListPager(nil),
			func(p api.PermissionsClientListResponse) []*api.Permission { return p.Value })
		if err != nil {
			return err
		}
		if r.verb == "show" {
			for _, value := range values {
				if value.Name != nil && strings.EqualFold(*value.Name, r.name) {
					return r.write(value, columns("PERMISSION", ".name", "DESCRIPTION", ".description"))
				}
			}
			return fmt.Errorf("permission %q is not in this installation's catalog", r.name)
		}
		return r.write(values, columns("PERMISSION", ".name", "DESCRIPTION", ".description"))
	case "role":
		return r.runRole(ctx, client)
	case "assignment":
		return r.runAssignment(ctx, client)
	case "access":
		return r.runAccess(ctx, factory)
	case "enforcement":
		if !r.mutate {
			value, err := factory.NewAuthorizationSettingsClient().Get(ctx, nil)
			if err != nil {
				return err
			}
			return r.write(value.AuthorizationSettingsResource, columns("MODE", ".properties.mode", "GENERATION", ".properties.generation"))
		}
		if err := r.confirm("Change installation user authorization mode to " + r.mode + ". This affects all callers."); err != nil {
			return err
		}
		value, err := factory.NewAuthorizationSettingsClient().CreateOrUpdate(ctx, api.AuthorizationSettingsResource{
			Properties: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationMode(r.mode))},
		}, nil)
		if err != nil {
			return err
		}
		return r.write(value.AuthorizationSettingsResource, columns("MODE", ".properties.mode", "GENERATION", ".properties.generation"))
	case "history":
		if _, err := fmt.Fprintln(r.cmd.ErrOrStderr(), "POC limitation: history is incomplete and contains at most the last 100 policy generations."); err != nil {
			return err
		}
		if r.verb == "show" {
			value, err := factory.NewAccessChangesClient().Get(ctx, r.name, nil)
			if err != nil {
				return err
			}
			return r.write(value.AccessChangeResource, columns("ID", ".id", "GENERATION", ".properties.generation", "ACTOR", ".properties.principal.subject"))
		}
		values, err := collectPages(ctx, factory.NewAccessChangesClient().NewListPager(nil),
			func(p api.AccessChangesClientListResponse) []*api.AccessChangeResource { return p.Value })
		if err != nil {
			return err
		}
		return r.write(values, columns("ID", ".id", "GENERATION", ".properties.generation", "ACTOR", ".properties.principal.subject"))
	}
	return fmt.Errorf("unsupported authorization command %s %s", r.kind, r.verb)
}

func columns(values ...string) output.FormatterOptions {
	result := output.FormatterOptions{}
	for i := 0; i < len(values); i += 2 {
		result.Columns = append(result.Columns, output.Column{Heading: values[i], JSONPath: "{" + values[i+1] + "}"})
	}
	return result
}

func (r *runner) write(value any, options output.FormatterOptions) error {
	if r.format != output.FormatJson {
		// Generated API models implement JSON marshaling rather than struct tags.
		// Normalize them so table JSONPaths use the same names as JSON output.
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if len(raw) > 0 && raw[0] == '[' {
			var rows []map[string]any
			if err := json.Unmarshal(raw, &rows); err != nil {
				return err
			}
			value = rows
		} else {
			var row map[string]any
			if err := json.Unmarshal(raw, &row); err != nil {
				return err
			}
			value = []map[string]any{row}
		}
	}
	return r.factory.GetOutput().WriteFormatted(r.format, value, options)
}

func (r *runner) runAccess(ctx context.Context, factory *api.ClientFactory) error {
	if r.subject != nil && r.subject.Type != nil && *r.subject.Type == api.PrincipalTypeUser {
		if _, err := fmt.Fprintln(r.cmd.ErrOrStderr(), "POC limitation: inspecting another user evaluates direct grants only; that user's group membership is unavailable."); err != nil {
			return err
		}
	}
	if r.verb == "list" {
		var principal *string
		if r.subject != nil {
			raw, err := json.Marshal(r.subject)
			if err != nil {
				return err
			}
			principal = to.Ptr(string(raw))
		}
		var value api.EffectiveAccessResponse
		if r.scope == "/" {
			result, err := factory.NewInstallationAccessClient().EffectiveAccess(ctx, &api.InstallationAccessClientEffectiveAccessOptions{Principal: principal})
			if err != nil {
				return err
			}
			value = result.EffectiveAccessResponse
		} else {
			result, err := factory.NewScopedAccessClient().EffectiveAccess(ctx, r.scope, &api.ScopedAccessClientEffectiveAccessOptions{Principal: principal})
			if err != nil {
				return err
			}
			value = result.EffectiveAccessResponse
		}
		if r.format == output.FormatJson {
			return r.write(value, output.FormatterOptions{})
		}
		return r.write(value.Assignments, columns("ASSIGNMENT", ".assignment.id", "ROLE", ".assignment.properties.roleDefinitionId", "INHERITED", ".inherited"))
	}
	request := api.AccessCheckRequest{Principal: r.subject}
	for _, action := range r.actions {
		request.Checks = append(request.Checks, &api.AccessCheck{Permission: to.Ptr(action), Target: &r.scope})
	}
	if r.verb == "explain" {
		var value api.ExplainAccessResponse
		if r.scope == "/" {
			result, err := factory.NewInstallationAccessClient().ExplainAccess(ctx, request, nil)
			if err != nil {
				return err
			}
			value = result.ExplainAccessResponse
		} else {
			result, err := factory.NewScopedAccessClient().ExplainAccess(ctx, r.scope, request, nil)
			if err != nil {
				return err
			}
			value = result.ExplainAccessResponse
		}
		if r.format == output.FormatJson {
			return r.write(value, output.FormatterOptions{})
		}
		return r.write(value.Checks, columns("PERMISSION", ".permission", "TARGET", ".target", "ALLOWED", ".allowed", "GRANTS", ".grants[*].assignmentId"))
	}
	var value api.CheckAccessResponse
	if r.scope == "/" {
		result, err := factory.NewInstallationAccessClient().CheckAccess(ctx, request, nil)
		if err != nil {
			return err
		}
		value = result.CheckAccessResponse
	} else {
		result, err := factory.NewScopedAccessClient().CheckAccess(ctx, r.scope, request, nil)
		if err != nil {
			return err
		}
		value = result.CheckAccessResponse
	}
	if r.format == output.FormatJson {
		return r.write(value, output.FormatterOptions{})
	}
	return r.write(value.Checks, columns("PERMISSION", ".permission", "TARGET", ".target", "ALLOWED", ".allowed"))
}
