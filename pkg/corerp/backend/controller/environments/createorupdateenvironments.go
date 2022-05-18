// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"

	"github.com/project-radius/radius/pkg/basedatamodel"
	asyncctrl "github.com/project-radius/radius/pkg/corerp/backend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/store"
)

var _ asyncctrl.AsyncControllerInterface = (*CreateOrUpdateEnvironmentAsync)(nil)

type CreateOrUpdateEnvironmentAsync struct {
	asyncctrl.BaseAsyncController
}

func NewCreateOrUpdateEnvironmentAsync(sc store.StorageClient) (asyncctrl.AsyncControllerInterface, error) {
	return &CreateOrUpdateEnvironmentAsync{
		BaseAsyncController: asyncctrl.BaseAsyncController{
			StoreClient: sc,
			AsyncResp:   make(chan *asyncctrl.AsyncResponse),
		},
	}, nil
}

func (c *CreateOrUpdateEnvironmentAsync) Run(ctx context.Context) error {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)
	resp := asyncctrl.NewAsyncResponse(sCtx.OperationID, sCtx.OperationName, sCtx.ResourceID, basedatamodel.ProvisioningStateUpdating)
	c.Reply(resp)

	// TODO: Do something

	resp.SetSucceeded()
	c.Reply(resp)

	return nil
}
