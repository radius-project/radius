/*
Copyright 2023 The Radius Authors.

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

package startup

import (
	"context"
	"errors"
	"fmt"

	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/controlplane"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/pgbackup"
	"github.com/radius-project/radius/pkg/cli/tfstate"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/sdk"
)

// ControlPlaneScaler scales the database-backed control-plane deployments to zero and back, so
// state can be restored while no resource provider holds a live PostgreSQL connection.
type ControlPlaneScaler interface {
	// ScaleDown scales the control-plane deployments to zero and returns their previous replica
	// counts so they can be restored by ScaleUp.
	ScaleDown(ctx context.Context) (map[string]int32, error)

	// ScaleUp restores the deployments to the replica counts captured by ScaleDown and waits until
	// they are available again.
	ScaleUp(ctx context.Context, saved map[string]int32) error
}

// newScalerForContext is the production factory for a ControlPlaneScaler. It is a package variable
// so tests can replace it without a cluster.
var newScalerForContext = func(kubeContext, namespace string) (ControlPlaneScaler, error) {
	return controlplane.NewScalerForContext(kubeContext, namespace)
}

// StateRestoreClient restores the durable Radius state for a Kubernetes context. It wraps the
// pgbackup and tfstate packages so the command can be unit tested without a cluster.
type StateRestoreClient interface {
	// WaitForDatabaseReady blocks until the control-plane PostgreSQL instance is ready.
	WaitForDatabaseReady(ctx context.Context, kubeContext, namespace string) error

	// RestoreDatabases loads the control-plane PostgreSQL dumps from stateDir.
	RestoreDatabases(ctx context.Context, kubeContext, namespace, stateDir string) error

	// RestoreTerraform re-creates the Terraform state Secrets from stateDir.
	RestoreTerraform(ctx context.Context, kubeContext, namespace, stateDir string) error

	// ReconcileHydratedState invokes the reconcile custom action on every application in the
	// workspace's plane. It is best-effort: individual per-application failures are recorded in
	// the returned reports but never propagate as a fatal error. Called by 'rad startup' after
	// ScaleUp so subsequent commands see reality-checked state.
	//
	// A non-nil error is returned only when the pass could not begin at all (for example, the
	// workspace's control plane is unreachable). Callers should treat such errors as advisory and
	// still return success from the outer startup command.
	ReconcileHydratedState(ctx context.Context, workspace *workspaces.Workspace) ([]ApplicationReconcileReport, error)
}

// ApplicationReconcileReport captures the per-application outcome of ReconcileHydratedState. One
// entry is produced for every application the reconcile pass attempted, whether it succeeded or
// not.
type ApplicationReconcileReport struct {
	// Name is the application resource name (not the fully-qualified resource ID).
	Name string
	// ResourceCount is the number of child resources the reconcile handler reported an outcome
	// for. Zero when the reconcile handler is still a stub, when the application has no
	// non-terminal children, or when the reconcile call itself failed.
	ResourceCount int
	// Err is set when the reconcile call for this application failed. The pass continues to the
	// next application regardless.
	Err error
}

// defaultStateRestoreClient is the production implementation.
type defaultStateRestoreClient struct{}

// NewStateRestoreClient returns the production StateRestoreClient.
func NewStateRestoreClient() StateRestoreClient {
	return defaultStateRestoreClient{}
}

func (defaultStateRestoreClient) WaitForDatabaseReady(ctx context.Context, kubeContext, namespace string) error {
	return pgbackup.WaitForReady(ctx, kubeContext, namespace)
}

func (defaultStateRestoreClient) RestoreDatabases(ctx context.Context, kubeContext, namespace, stateDir string) error {
	return pgbackup.Restore(ctx, kubeContext, namespace, stateDir)
}

func (defaultStateRestoreClient) RestoreTerraform(ctx context.Context, kubeContext, namespace, stateDir string) error {
	client, err := tfstate.NewClientForContext(kubeContext, namespace)
	if err != nil {
		return err
	}
	return client.Restore(ctx, stateDir)
}

// ReconcileHydratedState lists every application in the workspace's plane and POSTs the
// Radius.Core/applications 'reconcile' custom action on each. Reports are aggregated across
// pagination and returned to the caller.
func (defaultStateRestoreClient) ReconcileHydratedState(ctx context.Context, workspace *workspaces.Workspace) ([]ApplicationReconcileReport, error) {
	if workspace == nil {
		return nil, errors.New("workspace is required")
	}

	connection, err := workspace.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to workspace: %w", err)
	}

	clientOptions := sdk.NewClientOptions(connection)
	factory, err := corerpv20250801preview.NewClientFactory(&tokencredentials.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to build Radius.Core client factory: %w", err)
	}
	applications := factory.NewApplicationsClient()

	// Collect application names first so a stalled reconcile does not stall the LIST.
	names, err := listApplicationNames(ctx, applications, workspace.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list applications for reconcile: %w", err)
	}

	reports := make([]ApplicationReconcileReport, 0, len(names))
	for _, name := range names {
		report := ApplicationReconcileReport{Name: name}
		resp, err := applications.Reconcile(ctx, workspace.Scope, name, corerpv20250801preview.ReconcileRequest{}, nil)
		if err != nil {
			report.Err = err
		} else {
			report.ResourceCount = len(resp.Resources)
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// listApplicationNames walks the paginated ListByScope response for `scope` and returns the
// application resource names. Nil entries and entries without a Name are skipped.
func listApplicationNames(ctx context.Context, client *corerpv20250801preview.ApplicationsClient, scope string) ([]string, error) {
	pager := client.NewListByScopePager(scope, &corerpv20250801preview.ApplicationsClientListByScopeOptions{})
	var names []string
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, app := range page.Value {
			if app == nil || app.Name == nil {
				continue
			}
			names = append(names, *app.Name)
		}
	}
	return names, nil
}

// logReconcileReports emits one line per application, and a summary line if any application
// failed. Used by rad startup's ReconcileHydratedState stage to surface outcomes in the workflow
// log.
func logReconcileReports(out output.Interface, reports []ApplicationReconcileReport) {
	failed := 0
	for _, r := range reports {
		if r.Err != nil {
			failed++
			out.LogInfo("  reconcile %s: failed (%s)", r.Name, r.Err.Error())
			continue
		}
		out.LogInfo("  reconcile %s: reconciled %d resource(s)", r.Name, r.ResourceCount)
	}
	if failed > 0 {
		out.LogInfo("Reconcile completed with %d/%d application failures; continuing.", failed, len(reports))
	}
}
