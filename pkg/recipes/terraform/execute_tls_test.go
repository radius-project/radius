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
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddTLSEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name           string
		options        Options
		existingEnvVars map[string]string
		expectedEnvVars map[string]string
		wantErr        bool
	}{
		{
			name: "recipe TLS with skipVerify only",
			options: Options{
				EnvRecipe: &recipes.EnvironmentDefinition{
					TLS: &recipes.TLSConfig{
						SkipVerify: true,
					},
				},
			},
			existingEnvVars: map[string]string{},
			expectedEnvVars: map[string]string{
				"GIT_SSL_NO_VERIFY": "true",
			},
		},
		{
			name: "recipe TLS with CA certificate",
			options: Options{
				RootDir: "/tmp/test",
				EnvRecipe: &recipes.EnvironmentDefinition{
					TLS: &recipes.TLSConfig{
						CACertificate: &recipes.SecretReference{
							Source: "secret-store-1",
							Key:    "ca-cert",
						},
					},
				},
				Secrets: map[string]recipes.SecretData{
					"secret-store-1": {
						Data: map[string]string{
							"ca-cert": "-----BEGIN CERTIFICATE-----\ntest-ca-cert\n-----END CERTIFICATE-----",
						},
					},
				},
			},
			existingEnvVars: map[string]string{},
			expectedEnvVars: map[string]string{
				"GIT_SSL_CAINFO": "/tmp/test/.terraform/modules/.tls/ca.crt",
			},
		},
		{
			name: "recipe TLS with both skipVerify and CA certificate",
			options: Options{
				RootDir: "/tmp/test",
				EnvRecipe: &recipes.EnvironmentDefinition{
					TLS: &recipes.TLSConfig{
						SkipVerify: true,
						CACertificate: &recipes.SecretReference{
							Source: "secret-store-1",
							Key:    "ca-cert",
						},
					},
				},
				Secrets: map[string]recipes.SecretData{
					"secret-store-1": {
						Data: map[string]string{
							"ca-cert": "-----BEGIN CERTIFICATE-----\ntest-ca-cert\n-----END CERTIFICATE-----",
						},
					},
				},
			},
			existingEnvVars: map[string]string{},
			expectedEnvVars: map[string]string{
				"GIT_SSL_NO_VERIFY": "true",
				"GIT_SSL_CAINFO":    "/tmp/test/.terraform/modules/.tls/ca.crt",
			},
		},
		{
			name: "recipe TLS with registry env vars present",
			options: Options{
				RootDir: "/tmp/test",
				EnvRecipe: &recipes.EnvironmentDefinition{
					TLS: &recipes.TLSConfig{
						CACertificate: &recipes.SecretReference{
							Source: "secret-store-1",
							Key:    "ca-cert",
						},
					},
				},
				Secrets: map[string]recipes.SecretData{
					"secret-store-1": {
						Data: map[string]string{
							"ca-cert": "-----BEGIN CERTIFICATE-----\ntest-ca-cert\n-----END CERTIFICATE-----",
						},
					},
				},
			},
			existingEnvVars: map[string]string{
				"SSL_CERT_FILE":   "/tmp/registry-ca.crt",
				"CURL_CA_BUNDLE":  "/tmp/registry-ca.crt",
			},
			expectedEnvVars: map[string]string{
				"SSL_CERT_FILE":   "/tmp/registry-ca.crt", // Registry vars preserved
				"CURL_CA_BUNDLE":  "/tmp/registry-ca.crt", // Registry vars preserved
				"GIT_SSL_CAINFO":  "/tmp/test/.terraform/modules/.tls/ca.crt", // Recipe uses different var
			},
		},
		{
			name: "no TLS configuration",
			options: Options{
				EnvRecipe: &recipes.EnvironmentDefinition{},
			},
			existingEnvVars: map[string]string{
				"EXISTING_VAR": "value",
			},
			expectedEnvVars: map[string]string{
				"EXISTING_VAR": "value",
			},
		},
		{
			name: "missing CA certificate in secrets",
			options: Options{
				RootDir: "/tmp/test",
				EnvRecipe: &recipes.EnvironmentDefinition{
					TLS: &recipes.TLSConfig{
						CACertificate: &recipes.SecretReference{
							Source: "secret-store-1",
							Key:    "ca-cert",
						},
					},
				},
				Secrets: map[string]recipes.SecretData{},
			},
			existingEnvVars: map[string]string{},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of existing env vars
			envVars := make(map[string]string)
			for k, v := range tt.existingEnvVars {
				envVars[k] = v
			}

			// Call the function
			err := addTLSEnvironmentVariables(context.Background(), tt.options, envVars)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Check that expected env vars are set
			for k, v := range tt.expectedEnvVars {
				assert.Contains(t, envVars, k)
				// For file paths, just check they end with the expected filename
				if k == "GIT_SSL_CAINFO" {
					assert.Contains(t, envVars[k], ".tls/ca.crt")
				} else {
					assert.Equal(t, v, envVars[k])
				}
			}

			// Ensure no unexpected env vars were added
			assert.Equal(t, len(tt.expectedEnvVars), len(envVars))
		})
	}
}