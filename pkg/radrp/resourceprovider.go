// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radrp

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/deployment"
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/revision"
	"github.com/go-logr/logr"
	"github.com/go-playground/validator/v10"
)

// ResourceProvider defines the business logic of the resource provider for Radius.
type ResourceProvider interface {
	ListApplications(ctx context.Context, id resources.ResourceID) (rest.Response, error)
	GetApplication(ctx context.Context, id resources.ResourceID) (rest.Response, error)
	UpdateApplication(ctx context.Context, app *rest.Application) (rest.Response, error)
	DeleteApplication(ctx context.Context, id resources.ResourceID) (rest.Response, error)

	ListComponents(ctx context.Context, id resources.ResourceID) (rest.Response, error)
	GetComponent(ctx context.Context, id resources.ResourceID) (rest.Response, error)
	UpdateComponent(ctx context.Context, app *rest.Component) (rest.Response, error)
	DeleteComponent(ctx context.Context, id resources.ResourceID) (rest.Response, error)

	ListDeployments(ctx context.Context, id resources.ResourceID) (rest.Response, error)
	GetDeployment(ctx context.Context, id resources.ResourceID) (rest.Response, error)
	UpdateDeployment(ctx context.Context, app *rest.Deployment) (rest.Response, error)
	DeleteDeployment(ctx context.Context, id resources.ResourceID) (rest.Response, error)

	ListScopes(ctx context.Context, id resources.ResourceID) (rest.Response, error)
	GetScope(ctx context.Context, id resources.ResourceID) (rest.Response, error)
	UpdateScope(ctx context.Context, app *rest.Scope) (rest.Response, error)
	DeleteScope(ctx context.Context, id resources.ResourceID) (rest.Response, error)

	GetDeploymentOperationByID(ctx context.Context, id resources.ResourceID) (rest.Response, error)
}

// NewResourceProvider creates a new ResourceProvider.
func NewResourceProvider(db db.RadrpDB, deploy deployment.DeploymentProcessor) ResourceProvider {
	return &rp{
		db:     db,
		v:      validator.New(),
		deploy: deploy,
	}
}

type rp struct {
	db     db.RadrpDB
	v      *validator.Validate
	deploy deployment.DeploymentProcessor
}

func (r *rp) ListApplications(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	err := id.ValidateResourceType(resources.ApplicationCollectionType)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	dbitems, err := r.db.ListApplicationsByResourceGroup(ctx, id)
	if err != nil {
		return nil, nil
	}

	items := make([]interface{}, 0, len(dbitems))
	for _, dbitem := range dbitems {
		items = append(items, *newRESTApplicationFromDB(&dbitem))
	}

	list := &rest.ResourceList{Value: items}

	return rest.NewOKResponse(list), nil
}

func (r *rp) GetApplication(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	a, err := id.Application()
	if err != nil {
		return nil, err
	}

	dbitem, err := r.db.GetApplicationByID(ctx, a)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	item := newRESTApplicationFromDB(dbitem)
	return rest.NewOKResponse(item), nil
}

func (r *rp) UpdateApplication(ctx context.Context, a *rest.Application) (rest.Response, error) {
	ctx = radlogger.WrapLogContext(ctx,
		radlogger.LogFieldAppName, a.Name,
		radlogger.LogFieldAppID, a.ID,
	)
	_, err := a.GetApplicationID()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	response, err := r.validate(a)
	if err != nil {
		return nil, err
	} else if response != nil {
		return response, nil
	}

	dbitem := newDBApplicationPatchFromREST(a)
	created, err := r.db.PatchApplication(ctx, dbitem)
	if err != nil {
		return nil, err
	}

	body := newRESTApplicationFromDBPatch(dbitem)
	if created {
		return rest.NewCreatedResponse(body), nil
	}

	return rest.NewOKResponse(body), nil
}

