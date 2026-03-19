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

package environment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, Client) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := NewClientWithOptions(StaticTokenSource("test-token"), server.Client(), server.URL)
	return server, client
}

func TestCreateEnvironment(t *testing.T) {
	var called bool
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/repos/owner/repo/environments/dev", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	})

	err := client.CreateEnvironment(context.Background(), "owner", "repo", "dev")
	require.NoError(t, err)
	assert.True(t, called)
}

func TestEnvironmentExists_True(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	exists, err := client.EnvironmentExists(context.Background(), "owner", "repo", "dev")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestEnvironmentExists_False(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	exists, err := client.EnvironmentExists(context.Background(), "owner", "repo", "dev")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestListEnvironments(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/environments", r.URL.Path)
		resp := map[string]interface{}{
			"environments": []map[string]string{
				{"name": "dev"},
				{"name": "staging"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})

	names, err := client.ListEnvironments(context.Background(), "owner", "repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, names)
}

func TestSetVariable_Create(t *testing.T) {
	callCount := 0
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == "/repos/owner/repo" {
			// getRepoID call
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int64{"id": 123}) //nolint:errcheck
			return
		}
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/repositories/123/environments/dev/variables")
		w.WriteHeader(http.StatusCreated)
	})

	err := client.SetVariable(context.Background(), "owner", "repo", "dev", "AWS_REGION", "us-east-1")
	require.NoError(t, err)
}

func TestSetVariable_Update(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int64{"id": 123}) //nolint:errcheck
			return
		}
		if r.Method == http.MethodPost {
			// Simulate variable already exists
			w.WriteHeader(http.StatusConflict)
			return
		}
		if r.Method == http.MethodPatch {
			assert.Contains(t, r.URL.Path, "/variables/AWS_REGION")
			w.WriteHeader(http.StatusOK)
			return
		}
	})

	err := client.SetVariable(context.Background(), "owner", "repo", "dev", "AWS_REGION", "us-east-1")
	require.NoError(t, err)
}

func TestGetVariables(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int64{"id": 123}) //nolint:errcheck
			return
		}
		resp := map[string]interface{}{
			"variables": []map[string]string{
				{"name": "AWS_REGION", "value": "us-east-1"},
				{"name": "AWS_IAM_ROLE_ARN", "value": "arn:aws:iam::123:role/test"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})

	vars, err := client.GetVariables(context.Background(), "owner", "repo", "dev")
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", vars["AWS_REGION"])
	assert.Equal(t, "arn:aws:iam::123:role/test", vars["AWS_IAM_ROLE_ARN"])
}

func TestDeleteEnvironment(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/repos/owner/repo/environments/dev", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.DeleteEnvironment(context.Background(), "owner", "repo", "dev")
	require.NoError(t, err)
}

func TestSetSecret(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int64{"id": 123}) //nolint:errcheck
			return
		}
		if r.URL.Path == "/repositories/123/environments/dev/secrets/public-key" {
			// Return a valid 32-byte NaCl public key (base64-encoded).
			// Using a well-known test key (all zeros) is fine for unit tests.
			resp := map[string]string{
				"key_id": "key-123",
				"key":    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck
			return
		}
		if r.Method == http.MethodPut {
			assert.Contains(t, r.URL.Path, "/secrets/CLIENT_SECRET")
			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload) //nolint:errcheck
			assert.NotEmpty(t, payload["encrypted_value"])
			assert.Equal(t, "key-123", payload["key_id"])
			w.WriteHeader(http.StatusCreated)
			return
		}
	})

	err := client.SetSecret(context.Background(), "owner", "repo", "dev", "CLIENT_SECRET", "super-secret")
	require.NoError(t, err)
}

func TestEncryptSecret(t *testing.T) {
	// A valid 32-byte key (all zeros, base64-encoded)
	pubKeyB64 := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	encrypted, err := encryptSecret(pubKeyB64, "my-secret")
	require.NoError(t, err)
	assert.NotEmpty(t, encrypted)
}

func TestStaticTokenSource(t *testing.T) {
	ts := StaticTokenSource("my-token")
	token, err := ts.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "my-token", token)
}
