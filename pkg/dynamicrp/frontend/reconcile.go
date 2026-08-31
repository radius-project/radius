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

package frontend

import (
	"context"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// ReconcileResourceOutcome mirrors the wire shape defined in Radius.Core's TypeSpec so the
// corerp app-scoped orchestrator can aggregate reports across resource-provider namespaces
// without re-marshaling. Kept lowercase in JSON to match the client's expectations.
type ReconcileResourceOutcome struct {
	ResourceID string `json:"resourceId"`
	From       string `json:"from"`
	To         string `json:"to"`
	Reason     string `json:"reason,omitempty"`
}

// ReconcileResponse is the per-resource reconcile response emitted by dynamic-rp. The corerp
// orchestrator (which fans out per-application) collects one ReconcileResourceOutcome per
// resource; each dynamic-rp reconcile call returns a single-element resources array (or an empty
// array when the resource is already terminal and no work was needed).
type ReconcileResponse struct {
	Resources []ReconcileResourceOutcome `json:"resources"`
}

// Reconcile is the dynamic-rp handler for the reconcile custom action registered on every dynamic
// resource type. See specs/006-state-restoration: when 'rad startup' invokes the app-scoped
// reconcile on Radius.Core/applications, the corerp orchestrator will POST to this handler once
// per non-terminal child resource; the handler walks the resource's outputResources, checks each
// one against its underlying provider, and PATCHes provisioningState to reflect reality.
//
// This is the Phase 1 wiring commit: routing is in place and the handler returns an empty report,
// so the corerp orchestrator can start dispatching without breaking the build. The reality-check
// logic (Kubernetes GETs on outputResources, PATCH back through the RP's normal write path) lands
// in the next commit.
type Reconcile struct {
	ctrl.Operation[*datamodel.DynamicResource, datamodel.DynamicResource]
	resourceOptions ctrl.ResourceOptions[datamodel.DynamicResource]
	ucpClient       *v20231001preview.ClientFactory
}

// NewReconcile constructs the reconcile controller for a dynamic resource type.
func NewReconcile(opts ctrl.Options, resourceOptions ctrl.ResourceOptions[datamodel.DynamicResource], ucpClient *v20231001preview.ClientFactory) (ctrl.Controller, error) {
	return &Reconcile{
		Operation:       ctrl.NewOperation(opts, resourceOptions),
		resourceOptions: resourceOptions,
		ucpClient:       ucpClient,
	}, nil
}

// Run resolves the target resource (404 if missing) and returns an empty ReconcileResponse. The
// per-outputResource reality check that populates the response lands in a follow-up commit; this
// handler exists now so the corerp orchestrator has a stable endpoint to dispatch to while
// Phase 1 fills in.
func (c *Reconcile) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Route: /planes/radius/{plane}/resourceGroups/{rg}/providers/{ns}/{type}/{name}/reconcile
	resourceID := sCtx.ResourceID.Truncate()
	resource, _, err := c.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	return rest.NewOKResponse(&ReconcileResponse{Resources: []ReconcileResourceOutcome{}}), nil
}
