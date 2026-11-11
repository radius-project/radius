// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

// Package auth provides read-only inspection of the verified Radius caller.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/sdk"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewCommand creates the read-only auth command group.
func NewCommand(factory framework.Factory) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Inspect Radius identity and access"}
	cmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
	for _, operation := range []string{"whoami", "status"} {
		child, _ := newCommand(factory, operation)
		cmd.AddCommand(child)
	}
	access := &cobra.Command{Use: "access", Short: "Inspect Radius policy grants"}
	check, _ := newCommand(factory, "check")
	access.AddCommand(check)
	cmd.AddCommand(access)
	return cmd
}

func newCommand(factory framework.Factory, operation string) (*cobra.Command, *runner) {
	r := &runner{config: factory.GetConfigHolder(), output: factory.GetOutput(), operation: operation, connect: connect}
	cmd := &cobra.Command{
		Use: operation, Args: cobra.NoArgs, RunE: framework.RunCommand(r),
	}
	switch operation {
	case "whoami":
		cmd.Short = "Show the verified identity, issuer, and groups Radius sees"
		cmd.Example = "rad auth whoami --output json"
	case "status":
		cmd.Short = "Show current authorization mode, administrator status, and warnings"
		cmd.Example = "rad auth status"
	case "check":
		cmd.Short = "Check policy grants independently of workload enforcement mode"
		cmd.Long = "Check policy grants independently of workload enforcement mode. Self checks include verified groups. " +
			"Another group requires checkAccess permission; another user's or workload's memberships cannot be resolved and return an explicit error."
		cmd.Example = "rad auth access check --action Radius.Core/applications/read --scope /planes/radius/local/resourceGroups/team-a"
		cmd.Flags().StringVar(&r.scope, "scope", "", "Inspection scope (defaults to the workspace resource group); use / for installation")
		cmd.Flags().StringArrayVar(&r.actions, "action", nil, "Permission to check (repeat for a batch)")
		cmd.Flags().StringArrayVar(&r.targets, "target", nil, "Concrete target per action (defaults to scope; repeat in action order)")
		cmd.Flags().StringVar(&r.principal, "principal", "", "Principal as user:<subject>, group:<subject>, or workload:<subject>; defaults to the caller")
		cmd.Flags().StringVar(&r.issuer, "issuer", "", "Principal issuer (defaults to the current caller's verified issuer)")
	}
	commonflags.AddOutputFlag(cmd)
	return cmd, r
}

type inspectionClient interface {
	whoAmI(context.Context) (api.WhoAmIResult, error)
	checkAccess(context.Context, string, api.AccessCheckRequest) (api.CheckAccessResponse, error)
}

type runner struct {
	config    *framework.ConfigHolder
	output    output.Interface
	operation string
	workspace *workspaces.Workspace
	format    string
	scope     string
	actions   []string
	targets   []string
	principal string
	issuer    string
	connect   func(context.Context, workspaces.Workspace) (inspectionClient, error)
}

func (r *runner) Validate(cmd *cobra.Command, _ []string) error {
	var err error
	r.workspace, err = cli.RequireWorkspace(cmd, r.config.Config)
	if err != nil {
		return err
	}
	r.format, err = cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	if r.operation != "check" {
		return nil
	}
	if r.scope == "" {
		r.scope = r.workspace.Scope
	}
	if r.scope == "" || !strings.HasPrefix(r.scope, "/") {
		return fmt.Errorf("scope must be an absolute Radius scope ID")
	}
	if len(r.actions) == 0 {
		return fmt.Errorf("at least one --action is required")
	}
	for _, action := range r.actions {
		if strings.TrimSpace(action) == "" {
			return fmt.Errorf("action cannot be empty")
		}
	}
	if len(r.targets) != 0 && len(r.targets) != len(r.actions) {
		return fmt.Errorf("supply one --target per --action, or omit all targets to check at scope")
	}
	if r.principal == "" {
		if r.issuer != "" {
			return fmt.Errorf("--issuer requires --principal")
		}
		return nil
	}
	_, err = parsePrincipal(r.principal, r.issuer)
	return err
}

func parsePrincipal(value, issuer string) (*api.Principal, error) {
	kind, subject, found := strings.Cut(value, ":")
	typ := api.PrincipalType(kind)
	if !found || subject == "" || (typ != api.PrincipalTypeUser && typ != api.PrincipalTypeGroup && typ != api.PrincipalTypeWorkload) {
		return nil, fmt.Errorf("principal must be user:<subject>, group:<subject>, or workload:<subject>")
	}
	return &api.Principal{Type: &typ, Subject: &subject, Issuer: &issuer}, nil
}

