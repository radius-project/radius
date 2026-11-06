/*
Copyright 2026 The Radius Authors.

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

// Package secretstore provides API helpers shared by functional and upgrade tests.
package secretstore

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	core "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/test/rp"
)

// Client calls the public Applications.Core API through the workspace connection.
type Client struct {
	API        *core.SecretStoresClient
	Scope      string
	connection sdk.Connection
}

// New connects to the same API endpoint as the functional test workspace.
func New(t *testing.T, options rp.RPTestOptions) Client {
	t.Helper()
	api, err := core.NewSecretStoresClient(&tokencredentials.AnonymousCredential{}, sdk.NewClientOptions(options.Connection))
	require.NoError(t, err)
	return Client{API: api, Scope: strings.TrimPrefix(options.Workspace.Scope, "/"), connection: options.Connection}
}

// Resource constructs a generic store containing only a synthetic test value.
func Resource(reference, value string) core.SecretStoreResource {
	properties := &core.SecretStoreProperties{
		Type: new(core.SecretStoreDataTypeGeneric),
		Data: map[string]*core.SecretValueProperties{
			"token": {Encoding: new(core.SecretValueEncodingBase64)},
		},
	}
	if reference != "" {
		properties.Resource = new(reference)
	}
	if value != "" {
		properties.Data["token"].Value = new(value)
	}
	return core.SecretStoreResource{Location: new("global"), Properties: properties}
}

// Put waits for the create or update to finish.
func (c Client) Put(ctx context.Context, name string, resource core.SecretStoreResource) error {
	poller, err := c.API.BeginCreateOrUpdate(ctx, c.Scope, name, resource, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: time.Second})
	return err
}

// Patch waits for the partial update to finish.
func (c Client) Patch(ctx context.Context, name string, resource core.SecretStoreResource) error {
	poller, err := c.API.BeginUpdate(ctx, c.Scope, name, resource, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: time.Second})
	return err
}

// Delete exercises the synchronous controller's 200/204 contract directly. The generated
// SecretStores client incorrectly accepts only asynchronous 202 or absent-record 204.
func (c Client) Delete(ctx context.Context, name string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		strings.TrimRight(c.connection.Endpoint(), "/")+"/"+c.Scope+"/providers/Applications.Core/secretStores/"+url.PathEscape(name)+"?api-version="+core.Version, nil)
	if err != nil {
		return err
	}
	response, err := c.connection.Client().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return runtime.NewResponseError(response)
	}
	return nil
}

// RequireValue checks both metadata redaction and listSecrets without printing values.
func (c Client) RequireValue(t *testing.T, name, reference, value string) {
	t.Helper()
	result, err := c.API.Get(t.Context(), c.Scope, name, nil)
	require.NoError(t, err)
	require.NotNil(t, result.Properties)
	require.NotNil(t, result.Properties.Resource)
	require.Equal(t, reference, *result.Properties.Resource)
	require.NotNil(t, result.Properties.Data["token"])
	require.Nil(t, result.Properties.Data["token"].Value, "GET must redact values")
	secrets, err := c.API.ListSecrets(t.Context(), c.Scope, name, core.ListSecretsRequest{}, nil)
	require.NoError(t, err)
	require.NotNil(t, secrets.Data["token"])
	require.NotNil(t, secrets.Data["token"].Value)
	require.True(t, value == *secrets.Data["token"].Value, "listSecrets returned an unexpected value")
}

// RequireGone proves the Radius record is gone rather than relying on DELETE's status.
func (c Client) RequireGone(t *testing.T, name string) {
	t.Helper()
	_, err := c.API.Get(t.Context(), c.Scope, name, nil)
	var response *azcore.ResponseError
	require.ErrorAs(t, err, &response)
	require.Equal(t, http.StatusNotFound, response.StatusCode)
}