func (r *rp) DeleteApplication(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	a, err := id.Application()
	if err != nil {
		return nil, err
	}

	app, err := r.db.GetApplicationByID(ctx, a)
	if err == db.ErrNotFound {
		// it's not an error to 'delete' something that's already gone
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	if len(app.Deployments) > 0 {
		return rest.NewConflictResponse(fmt.Sprintf("the application '%v' has existing deployments", id)), nil
	}

	err = r.db.DeleteApplicationByID(ctx, a)
	if err != nil {
		return nil, err
	}

	return rest.NewNoContentResponse(), nil
}

func (r *rp) ListComponents(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	err := id.ValidateResourceType(resources.ComponentCollectionType)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	app, err := id.Application()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	dbitems, err := r.db.ListComponentsByApplicationID(ctx, app)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(app.ResourceID), nil
	} else if err != nil {
		return nil, err
	}

	items := make([]interface{}, 0, len(dbitems))
	for _, dbitem := range dbitems {
		items = append(items, *newRESTComponentFromDB(&dbitem))
	}

	list := &rest.ResourceList{Value: items}
	return rest.NewOKResponse(list), nil
}

func (r *rp) GetComponent(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	c, err := id.Component()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	dbitem, err := r.db.GetComponentByApplicationID(ctx, c.App, c.Resource.Name())
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	item := newRESTComponentFromDB(dbitem)
	return rest.NewOKResponse(item), nil
}

func (r *rp) UpdateComponent(ctx context.Context, c *rest.Component) (rest.Response, error) {
	id, err := c.GetComponentID()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// TODO - nothing here validates that the component is "known" type. We let the user declare any
	// component types and versions they want.

	response, err := r.validate(c)
	if err != nil {
		return nil, err
	} else if response != nil {
		return response, nil
	}

	// fetch the latest component so we can compare and generate a revision
	olddbitem, err := r.db.GetComponentByApplicationID(ctx, id.App, id.Resource.Name())
	if err == db.ErrNotFound {
		// this is fine - we don't have a previous version to compare against
	} else if err != nil {
		return nil, err
	}

	newdbitem := newDBComponentFromREST(c)

	equal := false
	if olddbitem != nil {
		equal, err = revision.Equals(olddbitem, newdbitem)
		if err != nil {
			return nil, err
		}
	}

	if equal {
		// No changes to the component - nothing to do.
		return rest.NewOKResponse(newRESTComponentFromDB(olddbitem)), nil
	}

	// Component has changes, update everything.
	previous := revision.Revision("")
	if olddbitem != nil {
		previous = olddbitem.Revision
	}
	newdbitem.Revision, err = revision.Compute(newdbitem, previous, []revision.Revision{})
	if err != nil {
		return nil, err
	}

	created, err := r.db.PatchComponentByApplicationID(ctx, id.App, id.Resource.Name(), newdbitem)
	if err == db.ErrNotFound {
		// If we get a not found here there's no application
		return rest.NewNotFoundResponse(id.App.ResourceID), nil
	} else if err != nil {
		return nil, err
	}
	body := newRESTComponentFromDB(newdbitem)
	if created {
		return rest.NewCreatedResponse(body), nil
	}

	return rest.NewOKResponse(body), nil
}

func (r *rp) DeleteComponent(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	c, err := id.Component()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	err = r.db.DeleteComponentByApplicationID(ctx, c.App, c.Resource.Name())
	if err == db.ErrNotFound {
		// it's not an error to 'delete' something that's already gone
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	return rest.NewNoContentResponse(), nil
}

func (r *rp) ListDeployments(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	err := id.ValidateResourceType(resources.DeploymentCollectionType)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	app, err := id.Application()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	dbitems, err := r.db.ListDeploymentsByApplicationID(ctx, app)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(app.ResourceID), nil
	} else if err != nil {
		return nil, err
	}

	items := make([]interface{}, 0, len(dbitems))
	for _, dbitem := range dbitems {
		items = append(items, *newRESTDeploymentFromDB(&dbitem))
	}

	list := &rest.ResourceList{Value: items}
	return rest.NewOKResponse(list), nil
}

func (r *rp) GetDeployment(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	d, err := id.Deployment()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	dbitem, err := r.db.GetDeploymentByApplicationID(ctx, d.App, d.Resource.Name())
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	item := newRESTDeploymentFromDB(dbitem)
	return rest.NewOKResponse(item), nil
}

