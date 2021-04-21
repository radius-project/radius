// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package curp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/Azure/radius/mocks"
	"github.com/Azure/radius/pkg/curp/armerrors"
	"github.com/Azure/radius/pkg/curp/db"
	"github.com/Azure/radius/pkg/curp/deployment"
	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/curp/rest"
	"github.com/Azure/radius/pkg/curp/revision"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const (
	TestSubscriptionID = "test-subscription"
	TestResourceGroup  = "test-resourcegroup"
)

const baseURI = "/subscriptions/test-subscription/resourceGroups/test-resourcegroup/providers/Microsoft.CustomProviders/resourceProviders/radius"

type test struct {
	t      *testing.T
	db     *mocks.MockCurpDB
	ctrl   *gomock.Controller
	k8s    *mocks.MockClient
	server *httptest.Server
	deploy *mocks.MockDeploymentProcessor
}

func start(t *testing.T) *test {
	ctrl := gomock.NewController(t)
	db := mocks.NewInMemoryCurpDB(ctrl)
	k8s := mocks.NewMockClient(ctrl)
	deploy := mocks.NewMockDeploymentProcessor(ctrl)

	options := ServerOptions{
		Address:      httptest.DefaultRemoteAddr,
		Authenticate: false,
		Deploy:       deploy,
		DB:           db,
		K8s:          k8s,
	}

	s := NewServer(options)

	server := httptest.NewServer(s.Handler)
	t.Cleanup(server.Close)

	return &test{
		t:      t,
		db:     db,
		k8s:    k8s,
		deploy: deploy,
		ctrl:   ctrl,
		server: server,
	}
}

func applicationList() resources.ResourceID {
	return parseOrPanic(baseURI + "/Applications/")
}

func applicationID(application string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s", application))
}

func componentList(application string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Components", application))
}

func componentID(application string, component string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Components/%s", application, component))
}

func deploymentList(application string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Deployments", application))
}

func deploymentID(application string, deployment string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Deployments/%s", application, deployment))
}

func scopeList(application string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Scopes", application))
}

func scopeID(application string, scope string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Scopes/%s", application, scope))
}

func parseOrPanic(id string) resources.ResourceID {
	res, err := resources.Parse(id)
	if err != nil {
		panic(err)
	}
	return res
}

func requireJSON(t *testing.T, expected interface{}, w *httptest.ResponseRecorder) {
	bytes, err := json.Marshal(expected)
	require.NoError(t, err)
	require.JSONEq(t, string(bytes), w.Body.String())
}

func (test *test) DBCreateApplication(application string, properties map[string]interface{}) {
	id := applicationID(application)
	_, err := test.db.PatchApplication(context.TODO(), &db.ApplicationPatch{
		ResourceBase: db.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: properties,
	})
	require.NoError(test.t, err)
}

func (test *test) DBDeleteApplication(application string) {
	id := applicationID(application)
	a, err := id.Application()
	require.NoError(test.t, err)

	err = test.db.DeleteApplicationByID(context.TODO(), a)
	require.NoError(test.t, err)
}

func (test *test) DBCreateComponent(application string, component string, kind string, properties db.ComponentProperties) revision.Revision {
	id := componentID(application, component)
	a, err := id.Application()
	require.NoError(test.t, err)

	c := &db.Component{
		ResourceBase: db.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Kind:       kind,
		Properties: properties,
	}

	previous := revision.Revision("")
	old, err := test.db.GetComponentByApplicationID(context.TODO(), a, component, revision.Revision(""))
	if err == db.ErrNotFound {
		// this is fine - we don't have a previous version to compare against
	} else if err != nil {
		require.NoError(test.t, err)
	} else if old != nil {
		previous = old.Revision
	}

	rev, err := revision.Compute(c, previous, []revision.Revision{})
	require.NoError(test.t, err)

	c.Revision = rev

	_, err = test.db.PatchComponentByApplicationID(context.TODO(), a, component, c, previous)
	require.NoError(test.t, err)

	return rev
}

func (test *test) DBDeleteComponent(application string, component string) {
	id := componentID(application, component)
	a, err := id.Application()
	require.NoError(test.t, err)

	err = test.db.DeleteComponentByApplicationID(context.TODO(), a, component)
	require.NoError(test.t, err)
}

