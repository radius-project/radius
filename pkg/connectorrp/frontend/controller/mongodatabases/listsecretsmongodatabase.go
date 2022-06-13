// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"errors"
	"net/http"

	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListSecretsMongoDatabase)(nil)

// ListSecretsMongoDatabase is the controller implementation to list secrets for the to access the connected mongo database resource resource id passed in the request body.
type ListSecretsMongoDatabase struct {
	ctrl.BaseController
}

// NewListSecretsMongoDatabase creates a new instance of ListSecretsMongoDatabase.
func NewListSecretsMongoDatabase(ds store.StorageClient, sm manager.StatusManager) (ctrl.Controller, error) {
	return &ListSecretsMongoDatabase{ctrl.NewBaseController(ds, sm)}, nil
}

// Run returns secrets values for the specified MongoDatabase resource
func (ctrl *ListSecretsMongoDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	resource := &datamodel.MongoDatabase{}
	_, err := ctrl.GetResource(ctx, sCtx.ResourceID.String(), resource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(sCtx.ResourceID), nil
		}
		return nil, err
	}

	// TODO integrate with deploymentprocessor
	// output, err := ctrl.JobEngine.FetchSecrets(ctx, sCtx.ResourceID, resource)
	// if err != nil {
	// 	return nil, err
	// }

	versioned, _ := converter.MongoDatabaseSecretsDataModelToVersioned(&datamodel.MongoDatabaseSecrets{}, sCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
