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

package status

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			Name:          "Valid Status Command",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Status Command with fallback workspace",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Status Command with too many args",
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
	t.Run("Success", func(t *testing.T) {
		installedAt := time.Now().UTC()
		lastUpdated := time.Now().UTC()

		statusResponse := installer.StatusResponse{
			CurrentVersion: "1.6.4",
			State:          installer.ResponseStateReady,
			BinaryPath:     "/terraform/versions/1.6.4/terraform",
			InstalledAt:    &installedAt,
			Source: &installer.SourceInfo{
				URL:      "https://releases.hashicorp.com/terraform/1.6.4/terraform_1.6.4_linux_amd64.zip",
				Checksum: "sha256:abc123",
			},
			Queue: &installer.QueueInfo{
				Pending: 0,
			},
			LastUpdated: lastUpdated,
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
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
			Output:    outputSink,
			Workspace: workspace,
			Format:    "table",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		require.Len(t, outputSink.Writes, 1)
		formattedOutput, ok := outputSink.Writes[0].(output.FormattedOutput)
		require.True(t, ok)
		require.Equal(t, "table", formattedOutput.Format)

		// Verify the response was passed through
		responseData, ok := formattedOutput.Obj.(*installer.StatusResponse)
		require.True(t, ok)
		require.Equal(t, "1.6.4", responseData.CurrentVersion)
		require.Equal(t, installer.ResponseStateReady, responseData.State)
	})

	t.Run("Error - Server Error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusOK)
			case r.URL.Path == "/apis/api.ucp.dev/v1alpha3/installer/terraform/status" && r.Method == http.MethodGet:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("internal server error"))
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
			Output:    outputSink,
			Workspace: workspace,
			Format:    "table",
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "500")
	})
}
