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

	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/curp/db"
	"github.com/Azure/radius/pkg/curp/metadata"
	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/curp/rest"
	"github.com/Azure/radius/pkg/curp/revision"
	"github.com/go-playground/validator/v10"
)

// ResourceProvider defines the business logic of the resource provider for Radius.
type ResourceProvider interface {
	ListApplications(ctx context.Context, id resources.ResourceID) (*rest.ResourceList, error)
	GetApplication(ctx context.Context, id resources.ResourceID) (*rest.Application, error)
	UpdateApplication(ctx context.Context, app *rest.Application) (*rest.Application, error)
	DeleteApplication(ctx context.Context, id resources.ResourceID) error

	ListComponents(ctx context.Context, id resources.ResourceID) (*rest.ResourceList, error)
	GetComponent(ctx context.Context, id resources.ResourceID) (*rest.Component, error)
	UpdateComponent(ctx context.Context, app *rest.Component) (*rest.Component, error)
	DeleteComponent(ctx context.Context, id resources.ResourceID) error

	ListDeployments(ctx context.Context, id resources.ResourceID) (*rest.ResourceList, error)
	GetDeployment(ctx context.Context, id resources.ResourceID) (*rest.Deployment, error)
	UpdateDeployment(ctx context.Context, app *rest.Deployment) (*rest.Deployment, error)
	DeleteDeployment(ctx context.Context, id resources.ResourceID) error

	ListScopes(ctx context.Context, id resources.ResourceID) (*rest.ResourceList, error)
	GetScope(ctx context.Context, id resources.ResourceID) (*rest.Scope, error)
	UpdateScope(ctx context.Context, app *rest.Scope) (*rest.Scope, error)
	DeleteScope(ctx context.Context, id resources.ResourceID) error
}

// NewResourceProvider creates a new ResourceProvider.
func NewResourceProvider(db db.CurpDB, deploy DeploymentProcessor) ResourceProvider {
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
	deploy DeploymentProcessor
	meta   metadata.Registry
}

func (r *rp) ListApplications(ctx context.Context, id resources.ResourceID) (*rest.ResourceList, error) {
	err := id.ValidateResourceType(resources.ApplicationCollectionType)
	if err != nil {
		return nil, &BadRequestError{err.Error()}
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
	return list, nil
}

func (r *rp) GetApplication(ctx context.Context, id resources.ResourceID) (*rest.Application, error) {
	a, err := id.Application()
	if err != nil {
		return nil, err
	}

	dbitem, err := r.db.GetApplicationByID(ctx, a)
	if err == db.ErrNotFound {
		return nil, NotFoundError{ID: id}
	} else if err != nil {
		return nil, err
	}

	item := newRESTApplicationFromDB(dbitem)
	return item, nil
}

func (r *rp) UpdateApplication(ctx context.Context, app *rest.Application) (*rest.Application, error) {
	_, err := app.GetApplicationID()
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	err = r.validate(app)
	if err != nil {
		return nil, err
	}

	dbitem := newDBApplicationPatchFromREST(app)
	err = r.db.PatchApplication(ctx, dbitem)
	if err != nil {
		return nil, err
	}

	app = newRESTApplicationFromDBPatch(dbitem)
	return app, nil
}

func (r *rp) DeleteApplication(ctx context.Context, id resources.ResourceID) error {
	a, err := id.Application()
	if err != nil {
		return err
	}

	app, err := r.db.GetApplicationByID(ctx, a)
	if err == db.ErrNotFound {
		// it's not an error to 'delete' something that's already gone
		return nil
	} else if err != nil {
		return err
	}

	if len(app.Deployments) > 0 {
		return ConflictError{
			fmt.Sprintf("the application '%v' has existing deployments", id),
		}
	}

	err = r.db.DeleteApplicationByID(ctx, a)
	if err != nil {
		return err
	}

	return nil
}

func (r *rp) ListComponents(ctx context.Context, id resources.ResourceID) (*rest.ResourceList, error) {
	err := id.ValidateResourceType(resources.ComponentCollectionType)
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}
	app, err := id.Application()
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	dbitems, err := r.db.ListComponentsByApplicationID(ctx, app)
	if err == db.ErrNotFound {
		return nil, &NotFoundError{id}
	} else if err != nil {
		return nil, err
	}

	items := make([]interface{}, 0, len(dbitems))
	for _, dbitem := range dbitems {
		items = append(items, *newRESTComponentFromDB(&dbitem))
	}

	list := &rest.ResourceList{Value: items}
	return list, nil
}