func (r *rp) UpdateDeployment(ctx context.Context, d *rest.Deployment) (rest.Response, error) {
	id, err := d.GetDeploymentID()
	ctx = radlogger.WrapLogContext(ctx,
		radlogger.LogFieldDeploymentName, d.Name,
		radlogger.LogFieldAppName, id.App.Name(),
		radlogger.LogFieldAppID, id.App.ID,
		radlogger.LogFieldDeploymentID, d.ID)
	logger := radlogger.GetLogger(ctx)

	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	response, err := r.validate(d)
	if err != nil {
		return nil, err
	} else if response != nil {
		return response, nil
	}

	// Start gathering all of the info we need to compose an update to the deployment
	//
	// That includes:
	// - the new deployment
	// - the old deployment (maybe null)
	// - all of the component revisions referenced by the new deployment
	newdbitem := newDBDeploymentFromREST(d)
	app, err := r.db.GetApplicationByID(ctx, id.App)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id.App.ResourceID), nil
	} else if err != nil {
		return nil, err
	}

	var olddbitem *db.Deployment
	obj, ok := app.Deployments[id.Resource.Name()]
	if ok {
		olddbitem = &obj
	}

	// TODO: support for cancellation of a deployment in flight. We don't have a good way now to
	// cancel a deployment that's in progress. If you deploy twice at once the results are not determinisitic.

	actions, err := r.computeDeploymentActions(ctx, app, olddbitem, newdbitem)
	if err != nil {
		// An error computing deployment actions is generally the users' fault.
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	eq := deploymentIsNoOp(actions)
	if eq && olddbitem != nil {
		// No changes to the deployment - nothing to do.
		logger.Info("Deployment is unchanged.")
		return rest.NewOKResponse(newRESTDeploymentFromDB(olddbitem)), nil
	}

	// Will update the deployment status in place - carry over existing status
	if olddbitem == nil {
		newdbitem.Status = db.DeploymentStatus{}
	} else {
		newdbitem.Status = olddbitem.Status
	}

	// Now that we've computed the set of deployment actions we're ready to start the deployment
	// asynchronously. We'll return an HTTP 201/202 status and link a to URL that can be used to
	// track the status.
	//
	// We need to track the value of the provisioning state as part of the deployment for any
	// asynchronous operations.
	//
	// When we start the operation we set the status to Deploying, and the background job
	// will update it.

	// First let's create the "operation" that tracks completion
	oid := id.NewOperation()
	operation := &db.Operation{
		ID:     oid.Resource.ID,
		Name:   oid.Resource.Name(),
		Status: string(rest.DeployingStatus),

		StartTime:       time.Now().UTC().Format(time.RFC3339),
		PercentComplete: 0,
	}

	logger = logger.WithValues(
		radlogger.LogFieldResourceID, oid.Resource.ID,
		radlogger.LogFieldResourceName, oid.Resource.Name())

	_, err = r.db.PatchOperationByID(ctx, oid.Resource, operation)
	if err != nil {
		return nil, err
	}

	newdbitem.Properties.ProvisioningState = string(rest.DeployingStatus)
	_, err = r.db.PatchDeploymentByApplicationID(ctx, id.App, id.Resource.Name(), newdbitem)
	if err != nil {
		return nil, err
	}

	// OK we've updated the database to denote that the deployment is in process - now we're ready
	// to start deploying in the background.
	go func() {
		ctx := logr.NewContext(context.Background(), logger)
		logger.Info("processing deployment in the background")
		var failure *armerrors.ErrorDetails = nil
		status := rest.SuccededStatus

		err := r.deploy.UpdateDeployment(ctx, app.FriendlyName(), newdbitem.Name, &newdbitem.Status, actions)
		if _, ok := err.(*deployment.CompositeError); ok {
			logger.WithValues(radlogger.LogFieldErrors, err).Info("deployment failed")
			// Composite error is what we use for validation problems
			status = rest.FailedStatus
			failure = &armerrors.ErrorDetails{
				Code:    armerrors.CodeInvalid,
				Message: err.Error(),
				Target:  id.Resource.ID,
			}
		} else if err != nil {
			logger.WithValues(radlogger.LogFieldErrors, err).Info("deployment failed")
			// Other errors represent a generic failure, this should map to a 500.
			status = rest.FailedStatus
			failure = &armerrors.ErrorDetails{
				Code:    armerrors.CodeInternal,
				Message: err.Error(),
				Target:  id.Resource.ID,
			}
		}

		// If we get here the deployment is complete (possibly failed)
		operation, err := r.db.GetOperationByID(ctx, oid.Resource)
		if err != nil {
			// If we get here we're not going to be able to update the operation
			// try to update the deployment as a cleanup step (if possible).
			logger.WithValues(radlogger.LogFieldErrors, err).
				Info("failed to retrieve operation. marking deployment as failed")
			if status == rest.SuccededStatus {
				status = rest.FailedStatus
			}
		} else {
			logger.Info("updating operation")
			operation.EndTime = time.Now().UTC().Format(time.RFC3339)
			operation.PercentComplete = 100
			operation.Status = string(status)
			operation.Error = failure

			_, err = r.db.PatchOperationByID(ctx, oid.Resource, operation)
			if err != nil {
				logger.WithValues(radlogger.LogFieldErrors, err).
					Info("failed to update operation. marking deployment as failed")
				if status == rest.SuccededStatus {
					status = rest.FailedStatus
				}
			}

			logger.WithValues(radlogger.LogFieldOperationStatus, status).Info("updated operation")
		}

		d, err := r.db.GetDeploymentByApplicationID(ctx, id.App, id.Resource.Name())
		if err != nil {
			logger.WithValues(radlogger.LogFieldErrors, err).Info("failed to retrieve operation")
			return
		}
		d.Properties.ProvisioningState = string(status)
		d.Status = newdbitem.Status
		a, err := r.db.GetApplicationByID(ctx, id.App)
		if err != nil {
			logger.WithValues(radlogger.LogFieldErrors, err).Info("failed to retrieve application")
			return
		}
		// Update the deployment in the application
		logger.Info("Updating deployment")
		a.Deployments[id.Resource.Name()] = *d

		// Update components to track output resources created during deployment
		for c, action := range actions {
			logger.Info(fmt.Sprintf("Updating component with %v output resources", len(action.Definition.Properties.OutputResources)))
			a.Components[c] = *action.Definition
		}

		logger.Info("Updating application")
		ok, err := r.db.UpdateApplication(ctx, a)
		if err != nil || !ok {
			logger.WithValues(radlogger.LogFieldErrors, err).Info("failed to update application")
			return
		}
		logger.WithValues(radlogger.LogFieldOperationStatus, status).Info("completed deployment in the background with status")
	}()

	// As a limitation of custom resource providers, we have to use HTTP 202 for this.
	body := newRESTDeploymentFromDB(newdbitem)
	return rest.NewAcceptedAsyncResponse(body, oid.Resource.ID), nil
}

