// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/backend/deployment"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/schema"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

var ErrUnsupportedResourceType = errors.New("unsupported resource type")

//go:generate mockgen -destination=./mock_resourceprovider.go -package=resourceprovider -self_package github.com/Azure/radius/pkg/radrp/frontend/resourceprovider github.com/Azure/radius/pkg/radrp/frontend/resourceprovider ResourceProvider

// ResourceProvider defines the business logic of the resource provider for Radius.
type ResourceProvider interface {
	ListApplications(ctx context.Context, id azresources.ResourceID) (rest.Response, error)
	GetApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error)
	UpdateApplication(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error)
	DeleteApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error)

	ListResources(ctx context.Context, id azresources.ResourceID) (rest.Response, error)
	GetResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error)
	UpdateResource(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error)
	DeleteResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error)

	GetOperation(ctx context.Context, id azresources.ResourceID) (rest.Response, error)

	ListSecrets(ctx context.Context, input ListSecretsInput) (rest.Response, error)

	ListAllV3ResourcesByApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error)

	GetSwaggerDoc(ctx context.Context) (rest.Response, error)
}

// NewResourceProvider creates a new ResourceProvider.
func NewResourceProvider(db db.RadrpDB, deploy deployment.DeploymentProcessor, completions chan<- struct{}) ResourceProvider {
	return &rp{db: db, deploy: deploy, completions: completions}
}

type rp struct {
	db     db.RadrpDB
	deploy deployment.DeploymentProcessor

	// completions is used to signal the completion of asynchronous processing. This is use for tests
	// So we can avoid panics happening when the test is finished.
	//
	// DO NOT use this to implement product functionality, this is a hook for testing.
	completions chan<- struct{}
}

// As a general design principle, returning an error from the RP signals an internal error (500).
// Code paths that validate input should return a rest.Response.

func (r *rp) ListApplications(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateApplicationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	items, err := r.db.ListV3Applications(ctx, id)
	if err != nil {
		return nil, err
	}

	output := ApplicationResourceList{}
	for _, item := range items {
		output.Value = append(output.Value, NewRestApplicationResource(item))
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) GetApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateApplicationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	item, err := r.db.GetV3Application(ctx, id)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	output := NewRestApplicationResource(item)
	return rest.NewOKResponse(output), nil
}

func (r *rp) UpdateApplication(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
	err := r.validateApplicationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	application := ApplicationResource{}
	err = json.Unmarshal(body, &application)
	if err != nil {
		return nil, err // Unexpected error, the payload has already been validated.
	}

	item := NewDBApplicationResource(id, application)
	created, err := r.db.UpdateV3ApplicationDefinition(ctx, item)
	if err != nil {
		return nil, err
	}

	output := NewRestApplicationResource(item)
	if created {
		return rest.NewCreatedResponse(output), nil
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) DeleteApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateApplicationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	err = r.db.DeleteV3Application(ctx, id)
	if err == db.ErrConflict {
		return rest.NewConflictResponse(err.Error()), nil
	} else if err == db.ErrNotFound {
		// Ignore not found for a delete: the resource is already gone.
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	return rest.NewNoContentResponse(), nil
}

func (r *rp) ListAllV3ResourcesByApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// Format of request url/id for list resources is:
	// /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/{applicationName}/RadiusResource

	// Validate request url has correct application resource type
	applicationID := id.Truncate()
	err := r.validateApplicationType(applicationID)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// Validate the application exists
	_, err = r.db.GetV3Application(ctx, applicationID)
	if err != nil {
		if err == db.ErrNotFound {
			return rest.NewNotFoundResponse(id), nil
		}
		return nil, err
	}

	applicationName := applicationID.Name()
	applicationSubscriptionID := id.SubscriptionID
	applicationResourceGroup := id.ResourceGroup

	// List radius resources
	radiusResources, err := r.db.ListAllV3ResourcesByApplication(ctx, id, applicationName)
	if err != nil {
		return nil, err
	}

	// List non-radius azure resources that are referenced from the application
	azureResources, err := r.db.ListAllAzureResourcesForApplication(ctx, applicationName, applicationSubscriptionID, applicationResourceGroup)
	if err != nil {
		return nil, err
	}

	outputResourceList := RadiusResourceList{}
	for _, radiusResource := range radiusResources {
		outputResourceList.Value = append(outputResourceList.Value, NewRestRadiusResource(radiusResource))
	}
	for _, azureResource := range azureResources {
		outputResourceList.Value = append(outputResourceList.Value, NewRestRadiusResourceFromAzureResource(azureResource))
	}

	return rest.NewOKResponse(outputResourceList), nil
}

