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

package terraform

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// TerraformRCFilename is the filename for Terraform registry configuration
	TerraformRCFilename = ".terraformrc"

	// EnvTerraformCLIConfigFile is the environment variable used to specify the location of Terraform config file
	EnvTerraformCLIConfigFile = "TF_CLI_CONFIG_FILE"

	// DefaultFilePerms defines secure file permissions for the Terraform config file (0600 = owner read/write only)
	DefaultFilePerms = 0600
)

// RegistryConfig tracks the configuration created for cleanup
type RegistryConfig struct {
	ConfigPath string
	EnvVars    map[string]string
	TempFiles  []string
}

// ConfigureTerraformRegistry sets up Terraform registry configuration for private registries.
// It creates a .terraformrc file with the registry mirror and sets up authentication via environment variables.
// Returns a RegistryConfig struct that tracks created resources for cleanup.
func ConfigureTerraformRegistry(
	ctx context.Context,
	config recipes.Configuration,
	secrets map[string]recipes.SecretData,
	dirPath string,
) (*RegistryConfig, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// DEBUG: Log configuration at entry point
	logger.Info("DEBUG: ConfigureTerraformRegistry called",
		"secretsCount", len(secrets),
		"hasProviderMirror", config.RecipeConfig.Terraform.ProviderMirror != nil,
		"moduleRegistriesCount", len(config.RecipeConfig.Terraform.ModuleRegistries),
		"note", "provider mirror type field is deprecated - always using network mirror")

	regConfig := &RegistryConfig{EnvVars: make(map[string]string)}

	pm := config.RecipeConfig.Terraform.ProviderMirror

	// DEBUG: Log provider mirror details step by step
	logger.Info("DEBUG: Checking provider mirror configuration", "step", "1-checking-pm-nil")
	if pm == nil {
		logger.Info("DEBUG: Provider mirror is nil", "pm", "nil")
	} else {
		logger.Info("DEBUG: Provider mirror exists",
			"pm_url", pm.URL,
			"pm_type", pm.Type,
			"url_empty", pm.URL == "",
			"type_deprecated_ignored", "type field is deprecated")
	}

	// Provider mirror is valid if configured (URL validation happens later)
	hasProviderMirror := pm != nil
	logger.Info("DEBUG: Final hasProviderMirror result", "hasProviderMirror", hasProviderMirror, "note", "type field deprecated, only URL required")

	// Determine if module registries exist
	hasModuleRegistries := len(config.RecipeConfig.Terraform.ModuleRegistries) > 0

	if !hasProviderMirror && !hasModuleRegistries {
		logger.Info("No Terraform provider mirror or module registries configured, skipping registry configuration")
		return nil, nil
	}

	logger.Info("Setting up Terraform registry configuration",
		"hasProviderMirror", hasProviderMirror,
		"hasModuleRegistries", hasModuleRegistries,
		"moduleRegistryCount", len(config.RecipeConfig.Terraform.ModuleRegistries),
		"secretsCount", len(secrets))

	// Begin building Terraform configuration
	var configContent strings.Builder
	credentialsHosts := make(map[string]bool)

	// Provider installation: always use network mirror (type field is deprecated)
	if hasProviderMirror {
		// Always use network mirror - the type field is deprecated
		mirrorType := "network"

		logger.Info("Using network mirror for provider installation (type field deprecated)",
			"url", pm.URL,
			"deprecated_type_field", pm.Type,
			"note", "provider mirrors do not support authentication")

		// Log the resolved provider mirror configuration for diagnostics
		logger.Info("Resolved Terraform provider mirror configuration",
			"type", mirrorType,
			"url", pm.URL)

		if pm.URL == "" {
			return nil, fmt.Errorf("provider mirror url is required for network mirror")
		}

		// Provider mirrors do not support authentication - only URL is needed.
		// Ensure the URL ends with exactly one trailing slash as required by the protocol.
		mirrorURL := strings.TrimRight(pm.URL, "/") + "/"
				configContent.WriteString(fmt.Sprintf(`provider_installation {
	network_mirror {
		url = %q
	}
}
`, mirrorURL))
	}

	// Module registries
	if hasModuleRegistries {
		for registryName, registryConfig := range config.RecipeConfig.Terraform.ModuleRegistries {
			redirectHost := registryConfig.URL
			if redirectHost == "" {
				continue
			}
			// We no longer emit host service mapping blocks. Terraform should use
			// normal service discovery or explicit module source addresses.

			// Module registry credentials
			if registryConfig.Authentication.Token != nil && registryConfig.Authentication.Token.Secret != "" {
				secretStoreID := registryConfig.Authentication.Token.Secret
				secretData, ok := secrets[secretStoreID]
				if !ok {
					return nil, fmt.Errorf("secret store %q not found for module registry %q", secretStoreID, registryName)
				}
				tokenString, ok := secretData.Data["token"]
				if !ok {
					return nil, fmt.Errorf("token not found in secret store %q for module registry %q", secretStoreID, registryName)
				}
				credentialsHost := registryConfig.URL
				if strings.Contains(credentialsHost, "/") {
					credentialsHost = strings.Split(credentialsHost, "/")[0]
				}
				if !credentialsHosts[credentialsHost] {
					configContent.WriteString(fmt.Sprintf(`
credentials %q {
  token = %q
}
`, credentialsHost, tokenString))
					credentialsHosts[credentialsHost] = true
				}
				for _, ah := range registryConfig.Authentication.AdditionalHosts {
					if ah == "" || ah == credentialsHost {
						continue
					}
					if !credentialsHosts[ah] {
						configContent.WriteString(fmt.Sprintf(`
credentials %q {
  token = %q
}
`, ah, tokenString))
						credentialsHosts[ah] = true
					}
				}

				// Configure Git authentication for module downloads using the same PAT
				// Strip port from host for .netrc since Git doesn't use ports in machine names
				gitHost := credentialsHost
				if colonIndex := strings.LastIndex(gitHost, ":"); colonIndex != -1 {
					gitHost = gitHost[:colonIndex]
				}

				logger.Info("Configuring Git authentication for module registry",
					"registryName", registryName,
					"host", gitHost)

				err := configureGitAuthentication(ctx, dirPath, gitHost, tokenString, regConfig)
				if err != nil {
					logger.Error(err, "Failed to configure Git authentication",
						"registryName", registryName,
						"host", gitHost)
					return nil, fmt.Errorf("failed to configure Git authentication for registry %q: %w", registryName, err)
				}

				// Also configure Git auth for additional hosts
				for _, ah := range registryConfig.Authentication.AdditionalHosts {
					if ah == "" || ah == credentialsHost {
						continue
					}

					// Strip port from additional host for .netrc
					gitAH := ah
					if colonIndex := strings.LastIndex(gitAH, ":"); colonIndex != -1 {
						gitAH = gitAH[:colonIndex]
					}

					logger.Info("Configuring Git authentication for additional host",
						"registryName", registryName,
						"additionalHost", gitAH)

					err := configureGitAuthentication(ctx, dirPath, gitAH, tokenString, regConfig)
					if err != nil {
						logger.Error(err, "Failed to configure Git authentication for additional host",
							"registryName", registryName,
							"additionalHost", gitAH)
						return nil, fmt.Errorf("failed to configure Git authentication for additional host %q: %w", gitAH, err)
					}
				}
			}

			// Implicit registry.terraform.io mapping removed; rely on DNS/proxy or explicit module sources.
		}
	}

	// Write config file
	terraformRCPath := filepath.Join(dirPath, TerraformRCFilename)
	if err := os.WriteFile(terraformRCPath, []byte(configContent.String()), DefaultFilePerms); err != nil {
		return nil, fmt.Errorf("failed to write Terraform registry configuration file: %w", err)
	}
	logger.Info("Terraform config file generated", "path", terraformRCPath, "content", configContent.String())
	regConfig.ConfigPath = terraformRCPath
	regConfig.EnvVars[EnvTerraformCLIConfigFile] = terraformRCPath

	return regConfig, nil
}