func (r *rp) DeleteDeployment(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	d, err := id.Deployment()
	ctx = radlogger.WrapLogContext(ctx,
		radlogger.LogFieldAppID, d.App.ID,
		radlogger.LogFieldAppName, d.App.Name(),
		radlogger.LogFieldResourceID, d.Resource.ID,
		radlogger.LogFieldResourceName, d.Resource.Name())
	logger := radlogger.GetLogger(ctx)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	current, err := r.db.GetDeploymentByApplicationID(ctx, d.App, d.Resource.Name())
	if err == db.ErrNotFound {
		// it's not an error to 'delete' something that's already gone
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	// We'll do the actual deletion in the background asynchronously
	//
	// First we need to create an operation to track the overall deletion.
	oid := d.NewOperation()
	operation := &db.Operation{
		ID:     oid.Resource.ID,
		Name:   oid.Resource.Name(),
		Status: string(rest.DeletingStatus),

		StartTime:       time.Now().UTC().Format(time.RFC3339),
		PercentComplete: 0,
	}
	_, err = r.db.PatchOperationByID(ctx, oid.Resource, operation)
	if err != nil {
		return nil, err
	}

	// Next we update the deployment to say that it's deleting.
	current.Properties.ProvisioningState = string(rest.DeletingStatus)
	_, err = r.db.PatchDeploymentByApplicationID(ctx, d.App, d.Resource.Name(), current)
	if err != nil {
		return nil, err
	}

	// OK we've updated the database to denote that the deployment is in process - now we're ready
	// to start deploying in the background.
	go func() {
		ctx := radlogger.WrapLogContext(context.Background(),
			radlogger.LogFieldAppName, d.App.Name(),
			radlogger.LogFieldDeploymentName, d.Resource.Name())
		logger := radlogger.GetLogger(ctx)
		logger.Info("processing deletion of deployment in the background")
		var failure *armerrors.ErrorDetails = nil
		status := rest.SuccededStatus
		err := r.deploy.DeleteDeployment(ctx, d.App.Name(), d.Resource.Name(), &current.Status)
		if _, ok := err.(*deployment.CompositeError); ok {
			// Composite error is what we use for validation problems
			status = rest.FailedStatus
			failure = &armerrors.ErrorDetails{
				Code:    armerrors.CodeInvalid,
				Message: err.Error(),
				Target:  d.Resource.ID,
			}
		} else if err != nil {
			status = rest.FailedStatus
			failure = &armerrors.ErrorDetails{
				Code:    armerrors.CodeInternal,
				Message: err.Error(),
				Target:  d.Resource.ID,
			}
		}

		// If we get here the deployment is complete (possibly failed)
		operation, err := r.db.GetOperationByID(ctx, oid.Resource)
		if err != nil {
			// If we get here we're not going to be able to update the operation
			// try to update the deployment as a cleanup step (if possible).
			logger.WithValues(radlogger.LogFieldErrors, err).Info("failed to retrieve operation. marking deletion as failed")
			if status == rest.SuccededStatus {
				status = rest.FailedStatus
			}
		} else {
			operation.EndTime = time.Now().UTC().Format(time.RFC3339)
			operation.PercentComplete = 100
			operation.Status = string(status)
			operation.Error = failure

			_, err = r.db.PatchOperationByID(ctx, oid.Resource, operation)
			if err != nil {
				logger.WithValues(radlogger.LogFieldErrors, err).Info("failed to update operation. marking deployment as failed")
				if status == rest.SuccededStatus {
					status = rest.FailedStatus
				}
			}
		}

		dd, err := r.db.GetDeploymentByApplicationID(ctx, d.App, d.Resource.Name())
		if err != nil {
			logger.WithValues(radlogger.LogFieldErrors, err).Info("failed to retrieve deployment")
			return
		}

		if status == rest.SuccededStatus {
			err := r.db.DeleteDeploymentByApplicationID(ctx, d.App, d.Resource.Name())
			if err != nil {
				logger.WithValues(radlogger.LogFieldErrors, err).Info("failed to delete deployment")
				return
			}
		} else {
			// If we get here then something about the operation failed - don't delete the
			// deployment record, mark it as failed.
			dd.Properties.ProvisioningState = string(status)
			_, err = r.db.PatchDeploymentByApplicationID(ctx, d.App, d.Resource.Name(), dd)
			if err != nil {
				logger.WithValues(radlogger.LogFieldErrors, err).Info("failed to update deployment")
				return
			}
		}

		logger.Info("completed deployment in the background")
	}()

	return rest.NewAcceptedAsyncResponse(newRESTDeploymentFromDB(current), operation.ID), nil
}

func (r *rp) ListScopes(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	err := id.ValidateResourceType(resources.ScopeCollectionType)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	app, err := id.Application()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	dbitems, err := r.db.ListScopesByApplicationID(ctx, app)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(app.ResourceID), nil
	} else if err != nil {
		return nil, err
	}

	items := make([]interface{}, 0, len(dbitems))
	for _, dbitem := range dbitems {
		items = append(items, *newRESTScopeFromDB(&dbitem))
	}

	list := &rest.ResourceList{Value: items}
	return rest.NewOKResponse(list), nil
}