func (r *rp) ListResources(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateResourceType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// GET ..../Application/{applicationName}/{resourceType}
	// GET ..../Application/{applicationName}/{resourceType}/{resourceName}

	// // ..../Application/hello/Container
	items, err := r.db.ListV3Resources(ctx, id)

	if err == db.ErrNotFound {
		// It's possible that the application does not exist.
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}
	output := RadiusResourceList{}
	for _, item := range items {
		output.Value = append(output.Value, NewRestRadiusResource(item))
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) GetResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateResourceType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	item, err := r.db.GetV3Resource(ctx, id)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	output := NewRestRadiusResource(item)
	return rest.NewOKResponse(output), nil
}

func (r *rp) UpdateResource(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
	err := r.validateResourceType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	resource := RadiusResource{}
	err = json.Unmarshal(body, &resource)
	if err != nil {
		return nil, err // Unexpected error, the payload has already been validated.
	}

	// We'll now begin asynchronous processing of the resource. Three things to do:
	// 1. Set resource to non-terminal state
	// 2. Create operation for tracking
	// 3. Start processing
	item := NewDBRadiusResource(id, resource)
	item.ProvisioningState = string(rest.DeployingStatus)
	_, err = r.db.UpdateV3ResourceDefinition(ctx, id, item)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	oid := id.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: uuid.New().String()})
	operation := db.NewOperation(oid, db.OperationKindUpdate, string(rest.DeployingStatus))
	_, err = r.db.PatchOperationByID(ctx, oid, &operation)
	if err != nil {
		return nil, err
	}

	r.ProcessDeploymentBackground(ctx, oid, item)

	output := NewRestRadiusResource(item)
	return rest.NewAcceptedAsyncResponse(output, oid.ID), nil
}

func (r *rp) DeleteResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateResourceType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	item, err := r.db.GetV3Resource(ctx, id)
	if err == db.ErrNotFound {
		// Ignore not found for a delete: the resource is already gone.
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	// We'll now begin asynchronous processing of the resource. Three things to do:
	// 1. Set resource to non-terminal state
	// 2. Create operation for tracking
	// 3. Start processing
	item.ProvisioningState = string(rest.DeletingStatus)
	_, err = r.db.UpdateV3ResourceDefinition(ctx, id, item)
	if err != nil {
		return nil, err
	}

	oid := id.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: uuid.New().String()})
	operation := db.NewOperation(oid, db.OperationKindDelete, string(rest.DeletingStatus))
	_, err = r.db.PatchOperationByID(ctx, oid, &operation)
	if err != nil {
		return nil, err
	}

	r.ProcessDeletionBackground(ctx, oid, item)

	output := NewRestRadiusResource(item)
	return rest.NewAcceptedAsyncResponse(output, oid.ID), nil
}

func (r *rp) ListSecrets(ctx context.Context, input ListSecretsInput) (rest.Response, error) {
	id, err := azresources.Parse(input.TargetID)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	err = r.validateResourceType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// This is not optimal but has to be done... Long explanation incoming:
	//
	// Custom RP only allows defining custom actions on the RP itself (not on child resources)
	// so in the case of `db.connectionString()` the operation is *actually* defined on the custom RP
	// resource, not on 'db'.
	//
	// The problem here is that there's no way to make the RP wait for 'db' to complete before trying to
	// access the connection string. So the best we can do for now is to just treat it as a 500 and expect
	// the deployment engine to retry.
	item, err := r.db.GetV3Resource(ctx, id)
	if err == db.ErrNotFound || item.ProvisioningState != string(rest.SuccededStatus) {
		return rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: "resource is not ready yet",
				Target:  id.ID,
			},
		}), nil
	} else if err != nil {
		return nil, err
	}

	output, err := r.deploy.FetchSecrets(ctx, id, item)
	if err != nil {
		return nil, err
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) GetOperation(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateOperationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// This code path is complex because there are a few different case to handle.
	//
	// 1. The operation has failed (terminal state): we want to return an ARM error
	// 2. The operation has completed (non-terminal state): we want to return the resource body
	// 3. The operation is ongoing (non-terminal state): we want to return the resource body
	//
	// Cases 2 & 3 are separate because we need to return different status codes and headers
	// based on the operation being performed and whether it's done.

	operation, err := r.db.GetOperationByID(ctx, id)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	// 1. Handle the cases where an asynchronous failure occurred.
	//
	// FYI: The resource body just has the provisioning status, and thus doesn't have the ability to give a reason
	// for failure. If there's a failure, return it in the ARM format.
	if operation.Error != nil && operation.Error.Code == armerrors.Invalid {
		// Operation failed with a validation or business logic error
		return rest.NewBadRequestARMResponse(armerrors.ErrorResponse{
			Error: *operation.Error,
		}), nil
	} else if operation.Error != nil {
		// Operation failed with an uncategorized error
		return rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
			Error: *operation.Error,
		}), nil
	} else if operation.Status == string(rest.FailedStatus) {
		// Operation failed with an uncategorized error
		return rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: "internal error",
			},
		}), nil
	}

	// If we get here we'll likely need the resource body for the response, so look it up.
	//
	// The resource being tracked by this operation can be found by truncating the last type/name
	// segment of the operation ID. We don't do long-running operations on Application, so it's
	// guaranteed to be a RadiusResource.
	targetID := id.Truncate()
	item, err := r.db.GetV3Resource(ctx, targetID)
	if err == db.ErrNotFound && operation.OperationKind == db.OperationKindDelete {
		// 2. As a special case: the original resource will be *gone* for a successful deletion.
		//
		// We need to return a 204 for this case.
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	output := NewRestRadiusResource(item)
	if rest.IsTeminalStatus(rest.OperationStatus(item.ProvisioningState)) {
		// 2. Operation is complete
		return rest.NewOKResponse(output), nil
	}

	// 3. Operation is still processing.
	// The ARM-RPC spec wants us to keep returning 202 from here until the operation is complete.
	return rest.NewAcceptedAsyncResponse(output, id.ID), nil
}

