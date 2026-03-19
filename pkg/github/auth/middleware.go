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

package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	// UserContextKey is the key used to store the authenticated user in the request context.
	UserContextKey contextKey = "github_user"

	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "radius_session"
)

// RequireAuth returns a chi-compatible middleware that validates the user's session.
// Unauthenticated requests receive a 401 response.
func RequireAuth(store *SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r, store)
			if token == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext returns the authenticated user token from the request context, or nil.
func UserFromContext(ctx context.Context) *UserToken {
	token, _ := ctx.Value(UserContextKey).(*UserToken)
	return token
}

// extractToken attempts to find a valid session from the request. It checks:
// 1. Authorization: Bearer <token> header (session ID lookup)
// 2. Session cookie
func extractToken(r *http.Request, store *SessionStore) *UserToken {
	// Check Authorization header.
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if sessionID, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
			sessionID = strings.TrimSpace(sessionID)
			if token := store.Get(sessionID); token != nil {
				return token
			}
		}
	}

	// Check session cookie.
	cookie, err := r.Cookie(SessionCookieName)
	if err == nil && cookie.Value != "" {
		if token := store.Get(cookie.Value); token != nil {
			return token
		}
	}

	return nil
}
