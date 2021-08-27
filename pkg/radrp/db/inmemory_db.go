// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"context"
	"sync"

	resources "github.com/Azure/radius/pkg/radrp/resources"
	"github.com/golang/mock/gomock"
)

// NewInMemoryRadrpDB returns an in-memory implementation of RadrpDB
func NewInMemoryRadrpDB(ctrl *gomock.Controller) *MockRadrpDB {
	base := NewMockRadrpDB(ctrl)

	store := &store{
		applications: map[applicationKey]*map[string]*Application{},
		operations:   map[string]*Operation{},
		mutex:        sync.Mutex{},
	}

	base.EXPECT().
		ListApplicationsByResourceGroup(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.ListApplicationsByResourceGroup)

	base.EXPECT().
		GetApplicationByID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.GetApplicationByID)

	base.EXPECT().
		PatchApplication(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.PatchApplication)

	base.EXPECT().
		UpdateApplication(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.UpdateApplication)

	base.EXPECT().
		DeleteApplicationByID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.DeleteApplicationByID)

	base.EXPECT().
		ListComponentsByApplicationID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.ListComponentsByApplicationID)

	base.EXPECT().
		GetComponentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.GetComponentByApplicationID)

	base.EXPECT().
		PatchComponentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.PatchComponentByApplicationID)

	base.EXPECT().
		DeleteComponentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.DeleteComponentByApplicationID)

	base.EXPECT().
		ListDeploymentsByApplicationID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.ListDeploymentsByApplicationID)

	base.EXPECT().
		GetDeploymentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.GetDeploymentByApplicationID)

	base.EXPECT().
		PatchDeploymentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.PatchDeploymentByApplicationID)

	base.EXPECT().
		DeleteDeploymentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.DeleteDeploymentByApplicationID)

	base.EXPECT().
		ListScopesByApplicationID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.ListScopesByApplicationID)

	base.EXPECT().
		GetScopeByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.GetScopeByApplicationID)

	base.EXPECT().
		PatchScopeByApplicationID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.PatchScopeByApplicationID)

	base.EXPECT().
		DeleteScopeByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.DeleteScopeByApplicationID)

	base.EXPECT().
		GetOperationByID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.GetOperationByID)

	base.EXPECT().
		PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.PatchOperationByID)

	base.EXPECT().
		DeleteOperationByID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.DeleteOperationByID)
	return base
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

func (s *store) GetOperationByID(ctx context.Context, id resources.ResourceID) (*Operation, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	op := s.operations[id.ID]
	if op == nil {
		return nil, ErrNotFound
	}

	return op, nil
}

func (s *store) PatchOperationByID(ctx context.Context, id resources.ResourceID, patch *Operation) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, ok := s.operations[id.ID]
	s.operations[id.ID] = patch
	return !ok, nil
}

func (s *store) DeleteOperationByID(ctx context.Context, id resources.ResourceID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.operations, id.ID)
	return nil
}
