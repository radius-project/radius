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
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/radius-project/radius/pkg/github/auth"
	"github.com/radius-project/radius/pkg/github/credential"
)

type handlers struct {
	svc      credential.Service
	verifier credential.Verifier
}

func (h *handlers) createAWSEnvironment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req CreateAWSEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.svc.CreateAWSEnvironment(r.Context(), owner, repo, credential.AWSEnvironmentConfig{
		EnvironmentName: req.Name,
		RoleARN:         req.RoleARN,
		Region:          req.Region,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toEnvironmentResponse(result))
}

func (h *handlers) createAzureEnvironment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req CreateAzureEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.svc.CreateAzureEnvironment(r.Context(), owner, repo, credential.AzureEnvironmentConfig{
		EnvironmentName: req.Name,
		TenantID:        req.TenantID,
		ClientID:        req.ClientID,
		SubscriptionID:  req.SubscriptionID,
		ResourceGroup:   req.ResourceGroup,
		AuthType:        req.AuthType,
		ClientSecret:    req.ClientSecret,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toEnvironmentResponse(result))
}

func (h *handlers) listEnvironments(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	result, err := h.svc.GetEnvironmentStatus(r.Context(), owner, repo, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if result == nil {
		writeJSON(w, http.StatusOK, []EnvironmentResponse{})
		return
	}

	writeJSON(w, http.StatusOK, []EnvironmentResponse{*toEnvironmentResponse(result)})
}

func (h *handlers) getEnvironment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "name")

	result, err := h.svc.GetEnvironmentStatus(r.Context(), owner, repo, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result == nil {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}

	writeJSON(w, http.StatusOK, toEnvironmentResponse(result))
}

func (h *handlers) deleteEnvironment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "name")

	if err := h.svc.DeleteEnvironment(r.Context(), owner, repo, name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// verifyCredentials commits a verification workflow to the repo (if needed),
// then triggers it via workflow_dispatch to test cloud access.
func (h *handlers) verifyCredentials(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "name")

	// Determine the provider from the environment variables.
	result, err := h.svc.GetEnvironmentStatus(r.Context(), owner, repo, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result == nil {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if result.Provider == "" {
		writeError(w, http.StatusBadRequest, "no cloud provider configured for this environment")
		return
	}

	// Commit the verification workflow file to the repo (idempotent).
	if err := h.verifier.CommitVerificationWorkflow(r.Context(), owner, repo, result.Provider, name); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit verification workflow: "+err.Error())
		return
	}

	// Trigger the verification.
	if err := h.verifier.TriggerVerification(r.Context(), owner, repo, name); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to trigger verification: "+err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, VerificationResponse{
		Provider: result.Provider,
		Status:   "pending",
		Message:  "Verification workflow triggered. Poll the status endpoint for results.",
	})
}

// getVerificationStatus returns the current status of the credential verification workflow.
func (h *handlers) getVerificationStatus(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "name")

	vr, err := h.verifier.GetVerificationStatus(r.Context(), owner, repo, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, VerificationResponse{
		Status:         vr.Status,
		Message:        vr.Message,
		WorkflowRunURL: vr.WorkflowRunURL,
	})
}

// handleLogin initiates the GitHub OAuth flow.
func handleLogin(oauthConfig *auth.OAuthConfig, _ *auth.SessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := auth.GenerateState()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate state")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_state",
			Value:    state,
			Path:     "/",
			MaxAge:   600,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil,
		})

		http.Redirect(w, r, oauthConfig.AuthorizationURL(state), http.StatusFound)
	}
}

// handleCallback completes the GitHub OAuth flow.
func handleCallback(oauthConfig *auth.OAuthConfig, sessionStore *auth.SessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stateCookie, err := r.Cookie("oauth_state")
		if err != nil || stateCookie.Value == "" {
			writeError(w, http.StatusBadRequest, "missing OAuth state cookie")
			return
		}
		if r.URL.Query().Get("state") != stateCookie.Value {
			writeError(w, http.StatusBadRequest, "OAuth state mismatch")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:   "oauth_state",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})

		code := r.URL.Query().Get("code")
		if code == "" {
			writeError(w, http.StatusBadRequest, "missing authorization code")
			return
		}

		userToken, err := oauthConfig.ExchangeCode(r.Context(), code)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to exchange OAuth code")
			return
		}

		sessionID, err := auth.GenerateState()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate session")
			return
		}
		sessionStore.Set(sessionID, userToken)

		http.SetCookie(w, &http.Cookie{
			Name:     auth.SessionCookieName,
			Value:    sessionID,
			Path:     "/",
			MaxAge:   86400,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil,
		})

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func toEnvironmentResponse(result *credential.EnvironmentResult) *EnvironmentResponse {
	return &EnvironmentResponse{
		Name:                     result.EnvironmentName,
		Provider:                 result.Provider,
		GitHubEnvironmentCreated: result.GitHubEnvironmentCreated,
		VariablesSet:             result.VariablesSet,
		CredentialsVerified:      result.CredentialsVerified,
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}
