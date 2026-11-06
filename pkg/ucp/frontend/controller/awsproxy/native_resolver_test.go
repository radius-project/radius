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

package awsproxy

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/retry"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

// resolvingFake is nativeType's fake handler with a Name in any case as one identifier: its one form is lowercase.
// It records the resolves and creates in order, and, at each create, the identifier the alias's record has.
type resolvingFake struct {
	*nativefake.Handler
	h   *nativeHarness
	log *[]string
	err error
}

func (r resolvingFake) ResolveIdentifier(ctx context.Context, region string, identifier string) (string, error) {
	*r.log = append(*r.log, "Resolve "+identifier)
	if r.err != nil {
		return "", r.err
	}
	return strings.ToLower(identifier), nil
}

func (r resolvingFake) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	*r.log = append(*r.log, "Create, record has "+r.h.record(nativePath).Properties.AWSIdentifier)
	properties := maps.Clone(request.Properties)
	properties["Name"] = strings.ToLower(properties["Name"].(string))
	request.Properties = properties
	return r.Handler.Create(ctx, request)
}

// withResolver registers nativeType's handler as a resolvingFake, which fails each resolve with err when it isn't nil,
// and returns the log.
func (h *nativeHarness) withResolver(err error) *[]string {
	h.t.Helper()
	log := &[]string{}
	catalog := schemas.Catalog{
		Native:      map[string]string{nativeType: nativeSchemas[nativeType], assignedType: nativeSchemas[assignedType]},
		HandWritten: map[string]string{handWrittenType: nativeSchemas[handWrittenType]},
		Metadata:    map[string]schemas.Metadata{},
	}
	registry, regErr := native.NewRegistry(catalog, resolvingFake{Handler: h.fake, h: h, log: log, err: err}, h.assigned, h.handWritten)
	require.NoError(h.t, regErr)
	h.native = registry
	return log
}

func (h *nativeHarness) getWithPost(body map[string]any) response {
	return h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
		return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
	}, http.MethodPost, providers+"AWS.Test/Native/:get", "", body)
}

// Test_Native_ResolveIdentifier deploys a template whose identifier isn't in the handler's one form. UCP resolves it
// before it reserves the record, so the record holds the resolved form, and another alias that names the resource in
// another form gets 409, whether it gives the identifier in its properties or as a name. GET and DELETE by alias use
// the record's identifier, so they resolve nothing.
func Test_Native_ResolveIdentifier(t *testing.T) {
	h := newNativeHarness(t)
	log := h.withResolver(nil)
	properties := nativeProperties(1)
	properties["Name"] = "App-Native"

	h.deploy(nativePath, properties)
	require.Equal(t, []string{"Resolve App-Native", "Create, record has app-native"}, (*log)[len(*log)-2:], "UCP resolves before it reserves the record")
	require.Equal(t, "app-native", h.record(nativePath).Properties.AWSIdentifier)

	other := strings.Replace(nativePath, "app-native", "other-native", 1)
	for _, body := range []map[string]any{
		{"properties": properties},
		{"name": "APP-NATIVE", "properties": nativeProperties(1)},
	} {
		resp := h.putBody(other, testOwner, body)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), `"app-native" is already managed under the alias "app-native"`)
		require.Nil(t, h.record(other))
	}

	resp := h.getWithPost(map[string]any{"name": "APP-NATIVE"})
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, providers+"AWS.Test/Native/app-native", resp.body["id"], "existing returns the resolved identifier")

	*log = nil
	resp = h.get(nativePath)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	resp = h.delete(nativePath, testOwner)
	require.Contains(t, []int{http.StatusAccepted, http.StatusNoContent}, resp.status, resp.raw)
	h.finish()
	require.Nil(t, h.record(nativePath))
	require.Empty(t, *log, "a request by alias resolves nothing")
}

// Test_Native_ResolveIdentifierFails fails the resolve. The PUT and POST /:get return the error as UCP returns a
// handler's error, 400 for a client failure and 500 otherwise, and nothing is saved or created.
func Test_Native_ResolveIdentifierFails(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{
		{"a client failure", &native.Failure{Message: "the identifier is not valid", ClientError: true, Outcome: native.OutcomeRejected}, http.StatusBadRequest},
		{"an AWS client error", nativefake.ClientError("AccessDenied", "denied"), http.StatusBadRequest},
		{"another error", errors.New("the account is unknown"), http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newNativeHarness(t)
			log := h.withResolver(tt.err)

			resp := h.put(nativePath, nativeProperties(1))
			require.Equal(t, tt.status, resp.status, resp.raw)
			resp = h.putBody(nativePath, testOwner, map[string]any{"name": "app-native", "properties": nativeProperties(1)})
			require.Equal(t, tt.status, resp.status, resp.raw)
			require.Nil(t, h.record(nativePath))
			require.Empty(t, h.fake.Calls(), "nothing reaches AWS")

			resp = h.getWithPost(map[string]any{"name": "app-native"})
			require.Equal(t, tt.status, resp.status, resp.raw)
			require.Len(t, *log, 3)
		})
	}
}
