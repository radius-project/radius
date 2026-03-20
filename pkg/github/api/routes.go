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
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/radius-project/radius/pkg/github/auth"
	"github.com/radius-project/radius/pkg/github/credential"
)

// RegisterRoutes registers the GitHub App API routes on the given chi router.
func RegisterRoutes(r chi.Router, svc credential.Service, verifier credential.Verifier, sessionStore *auth.SessionStore, oauthConfig *auth.OAuthConfig) {
	h := &handlers{svc: svc, verifier: verifier}

	// Auth routes — no middleware required.
	r.Get("/auth/github/login", handleLogin(oauthConfig, sessionStore))
	r.Get("/auth/github/callback", handleCallback(oauthConfig, sessionStore))

	// API routes — require authentication.
	r.Route("/api", func(r chi.Router) {
		r.Use(auth.RequireAuth(sessionStore))

		r.Get("/repos/{owner}/{repo}/environments", h.listEnvironments)
		r.Get("/repos/{owner}/{repo}/environments/{name}", h.getEnvironment)
		r.Post("/repos/{owner}/{repo}/environments/aws", h.createAWSEnvironment)
		r.Post("/repos/{owner}/{repo}/environments/azure", h.createAzureEnvironment)
		r.Delete("/repos/{owner}/{repo}/environments/{name}", h.deleteEnvironment)

		// Credential verification.
		r.Post("/repos/{owner}/{repo}/environments/{name}/verify", h.verifyCredentials)
		r.Get("/repos/{owner}/{repo}/environments/{name}/verify", h.getVerificationStatus)
	})

	// Health check.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	})
}