// prepareFilesystemMirror ensures a local filesystem mirror directory exists.
// DEPRECATED: Filesystem mirrors are deprecated. All provider mirrors now use network mirror protocol.
// If pm.URL is http(s) and points to a zip or tar.gz, download and extract to a temp dir under dirPath.
// If pm.URL is file:// or a local path, verify and return the path.
func prepareFilesystemMirror(ctx context.Context, pm *datamodel.TerraformProviderMirrorConfig, secrets map[string]recipes.SecretData, dirPath string, token []byte) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	src := pm.URL
	if src == "" {
		// Allow pre-provisioned mirror path via AdditionalHosts? For now, require URL
		return "", fmt.Errorf("provider mirror URL is required for filesystem mirror")
	}

	lower := strings.ToLower(src)
	if strings.HasPrefix(lower, "file://") || strings.HasPrefix(src, "/") || strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../") {
		// Local directory
		path := strings.TrimPrefix(src, "file://")
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		stat, err := os.Stat(abs)
		if err != nil {
			return "", err
		}
		if !stat.IsDir() {
			return "", fmt.Errorf("filesystem mirror path is not a directory: %s", abs)
		}
		return abs, nil
	}

	// Remote artifact
	u, err := url.Parse(src)
	if err != nil {
		return "", fmt.Errorf("invalid provider mirror URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported provider mirror URL scheme: %s", u.Scheme)
	}

	// Download
	artifactPath := filepath.Join(dirPath, "providers-mirror.zip")
	logger.Info("Downloading filesystem provider mirror", "url", src, "dest", artifactPath)
	if err := downloadWithTLSAndAuth(ctx, src, artifactPath, pm, secrets, token); err != nil {
		return "", fmt.Errorf("failed to download provider mirror: %w", err)
	}

	// Extract
	mirrorDir := filepath.Join(dirPath, "providers-mirror")
	_ = os.RemoveAll(mirrorDir)
	if err := os.MkdirAll(mirrorDir, 0755); err != nil {
		return "", err
	}
	logger.Info("Extracting filesystem provider mirror", "zip", artifactPath, "dest", mirrorDir)
	if err := unzip(artifactPath, mirrorDir); err != nil {
		return "", fmt.Errorf("failed to extract provider mirror: %w", err)
	}
	return mirrorDir, nil
}