func (r *rp) GetComponent(ctx context.Context, id resources.ResourceID) (*rest.Component, error) {
	c, err := id.Component()
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	dbitem, err := r.db.GetComponentByApplicationID(ctx, c.App, c.Resource.ShortName(), revision.Revision(""))
	if err == db.ErrNotFound {
		return nil, &NotFoundError{ID: id}
	} else if err != nil {
		return nil, err
	}

	item := newRESTComponentFromDB(dbitem)
	return item, nil
}

func (r *rp) UpdateComponent(ctx context.Context, c *rest.Component) (*rest.Component, error) {
	id, err := c.GetComponentID()
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	// TODO - nothing here validates that the component is "known" type. We let the user declare any
	// component types and versions they want.

	err = r.validate(c)
	if err != nil {
		return nil, err
	}

	// fetch the latest component so we can compare and generate a revision
	olddbitem, err := r.db.GetComponentByApplicationID(ctx, id.App, id.Resource.ShortName(), revision.Revision(""))
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
		return newRESTComponentFromDB(olddbitem), nil
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

	err = r.db.PatchComponentByApplicationID(ctx, id.App, id.Resource.ShortName(), newdbitem, previous)
	if err != nil {
		return nil, err
	}

	return newRESTComponentFromDB(newdbitem), nil
}

func (r *rp) DeleteComponent(ctx context.Context, id resources.ResourceID) error {
	c, err := id.Component()
	if err != nil {
		return &BadRequestError{err.Error()}
	}

	err = r.db.DeleteComponentByApplicationID(ctx, c.App, c.Resource.ShortName())
	if err == db.ErrNotFound {
		// it's not an error to 'delete' something that's already gone
		return nil
	} else if err != nil {
		return err
	}

	return nil
}

func (r *rp) ListDeployments(ctx context.Context, id resources.ResourceID) (*rest.ResourceList, error) {
	err := id.ValidateResourceType(resources.DeploymentCollectionType)
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}
	app, err := id.Application()
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	dbitems, err := r.db.ListDeploymentsByApplicationID(ctx, app)
	if err == db.ErrNotFound {
		return nil, &NotFoundError{id}
	} else if err != nil {
		return nil, err
	}

	items := make([]interface{}, 0, len(dbitems))
	for _, dbitem := range dbitems {
		items = append(items, *newRESTDeploymentFromDB(&dbitem))
	}

	list := &rest.ResourceList{Value: items}
	return list, nil
}

func (r *rp) GetDeployment(ctx context.Context, id resources.ResourceID) (*rest.Deployment, error) {
	d, err := id.Deployment()
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	dbitem, err := r.db.GetDeploymentByApplicationID(ctx, d.App, d.Resource.ShortName())
	if err == db.ErrNotFound {
		return nil, &NotFoundError{ID: id}
	} else if err != nil {
		return nil, err
	}

	item := newRESTDeploymentFromDB(dbitem)
	return item, nil
}

