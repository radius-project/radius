// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radrp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/Azure/radius/mocks"
	"github.com/Azure/radius/pkg/model/revision"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/deployment"
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const (
	TestSubscriptionID  = "test-subscription"
	TestResourceGroup   = "test-resourcegroup"
	TestApplicationName = "myapp"

	baseURI = "/subscriptions/test-subscription/resourceGroups/test-resourcegroup/providers/Microsoft.CustomProviders/resourceProviders/radius"
)

type test struct {
	t       *testing.T
	db      *mocks.MockRadrpDB
	ctrl    *gomock.Controller
	k8s     *mocks.MockClient
	server  *httptest.Server
	deploy  *mocks.MockDeploymentProcessor
	handler http.Handler
}

func start(t *testing.T) *test {
	ctrl := gomock.NewController(t)
	db := mocks.NewInMemoryRadrpDB(ctrl)
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
	h := rewriteHandler(t, server.Config.Handler)
	t.Cleanup(server.Close)

	return &test{
		t:       t,
		db:      db,
		k8s:     k8s,
		deploy:  deploy,
		ctrl:    ctrl,
		server:  server,
		handler: h,
	}
}

func applicationList() resources.ResourceID {
	return parseOrPanic(baseURI + "/Applications/")
}

func applicationID(applicationName string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s", applicationName))
}

func componentList(applicationName string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Components", applicationName))
}

func componentID(applicationName string, componentName string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Components/%s", applicationName, componentName))
}

func deploymentList(applicationName string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Deployments", applicationName))
}

func deploymentID(application string, deployment string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Deployments/%s", application, deployment))
}

func scopeList(applicationName string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Scopes", applicationName))
}