func (test *test) DBCreateDeployment(application string, deployment string, properties db.DeploymentProperties) {
	id := deploymentID(application, deployment)
	a, err := id.Application()
	require.NoError(test.t, err)

	d := &db.Deployment{
		ResourceBase: db.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: properties,
	}

	_, err = test.db.PatchDeploymentByApplicationID(context.TODO(), a, deployment, d)
	require.NoError(test.t, err)
}

func (test *test) DBDeleteDeployment(application string, deployment string) {
	id := deploymentID(application, deployment)
	a, err := id.Application()
	require.NoError(test.t, err)

	err = test.db.DeleteDeploymentByApplicationID(context.TODO(), a, deployment)
	require.NoError(test.t, err)
}

func (test *test) DBCreateScope(application string, scope string, properties map[string]interface{}) {
	id := scopeID(application, scope)
	a, err := id.Application()
	require.NoError(test.t, err)

	s := &db.Scope{
		ResourceBase: db.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: properties,
	}

	_, err = test.db.PatchScopeByApplicationID(context.TODO(), a, scope, s)
	require.NoError(test.t, err)
}

func (test *test) DBDeleteScope(application string, scope string) {
	id := scopeID(application, scope)
	a, err := id.Application()
	require.NoError(test.t, err)

	err = test.db.DeleteScopeByApplicationID(context.TODO(), a, scope)
	require.NoError(test.t, err)
}

