// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package curp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Azure/radius/pkg/curp/armerrors"
	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/curp/db"
	"github.com/Azure/radius/pkg/curp/deployment"
	"github.com/Azure/radius/pkg/curp/metadata"
	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/curp/rest"
	"github.com/Azure/radius/pkg/curp/revision"
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
func NewResourceProvider(db db.CurpDB, deploy deployment.DeploymentProcessor) ResourceProvider {
	return &rp{
		db:     db,
		v:      validator.New(),
		deploy: deploy,
		meta:   metadata.NewRegistry(),
	}
}

type rp struct {
	db     db.CurpDB
	v      *validator.Validate
	deploy deployment.DeploymentProcessor
	meta   metadata.Registry
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

	dbitem, err := r.db.GetComponentByApplicationID(ctx, c.App, c.Resource.Name(), revision.Revision(""))
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
	olddbitem, err := r.db.GetComponentByApplicationID(ctx, id.App, id.Resource.Name(), revision.Revision(""))
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

	created, err := r.db.PatchComponentByApplicationID(ctx, id.App, id.Resource.Name(), newdbitem, previous)
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

	actions, err := r.computeDeploymentActions(app, olddbitem, newdbitem)
	if err != nil {
		// An error computing deployment actions is generally the users' fault.
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	eq := deploymentIsNoOp(actions)
	if eq && olddbitem != nil {
		// No changes to the deployment - nothing to do.
		log.Printf("%T is unchanged.", olddbitem)
		return rest.NewOKResponse(newRESTDeploymentFromDB(olddbitem)), nil
	}

	// Will update the deployment status in place - carry over existing status
	if olddbitem == nil {
		newdbitem.Status = db.DeploymentStatus{
			Services: map[string]db.DeploymentService{},
		}
	} else {
		newdbitem.Status = olddbitem.Status
		if newdbitem.Status.Services == nil {
			newdbitem.Status.Services = map[string]db.DeploymentService{}
		}
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
		ctx := context.Background()
		log.Printf("processing deployment '%s' in the background", d.ID)
		var failure *armerrors.ErrorDetails = nil
		status := rest.SuccededStatus

		err := r.deploy.UpdateDeployment(ctx, app.FriendlyName(), newdbitem.Name, &newdbitem.Status, actions)
		if _, ok := err.(*deployment.CompositeError); ok {
			log.Printf("deployment '%s' failed with error: %v", d.ID, err)
			// Composite error is what we use for validation problems
			status = rest.FailedStatus
			failure = &armerrors.ErrorDetails{
				Code:    armerrors.CodeInvalid,
				Message: err.Error(),
				Target:  id.Resource.ID,
			}
		} else if err != nil {
			log.Printf("deployment '%s' failed with error: %v", d.ID, err)
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
			log.Printf("failed to retrieve operation '%s' - marking deployment as failed: %v", oid.Resource.ID, err)
			if status == rest.SuccededStatus {
				status = rest.FailedStatus
			}
		} else {
			log.Printf("updating operation '%s'", oid.Resource.ID)
			operation.EndTime = time.Now().UTC().Format(time.RFC3339)
			operation.PercentComplete = 100
			operation.Status = string(status)
			operation.Error = failure

			_, err = r.db.PatchOperationByID(ctx, oid.Resource, operation)
			if err != nil {
				log.Printf("failed to update operation '%s' - marking deployment as failed: %v", oid.Resource.ID, err)
				if status == rest.SuccededStatus {
					status = rest.FailedStatus
				}
			}

			log.Printf("updated operation '%s' with status %s", oid.Resource.ID, status)
		}

		d, err := r.db.GetDeploymentByApplicationID(ctx, id.App, id.Resource.Name())
		if err != nil {
			log.Printf("failed to retrieve deployment '%s': %v", oid.Resource.ID, err)
			return
		}

		log.Printf("updating deployment '%s'", d.ID)
		d.Properties.ProvisioningState = string(status)
		d.Status = newdbitem.Status
		_, err = r.db.PatchDeploymentByApplicationID(ctx, id.App, id.Resource.Name(), d)
		if err != nil {
			log.Printf("failed to update deployment '%s': %v", oid.Resource.ID, err)
			return
		}

		log.Printf("completed deployment '%s' in the background with status %s", d.ID, status)
	}()

	// As a limitation of custom resource providers, we have to use HTTP 202 for this.
	body := newRESTDeploymentFromDB(newdbitem)
	return rest.NewAcceptedAsyncResponse(body, oid.Resource.ID), nil
}

func (r *rp) DeleteDeployment(ctx context.Context, id resources.ResourceID) (rest.Response, error) {
	d, err := id.Deployment()
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

	if current.Status.Services == nil {
		current.Status.Services = map[string]db.DeploymentService{}
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
		ctx := context.Background()
		log.Printf("processing deletion of deployment '%s' in the background", d.Resource.ID)
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
			log.Printf("failed to retrieve operation '%s' - marking deletion as failed: %v", oid.Resource.ID, err)
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
				log.Printf("failed to update operation '%s' - marking deployment as failed: %v", oid.Resource.ID, err)
				if status == rest.SuccededStatus {
					status = rest.FailedStatus
				}
			}
		}

		dd, err := r.db.GetDeploymentByApplicationID(ctx, d.App, d.Resource.Name())
		if err != nil {
			log.Printf("failed to retrieve deployment '%s': %v", oid.Resource.ID, err)
			return
		}

		if status == rest.SuccededStatus {
			err := r.db.DeleteDeploymentByApplicationID(ctx, d.App, d.Resource.Name())
			if err != nil {
				log.Printf("failed to delete deployment '%s': %v", oid.Resource.ID, err)
				return
			}
		} else {
			// If we get here then something about the operation failed - don't delete the
			// deployment record, mark it as failed.
			dd.Properties.ProvisioningState = string(status)
			_, err = r.db.PatchDeploymentByApplicationID(ctx, d.App, d.Resource.Name(), dd)
			if err != nil {
				log.Printf("failed to update deployment '%s': %v", oid.Resource.ID, err)
				return
			}
		}

		log.Printf("completed deployment '%s' in the background", d.Resource.ID)
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

func (r *rp) computeDeploymentActions(app *db.Application, older *db.Deployment, newer *db.Deployment) (map[string]deployment.ComponentAction, error) {
	active, err := assignRevisions(app, newer)
	if err != nil {
		return nil, err
	}

	current, err := gatherCurrentRevisions(app, older)
	if err != nil {
		return nil, err
	}

	providers, err := r.bindProviders(newer, active)
	if err != nil {
		return nil, err
	}

	serviceBindings, err := r.bindServices(newer, active, providers)
	if err != nil {
		return nil, err
	}

	// If we previously deployed this deployment but failed, make sure we retry, treat
	// each component as an upgrade if it's unchanged.
	forceUpgradeToRetry := false
	if older != nil &&
		older.Properties.ProvisioningState != "" &&
		rest.OperationStatus(older.Properties.ProvisioningState) != rest.SuccededStatus {
		forceUpgradeToRetry = true
	}

	// gather all component names
	names := map[string]bool{}
	for name := range active {
		names[name] = true
	}
	for name := range current {
		names[name] = true
	}

	actions := map[string]deployment.ComponentAction{}
	for name := range names {
		n := active[name]
		o := current[name]

		var s map[string]deployment.ServiceBinding
		ninst, ok := newer.LookupComponent(name)
		if ok {
			s = serviceBindings[name]
		}

		var oinst *db.DeploymentComponent
		if older != nil {
			oinst, _ = older.LookupComponent(name)
		}

		provides := filterProvidersByComponent(name, providers)

		wd := deployment.ComponentAction{
			ApplicationName:       app.FriendlyName(),
			ComponentName:         name,
			Operation:             deployment.None, // Assume none until we find otherwise
			Definition:            n,
			Instantiation:         ninst,
			Provides:              provides,
			ServiceBindings:       s,
			PreviousDefinition:    o,
			PreviousInstanitation: oinst,
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
		if action.Operation == deployment.CreateWorkload {
			log.Printf("component %s is added in this update", action.ComponentName)
		} else if action.Operation == deployment.DeleteWorkload {
			log.Printf("component %s is removed in this update", action.ComponentName)
		} else if action.Operation == deployment.UpdateWorkload && action.PreviousDefinition.Revision != action.Definition.Revision {
			log.Printf("component %s is upgraded %s->%s in this update", action.ComponentName, action.PreviousDefinition.Revision, action.Definition.Revision)
		} else if action.Operation == deployment.UpdateWorkload && action.PreviousDefinition.Revision == action.Definition.Revision {
			log.Printf("component %s has parameter changes in this update", action.ComponentName)
		} else {
			log.Printf("component %s is unchanged in this update", action.ComponentName)
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

// stamp the latest version of component into the deployment unless otherwise specified - also
// grab the 'active' version of each component
func assignRevisions(app *db.Application, d *db.Deployment) (map[string]*db.ComponentRevision, error) {
	active := map[string]*db.ComponentRevision{}
	for _, dc := range d.Properties.Components {
		name := dc.FriendlyName()
		component, ok := app.Components[name]
		if !ok {
			return active, fmt.Errorf("component %s does not exist", name)
		}

		if component.Revision == "" {
			return active, fmt.Errorf("component %s has no revisions", name)
		}

		if dc.Revision == "" {
			// Use the latest
			dc.Revision = component.Revision
		}

		found := false
		for _, r := range component.RevisionHistory {
			if r.Revision == dc.Revision {
				active[name] = &r
				found = true
				break
			}
		}

		if !found {
			return active, fmt.Errorf("component %s does not have a revision %s", name, dc.Revision)
		}
	}

	return active, nil
}

// gather the set of revisions from the 'current' deployment object
func gatherCurrentRevisions(app *db.Application, d *db.Deployment) (map[string]*db.ComponentRevision, error) {
	current := map[string]*db.ComponentRevision{}

	// 'current' might be null
	if d == nil {
		return current, nil
	}

	for _, dc := range d.Properties.Components {
		name := dc.FriendlyName()
		component, ok := app.Components[name]
		if !ok {
			return current, fmt.Errorf("component %s does not exist", name)
		}

		found := false
		for _, r := range component.RevisionHistory {
			if r.Revision == dc.Revision {
				current[name] = &r
				found = true
				break
			}
		}

		if !found {
			return current, fmt.Errorf("component %s does not have a revision %s", name, dc.Revision)
		}
	}

	return current, nil
}

func assignOperation(wd *deployment.ComponentAction, forceUpgradeToRetry bool) error {
	if wd.Instantiation == nil && wd.PreviousInstanitation == nil {
		return errors.New("can't figure out operation")
	} else if wd.Instantiation != nil && wd.PreviousInstanitation == nil {
		wd.Operation = deployment.CreateWorkload
		return nil
	} else if wd.Instantiation == nil && wd.PreviousInstanitation != nil {
		wd.Operation = deployment.DeleteWorkload
		return nil
	}

	// Those are all of the *easy* cases. If we get here then the workload is either being upgraded
	// or is the same - so we can safely dereference any properties.
	if wd.Definition.Revision != wd.PreviousDefinition.Revision {
		// revision does not match.
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

func (r *rp) bindProviders(d *db.Deployment, cs map[string]*db.ComponentRevision) (map[string]deployment.ServiceBinding, error) {
	// find all services provided by all components
	providers := map[string]deployment.ServiceBinding{}
	for _, dc := range d.Properties.Components {
		// We don't expect this to fail except in tests
		c, ok := cs[dc.FriendlyName()]
		if !ok {
			return nil, fmt.Errorf("cannot find matching revision for component %s", dc.FriendlyName())
		}

		// Intrinsic bindings are provided by traits and the workload types
		// they can be overridden by declaring a service with the same name on the same component
		intrinsic := map[string]deployment.ServiceBinding{}

		s, ok := r.meta.WorkloadKindServices[c.Kind]
		if ok {
			name := dc.FriendlyName()
			_, ok := intrinsic[name]
			if ok {
				return nil, fmt.Errorf("service %v has multiple providers", name)
			}

			// Found one - add to both list - it will get removed later if it's
			// been rebound
			intrinsic[dc.FriendlyName()] = deployment.ServiceBinding{
				Name:     name,
				Kind:     s.Kind,
				Provider: dc.FriendlyName(),
			}

			// TODO: we currently allow a service from one component to 'hide' a service from another
			_, ok = providers[name]
			if !ok {
				providers[name] = deployment.ServiceBinding{
					Name:     name,
					Kind:     s.Kind,
					Provider: dc.FriendlyName(),
				}
			}
		}

		for _, t := range c.Properties.Traits {
			s, ok := r.meta.TraitServices[t.Kind]
			if ok {
				name := dc.FriendlyName()
				_, ok := intrinsic[name]
				if ok {
					return nil, fmt.Errorf("service %v has multiple providers", name)
				}

				// Found one - add to both list - it will get removed later if it's
				// been rebound
				intrinsic[name] = deployment.ServiceBinding{
					Name:     name,
					Kind:     s.Kind,
					Provider: dc.FriendlyName(),
				}

				_, ok = providers[name]
				if !ok {
					providers[name] = deployment.ServiceBinding{
						Name:     name,
						Kind:     s.Kind,
						Provider: dc.FriendlyName(),
					}
				}
			}
		}

		for _, s := range c.Properties.Provides {
			other, ok := providers[s.Name]
			if ok {
				// If this is in the intrinsic list for the same component, then
				// this is an override and it's allowed.
				_, ok := intrinsic[s.Name]
				if !ok || other.Provider != dc.FriendlyName() {
					return nil, fmt.Errorf("service %v has multiple providers", s.Name)
				}
			}

			providers[s.Name] = deployment.ServiceBinding{
				Name:     s.Name,
				Kind:     s.Kind,
				Provider: dc.FriendlyName(),
			}
		}
	}

	return providers, nil
}

func (r *rp) bindServices(d *db.Deployment, cs map[string]*db.ComponentRevision, providers map[string]deployment.ServiceBinding) (map[string]map[string]deployment.ServiceBinding, error) {
	// find the relationship between services declared and the components that match
	bindings := map[string]map[string]deployment.ServiceBinding{}

	// Now loop through all of the consumers and match them up
	for _, dc := range d.Properties.Components {
		// We don't expect this to fail except in tests
		c, ok := cs[dc.FriendlyName()]
		if !ok {
			return nil, fmt.Errorf("cannot find matching revision for component %s", dc.FriendlyName())
		}

		b := map[string]deployment.ServiceBinding{}
		for _, s := range c.Properties.DependsOn {
			p, ok := providers[s.Name]
			if !ok {
				return nil, fmt.Errorf("service %v has no provider", s.Name)
			}

			if s.Kind != p.Kind {
				return nil, fmt.Errorf("service %v is used with kind %v but was defined with kind %v", s.Name, s.Kind, p.Kind)
			}

			b[s.Name] = p
		}

		bindings[dc.FriendlyName()] = b
	}

	return bindings, nil
}

func filterProvidersByComponent(componentName string, providers map[string]deployment.ServiceBinding) map[string]deployment.ComponentService {
	results := map[string]deployment.ComponentService{}
	for _, sb := range providers {
		if sb.Provider == componentName {
			results[sb.Name] = deployment.ComponentService{
				Name:     sb.Name,
				Kind:     sb.Kind,
				Provider: componentName,
			}
		}
	}
	return results
}

// Convert our datatabase representation to the "baked" version of the component
func convertToComponent(name string, defn db.ComponentRevision, traits []db.ComponentTrait) (*components.GenericComponent, error) {
	raw := map[string]interface{}{
		"name":      name,
		"kind":      defn.Kind,
		"config":    defn.Properties.Config,
		"run":       defn.Properties.Run,
		"dependsOn": defn.Properties.DependsOn,
		"provides":  defn.Properties.Provides,
		"traits":    traits,
	}

	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal component: %w", err)
	}

	component := components.GenericComponent{}
	err = json.Unmarshal(bytes, &component)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal component: %w", err)
	}

	return &component, nil
}
