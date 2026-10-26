/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authzcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/records"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	devRG     = "/planes/radius/local/resourceGroups/dev"
	prodRG    = "/planes/radius/local/resourceGroups/prod"
	devApp    = devRG + "/providers/Applications.Core/applications/app"
	prodApp   = prodRG + "/providers/Applications.Core/applications/app"
	devDeploy = devRG + "/providers/Microsoft.Resources/deployments/d1"
)

var (
	rpURL = &url.URL{Scheme: "https", Host: "applications-rp.radius-system:5443"}
	deURL = &url.URL{Scheme: "http", Host: "bicep-de.radius-system:6443"}
)

func setMode(t *testing.T, mode authz.Mode, mappings ...authz.NamespaceMapping) {
	t.Helper()
	authz.SetProcessOptions(authz.Options{Mode: mode, NamespaceMappings: mappings})
	t.Cleanup(func() { authz.SetProcessOptions(authz.Options{}) })
}

func write(t *testing.T, ctx context.Context, caller Caller, method, id string, headers map[string]string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, "https://ucp"+id, strings.NewReader(`{"properties":{}}`))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req.WithContext(WithCaller(ctx, caller))
}

func deniedCode(t *testing.T, err error) string {
	t.Helper()
	var denied *authz.DeniedError
	if errors.As(err, &denied) {
		return denied.Code
	}
	require.NoError(t, err)
	return ""
}

func TestAuthorizeProxyWrite(t *testing.T) {
	ctx := t.Context()
	user := Caller{Kind: CallerUser, Name: "alice"}
	engine := Caller{Kind: CallerComponent, Name: ComponentDeploymentEngine}
	rp := Caller{Kind: CallerComponent, Name: "applications-rp"}

	t.Run("off does nothing", func(t *testing.T) {
		setMode(t, authz.ModeOff)
		g, err := AuthorizeProxyWrite(ctx, inmemory.NewClient(), write(t, ctx, rp, http.MethodPut, devApp, nil), resources.MustParse(devApp), rpURL)
		require.NoError(t, err)
		require.Nil(t, g)
	})

	t.Run("reads are not checked", func(t *testing.T) {
		setMode(t, authz.ModeEnforce)
		g, err := AuthorizeProxyWrite(ctx, inmemory.NewClient(), write(t, ctx, rp, http.MethodGet, devApp, nil), resources.MustParse(devApp), rpURL)
		require.NoError(t, err)
		require.Nil(t, g)
	})

	t.Run("user deploy creates a record the engine binds to", func(t *testing.T) {
		setMode(t, authz.ModeEnforce)
		db := inmemory.NewClient()
		g, err := AuthorizeProxyWrite(ctx, db, write(t, ctx, user, http.MethodPut, devDeploy, nil), resources.MustParse(devDeploy), deURL)
		require.NoError(t, err)
		require.NotNil(t, g)

		store := records.NewStore(db)
		op, _, err := store.GetOperation(ctx, g.OperationID)
		require.NoError(t, err)
		require.Equal(t, ComponentDeploymentEngine, op.AssignedComponent)
		require.Equal(t, records.InputHash([]byte(`{"properties":{}}`)), op.InputHash)

		// Engine writes in the same resource group bind to the record (prototype shortcut).
		g2, err := AuthorizeProxyWrite(ctx, db, write(t, ctx, engine, http.MethodPut, devApp, nil), resources.MustParse(devApp), rpURL)
		require.NoError(t, err)
		require.Equal(t, g.RecordID, g2.RecordID)
		op2, _, err := store.GetOperation(ctx, g2.OperationID)
		require.NoError(t, err)
		require.Equal(t, "applications-rp", op2.AssignedComponent)

		// Engine writes in another resource group are outside the record.
		_, err = AuthorizeProxyWrite(ctx, db, write(t, ctx, engine, http.MethodPut, prodApp, nil), resources.MustParse(prodApp), rpURL)
		require.Equal(t, v1.CodeGrantScopeExceeded, deniedCode(t, err))

		// Reusing the dev record against prod is rejected.
		_, err = AuthorizeProxyWrite(ctx, db, write(t, ctx, engine, http.MethodPut, prodApp, map[string]string{records.HeaderRecord: g.RecordID}), resources.MustParse(prodApp), rpURL)
		require.Equal(t, v1.CodeGrantScopeExceeded, deniedCode(t, err))
	})

	t.Run("engine without a record", func(t *testing.T) {
		setMode(t, authz.ModeEnforce)
		_, err := AuthorizeProxyWrite(ctx, inmemory.NewClient(), write(t, ctx, engine, http.MethodPut, devApp, nil), resources.MustParse(devApp), rpURL)
		require.Equal(t, v1.CodeExecutionRecordNotActive, deniedCode(t, err))
	})

	t.Run("rp without a record", func(t *testing.T) {
		setMode(t, authz.ModeEnforce)
		_, err := AuthorizeProxyWrite(ctx, inmemory.NewClient(), write(t, ctx, rp, http.MethodPut, devApp, nil), resources.MustParse(devApp), rpURL)
		require.Equal(t, v1.CodeExecutionRecordNotActive, deniedCode(t, err))
	})

	t.Run("rp with an assigned operation creates a child deployment record", func(t *testing.T) {
		setMode(t, authz.ModeEnforce)
		db := inmemory.NewClient()
		g, err := AuthorizeProxyWrite(ctx, db, write(t, ctx, user, http.MethodPut, devApp, nil), resources.MustParse(devApp), rpURL)
		require.NoError(t, err)

		headers := map[string]string{records.HeaderRecord: g.RecordID, records.HeaderOperation: g.OperationID}
		g2, err := AuthorizeProxyWrite(ctx, db, write(t, ctx, rp, http.MethodPut, devDeploy, headers), resources.MustParse(devDeploy), deURL)
		require.NoError(t, err)
		child, _, err := records.NewStore(db).GetRecord(ctx, g2.RecordID)
		require.NoError(t, err)
		require.Equal(t, g.RecordID, child.ParentID)

		// Another component cannot use the operation.
		other := Caller{Kind: CallerComponent, Name: "dynamic-rp"}
		_, err = AuthorizeProxyWrite(ctx, db, write(t, ctx, other, http.MethodPut, devDeploy, headers), resources.MustParse(devDeploy), deURL)
		require.Equal(t, v1.CodeAuthorizationFailed, deniedCode(t, err))
	})

	t.Run("closed record", func(t *testing.T) {
		setMode(t, authz.ModeEnforce)
		db := inmemory.NewClient()
		g, err := AuthorizeProxyWrite(ctx, db, write(t, ctx, user, http.MethodPut, devDeploy, nil), resources.MustParse(devDeploy), deURL)
		require.NoError(t, err)
		require.NoError(t, records.NewStore(db).CloseRecord(ctx, g.RecordID, records.RecordRevoked))
		_, err = AuthorizeProxyWrite(ctx, db, write(t, ctx, engine, http.MethodPut, devApp, map[string]string{records.HeaderRecord: g.RecordID}), resources.MustParse(devApp), rpURL)
		require.Equal(t, v1.CodeExecutionRecordNotActive, deniedCode(t, err))
	})

	t.Run("controller mapping", func(t *testing.T) {
		setMode(t, authz.ModeEnforce, authz.NamespaceMapping{Namespace: "dev", ResourceGroup: devRG})
		db := inmemory.NewClient()
		ctrl := Caller{Kind: CallerComponent, Name: ComponentController}

		_, err := AuthorizeProxyWrite(ctx, db, write(t, ctx, ctrl, http.MethodPut, devDeploy, map[string]string{records.HeaderSourceNamespace: "dev"}), resources.MustParse(devDeploy), deURL)
		require.NoError(t, err)

		prodDeploy := prodRG + "/providers/Microsoft.Resources/deployments/d1"
		_, err = AuthorizeProxyWrite(ctx, db, write(t, ctx, ctrl, http.MethodPut, prodDeploy, map[string]string{records.HeaderSourceNamespace: "dev"}), resources.MustParse(prodDeploy), deURL)
		require.Equal(t, v1.CodeGrantScopeExceeded, deniedCode(t, err))

		_, err = AuthorizeProxyWrite(ctx, db, write(t, ctx, ctrl, http.MethodPut, devDeploy, map[string]string{records.HeaderSourceNamespace: "other"}), resources.MustParse(devDeploy), deURL)
		require.Equal(t, v1.CodeAuthorizationFailed, deniedCode(t, err))
	})

	t.Run("dryRun continues under an unbound record", func(t *testing.T) {
		setMode(t, authz.ModeDryRun)
		g, err := AuthorizeProxyWrite(ctx, inmemory.NewClient(), write(t, ctx, rp, http.MethodPut, devApp, nil), resources.MustParse(devApp), rpURL)
		require.NoError(t, err)
		require.NotNil(t, g)
	})
}