func (r *runner) Run(ctx context.Context) error {
	client, err := r.connect(ctx, *r.workspace)
	if err != nil {
		return err
	}
	if r.operation != "check" {
		result, err := client.whoAmI(ctx)
		if err != nil {
			return err
		}
		options := output.FormatterOptions{Columns: []output.Column{
			{Heading: "TYPE", JSONPath: "{ .Principal.Type }"},
			{Heading: "SUBJECT", JSONPath: "{ .Principal.Subject }"},
			{Heading: "ISSUER", JSONPath: "{ .Issuer }"},
			{Heading: "GROUPS", JSONPath: "{ .Groups[*].Subject }"},
		}}
		if r.operation == "status" {
			options.Columns = []output.Column{
				{Heading: "MODE", JSONPath: "{ .Mode }"},
				{Heading: "RADIUS ADMINISTRATOR", JSONPath: "{ .IsRadiusAdministrator }"},
				{Heading: "WARNINGS", JSONPath: "{ .Warnings[*] }"},
			}
		}
		return r.output.WriteFormatted(r.format, result, options)
	}
	request := api.AccessCheckRequest{}
	if r.principal != "" {
		issuer := r.issuer
		if issuer == "" {
			current, err := client.whoAmI(ctx)
			if err != nil {
				return err
			}
			if current.Issuer == nil || *current.Issuer == "" {
				return fmt.Errorf("Radius returned no verified issuer")
			}
			issuer = *current.Issuer
		}
		request.Principal, err = parsePrincipal(r.principal, issuer)
		if err != nil {
			return err
		}
	}
	for i, action := range r.actions {
		target := r.scope
		if len(r.targets) != 0 {
			target = r.targets[i]
		}
		request.Checks = append(request.Checks, &api.AccessCheck{Permission: to.Ptr(action), Target: to.Ptr(target)})
	}
	result, err := client.checkAccess(ctx, r.scope, request)
	if err != nil {
		return err
	}
	if r.format == "json" {
		return r.output.WriteFormatted(r.format, result, output.FormatterOptions{})
	}
	type row struct {
		Principal  *api.Principal
		Generation *int64
		Mode       *api.UserAuthorizationMode
		*api.AccessCheckResult
	}
	rows := make([]row, 0, len(result.Checks))
	for _, check := range result.Checks {
		rows = append(rows, row{result.Principal, result.Generation, result.Mode, check})
	}
	return r.output.WriteFormatted(r.format, rows, output.FormatterOptions{Columns: []output.Column{
		{Heading: "SUBJECT", JSONPath: "{ .Principal.Subject }"},
		{Heading: "MODE", JSONPath: "{ .Mode }"},
		{Heading: "GENERATION", JSONPath: "{ .Generation }"},
		{Heading: "PERMISSION", JSONPath: "{ .Permission }"},
		{Heading: "TARGET", JSONPath: "{ .Target }"},
		{Heading: "ALLOWED", JSONPath: "{ .Allowed }"},
	}})
}

type apiClient struct {
	installation *api.InstallationAccessClient
	scoped       *api.ScopedAccessClient
}

func connect(ctx context.Context, workspace workspaces.Workspace) (inspectionClient, error) {
	connection, err := workspace.Connect(ctx)
	if err != nil {
		return nil, err
	}
	options := sdk.NewClientOptions(connection)
	installation, err := api.NewInstallationAccessClient(&aztoken.AnonymousCredential{}, options)
	if err != nil {
		return nil, err
	}
	scoped, err := api.NewScopedAccessClient(&aztoken.AnonymousCredential{}, options)
	if err != nil {
		return nil, err
	}
	return &apiClient{installation, scoped}, nil
}

func (c *apiClient) whoAmI(ctx context.Context) (api.WhoAmIResult, error) {
	response, err := c.installation.WhoAmI(ctx, nil)
	return response.WhoAmIResult, inspectionError(err)
}

func (c *apiClient) checkAccess(ctx context.Context, scope string, request api.AccessCheckRequest) (api.CheckAccessResponse, error) {
	if scope == "/" {
		response, err := c.installation.CheckAccess(ctx, request, nil)
		return response.CheckAccessResponse, inspectionError(err)
	}
	response, err := c.scoped.CheckAccess(ctx, strings.TrimPrefix(scope, "/"), request, nil)
	return response.CheckAccessResponse, inspectionError(err)
}

func inspectionError(err error) error {
	var responseError *azcore.ResponseError
	if !errors.As(err, &responseError) || responseError.RawResponse == nil {
		return err
	}
	body, readErr := runtime.Payload(responseError.RawResponse)
	if readErr != nil {
		return err
	}
	var status metav1.Status
	if json.Unmarshal(body, &status) != nil || status.Kind != "Status" || status.Status != metav1.StatusFailure || status.Message == "" {
		return err
	}
	// Kubernetes gateway errors are Status objects, not ARM error envelopes.
	// Keep their diagnostic instead of letting the CLI render an empty ARM error.
	status.Code = int32(responseError.StatusCode)
	return &apierrors.StatusError{ErrStatus: status}
}
