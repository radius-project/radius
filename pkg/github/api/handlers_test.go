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

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/radius-project/radius/pkg/github/auth"
	"github.com/radius-project/radius/pkg/github/credential"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockService implements credential.Service for handler tests.
type mockService struct {
	createAWSResult   *credential.EnvironmentResult
	createAzureResult *credential.EnvironmentResult
	getStatusResult   *credential.EnvironmentResult
	createAWSErr      error
	createAzureErr    error
	getStatusErr      error
	deleteErr         error
	deletedEnvs       []string
}

func (m *mockService) CreateAWSEnvironment(_ context.Context, _, _ string, _ credential.AWSEnvironmentConfig) (*credential.EnvironmentResult, error) {
	return m.createAWSResult, m.createAWSErr
}

func (m *mockService) CreateAzureEnvironment(_ context.Context, _, _ string, _ credential.AzureEnvironmentConfig) (*credential.EnvironmentResult, error) {
	return m.createAzureResult, m.createAzureErr
}

func (m *mockService) DeleteEnvironment(_ context.Context, _, _, envName string) error {
	m.deletedEnvs = append(m.deletedEnvs, envName)
	return m.deleteErr
}

func (m *mockService) GetEnvironmentStatus(_ context.Context, _, _, _ string) (*credential.EnvironmentResult, error) {
	if m.getStatusErr != nil {
		return nil, m.getStatusErr
	}
	return m.getStatusResult, nil
}

// mockVerifier implements credential.Verifier for handler tests.
type mockVerifier struct {
	commitErr  error
	triggerErr error
	statusResult *credential.VerificationResult
	statusErr    error
}

func (m *mockVerifier) CommitVerificationWorkflow(_ context.Context, _, _, _, _ string) error {
	return m.commitErr
}

func (m *mockVerifier) TriggerVerification(_ context.Context, _, _, _ string) error {
	return m.triggerErr
}

func (m *mockVerifier) GetVerificationStatus(_ context.Context, _, _, _ string) (*credential.VerificationResult, error) {
	return m.statusResult, m.statusErr
}

func setupTestRouter(t *testing.T, svc credential.Service, v credential.Verifier) (*chi.Mux, *auth.SessionStore) {
	t.Helper()
	store := auth.NewSessionStore()
	store.Set("test-session", &auth.UserToken{Login: "testuser", AccessToken: "tok"})

	oauthConfig := &auth.OAuthConfig{
		ClientID:    "test-client",
		RedirectURL: "http://localhost/callback",
	}

	r := chi.NewRouter()
	RegisterRoutes(r, svc, v, store, oauthConfig)
	return r, store
}

func authenticatedRequest(t *testing.T, method, path string, body interface{}) *http.Request {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-session")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestCreateAWSEnvironment_Handler(t *testing.T) {
	svc := &mockService{
		createAWSResult: &credential.EnvironmentResult{
			EnvironmentName:          "dev",
			Provider:                 "aws",
			GitHubEnvironmentCreated: true,
			VariablesSet:             []string{"AWS_IAM_ROLE_ARN", "AWS_REGION"},
		},
	}
	router, _ := setupTestRouter(t, svc, &mockVerifier{})

	req := authenticatedRequest(t, http.MethodPost, "/api/repos/owner/repo/environments/aws", CreateAWSEnvironmentRequest{
		Name:    "dev",
		RoleARN: "arn:aws:iam::123:role/test",
		Region:  "us-east-1",
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp EnvironmentResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "dev", resp.Name)
	assert.Equal(t, "aws", resp.Provider)
	assert.True(t, resp.GitHubEnvironmentCreated)
}

func TestCreateAzureEnvironment_Handler(t *testing.T) {
	svc := &mockService{
		createAzureResult: &credential.EnvironmentResult{
			EnvironmentName:          "staging",
			Provider:                 "azure",
			GitHubEnvironmentCreated: true,
			VariablesSet:             []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID"},
		},
	}
	router, _ := setupTestRouter(t, svc, &mockVerifier{})

	req := authenticatedRequest(t, http.MethodPost, "/api/repos/owner/repo/environments/azure", CreateAzureEnvironmentRequest{
		Name:           "staging",
		TenantID:       "t",
		ClientID:       "c",
		SubscriptionID: "s",
		AuthType:       "WorkloadIdentity",
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestGetEnvironment_Handler(t *testing.T) {
	svc := &mockService{
		getStatusResult: &credential.EnvironmentResult{
			EnvironmentName:          "dev",
			Provider:                 "aws",
			GitHubEnvironmentCreated: true,
			VariablesSet:             []string{"AWS_IAM_ROLE_ARN"},
		},
	}
	router, _ := setupTestRouter(t, svc, &mockVerifier{})

	req := authenticatedRequest(t, http.MethodGet, "/api/repos/owner/repo/environments/dev", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp EnvironmentResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "dev", resp.Name)
}

func TestGetEnvironment_NotFound(t *testing.T) {
	svc := &mockService{getStatusResult: nil}
	router, _ := setupTestRouter(t, svc, &mockVerifier{})

	req := authenticatedRequest(t, http.MethodGet, "/api/repos/owner/repo/environments/nonexistent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteEnvironment_Handler(t *testing.T) {
	svc := &mockService{}
	router, _ := setupTestRouter(t, svc, &mockVerifier{})

	req := authenticatedRequest(t, http.MethodDelete, "/api/repos/owner/repo/environments/dev", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Contains(t, svc.deletedEnvs, "dev")
}

func TestVerifyCredentials_Handler(t *testing.T) {
	svc := &mockService{
		getStatusResult: &credential.EnvironmentResult{
			EnvironmentName:          "dev",
			Provider:                 "aws",
			GitHubEnvironmentCreated: true,
		},
	}
	v := &mockVerifier{}
	router, _ := setupTestRouter(t, svc, v)

	req := authenticatedRequest(t, http.MethodPost, "/api/repos/owner/repo/environments/dev/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)

	var resp VerificationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "pending", resp.Status)
	assert.Equal(t, "aws", resp.Provider)
}

func TestGetVerificationStatus_Handler(t *testing.T) {
	v := &mockVerifier{
		statusResult: &credential.VerificationResult{
			Status:         "success",
			Message:        "Cloud credentials verified successfully.",
			WorkflowRunURL: "https://github.com/owner/repo/actions/runs/123",
		},
	}
	router, _ := setupTestRouter(t, &mockService{getStatusResult: &credential.EnvironmentResult{Provider: "aws"}}, v)

	req := authenticatedRequest(t, http.MethodGet, "/api/repos/owner/repo/environments/dev/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp VerificationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
}

func TestUnauthenticated_Returns401(t *testing.T) {
	router, _ := setupTestRouter(t, &mockService{}, &mockVerifier{})

	req := httptest.NewRequest(http.MethodGet, "/api/repos/owner/repo/environments", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHealthCheck(t *testing.T) {
	router, _ := setupTestRouter(t, &mockService{}, &mockVerifier{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}
