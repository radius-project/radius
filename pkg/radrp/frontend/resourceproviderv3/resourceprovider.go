// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceproviderv3

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
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/schemav3"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

var ErrUnsupportedResourceType = errors.New("unsupported resource type")

//go:generate mockgen -destination=./mock_resourceprovider.go -package=resourceproviderv3 -self_package github.com/Azure/radius/pkg/radrp/frontend/resourceproviderv3 github.com/Azure/radius/pkg/radrp/frontend/resourceproviderv3 ResourceProvider

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

func (r *rp) ListResources(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateResourceType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// GET ..../Application/{applicationName}/{resourceType}
	// GET ..../Application/{applicationName}/{resourceType}/{resourceName}

	// // ..../Application/hello/ContainerComponent
	lastType := id.Types[len(id.Types)-1].Type
	var items []db.RadiusResource
	if schemav3.IsGenericResource(lastType) {
		items, err = r.db.ListAllV3Resources(ctx, id)
	} else {
		items, err = r.db.ListV3Resources(ctx, id)
	}
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

	oid := id.Append(azresources.ResourceType{Type: resources.V3OperationResourceType, Name: uuid.New().String()})
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

	oid := id.Append(azresources.ResourceType{Type: resources.V3OperationResourceType, Name: uuid.New().String()})
	operation := db.NewOperation(oid, db.OperationKindDelete, string(rest.DeletingStatus))
	_, err = r.db.PatchOperationByID(ctx, oid, &operation)
	if err != nil {
		return nil, err
	}

	r.ProcessDeletionBackground(ctx, oid, item)

	output := NewRestRadiusResource(item)
	return rest.NewAcceptedAsyncResponse(output, oid.ID), nil
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
		!strings.EqualFold(id.Types[1].Type, resources.V3ApplicationResourceType) {
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}

// We don't really expect an invalid type to get through ARM's routing
// but we're testing it anyway to catch bugs.
func (r *rp) validateResourceType(id azresources.ResourceID) error {
	if len(id.Types) != 3 ||
		!strings.EqualFold(id.Types[0].Type, azresources.CustomProvidersResourceProviders) ||
		!strings.EqualFold(id.Types[1].Type, resources.V3ApplicationResourceType) ||
		!schemav3.HasType(id.Types[2].Type) {
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}

// We don't really expect an invalid type to get through ARM's routing
// but we're testing it anyway to catch bugs.
func (r *rp) validateOperationType(id azresources.ResourceID) error {
	if len(id.Types) != 4 ||
		!strings.EqualFold(id.Types[0].Type, azresources.CustomProvidersResourceProviders) ||
		!strings.EqualFold(id.Types[1].Type, resources.V3ApplicationResourceType) ||
		!schemav3.HasType(id.Types[2].Type) ||
		!strings.EqualFold(id.Types[3].Type, resources.V3OperationResourceType) {
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}