func (r *rp) UpdateDeployment(ctx context.Context, d *rest.Deployment) (*rest.Deployment, error) {
	id, err := d.GetDeploymentID()
	if err != nil {
		return nil, BadRequestError{err.Error()}
	}

	err = r.validate(d)
	if err != nil {
		return nil, err
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
		return nil, NotFoundError{}
	} else if err != nil {
		return nil, err
	}

	var olddbitem *db.Deployment
	obj, ok := app.Deployments[id.Resource.ShortName()]
	if ok {
		olddbitem = &obj
	}

	actions, err := r.computeDeploymentActions(app, olddbitem, newdbitem)
	if err != nil {
		// An error computing deployment actions is generally the users' fault.
		return nil, BadRequestError{err.Error()}
	}

	eq := deploymentIsNoOp(actions)
	if eq {
		// No changes to the deployment - nothing to do.
		log.Printf("%T is unchanged.", newdbitem)
		return newRESTDeploymentFromDB(olddbitem), nil
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

	err = r.deploy.UpdateDeployment(ctx, app.FriendlyName(), newdbitem.Name, &newdbitem.Status, actions)
	if _, ok := err.(*CompositeError); ok {
		return nil, BadRequestError{err.Error()}
	} else if err != nil {
		return nil, err
	}

	err = r.db.PatchDeploymentByApplicationID(ctx, id.App, id.Resource.ShortName(), newdbitem)
	if err != nil {
		return nil, err
	}

	return newRESTDeploymentFromDB(newdbitem), nil
}

func (r *rp) DeleteDeployment(ctx context.Context, id resources.ResourceID) error {
	d, err := id.Deployment()
	if err != nil {
		return BadRequestError{err.Error()}
	}

	current, err := r.db.GetDeploymentByApplicationID(ctx, d.App, d.Resource.ShortName())
	if err == db.ErrNotFound {
		// it's not an error to 'delete' something that's already gone
		return nil
	} else if err != nil {
		return err
	}

	if current.Status.Services == nil {
		current.Status.Services = map[string]db.DeploymentService{}
	}

	err = r.deploy.DeleteDeployment(ctx, d.Resource.ShortName(), &current.Status)
	if _, ok := err.(*CompositeError); ok {
		return BadRequestError{err.Error()}
	} else if err != nil {
		return err
	}

	return r.db.DeleteDeploymentByApplicationID(ctx, d.App, d.Resource.ShortName())
}

func (r *rp) ListScopes(ctx context.Context, id resources.ResourceID) (*rest.ResourceList, error) {
	err := id.ValidateResourceType(resources.ScopeCollectionType)
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	app, err := id.Application()
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	dbitems, err := r.db.ListScopesByApplicationID(ctx, app)
	if err == db.ErrNotFound {
		return nil, &NotFoundError{id}
	} else if err != nil {
		return nil, err
	}

	items := make([]interface{}, 0, len(dbitems))
	for _, dbitem := range dbitems {
		items = append(items, *newRESTScopeFromDB(&dbitem))
	}

	list := &rest.ResourceList{Value: items}
	return list, nil
}

func (r *rp) GetScope(ctx context.Context, id resources.ResourceID) (*rest.Scope, error) {
	s, err := id.Scope()
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	dbitem, err := r.db.GetScopeByApplicationID(ctx, s.App, s.Resource.ShortName())
	if err == db.ErrNotFound {
		return nil, &NotFoundError{ID: id}
	} else if err != nil {
		return nil, err
	}

	item := newRESTScopeFromDB(dbitem)
	return item, nil
}

func (r *rp) UpdateScope(ctx context.Context, s *rest.Scope) (*rest.Scope, error) {
	id, err := s.GetScopeID()
	if err != nil {
		return nil, &BadRequestError{err.Error()}
	}

	err = r.validate(s)
	if err != nil {
		return nil, err
	}

	dbitem := newDBScopeFromREST(s)
	err = r.db.PatchScopeByApplicationID(ctx, id.App, id.Resource.ShortName(), dbitem)
	if err != nil {
		return nil, err
	}

	return newRESTScopeFromDB(dbitem), nil
}

func (r *rp) DeleteScope(ctx context.Context, id resources.ResourceID) error {
	s, err := id.Scope()
	if err != nil {
		return &BadRequestError{err.Error()}
	}

	err = r.db.DeleteScopeByApplicationID(ctx, s.App, s.Resource.ShortName())
	if err != nil {
		return err
	}

	return nil
}

func (r *rp) validate(obj interface{}) error {
	err := r.v.Struct(obj)
	if val, ok := err.(validator.ValidationErrors); ok {
		err = ValidationError{
			Value:  obj,
			Errors: val,
		}
	}

	return err
}

func (r *rp) computeDeploymentActions(app *db.Application, older *db.Deployment, newer *db.Deployment) (map[string]ComponentAction, error) {
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

	// gather all component names
	names := map[string]bool{}
	for name := range active {
		names[name] = true
	}
	for name := range current {
		names[name] = true
	}

	actions := map[string]ComponentAction{}
	for name := range names {
		n := active[name]
		o := current[name]

		var s map[string]ServiceBinding
		ninst, ok := newer.LookupComponent(name)
		if ok {
			s = serviceBindings[name]
		}

		var oinst *db.DeploymentComponent
		if older != nil {
			oinst, _ = older.LookupComponent(name)
		}

		traits, err := combineTraits(ninst, n)
		if err != nil {
			return nil, err
		}

		provides := filterProvidersByComponent(name, providers)

		wd := ComponentAction{
			ApplicationName:       app.FriendlyName(),
			ComponentName:         name,
			Operation:             None, // Assume none until we find otherwise
			Definition:            n,
			Instantiation:         ninst,
			Provides:              provides,
			ServiceBindings:       s,
			Traits:                traits,
			PreviousDefinition:    o,
			PreviousInstanitation: oinst,
		}

		err = assignOperation(&wd)
		if err != nil {
			return nil, err
		}

		if wd.Operation != DeleteWorkload {
			wd.Component, err = convertToComponent(wd.ComponentName, *wd.Definition, traits)
			if err != nil {
				return nil, err
			}
		}

		actions[name] = wd
	}

	for _, action := range actions {
		if action.Operation == CreateWorkload {
			log.Printf("component %s is added in this update", action.ComponentName)
		} else if action.Operation == DeleteWorkload {
			log.Printf("component %s is removed in this update", action.ComponentName)
		} else if action.Operation == UpdateWorkload && action.PreviousDefinition.Revision != action.Definition.Revision {
			log.Printf("component %s is upgraded %s->%s in this update", action.ComponentName, action.PreviousDefinition.Revision, action.Definition.Revision)
		} else if action.Operation == UpdateWorkload && action.PreviousDefinition.Revision == action.Definition.Revision {
			log.Printf("component %s has parameter changes in this update", action.ComponentName)
		} else {
			log.Printf("component %s is unchanged in this update", action.ComponentName)
		}
	}

	return actions, nil
}

func deploymentIsNoOp(actions map[string]ComponentAction) bool {
	for _, action := range actions {
		if action.Operation != None {
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

func assignOperation(wd *ComponentAction) error {
	if wd.Instantiation == nil && wd.PreviousInstanitation == nil {
		return errors.New("can't figure out operation")
	} else if wd.Instantiation != nil && wd.PreviousInstanitation == nil {
		wd.Operation = CreateWorkload
		return nil
	} else if wd.Instantiation == nil && wd.PreviousInstanitation != nil {
		wd.Operation = DeleteWorkload
		return nil
	}

	// Those are all of the *easy* cases. If we get here then the workload is either being upgraded
	// or is the same - so we can safely dereference any properties.
	if wd.Definition.Revision != wd.PreviousDefinition.Revision {
		// revision does not match.
		wd.Operation = UpdateWorkload
		return nil
	}

	return nil
}

func (r *rp) bindProviders(d *db.Deployment, cs map[string]*db.ComponentRevision) (map[string]ServiceBinding, error) {
	// find all services provided by all components
	providers := map[string]ServiceBinding{}
	for _, dc := range d.Properties.Components {
		// We don't expect this to fail except in tests
		c, ok := cs[dc.FriendlyName()]
		if !ok {
			return nil, fmt.Errorf("cannot find matching revision for component %s", dc.FriendlyName())
		}

		// Intrinsic bindings are provided by traits and the workload types
		// they can be overridden by declaring a service with the same name on the same component
		intrinsic := map[string]ServiceBinding{}

		s, ok := r.meta.WorkloadKindServices[c.Kind]
		if ok {
			s.Name = dc.FriendlyName()
			_, ok := intrinsic[s.Name]
			if ok {
				return nil, fmt.Errorf("service %v has multiple providers", s.Name)
			}

			// Found one - add to both list - it will get removed later if it's
			// been rebound
			intrinsic[s.Name] = ServiceBinding{
				Name:     s.Name,
				Kind:     s.Kind,
				Provider: dc.FriendlyName(),
			}

			// TODO: we currently allow a service from one component to 'hide' a service from another
			_, ok = providers[s.Name]
			if !ok {
				providers[s.Name] = ServiceBinding{
					Name:     s.Name,
					Kind:     s.Kind,
					Provider: dc.FriendlyName(),
				}
			}
		}

		for _, t := range c.Properties.Traits {
			s, ok := r.meta.TraitServices[t.Kind]
			if ok {
				s.Name = dc.FriendlyName()
				_, ok := intrinsic[s.Name]
				if ok {
					return nil, fmt.Errorf("service %v has multiple providers", s.Name)
				}

				// Found one - add to both list - it will get removed later if it's
				// been rebound
				intrinsic[s.Name] = ServiceBinding{
					Name:     s.Name,
					Kind:     s.Kind,
					Provider: dc.FriendlyName(),
				}

				_, ok = providers[s.Name]
				if !ok {
					providers[s.Name] = ServiceBinding{
						Name:     s.Name,
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

			providers[s.Name] = ServiceBinding{
				Name:     s.Name,
				Kind:     s.Kind,
				Provider: dc.FriendlyName(),
			}
		}
	}

	return providers, nil
}

func (r *rp) bindServices(d *db.Deployment, cs map[string]*db.ComponentRevision, providers map[string]ServiceBinding) (map[string]map[string]ServiceBinding, error) {
	// find the relationship between services declared and the components that match
	bindings := map[string]map[string]ServiceBinding{}

	// Now loop through all of the consumers and match them up
	for _, dc := range d.Properties.Components {
		// We don't expect this to fail except in tests
		c, ok := cs[dc.FriendlyName()]
		if !ok {
			return nil, fmt.Errorf("cannot find matching revision for component %s", dc.FriendlyName())
		}

		b := map[string]ServiceBinding{}
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

func combineTraits(dc *db.DeploymentComponent, cr *db.ComponentRevision) ([]db.ComponentTrait, error) {
	if dc == nil || cr == nil {
		return []db.ComponentTrait{}, nil
	}

	deployment := map[string]db.ComponentTrait{}
	for _, t := range dc.Traits {
		_, ok := deployment[t.Kind]
		if ok {
			return nil, fmt.Errorf("duplicate trait in deployment '%v'", t.Kind)
		}

		deployment[t.Kind] = db.ComponentTrait(t)
	}

	component := map[string]db.ComponentTrait{}
	for _, t := range cr.Properties.Traits {
		_, ok := component[t.Kind]
		if ok {
			return nil, fmt.Errorf("duplicate trait in component '%v'", t.Kind)
		}

		component[t.Kind] = db.ComponentTrait(t)
	}

	// traits defined in components are superseded by those in the deployment
	traits := []db.ComponentTrait{}
	for _, t := range deployment {
		traits = append(traits, t)
	}
	for k, v := range component {
		_, ok := deployment[k]
		if !ok {
			traits = append(traits, v)
		}
	}

	return traits, nil
}

func filterProvidersByComponent(componentName string, providers map[string]ServiceBinding) map[string]ComponentService {
	results := map[string]ComponentService{}
	for _, sb := range providers {
		if sb.Provider == componentName {
			results[sb.Name] = ComponentService{
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
