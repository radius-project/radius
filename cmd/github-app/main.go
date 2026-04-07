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

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/radius-project/radius/pkg/github/api"
	"github.com/radius-project/radius/pkg/github/auth"
	"github.com/radius-project/radius/pkg/github/azure"
	"github.com/radius-project/radius/pkg/github/credential"
	"github.com/radius-project/radius/pkg/github/environment"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- Configuration from environment variables ---
	port := envOrDefault("PORT", "8080")
	ghAppID, err := strconv.ParseInt(envOrDefault("GITHUB_APP_ID", "0"), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
	}
	ghInstallationID, err := strconv.ParseInt(envOrDefault("GITHUB_INSTALLATION_ID", "0"), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid GITHUB_INSTALLATION_ID: %w", err)
	}
	ghPrivateKeyPath := os.Getenv("GITHUB_PRIVATE_KEY_PATH")
	ghClientID := os.Getenv("GITHUB_CLIENT_ID")
	ghClientSecret := os.Getenv("GITHUB_CLIENT_SECRET")
	redirectURL := envOrDefault("GITHUB_REDIRECT_URL", fmt.Sprintf("http://localhost:%s/auth/github/callback", port))

	// --- GitHub App Auth ---
	var ghTokenSource environment.TokenSource
	if ghAppID != 0 && ghPrivateKeyPath != "" {
		pemData, err := os.ReadFile(ghPrivateKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read GitHub private key from %s: %w", ghPrivateKeyPath, err)
		}
		privateKey, err := auth.ParsePrivateKey(pemData)
		if err != nil {
			return err
		}
		appAuth := auth.NewAppAuth(auth.AppConfig{
			AppID:          ghAppID,
			PrivateKey:     privateKey,
			InstallationID: ghInstallationID,
		})
		ghTokenSource = appAuth.TokenSource()
		log.Printf("GitHub App authentication configured (app ID: %d, installation ID: %d)", ghAppID, ghInstallationID)
	} else {
		// Fall back to a static token for development.
		ghToken := os.Getenv("GITHUB_TOKEN")
		if ghToken == "" {
			log.Println("WARNING: No GitHub authentication configured. Set GITHUB_APP_ID + GITHUB_PRIVATE_KEY_PATH or GITHUB_TOKEN.")
		}
		ghTokenSource = environment.StaticTokenSource(ghToken)
		log.Println("Using static GitHub token (development mode)")
	}

	// --- GitHub Environment Client ---
	ghEnvClient := environment.NewClient(ghTokenSource)

	// --- Azure Federation Client ---
	federationClient := azure.NewFederationClient()

	// --- Credential Service (GitHub Environment only, no Radius registration) ---
	credService := credential.NewService(ghEnvClient, federationClient)

	// --- Credential Verifier (commits + triggers verification workflow) ---
	verifier := credential.NewVerifier(ghTokenSource)

	// --- OAuth Config ---
	oauthConfig := &auth.OAuthConfig{
		ClientID:     ghClientID,
		ClientSecret: ghClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"repo"},
	}

	// --- Session Store ---
	sessionStore := auth.NewSessionStore()

	// --- HTTP Router ---
	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(corsMiddleware(envOrDefault("CORS_ALLOWED_ORIGINS", "*")))

	// In dev mode (static GITHUB_TOKEN, no App ID), skip auth on API routes.
	devMode := ghAppID == 0

	devAPIKey := os.Getenv("API_KEY")
	if devMode && devAPIKey == "" {
		log.Println("Development mode — API authentication is disabled")
	} else if devAPIKey != "" {
		log.Println("Development API key configured — use as Bearer token to authenticate")
	}

	api.RegisterRoutes(r, credService, verifier, sessionStore, oauthConfig, devAPIKey, devMode)

	// --- Start Server ---
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		log.Println("Shutting down server...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx) //nolint:errcheck
	}()

	log.Printf("Starting GitHub App server on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// corsMiddleware adds CORS headers to allow requests from browser extensions
// and configured origins. Set CORS_ALLOWED_ORIGINS to a comma-separated list
// of origins, or "*" (not recommended for production).
func corsMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	origins := strings.Split(allowedOrigins, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := false

			for _, o := range origins {
				if o == "*" || o == origin || strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "moz-extension://") {
					allowed = true
					break
				}
			}

			if allowed && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
