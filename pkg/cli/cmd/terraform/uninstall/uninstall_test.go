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

package uninstall

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			Name:          "Valid Uninstall Command",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid Uninstall with wait",
			Input:         []string{"--wait"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid Uninstall with wait and timeout",
			Input:         []string{"--wait", "--timeout", "5m"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Invalid - timeout without wait",
			Input:         []string{"--timeout", "5m"},
			ExpectedValid: false,
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
	t.Run("Success - Uninstall without wait", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				// Status is now fetched to check if there's a current version
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(installer.StatusResponse{
					CurrentVersion: "1.6.4",
					State:          installer.ResponseStateReady,
				})
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/uninstall" && r.Method == http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "uninstall enqueued",
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
			Wait:         false,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Verify output messages
		require.True(t, len(outputSink.Writes) >= 2)
		require.Contains(t, outputSink.Writes[0].(output.LogOutput).Format, "Uninstalling Terraform")
		require.Contains(t, outputSink.Writes[1].(output.LogOutput).Format, "Terraform uninstall queued")
	})

	t.Run("Success - Uninstall with wait", func(t *testing.T) {
		var statusCalls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				calls := statusCalls.Add(1)

				var currentVersion string
				var versions map[string]installer.VersionStatus

				if calls <= 1 {
					// First call (before uninstall request) - return current version
					currentVersion = "1.6.4"
					versions = map[string]installer.VersionStatus{
						"1.6.4": {
							Version: "1.6.4",
							State:   installer.VersionStateSucceeded,
						},
					}
				} else if calls == 2 {
					// Second call - still uninstalling
					currentVersion = "1.6.4"
					versions = map[string]installer.VersionStatus{
						"1.6.4": {
							Version: "1.6.4",
							State:   installer.VersionStateUninstalling,
						},
					}
				} else {
					// Third call and beyond - uninstalled
					currentVersion = ""
					versions = map[string]installer.VersionStatus{
						"1.6.4": {
							Version: "1.6.4",
							State:   installer.VersionStateUninstalled,
						},
					}
				}

				statusResponse := installer.StatusResponse{
					CurrentVersion: currentVersion,
					Versions:       versions,
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(statusResponse)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/uninstall" && r.Method == http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "uninstall enqueued",
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
			Wait:         true,
			Timeout:      10 * time.Second,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Verify status calls were made
		require.GreaterOrEqual(t, statusCalls.Load(), int32(3))
	})

	t.Run("Success - No current version installed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				statusResponse := installer.StatusResponse{
					CurrentVersion: "",
					State:          installer.ResponseStateNotInstalled,
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
			Wait:         true,
			Timeout:      10 * time.Second,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		// Should indicate no version is installed
		require.True(t, len(outputSink.Writes) >= 1)
		require.Contains(t, outputSink.Writes[0].(output.LogOutput).Format, "No Terraform version is currently installed")
	})

	t.Run("Error - Current version changed during wait", func(t *testing.T) {
		var statusCalls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				calls := statusCalls.Add(1)

				var statusResponse installer.StatusResponse
				if calls == 1 {
					// Before uninstall, current version is 1.6.4
					statusResponse = installer.StatusResponse{
						CurrentVersion: "1.6.4",
						Versions: map[string]installer.VersionStatus{
							"1.6.4": {
								Version: "1.6.4",
								State:   installer.VersionStateSucceeded,
							},
							"1.5.0": {
								Version: "1.5.0",
								State:   installer.VersionStateSucceeded,
							},
						},
					}
				} else {
					// After uninstall, previous version is promoted
					statusResponse = installer.StatusResponse{
						CurrentVersion: "1.5.0",
						Versions: map[string]installer.VersionStatus{
							"1.6.4": {
								Version: "1.6.4",
								State:   installer.VersionStateUninstalled,
							},
							"1.5.0": {
								Version: "1.5.0",
								State:   installer.VersionStateSucceeded,
							},
						},
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(statusResponse)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/uninstall" && r.Method == http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "uninstall enqueued",
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
			Wait:         true,
			Timeout:      10 * time.Second,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "now installed")
	})

	t.Run("Error - Install in progress during wait", func(t *testing.T) {
		var statusCalls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				calls := statusCalls.Add(1)

				var statusResponse installer.StatusResponse
				if calls == 1 {
					statusResponse = installer.StatusResponse{
						CurrentVersion: "1.6.4",
						Versions: map[string]installer.VersionStatus{
							"1.6.4": {
								Version: "1.6.4",
								State:   installer.VersionStateSucceeded,
							},
						},
					}
				} else {
					inProgress := "install:1.7.0"
					statusResponse = installer.StatusResponse{
						CurrentVersion: "",
						Queue: &installer.QueueInfo{
							Pending:    0,
							InProgress: &inProgress,
						},
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(statusResponse)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/uninstall" && r.Method == http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "uninstall enqueued",
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
			Wait:         true,
			Timeout:      10 * time.Second,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "install in progress")
	})

	t.Run("Error - Uninstall failed during wait", func(t *testing.T) {
		var statusCalls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				calls := statusCalls.Add(1)

				var statusResponse installer.StatusResponse
				if calls <= 1 {
					// First call (before uninstall) - return current version
					statusResponse = installer.StatusResponse{
						CurrentVersion: "1.6.4",
						Versions: map[string]installer.VersionStatus{
							"1.6.4": {
								Version: "1.6.4",
								State:   installer.VersionStateSucceeded,
							},
						},
					}
				} else {
					// Subsequent calls - return failed state
					statusResponse = installer.StatusResponse{
						CurrentVersion: "1.6.4",
						State:          installer.ResponseStateFailed,
						Versions: map[string]installer.VersionStatus{
							"1.6.4": {
								Version:   "1.6.4",
								State:     installer.VersionStateFailed,
								LastError: "terraform in use",
							},
						},
						LastError: "terraform in use",
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(statusResponse)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/uninstall" && r.Method == http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"message": "uninstall enqueued",
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
			Wait:         true,
			Timeout:      10 * time.Second,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "terraform in use")
	})

	t.Run("Error - Server rejects uninstall request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				// Status is now fetched to check if there's a current version
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(installer.StatusResponse{
					CurrentVersion: "1.6.4",
					State:          installer.ResponseStateReady,
				})
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/uninstall" && r.Method == http.MethodPost:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("uninstall rejected by server"))
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
			Wait:         false,
			PollInterval: 10 * time.Millisecond,
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "uninstall rejected by server")
	})
}