func (r *rp) GetSwaggerDoc(ctx context.Context) (rest.Response, error) {
	return rest.NewOKResponse([]byte{}), nil
}

func (r *rp) ProcessDeploymentBackground(ctx context.Context, operationID azresources.ResourceID, resource db.RadiusResource) {
	err := r.validateOperationType(operationID)
	if err != nil {
		// These functions should always be passed the resource ID of an operation. This is a programing error
		// if it's not.
		panic(err)
	}

	// We need to create a new context to pass to the background process. We can't use the current
	// context because it is tied to the request lifecycle.
	ctx = logr.NewContext(context.Background(), radlogger.GetLogger(ctx))

	go func() {
		// Signal compeletion of the operation FOR TESTING ONLY
		defer r.complete()

		logger := radlogger.GetLogger(ctx)
		err := r.deploy.Deploy(ctx, operationID, resource)
		if err != nil {
			logger.Error(err, "deployment failed")
			return
		}

		logger.Info("deployment completed")
	}()
}

func (r *rp) ProcessDeletionBackground(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) {
	err := r.validateOperationType(id)
	if err != nil {
		// These functions should always be passed the resource ID of an operation. This is a programing error
		// if it's not.
		panic(err)
	}

	// We need to create a new context to pass to the background process. We can't use the current
	// context because it is tied to the request lifecycle.
	ctx = logr.NewContext(context.Background(), radlogger.GetLogger(ctx))

	go func() {
		// Signal compeletion of the operation FOR TESTING ONLY
		defer r.complete()

		logger := radlogger.GetLogger(ctx)
		err := r.deploy.Delete(ctx, id, resource)
		if err != nil {
			logger.Error(err, "deletion failed")
			return
		}

		logger.Info("deletion completed")
	}()
}

func (r *rp) complete() {
	// Performing logging after a test completes will cause panics since we're using
	// the test system for logging.
	//
	// Since deployment/deletion is an asynchronous process we thus need a notification mechanism
	// so that the test can block until processing is complete. This channel is that mechanism.
	//
	// If we switch to a database-driven mechanism for scheduling work this can be removed
	// since the RP will not be involved in starting any asynchronous process.
	if r.completions != nil {
		r.completions <- struct{}{}
	}
}

// We don't really expect an invalid type to get through ARM's routing
// but we're testing it anyway to catch bugs.
func (r *rp) validateApplicationType(id azresources.ResourceID) error {
	if len(id.Types) != 2 ||
		!strings.EqualFold(id.Types[0].Type, azresources.CustomProvidersResourceProviders) ||
		!strings.EqualFold(id.Types[1].Type, azresources.ApplicationResourceType) {
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}

// We don't really expect an invalid type to get through ARM's routing
// but we're testing it anyway to catch bugs.
func (r *rp) validateResourceType(id azresources.ResourceID) error {
	if len(id.Types) != 3 ||
		!strings.EqualFold(id.Types[0].Type, azresources.CustomProvidersResourceProviders) ||
		!strings.EqualFold(id.Types[1].Type, azresources.ApplicationResourceType) ||
		!schema.HasType(id.Types[2].Type) {
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}

// We don't really expect an invalid type to get through ARM's routing
// but we're testing it anyway to catch bugs.
func (r *rp) validateOperationType(id azresources.ResourceID) error {
	if len(id.Types) != 4 ||
		!strings.EqualFold(id.Types[0].Type, azresources.CustomProvidersResourceProviders) ||
		!strings.EqualFold(id.Types[1].Type, azresources.ApplicationResourceType) ||
		!schema.HasType(id.Types[2].Type) ||
		!strings.EqualFold(id.Types[3].Type, azresources.OperationResourceType) {
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}
