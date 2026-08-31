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
	"context"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/sdk"
)

var _ ctrl.Controller = (*Reconcilev20250801preview)(nil)

// Reconcilev20250801preview is the controller implementation for the reconcile custom action on
// Radius.Core/applications. It reconciles the hydrated state of every non-terminal child resource
// against its underlying provider (Kubernetes, cloud SDKs) and rewrites provisioningState in the
// state store to match reality. Called by `rad startup` after the state archive is loaded, so a
// subsequent `rad app delete` is not blocked by 409s on resources whose real state has moved on.
// See specs/006-state-restoration for the design.
//
// This is the Phase 0 stub: it validates the target application exists and returns an empty
// report. The corerp orchestrator that walks children and dispatches per-resource reconcile to
// dynamic-rp is added in a follow-up commit alongside the dynamic-rp handler that does the
// reality check.
type Reconcilev20250801preview struct {
	ctrl.Operation[*datamodel.Application_v20250801preview, datamodel.Application_v20250801preview]
	// connection is unused in the Phase 0 stub but wired now so the constructor signature stays
	// stable when the orchestrator lands and needs to fan out through UCP.
	connection sdk.Connection
}

// NewReconcilev20250801preview creates a new instance of the Reconcilev20250801preview controller.
func NewReconcilev20250801preview(opts ctrl.Options, connection sdk.Connection) (ctrl.Controller, error) {
	return &Reconcilev20250801preview{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Application_v20250801preview]{
				RequestConverter:  converter.Application20250801DataModelFromVersioned,
				ResponseConverter: converter.Application20250801DataModelToVersioned,
			},
		),
		connection,
	}, nil
}

// Run handles the reconcile custom action for Radius.Core/applications. In this Phase 0 stub it
// looks up the application (404s if missing) and returns an empty ReconcileResponse. The child
// walk and per-resource dispatch land in a follow-up commit.
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

	return rest.NewOKResponse(&corerpv20250801preview.ReconcileResponse{
		Resources: []*corerpv20250801preview.ReconcileResourceOutcome{},
	}), nil
}
