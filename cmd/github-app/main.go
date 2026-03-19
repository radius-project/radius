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
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/radius-project/radius/pkg/github/api"
	"github.com/radius-project/radius/pkg/github/auth"
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

	// --- Credential Service (GitHub Environment only, no Radius registration) ---
	credService := credential.NewService(ghEnvClient)

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

	api.RegisterRoutes(r, credService, verifier, sessionStore, oauthConfig)

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

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
