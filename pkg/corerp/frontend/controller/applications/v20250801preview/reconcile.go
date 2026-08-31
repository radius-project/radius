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

package v20250801preview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*Reconcilev20250801preview)(nil)

// staticResourceProviderNamespaces are the built-in resource providers whose resources are not
// reality-checked by the orchestrator (either because they're served by a static RP that does not
// implement /reconcile, or because they're the parent Radius.Core namespace itself). Everything
// else is assumed to be served by dynamic-rp, which registers /reconcile on every dynamic type.
var staticResourceProviderNamespaces = map[string]struct{}{
	"Applications.Core":       {},
	"Applications.Dapr":       {},
	"Applications.Datastores": {},
	"Applications.Messaging":  {},
	"Radius.Core":             {},
	"Microsoft.Resources":     {},
}

const (
	// reconcileChildTimeout caps one child RP's reconcile call so a single unresponsive RP cannot
	// hang the whole orchestrator (plan §Risks).
	reconcileChildTimeout = 15 * time.Second
	// reconcileChildConcurrency bounds fan-out so we don't stampede UCP or the target RPs.
	reconcileChildConcurrency = 8
)

// Reconcilev20250801preview is the controller for the reconcile custom action on
// Radius.Core/applications. When `rad startup` invokes it after loading a state archive, it walks
// the application's dynamic children, POSTs the reconcile action to each one's RP (dynamic-rp
// today), and aggregates the per-resource outcomes into one response. Terminal children are
// skipped and do not appear in the response. See specs/006-state-restoration for the end-to-end
// design.
type Reconcilev20250801preview struct {
	ctrl.Operation[*datamodel.Application_v20250801preview, datamodel.Application_v20250801preview]
	connection sdk.Connection

	// listChildren enumerates the resources associated with the application, restricted to
	// dynamic-rp-served namespaces. Injectable so unit tests can stub the child walk without
	// standing up UCP.
	listChildren func(ctx context.Context, applicationID resources.ID) ([]generated.GenericResource, error)
	// reconcileChild POSTs the reconcile action to one child resource and returns its RP's
	// per-resource outcomes. Also injectable.
	reconcileChild func(ctx context.Context, child generated.GenericResource) ([]*corerpv20250801preview.ReconcileResourceOutcome, error)
}

// NewReconcilev20250801preview constructs the reconcile controller with production defaults
// (UCP-backed child walk and per-child dispatch).
func NewReconcilev20250801preview(opts ctrl.Options, connection sdk.Connection) (ctrl.Controller, error) {
	return newReconcilev20250801preview(opts, connection, nil, nil), nil
}

// newReconcilev20250801preview constructs a Reconcile controller with optional hook overrides.
// Passing nil for a hook installs the default UCP-backed implementation. Tests use this directly
// to inject fakes without standing up a UCP endpoint.
func newReconcilev20250801preview(
	opts ctrl.Options,
	connection sdk.Connection,
	listChildren func(ctx context.Context, applicationID resources.ID) ([]generated.GenericResource, error),
	reconcileChild func(ctx context.Context, child generated.GenericResource) ([]*corerpv20250801preview.ReconcileResourceOutcome, error),
) *Reconcilev20250801preview {
	c := &Reconcilev20250801preview{
		Operation: ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Application_v20250801preview]{
				RequestConverter:  converter.Application20250801DataModelFromVersioned,
				ResponseConverter: converter.Application20250801DataModelToVersioned,
			},
		),
		connection: connection,
	}
	if listChildren != nil {
		c.listChildren = listChildren
	} else {
		c.listChildren = c.defaultListChildren
	}
	if reconcileChild != nil {
		c.reconcileChild = reconcileChild
	} else {
		c.reconcileChild = c.defaultReconcileChild
	}
	return c
}