func (r *rp) GetScope(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	s, err := id.Scope()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	dbitem, err := r.db.GetScopeByApplicationID(ctx, s.App, s.Resource.Name())
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	item := newRESTScopeFromDB(dbitem)
	return rest.NewOKResponse(item), nil
}

func (r *rp) UpdateScope(ctx context.Context, s *rest.Scope) (rest.Response, error) {
	id, err := s.GetScopeID()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	response, err := r.validate(s)
	if err != nil {
		return nil, err
	} else if response != nil {
		return response, nil
	}

	dbitem := newDBScopeFromREST(s)
	created, err := r.db.PatchScopeByApplicationID(ctx, id.App, id.Resource.Name(), dbitem)
	if err == db.ErrNotFound {
		return rest.NewNotFoundResponse(id.App.ResourceID), nil
	} else if err != nil {
		return nil, err
	}

	body := newRESTScopeFromDB(dbitem)
	if created {
		return rest.NewCreatedResponse(body), nil
	}

	return rest.NewOKResponse(body), nil
}

func (r *rp) DeleteScope(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	s, err := id.Scope()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	err = r.db.DeleteScopeByApplicationID(ctx, s.App, s.Resource.Name())
	if err == db.ErrNotFound {
		// It's not an error for the application to be missing here.
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	return rest.NewNoContentResponse(), nil
}

// The contract of this function is a little wierd. We need to return the deployment resource.
func (r *rp) GetDeploymentOperationByID(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	oid, err := id.DeploymentOperation()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	did, err := oid.Deployment()
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	operation, err := r.db.GetOperationByID(ctx, oid.Resource)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// Handle the cases where the change to the deployment resource triggered an asynchronous failure.
	//
	// The resource body just has the provisioning status, and doesn't have the ability to give a reason
	// for failure. We use the operation for that. If there's a failure, return it in the ARM format,
	// otherwise we just want to return the same thing the deployment resource would return.
	if operation.Error != nil && operation.Error.Code == armerrors.CodeInvalid {
		// Operation failed with a validation or business logic error
		return rest.NewBadRequestARMResponse(armerrors.ErrorResponse{
			Error: *operation.Error,
		}), nil
	} else if operation.Error != nil {
		// Operation failed with an uncategorized error
		return rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
			Error: *operation.Error,
		}), nil
	}

	deployment, err := r.db.GetDeploymentByApplicationID(ctx, did.App, did.Resource.Name())
	if err == db.ErrNotFound {
		// If we get a 404 then this should mean that the resource was deleted successfully.
		// Return a 204 for that case
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	if rest.IsTeminalStatus(rest.OperationStatus(deployment.Properties.ProvisioningState)) {
		// Operation is complete
		return rest.NewOKResponse(newRESTDeploymentFromDB(deployment)), nil
	}

	// The ARM-RPC spec wants us to keep returning 202 from here until the operation is complete.
	return rest.NewAcceptedAsyncResponse(newRESTDeploymentFromDB(deployment), id.ID), nil
}

