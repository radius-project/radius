// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mocks

import (
	"context"
	"sync"

	db "github.com/Azure/radius/pkg/radrp/db"
	resources "github.com/Azure/radius/pkg/radrp/resources"
	"github.com/golang/mock/gomock"
)

// NewInMemoryRadrpDB returns an in-memory implementation of RadrpDB
func NewInMemoryRadrpDB(ctrl *gomock.Controller) *MockRadrpDB {
	base := NewMockRadrpDB(ctrl)

	store := &store{
		applications: map[applicationKey]*map[string]*db.Application{},
		operations:   map[string]*db.Operation{},
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
	applications map[applicationKey]*map[string]*db.Application
	operations   map[string]*db.Operation
	mutex        sync.Mutex
}

func applicationKeyFromID(id resources.ResourceID) applicationKey {
	return applicationKey{
		subscriptionID: id.SubscriptionID,
		resourceGroup:  id.ResourceGroup,
	}
}

var _ db.RadrpDB = &store{}

func (s *store) findApplication(id resources.ApplicationID) *db.Application {
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

func (s *store) ListApplicationsByResourceGroup(ctx context.Context, id resources.ResourceID) ([]db.Application, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := applicationKeyFromID(id)
	list := s.applications[k]
	if list == nil {
		return []db.Application{}, nil
	}

	apps := []db.Application{}
	for _, v := range *list {
		apps = append(apps, *v)
	}

	return apps, nil
}

func (s *store) GetApplicationByID(ctx context.Context, id resources.ApplicationID) (*db.Application, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	return app, nil
}

func (s *store) PatchApplication(ctx context.Context, patch *db.ApplicationPatch) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := applicationKey{patch.SubscriptionID, patch.ResourceGroup}
	list := s.applications[k]
	if list == nil {
		list = &map[string]*db.Application{}
		s.applications[k] = list
	}

	old := (*list)[patch.FriendlyName()]
	new := &db.Application{}

	if old == nil {
		new.Components = map[string]db.Component{}
		new.Deployments = map[string]db.Deployment{}
		new.Scopes = map[string]db.Scope{}
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

func (s *store) UpdateApplication(ctx context.Context, app *db.Application) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := applicationKey{app.SubscriptionID, app.ResourceGroup}
	list := s.applications[k]
	if list == nil {
		list = &map[string]*db.Application{}
		s.applications[k] = list
	}

	old := (*list)[app.FriendlyName()]
	new := &db.Application{}

	if old == nil {
		new.Components = map[string]db.Component{}
		new.Deployments = map[string]db.Deployment{}
		new.Scopes = map[string]db.Scope{}
	} else {
		new.Components = old.Components
		new.Deployments = old.Deployments
		new.Scopes = old.Scopes
	}

	new.ResourceBase = app.ResourceBase
	new.Properties = app.Properties

	(*list)[app.FriendlyName()] = new
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

func (s *store) ListComponentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]db.Component, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	items := []db.Component{}
	for _, item := range app.Components {
		items = append(items, item)
	}

	return items, nil
}

func (s *store) GetComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*db.Component, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	item, ok := app.Components[name]
	if !ok {
		return nil, db.ErrNotFound
	}

	return &item, nil
}

func (s *store) PatchComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *db.Component) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return false, db.ErrNotFound
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
		return db.ErrNotFound
	}

	delete(app.Components, name)
	return nil
}

func (s *store) ListDeploymentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]db.Deployment, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	items := []db.Deployment{}
	for _, d := range app.Deployments {
		items = append(items, d)
	}

	return items, nil
}

func (s *store) GetDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*db.Deployment, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	d, ok := app.Deployments[name]
	if !ok {
		return nil, db.ErrNotFound
	}

	return &d, nil
}

func (s *store) PatchDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *db.Deployment) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return false, db.ErrNotFound
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
		return db.ErrNotFound
	}

	delete(app.Deployments, name)
	return nil
}

func (s *store) ListScopesByApplicationID(ctx context.Context, id resources.ApplicationID) ([]db.Scope, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	items := []db.Scope{}
	for _, s := range app.Scopes {
		items = append(items, s)
	}

	return items, nil
}

func (s *store) GetScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*db.Scope, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	scope, ok := app.Scopes[name]
	if !ok {
		return nil, db.ErrNotFound
	}

	return &scope, nil
}

func (s *store) PatchScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *db.Scope) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return false, db.ErrNotFound
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
		return db.ErrNotFound
	}

	delete(app.Scopes, name)
	return nil
}

func (s *store) GetOperationByID(ctx context.Context, id resources.ResourceID) (*db.Operation, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	op := s.operations[id.ID]
	if op == nil {
		return nil, db.ErrNotFound
	}

	return op, nil
}

func (s *store) PatchOperationByID(ctx context.Context, id resources.ResourceID, patch *db.Operation) (bool, error) {
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