// Run handles the reconcile custom action for Radius.Core/applications.
func (c *Reconcilev20250801preview) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Route: /planes/radius/local/resourcegroups/{rg}/providers/Radius.Core/applications/{app}/reconcile
	applicationID := sCtx.ResourceID.Truncate()
	applicationResource, _, err := c.GetResource(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if applicationResource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	children, err := c.listChildren(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate application children: %w", err)
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	outcomes := make([]*corerpv20250801preview.ReconcileResourceOutcome, 0, len(children))
	var mu sync.Mutex
	sem := make(chan struct{}, reconcileChildConcurrency)
	g, gCtx := errgroup.WithContext(ctx)

	for _, child := range children {
		child := child

		if isTerminalProvisioningState(child.Properties) {
			continue
		}

		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()

			perChildCtx, cancel := context.WithTimeout(gCtx, reconcileChildTimeout)
			defer cancel()

			childOutcomes, err := c.reconcileChild(perChildCtx, child)
			if err != nil {
				// One unreachable RP does not fail the whole reconcile: record the failure as a
				// per-child skipped outcome and move on.
				logger.V(ucplog.LevelDebug).Info("reconcile dispatch failed", "id", to.String(child.ID), "error", err.Error())
				state := readProvisioningState(child.Properties)
				childOutcomes = []*corerpv20250801preview.ReconcileResourceOutcome{{
					ResourceID: child.ID,
					From:       state,
					To:         state,
					Reason:     to.Ptr(fmt.Sprintf("reconcile dispatch failed: %v", err)),
				}}
			}

			mu.Lock()
			outcomes = append(outcomes, childOutcomes...)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return rest.NewOKResponse(&corerpv20250801preview.ReconcileResponse{Resources: outcomes}), nil
}

// defaultListChildren walks the resource-type registry restricted to dynamic-rp-served namespaces
// and returns every resource associated with the application. Reuses the same helpers getGraph
// uses so the two custom actions stay consistent.
func (c *Reconcilev20250801preview) defaultListChildren(ctx context.Context, applicationID resources.ID) ([]generated.GenericResource, error) {
	clientOptions := sdk.NewClientOptions(c.connection)

	ucpMgmt := &clients.UCPApplicationsManagementClient{
		RootScope:     radiusPlane + planeName,
		ClientOptions: clientOptions,
	}

	allTypes, err := ucpMgmt.ListAllResourceTypesNames(ctx, planeName)
	if err != nil {
		return nil, err
	}

	dynamicTypes := make([]string, 0, len(allTypes))
	for _, t := range allTypes {
		ns, _, ok := strings.Cut(t, "/")
		if !ok {
			continue
		}
		if _, static := staticResourceProviderNamespaces[ns]; static {
			continue
		}
		dynamicTypes = append(dynamicTypes, t)
	}

	return listAllResourcesByApplication(ctx, applicationID, dynamicTypes, clientOptions)
}

// defaultReconcileChild POSTs the reconcile action to a single child through the shared UCP
// connection. A 404 or 405 from the RP means "no reconcile route" — recorded as skipped so the
// orchestrator remains forward-compatible with RPs that don't (yet) implement /reconcile.
func (c *Reconcilev20250801preview) defaultReconcileChild(ctx context.Context, child generated.GenericResource) ([]*corerpv20250801preview.ReconcileResourceOutcome, error) {
	if child.ID == nil || *child.ID == "" || child.Type == nil {
		return nil, fmt.Errorf("child resource is missing id or type")
	}

	clientOptions := sdk.NewClientOptions(c.connection)
	apiVersion, err := getAPIVersionForResourceType(ctx, *child.Type, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve api-version for %s: %w", *child.Type, err)
	}

	endpoint := strings.TrimSuffix(c.connection.Endpoint(), "/")
	url := fmt.Sprintf("%s%s/reconcile?api-version=%s", endpoint, *child.ID, apiVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.connection.Client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotFound {
		state := readProvisioningState(child.Properties)
		return []*corerpv20250801preview.ReconcileResourceOutcome{{
			ResourceID: child.ID,
			From:       state,
			To:         state,
			Reason:     to.Ptr(fmt.Sprintf("skipped: RP does not implement reconcile (%d)", resp.StatusCode)),
		}}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("reconcile returned status %d", resp.StatusCode)
	}

	var body corerpv20250801preview.ReconcileResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode reconcile response: %w", err)
	}
	return body.Resources, nil
}

// isTerminalProvisioningState returns true when the resource's recorded provisioningState is
// terminal. Used to skip children that don't need reconciliation.
func isTerminalProvisioningState(props map[string]any) bool {
	state := readProvisioningState(props)
	if state == nil {
		return false
	}
	return v1.ProvisioningState(*state).IsTerminal()
}

// readProvisioningState pulls provisioningState out of a raw properties map. Returns nil when the
// field is absent or not a string.
func readProvisioningState(props map[string]any) *string {
	if props == nil {
		return nil
	}
	raw, ok := props["provisioningState"]
	if !ok {
		return nil
	}
	s, ok := raw.(string)
	if !ok {
		return nil
	}
	return to.Ptr(s)
}
