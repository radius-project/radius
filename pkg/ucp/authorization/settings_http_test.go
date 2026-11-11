// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"net/http"
	"testing"
	"testing/synctest"

	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
)

func TestSettingsHTTPBootstrap(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := settingsReviewClient(t, authorizationv1.SubjectAccessReviewStatus{Allowed: true}, nil)
				guard, err := NewKubernetesSettingsGuard(client.AuthorizationV1().SubjectAccessReviews(), "kubernetes:cluster-a", nil)
				require.NoError(t, err)
				policy := initialPolicy()
				policy.Assignments = nil
				policy.Settings.Properties.Mode = &mode
				store, runtime := managementRuntime(t, inmemory.NewClient(), policy)
				store.settings = guard
				handler := managementHandler(runtime, "")
				actor := settingsActor()
				path := settingsID + managementVersion
				response := managementRequest(t, handler, &actor.Identity, http.MethodPut, path, `{"properties":{"mode":"DryRun"}}`)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Contains(t, response.Body.String(), `"generation":1`)
				require.Contains(t, response.Body.String(), `"mode":"DryRun"`)
				require.Len(t, client.Actions(), 3)
				read := managementRequest(t, handler, &actor.Identity, http.MethodGet, path, "")
				require.Equal(t, http.StatusOK, read.Code, read.Body.String())
				require.JSONEq(t, response.Body.String(), read.Body.String())
				for _, body := range []string{
					`{"properties":{"mode":"DryRun","generation":2}}`,
					`{"properties":{"mode":"DryRun","bootstrapAdministrators":[]}}`,
					`{"properties":{"mode":"DryRun"},"recovery":true}`,
					`{"properties":{"mode":"DryRun"}} {}`,
					`{"properties":{"mode":"Unknown"}}`,
				} {
					rejected := managementRequest(t, handler, &actor.Identity, http.MethodPut, path, body)
					require.Equal(t, http.StatusBadRequest, rejected.Code, rejected.Body.String())
					require.Equal(t, int64(1), runtime.State().Generation)
				}
				enforce := managementRequest(t, handler, &actor.Identity, http.MethodPut, path, `{"properties":{"mode":"Enforce"}}`)
				require.Equal(t, http.StatusServiceUnavailable, enforce.Code, enforce.Body.String())
				require.Equal(t, int64(1), runtime.State().Generation)
				deleted := managementRequest(t, handler, &actor.Identity, http.MethodDelete, path, "")
				require.Equal(t, http.StatusMethodNotAllowed, deleted.Code, deleted.Body.String())
				require.Equal(t, "GET, PUT", deleted.Header().Get("Allow"))
			})
		})
	}
}

func TestSettingsHTTPKubernetesDenial(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := settingsReviewClient(t, authorizationv1.SubjectAccessReviewStatus{Denied: true}, nil)
		guard, err := NewKubernetesSettingsGuard(client.AuthorizationV1().SubjectAccessReviews(), "kubernetes:cluster-a", nil)
		require.NoError(t, err)
		store, runtime := managementRuntime(t, inmemory.NewClient(), initialPolicy())
		store.settings = guard
		handler := managementHandler(runtime, "")
		actor := settingsActor()
		denied := managementRequest(t, handler, &actor.Identity, http.MethodPut, settingsID+managementVersion, "invalid body")
		require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
		require.Contains(t, denied.Body.String(), `"source":"Kubernetes"`)
		require.Len(t, client.Actions(), 1)
		missing := managementRequest(t, handler, nil, http.MethodPut, settingsID+managementVersion, "")
		require.Equal(t, http.StatusUnauthorized, missing.Code, missing.Body.String())
		require.Len(t, client.Actions(), 1)
		require.Zero(t, runtime.State().Generation)
	})
}
