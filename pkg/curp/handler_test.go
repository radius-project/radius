// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package curp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/mocks"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/curp/rest"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const baseURI = "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications"

type test struct {
	db     *mocks.MockCurpDB
	ctrl   *gomock.Controller
	k8s    *mocks.MockClient
	server *httptest.Server
}

func start(t *testing.T) *test {
	ctrl := gomock.NewController(t)
	db := mocks.NewInMemoryCurpDB(ctrl)
	k8s := mocks.NewMockClient(ctrl)
	arm := armauth.ArmConfig{
		SubscriptionID: "test-subscription",
		ResourceGroup:  "test-resource-group",
		Auth:           autorest.NewBasicAuthorizer("test", "p@ssw0rd"),
	}
	s := NewServer(db, arm, k8s, httptest.DefaultRemoteAddr, ServerOptions{Authenticate: false})

	server := httptest.NewServer(s.Handler)
	t.Cleanup(server.Close)

	return &test{
		db:     db,
		k8s:    k8s,
		ctrl:   ctrl,
		server: server,
	}
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

func createApplication(t *testing.T, test *test, app string) {
	uri := baseURI + "/" + app

	// we only require the presence of "properties" for an application
	a := map[string]interface{}{
		"properties": map[string]interface{}{},
	}
	b, err := json.Marshal(a)
	require.NoError(t, err)

	req := httptest.NewRequest("PUT", uri, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code)

	id := parseOrPanic(uri)

	expected := map[string]interface{}{
		"id":            id.ID,
		"name":          id.QualifiedName(),
		"resourceGroup": "test-group",
		"type":          id.Kind(),
		"properties":    map[string]interface{}{},
	}
	requireJSON(t, expected, w)
}

func deleteApplication(t *testing.T, test *test, app string) {
	uri := baseURI + "/" + app
	req := httptest.NewRequest("DELETE", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func createComponent(t *testing.T, test *test, app string, name string, kind string, properties map[string]interface{}) {
	uri := baseURI + "/" + app + "/" + "Components" + "/" + name

	// we only require the presence of "properties" for a component
	c := map[string]interface{}{
		"kind":       kind,
		"properties": properties,
	}
	b, err := json.Marshal(c)
	require.NoError(t, err)

	req := httptest.NewRequest("PUT", uri, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code)

	actual := &rest.Component{}
	err = json.Unmarshal(w.Body.Bytes(), actual)
	require.NoError(t, err)
	rev := actual.Properties.Revision
	require.NotEmpty(t, rev, "component should have a revision")

	updated := map[string]interface{}{}
	for k, v := range properties {
		updated[k] = v
	}
	updated["revision"] = rev

	id := parseOrPanic(uri)
	expected := map[string]interface{}{
		"id":            id.ID,
		"name":          id.QualifiedName(),
		"resourceGroup": "test-group",
		"type":          id.Kind(),
		"kind":          kind,
		"properties":    updated,
	}
	requireJSON(t, expected, w)
}

func deleteComponent(t *testing.T, test *test, app string, name string) {
	uri := baseURI + "/" + app + "/" + "Components" + "/" + name
	req := httptest.NewRequest("DELETE", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func createDeployment(t *testing.T, test *test, app string, name string, properties map[string]interface{}) {
	uri := baseURI + "/" + app + "/" + "Deployments" + "/" + name

	// we only require the presence of "properties" for a deployment
	c := map[string]interface{}{
		"properties": properties,
	}
	b, err := json.Marshal(c)
	require.NoError(t, err)

	req := httptest.NewRequest("PUT", uri, bytes.NewReader(b))
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code)

	actual := &rest.Deployment{}
	err = json.Unmarshal(w.Body.Bytes(), actual)
	require.NoError(t, err)

	// make a copy of the properties so we can edit
	b, err = json.Marshal(properties)
	require.NoError(t, err)
	updated := map[string]interface{}{}
	err = json.Unmarshal(b, &updated)
	require.NoError(t, err)

	require.Contains(t, updated, "components")
	require.IsType(t, []interface{}{}, updated["components"])
	components := (updated["components"]).([]interface{})

	// Update the "expected" with revisions
	for i := range actual.Properties.Components {
		n := actual.Properties.Components[i].ComponentName
		for j := range components {
			require.IsType(t, map[string]interface{}{}, components[j])

			name, ok := (components[j]).(map[string]interface{})["componentName"]
			require.True(t, ok, "component must have a name")

			if name != n {
				continue
			}

			_, ok = (components[j]).(map[string]interface{})["revision"]
			if ok {
				// already has a revision
				continue
			}

			require.NotEmpty(t, string(actual.Properties.Components[i].Revision))
			(components[j]).(map[string]interface{})["revision"] = actual.Properties.Components[i].Revision
		}
	}

	updated["components"] = components

	id := parseOrPanic(uri)
	expected := map[string]interface{}{
		"id":            id.ID,
		"name":          id.QualifiedName(),
		"resourceGroup": "test-group",
		"type":          id.Kind(),
		"properties":    updated,
	}
	requireJSON(t, expected, w)
}

func deleteDeployment(t *testing.T, test *test, app string, name string) {
	uri := baseURI + "/" + app + "/" + "Deployments" + "/" + name
	req := httptest.NewRequest("DELETE", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 204, w.Code)
}

func Test_ListApplications_Empty(t *testing.T) {
	test := start(t)

	uri := baseURI
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_ListComponents_NoApplication(t *testing.T) {
	test := start(t)

	uri := baseURI + "/testapp/Components"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)
}

func Test_ListComponents_Empty(t *testing.T) {
	test := start(t)

	createApplication(t, test, "testapp")

	uri := baseURI + "/testapp/Components"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_GetComponent_NoApplication(t *testing.T) {
	test := start(t)

	uri := baseURI + "/testapp/Components/A"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)
}

func Test_GetComponent_NotFound(t *testing.T) {
	test := start(t)

	createApplication(t, test, "testapp")

	uri := baseURI + "/testapp/Components/A"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)
}

func Test_ListDeployments_NoApplication(t *testing.T) {
	test := start(t)

	uri := baseURI + "/testapp/Deployments"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)
}

func Test_ListDeployments_Empty(t *testing.T) {
	test := start(t)

	createApplication(t, test, "testapp")

	uri := baseURI + "/testapp/Deployments"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_GetDeployment_NoApplication(t *testing.T) {
	test := start(t)

	uri := baseURI + "/testapp/Deployments/test-deploy"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)
}

func Test_GetDeployment_NotFound(t *testing.T) {
	test := start(t)

	createApplication(t, test, "testapp")

	uri := baseURI + "/testapp/Deployments/test-deploy"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)
}

func Test_ListScopes_NoApplication(t *testing.T) {
	test := start(t)

	uri := baseURI + "/testapp/Scopes"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)
}

