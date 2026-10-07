/*
Copyright 2026 The Radius Authors.

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

package compatibility

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/cli/bicep"
	deploycmd "github.com/radius-project/radius/pkg/cli/cmd/deploy"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/tools/clientcmd"
)

// TestMain supplies a compiler subprocess, while the tests exercise the real CLI,
// workspace transport, version route, and deployment HTTP client without a cluster.
func TestMain(m *testing.M) {
	if os.Getenv("RADIUS_COMPATIBILITY_COMPILER") == "true" && len(os.Args) > 1 && os.Args[1] == "build" {
		fmt.Print(os.Getenv("RADIUS_COMPATIBILITY_TEMPLATE"))
		return
	}
	m.Run()
}

type lockedBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.String()
}

const radiusTemplate = `{
	"resources":{
		"env":{"type":"Applications.Core/environments@2023-10-01-preview","name":"test"},
		"app":{"type":"Radius.Core/applications@2025-08-01-preview","name":"test"}
	}
}`

func Test_DeployCompatibilityWarning(t *testing.T) {
	tests := []struct {
		name     string
		pin      string
		cpBody   string
		cpCode   int
		input    string
		warning  string
		template string
	}{
		{name: "newer exact pin", pin: "0.60.2", cpBody: `{"release":"0.60.0"}`, warning: "extension 0.60.2 differs from the target"},
		{name: "older exact pin", pin: "0.60.0", cpBody: `{"release":"0.60.2"}`, warning: "extension 0.60.0 differs from the target"},
		{name: "floating channel", pin: "0.60", cpBody: `{"release":"0.60.0"}`, warning: "not compiler provenance"},
		{name: "latest", pin: "latest", cpBody: `{"release":"0.60.0"}`, warning: "tag(s): latest"},
		{name: "custom", pin: "custom", cpBody: `{"release":"0.60.0"}`, warning: "tag(s): unknown"},
		{name: "prerelease", pin: "0.61.0-rc2", cpBody: `{"release":"0.61.0-rc1"}`, warning: "extension 0.61.0-rc2 differs"},
		{name: "old target", pin: "0.60.0", cpCode: 404, warning: "HTTP 404"},
		{name: "invalid version response", pin: "0.60.0", cpBody: "<html>", warning: "invalid Radius version response"},
		{name: "missing target release", pin: "0.60.0", cpBody: `{}`, warning: "has no release"},
		{name: "actual UCP version route", pin: "0.60.0", warning: "control plane does not report a full release"},
		{name: "JSON without provenance", input: "json", cpBody: `{"release":"0.60.0"}`, warning: "no Radius extension pin metadata"},
		{name: "remote Bicep", pin: "0.60.2", input: "remote", cpBody: `{"release":"0.60.0"}`, warning: "extension 0.60.2 differs"},
		{name: "no configured alias", input: "inline", cpBody: `{"release":"0.60.0"}`, warning: "has no radius extension reference"},
		{name: "nested module has separate provenance", pin: "0.60.0", cpBody: `{"release":"0.60.0"}`, warning: "no Radius extension pin metadata", template: `{
			"resources":{
				"env":{"type":"Applications.Core/environments@2023-10-01-preview","name":"test"},
				"module":{"type":"Microsoft.Resources/deployments","properties":{"template":{
					"resources":[{"type":"Radius.Core/applications@2025-08-01-preview","name":"nested"}]
				}}}
			}
		}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			log := &lockedBuffer{}
			var submissions atomic.Int32
			var versionRequests atomic.Int32

			router := chi.NewRouter()
			require.NoError(t, api.Register(t.Context(), router, nil, &ucp.Options{
				Config:           &ucp.Config{Server: hostoptions.ServerOptions{PathBase: "/apis/api.ucp.dev/v1alpha3"}},
				DatabaseProvider: databaseprovider.FromMemory(),
				StatusManager:    statusmanager.NewMockStatusManager(gomock.NewController(t)),
			}))
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/version"):
					versionRequests.Add(1)
					if tt.cpCode != 0 {
						w.WriteHeader(tt.cpCode)
					} else if tt.cpBody != "" {
						_, _ = io.WriteString(w, tt.cpBody)
					} else {
						router.ServeHTTP(w, r)
					}
				case r.URL.Path == "/app.bicep":
					_, _ = io.WriteString(w, "extension radius\n")
				case r.Method == http.MethodPut:
					submissions.Add(1)
					require.Contains(t, log.String(), tt.warning, "warning must precede the first write")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"error":{"code":"TestDeploymentRejected","message":"target rejected template"}}`)
				default:
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{}`)
				}
			}))
			defer target.Close()

			kubeconfig := filepath.Join(root, "kubeconfig")
			require.NoError(t, os.WriteFile(kubeconfig, []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: test
contexts:
- name: test
  context:
    cluster: test
    user: test
clusters:
- name: test
  cluster:
    server: %s
users:
- name: test
  user: {}
`, target.URL)), 0600))
			previousKubeconfig := clientcmd.RecommendedHomeFile
			clientcmd.RecommendedHomeFile = kubeconfig
			t.Cleanup(func() { clientcmd.RecommendedHomeFile = previousKubeconfig })
			compiler, err := os.Executable()
			require.NoError(t, err)
			t.Setenv("BICEP", compiler)
			t.Setenv("RADIUS_COMPATIBILITY_COMPILER", "true")
			template := tt.template
			if template == "" {
				template = radiusTemplate
			}
			t.Setenv("RADIUS_COMPATIBILITY_TEMPLATE", template)

			config := map[string]any{"extensions": map[string]any{}}
			if tt.pin != "" {
				config["extensions"] = map[string]any{"radius": "br:example.io/radius:" + tt.pin}
			}
			configJSON, err := json.Marshal(config)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"), configJSON, 0600))
			source := filepath.Join(root, "app.bicep")
			require.NoError(t, os.WriteFile(source, []byte("extension radius\n"), 0600))
			if tt.input == "json" {
				source = filepath.Join(root, "app.json")
				require.NoError(t, os.WriteFile(source, []byte(template), 0600))
			} else if tt.input == "remote" {
				source = target.URL + "/app.bicep"
			}

			configuration := viper.New()
			configuration.Set("workspaces.default", "test")
			configuration.Set("workspaces.items.test", map[string]any{
				"connection": map[string]any{"kind": "kubernetes", "context": "test"},
				"scope":      "/planes/radius/local/resourceGroups/test",
			})
			writer := &output.OutputWriter{Writer: log}
			command, _ := deploycmd.NewCommand(&framework.Impl{
				ConfigHolder:      &framework.ConfigHolder{Config: configuration},
				Bicep:             &bicep.Impl{FileSystem: filesystem.NewOSFS(), Output: writer},
				ConnectionFactory: connections.DefaultFactory,
				Deploy:            &deploy.Impl{},
				Output:            writer,
			})
			command.SetContext(t.Context())
			command.SetArgs([]string{source})
			err = command.Execute()
			require.ErrorContains(t, err, "TestDeploymentRejected")
			require.EqualValues(t, 1, submissions.Load(), "version skew must not block deployment")
			require.EqualValues(t, 1, versionRequests.Load(), "do not repeat the preflight check")
			require.Contains(t, log.String(), "Deployment will continue.")
		})
	}
}
