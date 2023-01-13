// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	fctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*DeleteDaprPubSubBroker)(nil)

// DeleteDaprPubSubBroker is the controller implementation to delete daprPubSubBroker link resource.
type DeleteDaprPubSubBroker struct {
	ctrl.Operation[*datamodel.DaprPubSubBroker, datamodel.DaprPubSubBroker]

	KubeClient runtimeclient.Client
	de         deployment.DeploymentProcessor
}

// NewDeleteDaprPubSubBroker creates a new instance DeleteDaprPubSubBroker.
func NewDeleteDaprPubSubBroker(opts fctrl.Options) (ctrl.Controller, error) {
	return &DeleteDaprPubSubBroker{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
				RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
				ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
		de:         opts.DeployProcessor,
	}, nil
}

func (d *DeleteDaprPubSubBroker) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := d.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := d.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	err = d.de.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: old, OutputResources: old.Properties.Status.OutputResources, ComputedValues: old.ComputedValues, SecretValues: old.SecretValues, RecipeData: old.RecipeData})
	if err != nil {
		return nil, err
	}

	if err := d.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