func Test_ListScopes_Empty(t *testing.T) {
	test := start(t)

	createApplication(t, test, "testapp")

	uri := baseURI + "/testapp/Scopes"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	expected := &rest.ResourceList{Value: []interface{}{}}
	requireJSON(t, expected, w)
}

func Test_GetScope_NoApplication(t *testing.T) {
	test := start(t)

	uri := baseURI + "/testapp/Scopes/test-scope"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)
}

func Test_GetScope_NotFound(t *testing.T) {
	test := start(t)

	createApplication(t, test, "testapp")

	uri := baseURI + "/testapp/Scopes/test-scope"
	req := httptest.NewRequest("GET", uri, nil)
	w := httptest.NewRecorder()

	test.server.Config.Handler.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code)
}

func Test_E2E_frontendbackend(t *testing.T) {
	test := start(t)

	createApplication(t, test, "frontend-backend")
	createComponent(t, test, "frontend-backend", "frontend", "radius.dev/Container@v1alpha1", map[string]interface{}{
		"run": map[string]interface{}{
			"container": map[string]interface{}{
				"name":  "frontend",
				"image": "rynowak/frontend:0.5.0-dev",
			},
		},
		"provides": []interface{}{
			map[string]interface{}{
				"name":          "frontend",
				"kind":          "http",
				"containerPort": 80,
			},
		},
		"dependsOn": []interface{}{
			map[string]interface{}{
				"name": "backend",
				"kind": "http",
				"setEnv": map[string]interface{}{
					"SERVICE__BACKEND__HOST": "host",
					"SERVICE__BACKEND__PORT": "port",
				},
			},
		},
	})
	createComponent(t, test, "frontend-backend", "backend", "radius.dev/Container@v1alpha1", map[string]interface{}{
		"run": map[string]interface{}{
			"container": map[string]interface{}{
				"name":  "backend",
				"image": "rynowak/backend:0.5.0-dev",
			},
		},
		"provides": []interface{}{
			map[string]interface{}{
				"name":          "backend",
				"kind":          "http",
				"containerPort": 80,
			},
		},
	})

	type key struct {
		APIVersion string
		Kind       string
		Name       string
	}

	deployed := map[key]unstructured.Unstructured{}
	test.k8s.EXPECT().
		Patch(gomock.Any(), gomock.Any(), gomock.Any(), []interface{}{gomock.Any()}...).AnyTimes().
		DoAndReturn(func(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
			b, err := json.Marshal(obj)
			require.NoError(t, err)

			content := map[string]interface{}{}
			err = json.Unmarshal(b, &content)
			require.NoError(t, err)

			uns := unstructured.Unstructured{Object: content}
			key := key{APIVersion: uns.GetAPIVersion(), Kind: uns.GetKind(), Name: uns.GetName()}
			deployed[key] = uns
			return nil
		})

	createDeployment(t, test, "frontend-backend", "default", map[string]interface{}{
		"components": []interface{}{
			map[string]interface{}{
				"componentName": "radius/frontend-backend/frontend",
			},
			map[string]interface{}{
				"componentName": "backend",
			},
		},
	})

	// validate deployment
	require.Len(t, deployed, 5)
	require.Contains(t, deployed, key{APIVersion: "v1", Kind: "Namespace", Name: "frontend-backend"})
	require.Contains(t, deployed, key{APIVersion: "apps/v1", Kind: "Deployment", Name: "frontend"})
	require.Contains(t, deployed, key{APIVersion: "v1", Kind: "Service", Name: "frontend"})
	require.Contains(t, deployed, key{APIVersion: "apps/v1", Kind: "Deployment", Name: "backend"})
	require.Contains(t, deployed, key{APIVersion: "v1", Kind: "Service", Name: "backend"})

	ns := deployed[key{APIVersion: "v1", Kind: "Namespace", Name: "frontend-backend"}]
	require.Equal(t, "v1", ns.GetAPIVersion())

	w := deployed[key{APIVersion: "apps/v1", Kind: "Deployment", Name: "frontend"}]
	require.Equal(t, "frontend-backend", w.GetNamespace())
	d := &appsv1.Deployment{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(w.UnstructuredContent(), d)
	require.NoError(t, err)
	require.Len(t, d.Spec.Template.Spec.Containers, 1)
	c := d.Spec.Template.Spec.Containers[0]
	require.Len(t, c.Env, 2)

	for _, env := range c.Env {
		if env.Name == "SERVICE__BACKEND__HOST" {
			require.Equal(t, "backend.frontend-backend.svc.cluster.local", env.Value)
		} else if env.Name == "SERVICE__BACKEND__PORT" {
			require.Equal(t, "80", env.Value)
		} else {
			require.Fail(t, "unexpected name %v", env.Name)
		}
	}

	w = deployed[key{APIVersion: "v1", Kind: "Service", Name: "frontend"}]
	require.Equal(t, "frontend-backend", w.GetNamespace())

	w = deployed[key{APIVersion: "apps/v1", Kind: "Deployment", Name: "backend"}]
	require.Equal(t, "frontend-backend", w.GetNamespace())

	w = deployed[key{APIVersion: "v1", Kind: "Service", Name: "backend"}]
	require.Equal(t, "frontend-backend", w.GetNamespace())

	deleted := map[key]unstructured.Unstructured{}
	test.k8s.EXPECT().
		Delete(gomock.Any(), gomock.Any(), []interface{}{gomock.Any()}...).AnyTimes().
		DoAndReturn(func(ctx context.Context, obj runtime.Object, opts ...client.PatchOption) error {
			b, err := json.Marshal(obj)
			require.NoError(t, err)

			content := map[string]interface{}{}
			err = json.Unmarshal(b, &content)
			require.NoError(t, err)

			uns := unstructured.Unstructured{Object: content}
			key := key{APIVersion: uns.GetAPIVersion(), Kind: uns.GetKind(), Name: uns.GetName()}
			deleted[key] = uns
			return nil
		})

	deleteDeployment(t, test, "frontend-backend", "default")

	// validate undeployment
	require.Len(t, deleted, 4)
	require.Contains(t, deployed, key{APIVersion: "apps/v1", Kind: "Deployment", Name: "frontend"})
	require.Contains(t, deployed, key{APIVersion: "v1", Kind: "Service", Name: "frontend"})
	require.Contains(t, deployed, key{APIVersion: "apps/v1", Kind: "Deployment", Name: "backend"})
	require.Contains(t, deployed, key{APIVersion: "v1", Kind: "Service", Name: "backend"})

	w = deleted[key{APIVersion: "apps/v1", Kind: "Deployment", Name: "frontend"}]
	require.Equal(t, "frontend-backend", w.GetNamespace())

	w = deleted[key{APIVersion: "v1", Kind: "Service", Name: "frontend"}]
	require.Equal(t, "frontend-backend", w.GetNamespace())

	w = deleted[key{APIVersion: "apps/v1", Kind: "Deployment", Name: "backend"}]
	require.Equal(t, "frontend-backend", w.GetNamespace())

	w = deleted[key{APIVersion: "v1", Kind: "Service", Name: "backend"}]
	require.Equal(t, "frontend-backend", w.GetNamespace())

	deleteComponent(t, test, "frontend-backend", "frontend")
	deleteComponent(t, test, "frontend-backend", "backend")
	deleteApplication(t, test, "frontend-backend")
}