func (test *test) ValidateDeploymentOperationInProgress(location string) *rest.Deployment {
	// At this point deployment has started and is waiting for us to signal that channel to
	// complete it. We should also be able to query the operation directly.
	req := httptest.NewRequest("GET", location, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(test.t, 202, w.Code)
	require.Equal(test.t, location, w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location")))

	deployment := &rest.Deployment{}
	err := json.Unmarshal(w.Body.Bytes(), deployment)
	require.NoError(test.t, err)

	return deployment
}

func (test *test) ValidateDeploymentOperationComplete(location string) *rest.Deployment {
	// At this point deployment has started and is waiting for us to signal that channel to
	// complete it. We should also be able to query the operation directly.
	req := httptest.NewRequest("GET", location, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	if w.Code == 200 {
		deployment := &rest.Deployment{}
		err := json.Unmarshal(w.Body.Bytes(), deployment)
		require.NoError(test.t, err)
		return deployment
	}

	if w.Code == 204 {
		return nil
	}

	require.Equal(test.t, 200, w.Code, "Operation is still running")
	return nil
}

func (test *test) PollForOperationCompletion(id resources.ResourceID) rest.OperationStatus {
	// Poll with a backoff for completion
	status := rest.DeployingStatus
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", id.ID, nil)
		w := httptest.NewRecorder()

		test.server.Config.Handler.ServeHTTP(w, req)
		if w.Code == 200 {
			actual := &rest.Deployment{}
			err := json.Unmarshal(w.Body.Bytes(), actual)
			require.NoError(test.t, err)

			if rest.IsTeminalStatus(actual.Properties.ProvisioningState) {
				status = actual.Properties.ProvisioningState
				break
			}
		} else if w.Code == 404 {
			return rest.SuccededStatus
		} else {
			require.Equal(test.t, 200, w.Code)
		}

		time.Sleep(100 * time.Millisecond)
	}

	return status
}

func Test_GetApplication_NotFound(t *testing.T) {
	test := start(t)

	id := applicationID("myapp")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetApplication_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := applicationID("myapp")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Application{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListApplications_Empty(t *testing.T) {
	test := start(t)

	req := httptest.NewRequest("GET", applicationList().ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_ListApplications_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	req := httptest.NewRequest("GET", applicationList().ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	id := applicationID("myapp")
	expected := &rest.ResourceList{Value: []interface{}{
		&rest.Application{
			ResourceBase: rest.ResourceBase{
				ID:             id.ID,
				SubscriptionID: id.SubscriptionID,
				ResourceGroup:  id.ResourceGroup,
				Name:           id.QualifiedName(),
				Type:           id.Kind(),
			},
		},
	}}
	requireJSON(t, expected, w)
}

func Test_UpdateApplication_Create(t *testing.T) {
	test := start(t)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := applicationID("myapp")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code)

	actual := &rest.Application{}
	err = json.Unmarshal(w.Body.Bytes(), actual)
	require.NoError(t, err)

	expected := &rest.Application{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateApplication_Update(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := applicationID("myapp")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	actual := &rest.Application{}
	err = json.Unmarshal(w.Body.Bytes(), actual)
	require.NoError(t, err)

	expected := &rest.Application{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_DeleteApplication_NotFound(t *testing.T) {
	test := start(t)

	id := applicationID("myapp")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteApplication_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := applicationID("myapp")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_GetComponent_NoApplication(t *testing.T) {
	test := start(t)

	id := componentID("myapp", "A")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetComponent_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := componentID("myapp", "A")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetComponent_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	rev := test.DBCreateComponent("myapp", "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	id := componentID("myapp", "A")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Component{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Kind: "radius.dev/Test@v1alpha1",
		Properties: rest.ComponentProperties{
			Revision: rev,
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListComponents_NoApplication(t *testing.T) {
	test := start(t)

	id := componentList("myapp")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListComponents_Empty(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := componentList("myapp")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_ListComponents_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	rev := test.DBCreateComponent("myapp", "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	req := httptest.NewRequest("GET", componentList("myapp").ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	id := componentID("myapp", "A")
	expected := &rest.ResourceList{Value: []interface{}{
		&rest.Component{
			ResourceBase: rest.ResourceBase{
				ID:             id.ID,
				SubscriptionID: id.SubscriptionID,
				ResourceGroup:  id.ResourceGroup,
				Name:           id.QualifiedName(),
				Type:           id.Kind(),
			},
			Kind: "radius.dev/Test@v1alpha1",
			Properties: rest.ComponentProperties{
				Revision: rev,
			},
		},
	}}
	requireJSON(t, expected, w)
}

func Test_UpdateComponent_NoApplication(t *testing.T) {
	test := start(t)

	body := map[string]interface{}{
		"kind":       "radius.dev/Test@v1alpha1",
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := componentID("myapp", "A")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateComponent_Create(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	// simulate the operation to get the revision
	rev := test.DBCreateComponent("myapp", "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})
	test.DBDeleteComponent("myapp", "A")

	body := map[string]interface{}{
		"kind":       "radius.dev/Test@v1alpha1",
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := componentID("myapp", "A")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code)

	expected := &rest.Component{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Kind: "radius.dev/Test@v1alpha1",
		Properties: rest.ComponentProperties{
			Revision: rev,
		},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateComponent_UpdateNoOp(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	rev := test.DBCreateComponent("myapp", "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	body := map[string]interface{}{
		"kind":       "radius.dev/Test@v1alpha1",
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := componentID("myapp", "A")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Component{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Kind: "radius.dev/Test@v1alpha1",
		Properties: rest.ComponentProperties{
			Revision: rev,
		},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateComponent_Update(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	// Simulate the operation to get the revision
	test.DBCreateComponent("myapp", "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})
	rev := test.DBCreateComponent("myapp", "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{
		Run: map[string]interface{}{
			"cool": true,
		},
	})
	test.DBDeleteComponent("myapp", "A")
	test.DBCreateComponent("myapp", "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	body := map[string]interface{}{
		"kind": "radius.dev/Test@v1alpha1",
		"properties": map[string]interface{}{
			"run": map[string]interface{}{
				"cool": true,
			},
		},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := componentID("myapp", "A")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Component{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Kind: "radius.dev/Test@v1alpha1",
		Properties: rest.ComponentProperties{
			Revision: rev,
			Run: map[string]interface{}{
				"cool": true,
			},
		},
	}
	requireJSON(t, expected, w)
}

func Test_DeleteComponent_NoApplication(t *testing.T) {
	test := start(t)

	id := componentID("myapp", "A")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteComponent_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := componentID("myapp", "A")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteComponent_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	test.DBCreateComponent("myapp", "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	id := componentID("myapp", "default")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_GetDeployment_NoApplication(t *testing.T) {
	test := start(t)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetDeployment_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetDeployment_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	test.DBCreateDeployment("myapp", "default", db.DeploymentProperties{})

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: rest.DeploymentProperties{},
	}
	requireJSON(t, expected, w)
}

func Test_ListDeployments_NoApplication(t *testing.T) {
	test := start(t)

	id := deploymentList("myapp")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListDeployments_Empty(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := deploymentList("myapp")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_ListDeployments_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	test.DBCreateDeployment("myapp", "default", db.DeploymentProperties{})

	req := httptest.NewRequest("GET", deploymentList("myapp").ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	id := deploymentID("myapp", "default")
	expected := &rest.ResourceList{Value: []interface{}{
		&rest.Deployment{
			ResourceBase: rest.ResourceBase{
				ID:             id.ID,
				SubscriptionID: id.SubscriptionID,
				ResourceGroup:  id.ResourceGroup,
				Name:           id.QualifiedName(),
				Type:           id.Kind(),
			},
			Properties: rest.DeploymentProperties{},
		},
	}}
	requireJSON(t, expected, w)
}

func Test_UpdateDeployment_NoApplication(t *testing.T) {
	test := start(t)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateDeployment_Create(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to create a deployment. For now we don't validate any
	// of the data, and just simulate a successful deployment.
	complete := make(chan struct{})
	test.deploy.EXPECT().
		UpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c, d, e interface{}) error {
			select {
			case <-complete:
				return nil
			case <-time.After(10 * time.Second):
				return errors.New("Timeout!")
			}
		})

	test.DBCreateApplication("myapp", nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeployingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	status := test.PollForOperationCompletion(id)
	require.Equal(t, rest.SuccededStatus, status)
}

func Test_UpdateDeployment_Create_ValidationFailure(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to create a deployment. For now we don't validate any
	// of the data, and just simulate invalid data.
	complete := make(chan struct{})
	test.deploy.EXPECT().
		UpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c, d, e interface{}) error {
			select {
			case <-complete:
				return &deployment.CompositeError{
					Errors: []error{
						errors.New("deployment was invalid :("),
					},
				}
			case <-time.After(10 * time.Second):
				return errors.New("Timeout!")
			}
		})

	test.DBCreateApplication("myapp", nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeployingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	status := test.PollForOperationCompletion(id)
	require.Equal(t, rest.FailedStatus, status)
}

func Test_UpdateDeployment_Create_Failure(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to create a deployment. For now we don't validate any
	// of the data, and just simulate a failure.
	complete := make(chan struct{})
	test.deploy.EXPECT().
		UpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c, d, e interface{}) error {
			select {
			case <-complete:
				return &deployment.CompositeError{
					Errors: []error{
						errors.New("deployment failed :("),
					},
				}
			case <-time.After(10 * time.Second):
				return errors.New("Timeout!")
			}
		})

	test.DBCreateApplication("myapp", nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeployingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	status := test.PollForOperationCompletion(id)
	require.Equal(t, rest.FailedStatus, status)
}

func Test_UpdateDeployment_UpdateSuccess(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to update a deployment. For now we don't validate any
	// of the data, and just simulate a successful deployment.
	complete := make(chan struct{}, 1)
	test.deploy.EXPECT().
		UpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c, d, e interface{}) error {
			select {
			case <-complete:
				return nil
			case <-time.After(10 * time.Second):
				return errors.New("Timeout!")
			}
		})

	test.DBCreateApplication("myapp", nil)
	test.DBCreateDeployment("myapp", "default", db.DeploymentProperties{})
	rev := test.DBCreateComponent("myapp", "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	body := map[string]interface{}{
		"properties": rest.DeploymentProperties{
			Components: []rest.DeploymentComponent{
				{
					ComponentName: "A",
					Revision:      rev,
				},
			},
		},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeployingStatus,
			Components: []rest.DeploymentComponent{
				{
					ComponentName: "A",
					Revision:      rev,
				},
			},
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	status := test.PollForOperationCompletion(id)
	require.Equal(t, rest.SuccededStatus, status)
}

func Test_UpdateDeployment_UpdateNoOp(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	test.DBCreateDeployment("myapp", "default", db.DeploymentProperties{})

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: rest.DeploymentProperties{},
	}
	requireJSON(t, expected, w)
}

func Test_DeleteDeployment_NoApplication(t *testing.T) {
	test := start(t)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteDeployment_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteDeployment_Found_Success(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to delete a deployment. For now we don't validate any
	// of the data, and just simulate a successful deployment.
	complete := make(chan struct{}, 1)
	test.deploy.EXPECT().
		DeleteDeployment(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c interface{}) error {
			select {
			case <-complete:
				return nil
			case <-time.After(10 * time.Second):
				return errors.New("Timeout!")
			}
		})

	test.DBCreateApplication("myapp", nil)
	test.DBCreateDeployment("myapp", "default", db.DeploymentProperties{})

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeletingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	status := test.PollForOperationCompletion(id)
	require.Equal(t, rest.SuccededStatus, status)
}

func Test_DeleteDeployment_Found_ValidationFailure(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to delete a deployment. For now we don't validate any
	// of the data, and just simulate invalid data.
	complete := make(chan struct{}, 1)
	test.deploy.EXPECT().
		DeleteDeployment(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c interface{}) error {
			select {
			case <-complete:
				return &deployment.CompositeError{
					Errors: []error{
						errors.New("deletion was invalid :("),
					},
				}
			case <-time.After(10 * time.Second):
				return errors.New("Timeout!")
			}
		})

	test.DBCreateApplication("myapp", nil)
	test.DBCreateDeployment("myapp", "default", db.DeploymentProperties{})

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeletingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	status := test.PollForOperationCompletion(id)
	require.Equal(t, rest.FailedStatus, status)
}

func Test_DeleteDeployment_Found_Failed(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to delete a deployment. For now we don't validate any
	// of the data, and just simulate a failure.
	complete := make(chan struct{}, 1)
	test.deploy.EXPECT().
		DeleteDeployment(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c interface{}) error {
			select {
			case <-complete:
				return errors.New("deletion failed :(")
			case <-time.After(10 * time.Second):
				return errors.New("Timeout!")
			}
		})

	test.DBCreateApplication("myapp", nil)
	test.DBCreateDeployment("myapp", "default", db.DeploymentProperties{})

	id := deploymentID("myapp", "default")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeletingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	status := test.PollForOperationCompletion(id)
	require.Equal(t, rest.FailedStatus, status)
}

func Test_GetScope_NoApplication(t *testing.T) {
	test := start(t)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetScope_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetScope_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	test.DBCreateScope("myapp", "scope1", nil)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Scope{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListScopes_NoApplication(t *testing.T) {
	test := start(t)

	id := scopeList("myapp")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListScopes_Empty(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := scopeList("myapp")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_ListScopes_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	test.DBCreateScope("myapp", "scope1", nil)

	req := httptest.NewRequest("GET", scopeList("myapp").ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	id := scopeID("myapp", "scope1")
	expected := &rest.ResourceList{Value: []interface{}{
		&rest.Scope{
			ResourceBase: rest.ResourceBase{
				ID:             id.ID,
				SubscriptionID: id.SubscriptionID,
				ResourceGroup:  id.ResourceGroup,
				Name:           id.QualifiedName(),
				Type:           id.Kind(),
			},
		},
	}}
	requireJSON(t, expected, w)
}

func Test_UpdateScopes_NoApplication(t *testing.T) {
	test := start(t)

	body := map[string]interface{}{
		"kind":       "radius.dev/Test@v1alpha1",
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateScopes_Create(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	body := map[string]interface{}{
		"kind":       "radius.dev/Test@v1alpha1",
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code)

	expected := &rest.Scope{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateScopes_UpdateNoOp(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	test.DBCreateScope("myapp", "scope1", nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Scope{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateScopes_Update(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	test.DBCreateScope("myapp", "scope1", nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Scope{
		ResourceBase: rest.ResourceBase{
			ID:             id.ID,
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Name:           id.QualifiedName(),
			Type:           id.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_DeleteScope_NoApplication(t *testing.T) {
	test := start(t)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteScope_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteScope_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication("myapp", nil)
	test.DBCreateScope("myapp", "scope1", nil)

	id := scopeID("myapp", "scope1")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}