func (r *rp) validate(obj interface{}) (rest.Response, error) {
	err := r.v.Struct(obj)
	if val, ok := err.(validator.ValidationErrors); ok {
		return rest.NewValidationErrorResponse(val), nil
	}

	return nil, err
}

func (r *rp) computeDeploymentActions(ctx context.Context, app *db.Application, older *db.Deployment, newer *db.Deployment) (map[string]deployment.ComponentAction, error) {
	logger := radlogger.GetLogger(ctx)
	// This will stamp the deployment object with the identity of all of the 'latest' revisions
	// of the components.
	active, err := newer.AssignRevisions(app)
	if err != nil {
		return nil, err
	}

	// Next we gather the currently deployed revisions so that we can determine the operation
	// for each component
	var current map[string]revision.Revision
	if older == nil {
		current = map[string]revision.Revision{}
	} else {
		current = older.GetRevisions()
	}

	// If we previously deployed this deployment but failed, make sure we retry, treat
	// each component as an upgrade if it's unchanged.
	forceUpgradeToRetry := false
	if older != nil &&
		older.Properties.ProvisioningState != "" &&
		rest.OperationStatus(older.Properties.ProvisioningState) != rest.SuccededStatus {
		forceUpgradeToRetry = true
	}

	// gather all components that have an operation (union of component names from old and new set)
	names := map[string]bool{}
	for name := range active {
		names[name] = true
	}
	for name := range current {
		names[name] = true
	}

	actions := map[string]deployment.ComponentAction{}
	for name := range names {
		newRevision, isActive := active[name]
		oldRevision := current[name]

		wd := deployment.ComponentAction{
			ApplicationName: app.FriendlyName(),
			ComponentName:   name,
			Operation:       deployment.None, // Assume none until we find otherwise
			NewRevision:     newRevision,
			OldRevision:     oldRevision,
		}

		if isActive {
			// For a component in the active set (being deployed or staying deployed)
			// Then we have access to more info
			definition := app.Components[name]
			wd.Definition = &definition
		}

		err = assignOperation(&wd, forceUpgradeToRetry)
		if err != nil {
			return nil, err
		}

		if wd.Operation != deployment.DeleteWorkload {
			wd.Component, err = convertToComponent(wd.ComponentName, *wd.Definition, wd.Definition.Properties.Traits)
			if err != nil {
				return nil, err
			}
		}

		actions[name] = wd
	}

	for _, action := range actions {
		logger := logger.WithValues(radlogger.LogFieldComponentName, action.ComponentName)
		if action.Operation == deployment.CreateWorkload {
			logger.Info("component is added in this update")
		} else if action.Operation == deployment.DeleteWorkload {
			logger.Info("component is removed in this update")
		} else if action.Operation == deployment.UpdateWorkload && action.NewRevision != action.OldRevision {
			logger.Info(fmt.Sprintf("component is upgraded %s->%s in this update", action.OldRevision, action.NewRevision))
		} else if action.Operation == deployment.UpdateWorkload && action.NewRevision == action.Definition.Revision {
			logger.Info("component is being upgraded without a definition change")
		} else {
			logger.Info("component is unchanged in this update")
		}
	}

	return actions, nil
}

