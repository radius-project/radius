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

package install

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/terraform/installer"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid Install with version",
			Input:         []string{"--version", "1.6.4"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid Install with URL",
			Input:         []string{"--url", "https://example.com/terraform.zip"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid Install with version and wait",
			Input:         []string{"--version", "1.6.4", "--wait"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - neither version nor URL",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - wait without version (URL only)",
			Input:         []string{"--url", "https://example.com/terraform.zip", "--wait"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - timeout without wait",
			Input:         []string{"--version", "1.6.4", "--timeout", "5m"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid - ca-bundle with URL",
			Input:         []string{"--url", "https://example.com/terraform.zip", "--ca-bundle", "/path/to/ca.pem"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - ca-bundle without URL",
			Input:         []string{"--version", "1.6.4", "--ca-bundle", "/path/to/ca.pem"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid - auth-header with URL",
			Input:         []string{"--url", "https://example.com/terraform.zip", "--auth-header", "Bearer token123"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - auth-header without URL",
			Input:         []string{"--version", "1.6.4", "--auth-header", "Bearer token123"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid - client-cert and client-key with URL",
			Input:         []string{"--url", "https://example.com/terraform.zip", "--client-cert", "/path/to/cert.pem", "--client-key", "/path/to/key.pem"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - client-cert without client-key",
			Input:         []string{"--url", "https://example.com/terraform.zip", "--client-cert", "/path/to/cert.pem"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - client-key without client-cert",
			Input:         []string{"--url", "https://example.com/terraform.zip", "--client-key", "/path/to/key.pem"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - client-cert without URL",
			Input:         []string{"--version", "1.6.4", "--client-cert", "/path/to/cert.pem", "--client-key", "/path/to/key.pem"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid - proxy with URL",
			Input:         []string{"--url", "https://example.com/terraform.zip", "--proxy", "http://proxy:8080"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - proxy without URL",
			Input:         []string{"--version", "1.6.4", "--proxy", "http://proxy:8080"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid - all options with URL",
			Input:         []string{"--url", "https://example.com/terraform.zip", "--ca-bundle", "/ca.pem", "--auth-header", "Bearer token", "--client-cert", "/cert.pem", "--client-key", "/key.pem", "--proxy", "http://proxy:8080"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - too many args",
			Input:         []string{"extra-arg"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success - Install without wait", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/install" && r.Method == http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "install enqueued",
					"version": "1.6.4",
				})
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindKubernetes,
				"context": "my-context",
				"overrides": map[string]any{
					"ucp": server.URL,
				},
			},
		}

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output:       outputSink,
			Workspace:    workspace,
			Version:      "1.6.4",
			Wait:         false,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Verify output messages
		require.True(t, len(outputSink.Writes) >= 2)
		require.Contains(t, outputSink.Writes[0].(output.LogOutput).Format, "Installing Terraform")
		require.Contains(t, outputSink.Writes[1].(output.LogOutput).Format, "Terraform install queued")
	})

	t.Run("Success - Install with wait", func(t *testing.T) {
		var statusCalls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/install" && r.Method == http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "install enqueued",
					"version": "1.6.4",
				})
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				calls := statusCalls.Add(1)
				var state installer.VersionState
				var currentVersion string
				if calls < 2 {
					state = installer.VersionStateInstalling
					currentVersion = ""
				} else {
					state = installer.VersionStateSucceeded
					currentVersion = "1.6.4"
				}

				statusResponse := installer.StatusResponse{
					CurrentVersion: currentVersion,
					State:          installer.ResponseStateReady,
					Versions: map[string]installer.VersionStatus{
						"1.6.4": {
							Version: "1.6.4",
							State:   state,
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(statusResponse)
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindKubernetes,
				"context": "my-context",
				"overrides": map[string]any{
					"ucp": server.URL,
				},
			},
		}

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output:       outputSink,
			Workspace:    workspace,
			Version:      "1.6.4",
			Wait:         true,
			Timeout:      10 * time.Second,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Verify at least 2 status calls were made
		require.GreaterOrEqual(t, statusCalls.Load(), int32(2))
	})

	t.Run("Error - Install failed during wait", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/install" && r.Method == http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "install enqueued",
					"version": "1.6.4",
				})
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				statusResponse := installer.StatusResponse{
					CurrentVersion: "",
					State:          installer.ResponseStateFailed,
					Versions: map[string]installer.VersionStatus{
						"1.6.4": {
							Version:   "1.6.4",
							State:     installer.VersionStateFailed,
							LastError: "download failed",
						},
					},
					LastError: "download failed",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(statusResponse)
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindKubernetes,
				"context": "my-context",
				"overrides": map[string]any{
					"ucp": server.URL,
				},
			},
		}

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output:       outputSink,
			Workspace:    workspace,
			Version:      "1.6.4",
			Wait:         true,
			Timeout:      10 * time.Second,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "download failed")
	})

	t.Run("Error - Overall state failed without version status", func(t *testing.T) {
		// Tests the case where the server fails before populating version status
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/install" && r.Method == http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "install enqueued",
					"version": "1.6.4",
				})
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				// Return failed state without populating version status
				statusResponse := installer.StatusResponse{
					CurrentVersion: "",
					State:          installer.ResponseStateFailed,
					Versions:       nil, // No version status populated
					LastError:      "queue processing error",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(statusResponse)
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindKubernetes,
				"context": "my-context",
				"overrides": map[string]any{
					"ucp": server.URL,
				},
			},
		}

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output:       outputSink,
			Workspace:    workspace,
			Version:      "1.6.4",
			Wait:         true,
			Timeout:      10 * time.Second,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "queue processing error")
	})

	t.Run("Error - Server rejects install request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/install" && r.Method == http.MethodPost:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("invalid version format"))
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindKubernetes,
				"context": "my-context",
				"overrides": map[string]any{
					"ucp": server.URL,
				},
			},
		}

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output:       outputSink,
			Workspace:    workspace,
			Version:      "invalid",
			Wait:         false,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid version format")
	})

	t.Run("Success - Install with CA bundle", func(t *testing.T) {
		// Create a temporary CA bundle file
		tempDir := t.TempDir()
		caFile := filepath.Join(tempDir, "ca.pem")
		testCACert := `-----BEGIN CERTIFICATE-----
MIIDAzCCAeugAwIBAgIUM06Yo/BKCPvBfZwztaJPszhAO98wDQYJKoZIhvcNAQEL
BQAwETEPMA0GA1UEAwwGdGVzdGNhMB4XDTI2MDEyMTEwMjAzNVoXDTI3MDEyMTEw
MjAzNVowETEPMA0GA1UEAwwGdGVzdGNhMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEA0wyOmcNaSz1AQHGNVmNzzkDO5VhUCv56KRybhLR/uXhapxQ4T+Rr
beMUExEaxyWDnTjsnirNUvwadBONWzm8cDQSW2KldbnzjteBRlNDbRI6TgKE0TRR
ljAM77Dczzuye2PsQS002Ny3UR+MnzI1kA3/XjAeAVefKn31Col0Ssn7OdvZ1VTH
aK04b2szaAla5Sl+eWKUsxj6UA/V/Xq94Z4AEnqk7zkGxnpILvxcz0QY/U/7e5iQ
IM/NkIeMoJe+Cfij+yPqLgh2f5L4Vi9WvRB8P0rbvl5WrEU6K6bjuZ5zKxiC+rbU
5hjAlR5lyrgo8cwiB5cOah+qQzl/3c26yQIDAQABo1MwUTAdBgNVHQ4EFgQU8/CI
UhXWPvHMCIynxKS4D+PQdy0wHwYDVR0jBBgwFoAU8/CIUhXWPvHMCIynxKS4D+PQ
dy0wDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAevFg7NV4D6UP
qYdvGjWgMFEUiUBp5EtEU5KD7FZwKop/lFqnvo+L1bUUy2hab76eO+g0perp8b8j
/ZwMgdIVNjNEWgM8h+Gg3HG8Rvdle5NqMq4lIGzmTN+MhPnQ8rECMSm0nVGTtFA0
qE+O0LoSl/4FL9pUQuwZi+WibxoTOlw3NXpxx2WUFzU/Giwx6OYCTb773M9noKCH
7VAkvFImjSbr4SU05DGe+cUcWmtWcfhj2geiCHl/EEpe/oEi5/XnpgeMj4vkE6zK
fiCLJ0WJ77/ohDKnNecDZKIWLsUo9ywMJqi9TLSiBf5oMOc9uZtDoPTPzsXzcPZP
2JkLUbkliQ==
-----END CERTIFICATE-----`
		err := os.WriteFile(caFile, []byte(testCACert), 0o600)
		require.NoError(t, err)

		var receivedCABundle string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/install" && r.Method == http.MethodPost:
				// Capture the CA bundle from the request
				var req installer.InstallRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
					receivedCABundle = req.CABundle
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "install enqueued",
				})
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindKubernetes,
				"context": "my-context",
				"overrides": map[string]any{
					"ucp": server.URL,
				},
			},
		}

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output:       outputSink,
			Workspace:    workspace,
			SourceURL:    "https://internal.example.com/terraform.zip",
			CABundle:     caFile,
			Wait:         false,
			PollInterval: 10 * time.Millisecond,
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)

		// Verify CA bundle was sent to server
		require.Equal(t, testCACert, receivedCABundle)
	})

	t.Run("Error - CA bundle file not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindKubernetes,
				"context": "my-context",
				"overrides": map[string]any{
					"ucp": server.URL,
				},
			},
		}

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output:       outputSink,
			Workspace:    workspace,
			SourceURL:    "https://internal.example.com/terraform.zip",
			CABundle:     "/nonexistent/path/to/ca.pem",
			Wait:         false,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed to read CA bundle file")
	})

	t.Run("Success - Install with URL and checksum (no CA bundle)", func(t *testing.T) {
		var receivedReq installer.InstallRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/install" && r.Method == http.MethodPost:
				_ = json.NewDecoder(r.Body).Decode(&receivedReq)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "install enqueued",
				})
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		workspace := &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindKubernetes,
				"context": "my-context",
				"overrides": map[string]any{
					"ucp": server.URL,
				},
			},
		}

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output:       outputSink,
			Workspace:    workspace,
			SourceURL:    "https://example.com/terraform.zip",
			Checksum:     "sha256:abc123",
			Wait:         false,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Verify request contents
		require.Equal(t, "https://example.com/terraform.zip", receivedReq.SourceURL)
		require.Equal(t, "sha256:abc123", receivedReq.Checksum)
		require.Empty(t, receivedReq.CABundle, "CABundle should be empty when not specified")
	})
}