func scopeID(applicationName string, scopeName string) resources.ResourceID {
	return parseOrPanic(baseURI + fmt.Sprintf("/Applications/%s/Scopes/%s", applicationName, scopeName))
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

func (test *test) DBCreateApplication(applicationName string, properties map[string]interface{}) {
	applicationID := applicationID(applicationName)
	_, err := test.db.PatchApplication(context.TODO(), &db.ApplicationPatch{
		ResourceBase: db.ResourceBase{
			ID:             applicationID.ID,
			SubscriptionID: applicationID.SubscriptionID,
			ResourceGroup:  applicationID.ResourceGroup,
			Name:           applicationID.Name(),
			Type:           applicationID.Kind(),
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

func (test *test) DBCreateComponent(applicationName string, componentName string, kind string, properties db.ComponentProperties) revision.Revision {
	componentID := componentID(applicationName, componentName)
	applicationID, err := componentID.Application()
	require.NoError(test.t, err)

	if properties.Bindings == nil {
		properties.Bindings = map[string]db.ComponentBinding{}
	}

	component := &db.Component{
		ResourceBase: db.ResourceBase{
			ID:             componentID.ID,
			SubscriptionID: componentID.SubscriptionID,
			ResourceGroup:  componentID.ResourceGroup,
			Name:           componentID.Name(),
			Type:           componentID.Kind(),
		},
		Kind:       kind,
		Properties: properties,
	}

	previous := revision.Revision("")
	old, err := test.db.GetComponentByApplicationID(context.TODO(), applicationID, componentName)
	if err == db.ErrNotFound {
		// this is fine - we don't have a previous version to compare against
	} else if err != nil {
		require.NoError(test.t, err)
	} else if old != nil {
		previous = old.Revision
	}

	rev, err := revision.Compute(component, previous, []revision.Revision{})
	require.NoError(test.t, err)

	component.Revision = rev

	_, err = test.db.PatchComponentByApplicationID(context.TODO(), applicationID, componentName, component)
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

func (test *test) DBCreateDeployment(applicationName string, deploymentName string, properties db.DeploymentProperties) {
	deploymentID := deploymentID(applicationName, deploymentName)
	applicationID, err := deploymentID.Application()
	require.NoError(test.t, err)

	deployment := &db.Deployment{
		ResourceBase: db.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
		},
		Properties: properties,
	}

	_, err = test.db.PatchDeploymentByApplicationID(context.TODO(), applicationID, deploymentName, deployment)
	require.NoError(test.t, err)
}

func (test *test) DBDeleteDeployment(application string, deployment string) {
	id := deploymentID(application, deployment)
	a, err := id.Application()
	require.NoError(test.t, err)

	err = test.db.DeleteDeploymentByApplicationID(context.TODO(), a, deployment)
	require.NoError(test.t, err)
}

func (test *test) DBCreateScope(applicationName string, scopeName string, properties map[string]interface{}) {
	scopeID := scopeID(applicationName, scopeName)
	applicationID, err := scopeID.Application()
	require.NoError(test.t, err)

	scope := &db.Scope{
		ResourceBase: db.ResourceBase{
			ID:             scopeID.ID,
			SubscriptionID: scopeID.SubscriptionID,
			ResourceGroup:  scopeID.ResourceGroup,
			Name:           scopeID.Name(),
			Type:           scopeID.Kind(),
		},
		Properties: properties,
	}

	_, err = test.db.PatchScopeByApplicationID(context.TODO(), applicationID, scopeName, scope)
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

	test.handler.ServeHTTP(w, req)

	require.Equal(test.t, 202, w.Code)
	require.Equal(test.t, location, w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location")))

	deployment := &rest.Deployment{}
	err := json.Unmarshal(w.Body.Bytes(), deployment)
	require.NoError(test.t, err)

	return deployment
}

func (test *test) PollForTerminalStatus(id resources.ResourceID) (*httptest.ResponseRecorder, *rest.Deployment) {
	// Poll with a backoff for completion
	var w *httptest.ResponseRecorder
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", id.ID, nil)
		w = httptest.NewRecorder()

		test.handler.ServeHTTP(w, req)
		if w.Code == 404 {
			return w, nil
		}

		actual := rest.Deployment{}
		err := json.Unmarshal(w.Body.Bytes(), &actual)
		require.NoError(test.t, err)

		if rest.IsTeminalStatus(actual.Properties.ProvisioningState) {
			return w, &actual
		}

		time.Sleep(100 * time.Millisecond)
	}

	require.Fail(test.t, "operation is still not complete after 1 second. Body: %s", w.Body.String())
	return nil, nil
}

func (test *test) PollForSuccessfulPut(id resources.ResourceID) rest.Deployment {
	w, deployment := test.PollForTerminalStatus(id)
	require.Equal(test.t, 200, w.Code)

	return *deployment
}

func (test *test) PollForSuccessfulDelete(id resources.ResourceID) {
	w, _ := test.PollForTerminalStatus(id)
	require.Equal(test.t, 404, w.Code)
}

func (test *test) PollForFailedOperation(id resources.ResourceID, location string) (int, rest.Deployment, armerrors.ErrorResponse) {
	// We're going to check both the deployment object as well as the operation
	// only the operation can tell us the root cause.
	_, deployment := test.PollForTerminalStatus(id)

	// Now fetch the operation so we can get the error
	req := httptest.NewRequest("GET", location, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	armerr := armerrors.ErrorResponse{}
	err := json.Unmarshal(w.Body.Bytes(), &armerr)
	require.NoError(test.t, err)

	return w.Code, *deployment, armerr
}

func Test_GetApplication_NotFound(t *testing.T) {
	test := start(t)

	id := applicationID(TestApplicationName)
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetApplication_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	applicationID := applicationID(TestApplicationName)
	req := httptest.NewRequest("GET", applicationID.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Application{
		ResourceBase: rest.ResourceBase{
			ID:             applicationID.ID,
			SubscriptionID: applicationID.SubscriptionID,
			ResourceGroup:  applicationID.ResourceGroup,
			Name:           applicationID.Name(),
			Type:           applicationID.Kind(),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListApplications_Empty(t *testing.T) {
	test := start(t)

	req := httptest.NewRequest("GET", applicationList().ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_ListApplications_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	req := httptest.NewRequest("GET", applicationList().ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	applicationID := applicationID(TestApplicationName)
	expected := &rest.ResourceList{Value: []interface{}{
		&rest.Application{
			ResourceBase: rest.ResourceBase{
				ID:             applicationID.ID,
				SubscriptionID: applicationID.SubscriptionID,
				ResourceGroup:  applicationID.ResourceGroup,
				Name:           applicationID.Name(),
				Type:           applicationID.Kind(),
			},
		},
	}}
	requireJSON(t, expected, w)
}

func rewriteHandler(t *testing.T, h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		logger, err := radlogger.NewTestLogger(t)
		if err != nil {
			t.Error(err)
			h.ServeHTTP(w, r.WithContext(context.Background()))
			return
		}
		ctx := logr.NewContext(context.Background(), logger)
		h.ServeHTTP(w, r.WithContext(ctx))
	}

	return http.HandlerFunc(fn)
}

func Test_UpdateApplication_Create(t *testing.T) {
	test := start(t)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	applicationID := applicationID(TestApplicationName)
	req := httptest.NewRequest("PUT", applicationID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code)

	actual := &rest.Application{}
	err = json.Unmarshal(w.Body.Bytes(), actual)
	require.NoError(t, err)

	expected := &rest.Application{
		ResourceBase: rest.ResourceBase{
			ID:             applicationID.ID,
			SubscriptionID: applicationID.SubscriptionID,
			ResourceGroup:  applicationID.ResourceGroup,
			Name:           applicationID.Name(),
			Type:           applicationID.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateApplication_Update(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	applicationID := applicationID(TestApplicationName)
	req := httptest.NewRequest("PUT", applicationID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	actual := &rest.Application{}
	err = json.Unmarshal(w.Body.Bytes(), actual)
	require.NoError(t, err)

	expected := &rest.Application{
		ResourceBase: rest.ResourceBase{
			ID:             applicationID.ID,
			SubscriptionID: applicationID.SubscriptionID,
			ResourceGroup:  applicationID.ResourceGroup,
			Name:           applicationID.Name(),
			Type:           applicationID.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_DeleteApplication_NotFound(t *testing.T) {
	test := start(t)

	id := applicationID(TestApplicationName)
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteApplication_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := applicationID(TestApplicationName)
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_GetComponent_NoApplication(t *testing.T) {
	test := start(t)

	id := componentID(TestApplicationName, "A")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetComponent_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := componentID(TestApplicationName, "A")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetComponent_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	rev := test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	componentID := componentID(TestApplicationName, "A")
	req := httptest.NewRequest("GET", componentID.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Component{
		ResourceBase: rest.ResourceBase{
			ID:             componentID.ID,
			SubscriptionID: componentID.SubscriptionID,
			ResourceGroup:  componentID.ResourceGroup,
			Name:           componentID.Name(),
			Type:           componentID.Kind(),
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

	id := componentList(TestApplicationName)
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListComponents_Empty(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := componentList(TestApplicationName)
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_ListComponents_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	rev := test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	req := httptest.NewRequest("GET", componentList(TestApplicationName).ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	componentID := componentID(TestApplicationName, "A")
	expected := &rest.ResourceList{Value: []interface{}{
		&rest.Component{
			ResourceBase: rest.ResourceBase{
				ID:             componentID.ID,
				SubscriptionID: componentID.SubscriptionID,
				ResourceGroup:  componentID.ResourceGroup,
				Name:           componentID.Name(),
				Type:           componentID.Kind(),
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

	id := componentID(TestApplicationName, "A")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateComponent_Create(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	// simulate the operation to get the revision
	rev := test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})
	test.DBDeleteComponent(TestApplicationName, "A")

	body := map[string]interface{}{
		"kind":       "radius.dev/Test@v1alpha1",
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	componentID := componentID(TestApplicationName, "A")
	req := httptest.NewRequest("PUT", componentID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code)

	expected := &rest.Component{
		ResourceBase: rest.ResourceBase{
			ID:             componentID.ID,
			SubscriptionID: componentID.SubscriptionID,
			ResourceGroup:  componentID.ResourceGroup,
			Name:           componentID.Name(),
			Type:           componentID.Kind(),
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

	test.DBCreateApplication(TestApplicationName, nil)
	rev := test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	body := map[string]interface{}{
		"kind":       "radius.dev/Test@v1alpha1",
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	componentID := componentID(TestApplicationName, "A")
	req := httptest.NewRequest("PUT", componentID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Component{
		ResourceBase: rest.ResourceBase{
			ID:             componentID.ID,
			SubscriptionID: componentID.SubscriptionID,
			ResourceGroup:  componentID.ResourceGroup,
			Name:           componentID.Name(),
			Type:           componentID.Kind(),
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

	test.DBCreateApplication(TestApplicationName, nil)
	// Simulate the operation to get the revision
	test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})
	rev := test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{
		Run: map[string]interface{}{
			"cool": true,
		},
	})
	test.DBDeleteComponent(TestApplicationName, "A")
	test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

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

	componentID := componentID(TestApplicationName, "A")
	req := httptest.NewRequest("PUT", componentID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Component{
		ResourceBase: rest.ResourceBase{
			ID:             componentID.ID,
			SubscriptionID: componentID.SubscriptionID,
			ResourceGroup:  componentID.ResourceGroup,
			Name:           componentID.Name(),
			Type:           componentID.Kind(),
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

	id := componentID(TestApplicationName, "A")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteComponent_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := componentID(TestApplicationName, "A")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteComponent_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

	id := componentID(TestApplicationName, "default")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_GetDeployment_NoApplication(t *testing.T) {
	test := start(t)

	id := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetDeployment_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetDeployment_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateDeployment(TestApplicationName, "default", db.DeploymentProperties{})

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("GET", deploymentID.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
		},
		Properties: rest.DeploymentProperties{},
	}
	requireJSON(t, expected, w)
}

func Test_ListDeployments_NoApplication(t *testing.T) {
	test := start(t)

	id := deploymentList(TestApplicationName)
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListDeployments_Empty(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := deploymentList(TestApplicationName)
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_ListDeployments_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateDeployment(TestApplicationName, "default", db.DeploymentProperties{})

	req := httptest.NewRequest("GET", deploymentList(TestApplicationName).ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	deploymentID := deploymentID(TestApplicationName, "default")
	expected := &rest.ResourceList{Value: []interface{}{
		&rest.Deployment{
			ResourceBase: rest.ResourceBase{
				ID:             deploymentID.ID,
				SubscriptionID: deploymentID.SubscriptionID,
				ResourceGroup:  deploymentID.ResourceGroup,
				Name:           deploymentID.Name(),
				Type:           deploymentID.Kind(),
			},
			Properties: rest.DeploymentProperties{},
		},
	}}
	requireJSON(t, expected, w)
}

func Test_UpdateDeployment_NoApplication(t *testing.T) {
	test := start(t)

	body := map[string]interface{}{
		"properties": map[string]interface{}{
			"components": []interface{}{},
		},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	id := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
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
				return errors.New("timed out")
			}
		})

	test.DBCreateApplication(TestApplicationName, nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{
			"components": []interface{}{},
		},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("PUT", deploymentID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeployingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	actual := test.PollForSuccessfulPut(deploymentID)
	require.Equal(t, rest.SuccededStatus, actual.Properties.ProvisioningState)
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
				return errors.New("timed out")
			}
		})

	test.DBCreateApplication(TestApplicationName, nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{
			"components": []interface{}{},
		},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("PUT", deploymentID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeployingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	code, actual, armerr := test.PollForFailedOperation(deploymentID, location)
	require.Equal(t, rest.FailedStatus, actual.Properties.ProvisioningState)

	require.Equal(t, 400, code)
	require.NotNil(t, armerr)
	require.Equal(t, armerrors.Invalid, armerr.Error.Code)
	require.Equal(t, "deployment was invalid :(", armerr.Error.Message)
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
				return errors.New("deployment failed :(")
			case <-time.After(10 * time.Second):
				return errors.New("Timeout!")
			}
		})

	test.DBCreateApplication(TestApplicationName, nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{
			"components": []interface{}{},
		},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("PUT", deploymentID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeployingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	code, actual, armerr := test.PollForFailedOperation(deploymentID, location)
	require.Equal(t, rest.FailedStatus, actual.Properties.ProvisioningState)

	require.Equal(t, 500, code)
	require.NotNil(t, armerr)
	require.Equal(t, armerrors.Internal, armerr.Error.Code)
	require.Equal(t, "deployment failed :(", armerr.Error.Message)
}

// Regressiont test for: https://github.com/Azure/radius/issues/375
func Test_UpdateDeployment_FailureCanBeRetried(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to create a deployment. For now we don't validate any
	// of the data, and just simulate a failure.
	fail := true
	complete := make(chan struct{})
	test.deploy.EXPECT().
		UpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(a, b, c, d, e interface{}) error {
			select {
			case <-complete:
				if fail {
					return errors.New("deployment failed :(")
				}

				return nil
			case <-time.After(10 * time.Second):
				return errors.New("Timeout!")
			}
		})

	test.DBCreateApplication(TestApplicationName, nil)
	rev := test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

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

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("PUT", deploymentID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
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

	code, actual, armerr := test.PollForFailedOperation(deploymentID, location)
	require.Equal(t, rest.FailedStatus, actual.Properties.ProvisioningState)

	require.Equal(t, 500, code)
	require.NotNil(t, armerr)
	require.Equal(t, armerrors.Internal, armerr.Error.Code)
	require.Equal(t, "deployment failed :(", armerr.Error.Message)

	// Now retry and it should succeed
	fail = false

	req = httptest.NewRequest("PUT", deploymentID.ID, bytes.NewReader(b))
	w = httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location = w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	deployment := test.PollForSuccessfulPut(deploymentID)
	require.Equal(t, rest.SuccededStatus, deployment.Properties.ProvisioningState)
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
				return errors.New("timed out")
			}
		})

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateDeployment(TestApplicationName, "default", db.DeploymentProperties{})
	rev := test.DBCreateComponent(TestApplicationName, "A", "radius.dev/Test@v1alpha1", db.ComponentProperties{})

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

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("PUT", deploymentID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
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

	deployment := test.PollForSuccessfulPut(deploymentID)
	require.Equal(t, rest.SuccededStatus, deployment.Properties.ProvisioningState)
}

func Test_UpdateDeployment_UpdateNoOp(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateDeployment(TestApplicationName, "default", db.DeploymentProperties{})

	body := map[string]interface{}{
		"properties": map[string]interface{}{
			"components": []interface{}{},
		},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("PUT", deploymentID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
		},
		Properties: rest.DeploymentProperties{},
	}
	requireJSON(t, expected, w)
}

func Test_DeleteDeployment_NoApplication(t *testing.T) {
	test := start(t)

	id := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteDeployment_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteDeployment_Found_Success(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to delete a deployment. For now we don't validate any
	// of the data, and just simulate a successful deployment.
	complete := make(chan struct{}, 1)
	test.deploy.EXPECT().
		DeleteDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c, d interface{}) error {
			select {
			case <-complete:
				return nil
			case <-time.After(10 * time.Second):
				return errors.New("timed out")
			}
		})

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateDeployment(TestApplicationName, "default", db.DeploymentProperties{})

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("DELETE", deploymentID.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeletingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	test.PollForSuccessfulDelete(deploymentID)
}

func Test_DeleteDeployment_Found_ValidationFailure(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to delete a deployment. For now we don't validate any
	// of the data, and just simulate invalid data.
	complete := make(chan struct{}, 1)
	test.deploy.EXPECT().
		DeleteDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c, d interface{}) error {
			select {
			case <-complete:
				return &deployment.CompositeError{
					Errors: []error{
						errors.New("deletion was invalid :("),
					},
				}
			case <-time.After(10 * time.Second):
				return errors.New("timed out")
			}
		})

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateDeployment(TestApplicationName, "default", db.DeploymentProperties{})

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("DELETE", deploymentID.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeletingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	code, actual, armerr := test.PollForFailedOperation(deploymentID, location)
	require.Equal(t, rest.FailedStatus, actual.Properties.ProvisioningState)

	require.Equal(t, 400, code)
	require.NotNil(t, armerr)
	require.Equal(t, armerrors.Invalid, armerr.Error.Code)
	require.Equal(t, "deletion was invalid :(", armerr.Error.Message)
}

func Test_DeleteDeployment_Found_Failed(t *testing.T) {
	test := start(t)

	// This test will call through to the deployment processor to delete a deployment. For now we don't validate any
	// of the data, and just simulate a failure.
	complete := make(chan struct{}, 1)
	test.deploy.EXPECT().
		DeleteDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(a, b, c, d interface{}) error {
			select {
			case <-complete:
				return errors.New("deletion failed :(")
			case <-time.After(10 * time.Second):
				return errors.New("timed out")
			}
		})

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateDeployment(TestApplicationName, "default", db.DeploymentProperties{})

	deploymentID := deploymentID(TestApplicationName, "default")
	req := httptest.NewRequest("DELETE", deploymentID.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	location := w.Result().Header.Get(textproto.CanonicalMIMEHeaderKey("Location"))
	require.NotEmpty(t, location)

	expected := &rest.Deployment{
		ResourceBase: rest.ResourceBase{
			ID:             deploymentID.ID,
			SubscriptionID: deploymentID.SubscriptionID,
			ResourceGroup:  deploymentID.ResourceGroup,
			Name:           deploymentID.Name(),
			Type:           deploymentID.Kind(),
		},
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.DeletingStatus,
		},
	}
	requireJSON(t, expected, w)

	test.ValidateDeploymentOperationInProgress(location)

	// Now unblock the completion of the deployment
	complete <- struct{}{}

	code, actual, armerr := test.PollForFailedOperation(deploymentID, location)
	require.Equal(t, rest.FailedStatus, actual.Properties.ProvisioningState)

	require.Equal(t, 500, code)
	require.NotNil(t, armerr)
	require.Equal(t, armerrors.Internal, armerr.Error.Code)
	require.Equal(t, "deletion failed :(", armerr.Error.Message)
}

func Test_GetScope_NoApplication(t *testing.T) {
	test := start(t)

	id := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetScope_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  id.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_GetScope_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateScope(TestApplicationName, "scope1", nil)

	scopeID := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("GET", scopeID.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Scope{
		ResourceBase: rest.ResourceBase{
			ID:             scopeID.ID,
			SubscriptionID: scopeID.SubscriptionID,
			ResourceGroup:  scopeID.ResourceGroup,
			Name:           scopeID.Name(),
			Type:           scopeID.Kind(),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListScopes_NoApplication(t *testing.T) {
	test := start(t)

	id := scopeList(TestApplicationName)
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_ListScopes_Empty(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := scopeList(TestApplicationName)
	req := httptest.NewRequest("GET", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_ListScopes_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateScope(TestApplicationName, "scope1", nil)

	req := httptest.NewRequest("GET", scopeList(TestApplicationName).ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	scopeID := scopeID(TestApplicationName, "scope1")
	expected := &rest.ResourceList{Value: []interface{}{
		&rest.Scope{
			ResourceBase: rest.ResourceBase{
				ID:             scopeID.ID,
				SubscriptionID: scopeID.SubscriptionID,
				ResourceGroup:  scopeID.ResourceGroup,
				Name:           scopeID.Name(),
				Type:           scopeID.Kind(),
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

	id := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("PUT", id.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)

	a, err := id.Application()
	require.NoError(t, err)

	expected := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.NotFound,
			Target:  a.ID,
			Message: fmt.Sprintf("the resource with id '%s' was not found", a.ID),
		},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateScopes_Create(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	body := map[string]interface{}{
		"kind":       "radius.dev/Test@v1alpha1",
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	scopeID := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("PUT", scopeID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code)

	expected := &rest.Scope{
		ResourceBase: rest.ResourceBase{
			ID:             scopeID.ID,
			SubscriptionID: scopeID.SubscriptionID,
			ResourceGroup:  scopeID.ResourceGroup,
			Name:           scopeID.Name(),
			Type:           scopeID.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateScopes_UpdateNoOp(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateScope(TestApplicationName, "scope1", nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	scopeID := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("PUT", scopeID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Scope{
		ResourceBase: rest.ResourceBase{
			ID:             scopeID.ID,
			SubscriptionID: scopeID.SubscriptionID,
			ResourceGroup:  scopeID.ResourceGroup,
			Name:           scopeID.Name(),
			Type:           scopeID.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_UpdateScopes_Update(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateScope(TestApplicationName, "scope1", nil)

	body := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	scopeID := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("PUT", scopeID.ID, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.Scope{
		ResourceBase: rest.ResourceBase{
			ID:             scopeID.ID,
			SubscriptionID: scopeID.SubscriptionID,
			ResourceGroup:  scopeID.ResourceGroup,
			Name:           scopeID.Name(),
			Type:           scopeID.Kind(),
		},
		Properties: map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func Test_DeleteScope_NoApplication(t *testing.T) {
	test := start(t)

	id := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteScope_NotFound(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)

	id := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_DeleteScope_Found(t *testing.T) {
	test := start(t)

	test.DBCreateApplication(TestApplicationName, nil)
	test.DBCreateScope(TestApplicationName, "scope1", nil)

	id := scopeID(TestApplicationName, "scope1")
	req := httptest.NewRequest("DELETE", id.ID, nil)
	w := httptest.NewRecorder()

	test.handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}
