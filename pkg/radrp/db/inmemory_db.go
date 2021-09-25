// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"context"
	"errors"
	"sync"

	"github.com/Azure/radius/pkg/azure/azresources"
	resources "github.com/Azure/radius/pkg/radrp/resources"
)

// NewInMemoryRadrpDB returns an in-memory implementation of RadrpDB
func NewInMemoryRadrpDB() RadrpDB {
	store := &store{
		applications: map[applicationKey]*map[string]*Application{},
		operations:   map[string]*Operation{},
		mutex:        sync.Mutex{},
	}

	return store
}

type applicationKey struct {
	subscriptionID string
	resourceGroup  string
}

type store struct {
	applications map[applicationKey]*map[string]*Application
	operations   map[string]*Operation
	mutex        sync.Mutex
}

func applicationKeyFromID(id resources.ResourceID) applicationKey {
	return applicationKey{
		subscriptionID: id.SubscriptionID,
		resourceGroup:  id.ResourceGroup,
	}
}

var _ RadrpDB = &store{}

func (s *store) findApplication(id resources.ApplicationID) *Application {
	k := applicationKeyFromID(id.ResourceID)
	list := s.applications[k]
	if list == nil {
		return nil
	}

	app, ok := (*list)[id.Name()]
	if !ok {
		return nil
	}

	return app
}

func (s *store) ListApplicationsByResourceGroup(ctx context.Context, id resources.ResourceID) ([]Application, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := applicationKeyFromID(id)
	list := s.applications[k]
	if list == nil {
		return []Application{}, nil
	}

	apps := []Application{}
	for _, v := range *list {
		apps = append(apps, *v)
	}

	return apps, nil
}

func (s *store) GetApplicationByID(ctx context.Context, id resources.ApplicationID) (*Application, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, ErrNotFound
	}

	return app.DeepCopy(), nil
}

func (s *store) PatchApplication(ctx context.Context, patch *ApplicationPatch) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := applicationKey{patch.SubscriptionID, patch.ResourceGroup}
	list := s.applications[k]
	if list == nil {
		list = &map[string]*Application{}
		s.applications[k] = list
	}

	old := (*list)[patch.FriendlyName()]
	new := &Application{}

	if old == nil {
		new.Components = map[string]Component{}
		new.Deployments = map[string]Deployment{}
		new.Scopes = map[string]Scope{}
	} else {
		new.Components = old.Components
		new.Deployments = old.Deployments
		new.Scopes = old.Scopes
	}

	new.ResourceBase = patch.ResourceBase
	new.Properties = patch.Properties

	(*list)[patch.FriendlyName()] = new
	return old == nil, nil
}

func (s *store) UpdateApplication(ctx context.Context, app *Application) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := applicationKey{app.SubscriptionID, app.ResourceGroup}
	list := s.applications[k]
	if list == nil {
		list = &map[string]*Application{}
		s.applications[k] = list
	}
	old := (*list)[app.FriendlyName()]
	(*list)[app.FriendlyName()] = app.DeepCopy()
	return old == nil, nil
}

func (s *store) DeleteApplicationByID(ctx context.Context, id resources.ApplicationID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := applicationKeyFromID(id.ResourceID)
	list := s.applications[k]
	if list == nil {
		return nil
	}

	delete(*list, id.Name())
	return nil
}

func (s *store) ListComponentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Component, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, ErrNotFound
	}

	items := []Component{}
	for _, item := range app.Components {
		items = append(items, item)
	}

	return items, nil
}

func (s *store) GetComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Component, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, ErrNotFound
	}

	item, ok := app.Components[name]
	if !ok {
		return nil, ErrNotFound
	}

	return &item, nil
}

func (s *store) PatchComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Component) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return false, ErrNotFound
	}

	_, ok := app.Components[name]

	app.Components[name] = *patch
	return !ok, nil
}

func (s *store) DeleteComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return ErrNotFound
	}

	delete(app.Components, name)
	return nil
}

func (s *store) ListDeploymentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Deployment, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, ErrNotFound
	}

	items := []Deployment{}
	for _, d := range app.Deployments {
		items = append(items, d)
	}

	return items, nil
}

func (s *store) GetDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Deployment, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, ErrNotFound
	}

	d, ok := app.Deployments[name]
	if !ok {
		return nil, ErrNotFound
	}

	return &d, nil
}

func (s *store) PatchDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Deployment) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return false, ErrNotFound
	}

	_, ok := app.Deployments[name]

	app.Deployments[name] = *patch
	return !ok, nil
}

func (s *store) DeleteDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return ErrNotFound
	}

	delete(app.Deployments, name)
	return nil
}

func (s *store) ListScopesByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Scope, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, ErrNotFound
	}

	items := []Scope{}
	for _, s := range app.Scopes {
		items = append(items, s)
	}

	return items, nil
}

func (s *store) GetScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Scope, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, ErrNotFound
	}

	scope, ok := app.Scopes[name]
	if !ok {
		return nil, ErrNotFound
	}

	return &scope, nil
}

func (s *store) PatchScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Scope) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return false, ErrNotFound
	}

	_, ok := app.Scopes[name]

	app.Scopes[name] = *patch
	return !ok, nil
}

func (s *store) DeleteScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return ErrNotFound
	}

	delete(app.Scopes, name)
	return nil
}

func (s *store) GetOperationByID(ctx context.Context, id azresources.ResourceID) (*Operation, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	op := s.operations[id.ID]
	if op == nil {
		return nil, ErrNotFound
	}

	return op, nil
}

func (s *store) PatchOperationByID(ctx context.Context, id azresources.ResourceID, patch *Operation) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, ok := s.operations[id.ID]
	s.operations[id.ID] = patch
	return !ok, nil
}

func (s *store) DeleteOperationByID(ctx context.Context, id azresources.ResourceID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.operations, id.ID)
	return nil
}

func (s *store) ListV3Applications(ctx context.Context, id azresources.ResourceID) ([]ApplicationResource, error) {
	return nil, errors.New("not implemented")
}

func (s *store) GetV3Application(ctx context.Context, id azresources.ResourceID) (ApplicationResource, error) {
	return ApplicationResource{}, errors.New("not implemented")
}

func (s *store) UpdateV3ApplicationDefinition(ctx context.Context, application ApplicationResource) (bool, error) {
	return false, errors.New("not implemented")
}

func (s *store) DeleteV3Application(ctx context.Context, id azresources.ResourceID) error {
	return errors.New("not implemented")
}

func (s *store) ListAllV3Resources(ctx context.Context, id azresources.ResourceID) ([]RadiusResource, error) {
	return nil, errors.New("not implemented")
}

func (s *store) ListV3Resources(ctx context.Context, id azresources.ResourceID) ([]RadiusResource, error) {
	return nil, errors.New("not implemented")
}

func (s *store) GetV3Resource(ctx context.Context, id azresources.ResourceID) (RadiusResource, error) {
	return RadiusResource{}, errors.New("not implemented")
}

func (s *store) UpdateV3ResourceDefinition(ctx context.Context, id azresources.ResourceID, resource RadiusResource) (bool, error) {
	return false, errors.New("not implemented")
}

func (s *store) UpdateV3ResourceStatus(ctx context.Context, id azresources.ResourceID, resource RadiusResource) error {
	return errors.New("not implemented")
}

func (s *store) DeleteV3Resource(ctx context.Context, id azresources.ResourceID) error {
	return errors.New("not implemented")
}