func deploymentIsNoOp(actions map[string]deployment.ComponentAction) bool {
	for _, action := range actions {
		if action.Operation != deployment.None {
			return false
		}
	}

	return true
}

func assignOperation(wd *deployment.ComponentAction, forceUpgradeToRetry bool) error {
	if wd.OldRevision == "" && wd.NewRevision != "" {
		wd.Operation = deployment.CreateWorkload
		return nil
	} else if wd.OldRevision != "" && wd.NewRevision == "" {
		wd.Operation = deployment.DeleteWorkload
		return nil
	}

	// If we get here then the workload is either being upgraded or is the same.
	if wd.OldRevision != wd.NewRevision {
		// revision does not match, this is an upgrade.
		wd.Operation = deployment.UpdateWorkload
		return nil
	}

	// If the last deployment failed, then treat every unchanged component like an
	// upgrade so that it's applied again.
	if forceUpgradeToRetry {
		wd.Operation = deployment.UpdateWorkload
		return nil
	}

	return nil
}

// Convert our datatabase representation to the "baked" version of the component
func convertToComponent(name string, defn db.Component, traits []db.ComponentTrait) (*components.GenericComponent, error) {
	component := components.GenericComponent{
		Name:     name,
		Kind:     defn.Kind,
		Config:   defn.Properties.Config,
		Run:      defn.Properties.Run,
		Uses:     []components.GenericDependency{},
		Bindings: map[string]components.GenericBinding{},
		Traits:   []components.GenericTrait{},
	}

	for _, dependency := range defn.Properties.Uses {
		converted := components.GenericDependency{
			Binding: dependency.Binding,
			Env:     dependency.Env,
		}

		if dependency.Secrets != nil {
			converted.Secrets = &components.GenericDependencySecrets{
				Store: dependency.Secrets.Store,
				Keys:  dependency.Secrets.Keys,
			}
		}

		component.Uses = append(component.Uses, converted)
	}

	for n, binding := range defn.Properties.Bindings {
		converted := components.GenericBinding{
			Kind:                 binding.Kind,
			AdditionalProperties: binding.AdditionalProperties,
		}

		component.Bindings[n] = converted
	}

	for _, trait := range defn.Properties.Traits {
		converted := components.GenericTrait{
			Kind:                 trait.Kind,
			AdditionalProperties: trait.AdditionalProperties,
		}

		component.Traits = append(component.Traits, converted)
	}

	return &component, nil
}
