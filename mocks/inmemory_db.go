// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mocks

import (
	"context"
	"sync"

	db "github.com/Azure/radius/pkg/curp/db"
	resources "github.com/Azure/radius/pkg/curp/resources"
	revision "github.com/Azure/radius/pkg/curp/revision"
	"github.com/golang/mock/gomock"
)

// NewInMemoryCurpDB returns an in-memory implementation of CurpDB
func NewInMemoryCurpDB(ctrl *gomock.Controller) *MockCurpDB {
	base := NewMockCurpDB(ctrl)

	store := &store{map[key]*map[string]*db.Application{}, sync.Mutex{}}

	base.EXPECT().
		ListApplicationsByResourceGroup(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.listApplicationsByResourceGroup)

	base.EXPECT().
		GetApplicationByID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.getApplicationByID)

	base.EXPECT().
		PatchApplication(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.patchApplication)

	base.EXPECT().
		DeleteApplicationByID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.deleteApplicationByID)

	base.EXPECT().
		ListComponentsByApplicationID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.listComponentsByApplicationID)

	base.EXPECT().
		GetComponentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.getComponentByApplicationID)

	base.EXPECT().
		PatchComponentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.patchComponentByApplicationID)

	base.EXPECT().
		DeleteComponentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.deleteComponentByApplicationID)

	base.EXPECT().
		ListDeploymentsByApplicationID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.listDeploymentsByApplicationID)

	base.EXPECT().
		GetDeploymentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.getDeploymentByApplicationID)

	base.EXPECT().
		PatchDeploymentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.patchDeploymentByApplicationID)

	base.EXPECT().
		DeleteDeploymentByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.deleteDeploymentByApplicationID)

	base.EXPECT().
		ListScopesByApplicationID(gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.listScopesByApplicationID)

	base.EXPECT().
		GetScopeByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.getScopeByApplicationID)

	base.EXPECT().
		PatchScopeByApplicationID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.patchScopeByApplicationID)

	base.EXPECT().
		DeleteScopeByApplicationID(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(store.deleteScopeByApplicationID)

	return base
}

type key struct {
	subscriptionID string
	resourceGroup  string
}

type store struct {
	store map[key]*map[string]*db.Application
	mutex sync.Mutex
}

func keyFromID(id resources.ResourceID) key {
	return key{
		subscriptionID: id.SubscriptionID,
		resourceGroup:  id.ResourceGroup,
	}
}

func (s *store) findApplication(id resources.ApplicationID) *db.Application {
	k := keyFromID(id.ResourceID)
	list := s.store[k]
	if list == nil {
		return nil
	}

	app, ok := (*list)[id.ShortName()]
	if !ok {
		return nil
	}

	return app
}

func (s *store) listApplicationsByResourceGroup(ctx context.Context, id resources.ResourceID) ([]db.Application, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := keyFromID(id)
	list := s.store[k]
	if list == nil {
		return []db.Application{}, nil
	}

	apps := []db.Application{}
	for _, v := range *list {
		apps = append(apps, *v)
	}

	return apps, nil
}

func (s *store) getApplicationByID(ctx context.Context, id resources.ApplicationID) (*db.Application, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	return app, nil
}

func (s *store) patchApplication(ctx context.Context, patch *db.ApplicationPatch) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := key{patch.SubscriptionID, patch.ResourceGroup}
	list := s.store[k]
	if list == nil {
		list = &map[string]*db.Application{}
		s.store[k] = list
	}

	old := (*list)[patch.FriendlyName()]
	new := &db.Application{}

	if old == nil {
		new.Components = map[string]db.ComponentHistory{}
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
	return nil
}

func (s *store) deleteApplicationByID(ctx context.Context, id resources.ApplicationID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := keyFromID(id.ResourceID)
	list := s.store[k]
	if list == nil {
		return nil
	}

	delete(*list, id.ShortName())
	return nil
}

func (s *store) listComponentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]db.Component, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	items := []db.Component{}
	for _, ch := range app.Components {
		cr := ch.RevisionHistory[0]
		item := db.Component{
			ResourceBase: ch.ResourceBase,
			Kind:         cr.Kind,
			Revision:     cr.Revision,
			Properties:   cr.Properties,
		}
		items = append(items, item)
	}

	return items, nil
}

func (s *store) getComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, rev revision.Revision) (*db.Component, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return nil, db.ErrNotFound
	}

	history, ok := app.Components[name]
	if !ok {
		return nil, db.ErrNotFound
	}

	var cr *db.ComponentRevision
	if len(history.RevisionHistory) == 0 {
		// no revisions
	} else if rev == revision.Revision("") {
		// "latest", return the first one
		cr = &history.RevisionHistory[len(history.RevisionHistory)-1]
	} else {
		for _, r := range history.RevisionHistory {
			if rev == r.Revision {
				cr = &r
				break
			}
		}
	}

	if cr == nil {
		return nil, db.ErrNotFound
	}

	item := db.Component{
		ResourceBase: history.ResourceBase,
		Kind:         cr.Kind,
		Revision:     cr.Revision,
		Properties:   cr.Properties,
	}

	return &item, nil
}

func (s *store) patchComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *db.Component, previous revision.Revision) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return db.ErrNotFound
	}

	// If this is the first revision, we need to make sure the component history record exists.
	if previous == revision.Revision("") {
		app.Components[name] = db.ComponentHistory{
			ResourceBase: patch.ResourceBase,
		}
	}

	cr := db.ComponentRevision{
		Kind:       patch.Kind,
		Revision:   patch.Revision,
		Properties: patch.Properties,
	}

	ch := app.Components[name]
	ch.RevisionHistory = append(ch.RevisionHistory, cr)
	ch.Revision = cr.Revision

	app.Components[name] = ch
	return nil
}

func (s *store) deleteComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return db.ErrNotFound
	}

	delete(app.Components, name)
	return nil
}

func (s *store) listDeploymentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]db.Deployment, error) {
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

func (s *store) getDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*db.Deployment, error) {
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

func (s *store) patchDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *db.Deployment) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return db.ErrNotFound
	}

	app.Deployments[name] = *patch
	return nil
}

func (s *store) deleteDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return db.ErrNotFound
	}

	delete(app.Deployments, name)
	return nil
}

func (s *store) listScopesByApplicationID(ctx context.Context, id resources.ApplicationID) ([]db.Scope, error) {
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

func (s *store) getScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*db.Scope, error) {
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

func (s *store) patchScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *db.Scope) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return db.ErrNotFound
	}

	app.Scopes[name] = *patch
	return nil
}

func (s *store) deleteScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	app := s.findApplication(id)
	if app == nil {
		return db.ErrNotFound
	}

	delete(app.Scopes, name)
	return nil
}