func TestCheckDataAccess(t *testing.T) {
	ctx := t.Context()
	store := records.NewStore(inmemory.NewClient())
	rec := store.NewRecord(records.Record{Subject: "user:alice", Submitter: "ucp", Actions: []string{"*"}, Targets: []string{devRG}})
	_, err := store.SaveRecord(ctx, rec, "")
	require.NoError(t, err)
	op := &records.Operation{RecordID: rec.ID, ResourceID: devApp, Action: "write", AssignedComponent: "applications-rp"}
	require.NoError(t, store.CreateOperation(ctx, op))
	rp := Caller{Kind: CallerComponent, Name: "applications-rp"}

	tests := []struct {
		name   string
		caller Caller
		opID   string
		id     string
		code   string
	}{
		{"allowed", rp, op.ID, devApp, ""},
		{"operation status", rp, op.ID, "/planes/radius/local/providers/Applications.Core/locations/global/operationStatuses/x", ""},
		{"no operation", rp, "", devApp, v1.CodeOperationNotAssigned},
		{"unknown operation", rp, records.NewOperationID(), devApp, v1.CodeOperationNotAssigned},
		{"other component", Caller{Kind: CallerComponent, Name: "dynamic-rp"}, op.ID, devApp, v1.CodeOperationNotAssigned},
		{"other resource", rp, op.ID, devRG + "/providers/Applications.Core/applications/other", v1.CodeGrantScopeExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := CheckDataAccess(ctx, store, tt.caller, tt.opID, tt.id)
			require.NoError(t, err)
			require.Equal(t, tt.code, code)
		})
	}

	require.NoError(t, store.CloseRecord(ctx, rec.ID, records.RecordClosed))
	code, err := CheckDataAccess(ctx, store, rp, op.ID, devApp)
	require.NoError(t, err)
	require.Equal(t, v1.CodeExecutionRecordNotActive, code)
}