// downloadWithTLSAndAuth downloads a URL with optional token auth and CA bundle/skip verify from pm.TLS.
func downloadWithTLSAndAuth(ctx context.Context, src, dest string, pm *datamodel.TerraformProviderMirrorConfig, secrets map[string]recipes.SecretData, token []byte) error {
	// Reuse customsource HTTP client logic by creating a minimal client
	client, err := buildHTTPClientFromTLS(pm, secrets)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", src, nil)
	if err != nil {
		return err
	}
	if len(token) > 0 {
		// Use Bearer token; many proxies accept raw token in Authorization or PRIVATE-TOKEN. Use Authorization if not GitLab-specific
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", string(token)))
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func buildHTTPClientFromTLS(pm *datamodel.TerraformProviderMirrorConfig, secrets map[string]recipes.SecretData) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if pm.TLS != nil {
		if pm.TLS.CACertificate != nil && pm.TLS.CACertificate.Source != "" {
			secretData, ok := secrets[pm.TLS.CACertificate.Source]
			if !ok {
				return nil, fmt.Errorf("CA certificate secret store not found: %s", pm.TLS.CACertificate.Source)
			}
			pem, ok := secretData.Data[pm.TLS.CACertificate.Key]
			if !ok {
				return nil, fmt.Errorf("CA certificate key '%s' not found", pm.TLS.CACertificate.Key)
			}
			pool, _ := x509.SystemCertPool()
			if pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM([]byte(pem)) {
				return nil, fmt.Errorf("failed to parse CA certificate")
			}
			tlsCfg.RootCAs = pool
		}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}, nil
}

// unzip extracts a simple zip archive into destDir
func unzip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		fp := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fp, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(fp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

// getTerraformTokenEnv prepares the TF_TOKEN_* environment variable for a hostname
// Returns the environment variable name and value
func getTerraformTokenEnv(hostname string, token string) (string, string, error) {
	// Replace dots and colons with underscores in hostname (for ports)
	envHostname := strings.ReplaceAll(hostname, ".", "_")
	envHostname = strings.ReplaceAll(envHostname, ":", "_")
	envVarName := fmt.Sprintf("TF_TOKEN_%s", envHostname)
	return envVarName, token, nil
}

// configureGitAuthentication sets up Git authentication and URL rewriting for module downloads.
// This configures a per-directory Git config that:
// 1. Rewrites Git clone URLs from a public host (github.com) to the specified private host (e.g., gitlab.airgapped.local).
// 2. Injects an HTTP Authorization header for requests to the private host.
func configureGitAuthentication(ctx context.Context, dirPath, host, token string, regConfig *RegistryConfig) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Ensure host without trailing slashes and whitespace
	host = strings.TrimSpace(strings.TrimSuffix(host, "/"))
	if host == "" {
		return fmt.Errorf("git host is empty")
	}

	// Prepare config path in the working directory
	gitConfigPath := filepath.Join(dirPath, ".gitconfig")

	// Build Basic auth header: base64("oauth2:" + PAT)
	basic := base64.StdEncoding.EncodeToString([]byte("oauth2:" + token))
	authHeader := "Authorization: Basic " + basic

	// Build the .gitconfig content
	var gitConfigContent strings.Builder

	// Section for URL rewriting.
	// This tells Git to replace any github.com URL with our internal GitLab host.
	// The trailing slashes are important for Git's prefix matching.
	gitConfigContent.WriteString(fmt.Sprintf("[url \"https://%s/\"]\n", host))
	gitConfigContent.WriteString("\tinsteadOf = https://github.com/\n\n")

	// Section for authentication.
	// This injects the auth header for requests to our internal GitLab host.
	gitConfigContent.WriteString(fmt.Sprintf("[http \"https://%s\"]\n", host))
	gitConfigContent.WriteString(fmt.Sprintf("\textraHeader = %s\n", authHeader))

	// Write the config file. This will overwrite any existing .gitconfig in this directory.
	if err := os.WriteFile(gitConfigPath, []byte(gitConfigContent.String()), 0600); err != nil {
		return fmt.Errorf("failed to write .gitconfig: %w", err)
	}

	// Track for cleanup
	regConfig.TempFiles = append(regConfig.TempFiles, gitConfigPath)

	// Set environment variables for Git
	regConfig.EnvVars["GIT_CONFIG_GLOBAL"] = gitConfigPath
	regConfig.EnvVars["HOME"] = dirPath
	regConfig.EnvVars["GIT_TERMINAL_PROMPT"] = "0"

	logger.Info("Configured Git URL rewriting and authentication for module downloads",
		"host", host,
		"insteadOf", "https://github.com/",
		"config", gitConfigPath)
	return nil
}

// CleanupTerraformRegistryConfig removes the Terraform registry configuration and unsets environment variables
func CleanupTerraformRegistryConfig(ctx context.Context, config *RegistryConfig) error {
	if config == nil {
		return nil
	}

	logger := ucplog.FromContextOrDiscard(ctx)

	// Note: We no longer unset environment variables since they are now passed
	// to the Terraform process rather than set on the current process.
	// The cleanup is handled by the tfexec library when the process terminates.

	// Remove the config file if it exists
	if config.ConfigPath != "" {
		if err := os.Remove(config.ConfigPath); err != nil && !os.IsNotExist(err) {
			logger.Info("Failed to remove Terraform config file", "path", config.ConfigPath, "error", err)
			// Don't return error as this is just cleanup
		}
	}

	// Remove temporary certificate files
	for _, tempFile := range config.TempFiles {
		if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
			logger.Info("Failed to remove temporary file", "path", tempFile, "error", err)
			// Don't return error as this is just cleanup
		} else {
			logger.Info("Removed temporary file", "path", tempFile)
		}
	}

	return nil
}

// Helper functions for safe logging

// getTokenPrefix returns a safe preview of a token (first 8 chars only)
func getTokenPrefix(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:8] + "..."
}

// getCertPreview returns a safe preview of certificate content
func getCertPreview(cert string) string {
	lines := strings.Split(cert, "\n")
	if len(lines) > 0 {
		return lines[0] // Usually "-----BEGIN CERTIFICATE-----"
	}
	return "empty"
}
