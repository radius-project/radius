// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/graph/persistence"
	archivestore "github.com/radius-project/radius/pkg/graph/persistence/archive"
	"github.com/radius-project/radius/pkg/statearchive"
	archivefactory "github.com/radius-project/radius/pkg/statearchive/factory"
	"github.com/radius-project/radius/test/radcli"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Graph command with flag",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Graph command with positional arg",
			Input:         []string{"test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Graph command with fallback workspace",
			Input:         []string{"--application", "test-app", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Graph command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Enriched: -a name plus bicep file path selects enriched mode",
			Input:         []string{"-a", "test-app", "./app.bicep"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "test-app", runner.ApplicationName)
				require.Equal(t, "./app.bicep", runner.BicepFilePath)
				require.NotNil(t, runner.Workspace)
			},
		},
		{
			Name:          "Modeled: bicep file path without -a selects modeled mode",
			Input:         []string{"./app.bicep", "--include-icons"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Empty(t, runner.ApplicationName)
				require.Equal(t, "./app.bicep", runner.BicepFilePath)
				require.True(t, runner.IncludeIcons)
				require.Nil(t, runner.Workspace, "modeled mode must not resolve a workspace")
			},
		},
		{
			Name:          "Modeled: bicep file path does not require a workspace",
			Input:         []string{"./app.bicep"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				Config: radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Enriched: explicitly empty -a with bicep file path is invalid",
			Input:         []string{"./app.bicep", "-a", ""},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Enriched: explicitly empty --application= with bicep file path is invalid",
			Input:         []string{"./app.bicep", "--application="},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
		{
			Name:          "Deployed: positional name and -a are mutually exclusive",
			Input:         []string{"test-app", "-a", "other-app"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				Config: configWithWorkspace,
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	t.Run("Success: empty graph (table)", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  outputSink,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Len(t, outputSink.Writes, 1)

		logOutput, ok := outputSink.Writes[0].(output.LogOutput)
		require.True(t, ok)
		require.Contains(t, logOutput.Format, "Displaying application: test-app")
		require.Contains(t, logOutput.Format, "(empty)")
	})

	t.Run("Success: graph with resources (table)", func(t *testing.T) {
		graphServer := func() fake.ApplicationsServer {
			srv := test_client_factory.WithApplicationsServerNoError()
			srv.GetGraph = func(
				ctx context.Context,
				rootScope string,
				applicationName string,
				body corerpv20250801preview.GetGraphRequest,
				options *corerpv20250801preview.ApplicationsClientGetGraphOptions,
			) (resp azfake.Responder[corerpv20250801preview.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
				containerID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/web"
				containerName := "web"
				containerType := "Applications.Core/containers"

				resp.SetResponse(http.StatusOK, corerpv20250801preview.ApplicationsClientGetGraphResponse{
					Resources: []*corerpv20250801preview.ApplicationGraphResource{
						{
							ID:              &containerID,
							Name:            &containerName,
							Type:            &containerType,
							Connections:     []*corerpv20250801preview.ApplicationGraphConnection{},
							OutputResources: []*corerpv20250801preview.ApplicationGraphOutputResource{},
						},
					},
				}, nil)
				return
			}
			return srv
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, graphServer)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  outputSink,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Len(t, outputSink.Writes, 1)

		logOutput, ok := outputSink.Writes[0].(output.LogOutput)
		require.True(t, ok)
		require.Contains(t, logOutput.Format, "Name: web (Applications.Core/containers)")
	})

	t.Run("Success: graph (JSON)", func(t *testing.T) {
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "json",
			Output:                  outputSink,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Len(t, outputSink.Writes, 1)

		formatted, ok := outputSink.Writes[0].(output.FormattedOutput)
		require.True(t, ok)
		require.Equal(t, "json", formatted.Format)
	})

	// Assert the --include-icons flag threads through into GetGraphRequest.IncludeIcons.
	// The default (flag not set) must send nil so the server treats it as false.
	t.Run("Success: IncludeIcons defaults to nil on request body", func(t *testing.T) {
		var received corerpv20250801preview.GetGraphRequest
		observeServer := func() fake.ApplicationsServer {
			srv := test_client_factory.WithApplicationsServerNoError()
			srv.GetGraph = func(
				ctx context.Context,
				rootScope string,
				applicationName string,
				body corerpv20250801preview.GetGraphRequest,
				options *corerpv20250801preview.ApplicationsClientGetGraphOptions,
			) (resp azfake.Responder[corerpv20250801preview.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
				received = body
				resp.SetResponse(http.StatusOK, corerpv20250801preview.ApplicationsClientGetGraphResponse{}, nil)
				return
			}
			return srv
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, observeServer)
		require.NoError(t, err)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			IncludeIcons:            false,
			Output:                  &output.MockOutput{},
		}

		require.NoError(t, runner.Run(t.Context()))
		require.Nil(t, received.IncludeIcons, "IncludeIcons must be nil by default (opt-in only)")
	})

	t.Run("Success: --include-icons forwards includeIcons=true", func(t *testing.T) {
		var received corerpv20250801preview.GetGraphRequest
		observeServer := func() fake.ApplicationsServer {
			srv := test_client_factory.WithApplicationsServerNoError()
			srv.GetGraph = func(
				ctx context.Context,
				rootScope string,
				applicationName string,
				body corerpv20250801preview.GetGraphRequest,
				options *corerpv20250801preview.ApplicationsClientGetGraphOptions,
			) (resp azfake.Responder[corerpv20250801preview.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
				received = body
				resp.SetResponse(http.StatusOK, corerpv20250801preview.ApplicationsClientGetGraphResponse{}, nil)
				return
			}
			return srv
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, observeServer)
		require.NoError(t, err)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "json",
			IncludeIcons:            true,
			Output:                  &output.MockOutput{},
		}

		require.NoError(t, runner.Run(t.Context()))
		require.NotNil(t, received.IncludeIcons)
		require.True(t, *received.IncludeIcons)
	})

	t.Run("Error: application not found (404)", func(t *testing.T) {
		notFoundServer := func() fake.ApplicationsServer {
			return fake.ApplicationsServer{
				GetGraph: func(
					ctx context.Context,
					rootScope string,
					applicationName string,
					body corerpv20250801preview.GetGraphRequest,
					options *corerpv20250801preview.ApplicationsClientGetGraphOptions,
				) (resp azfake.Responder[corerpv20250801preview.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
					errResp.SetResponseError(http.StatusNotFound, "NotFound")
					return
				},
			}
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, notFoundServer)
		require.NoError(t, err)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  &output.MockOutput{},
		}

		err = runner.Run(t.Context())
		require.Error(t, err)
		require.Equal(t, clierrors.Message("Application %q does not exist or has been deleted.", "test-app"), err)
	})

	t.Run("Error: GetGraph failure is propagated", func(t *testing.T) {
		graphErrorServer := func() fake.ApplicationsServer {
			srv := test_client_factory.WithApplicationsServerNoError()
			srv.GetGraph = func(
				ctx context.Context,
				rootScope string,
				applicationName string,
				body corerpv20250801preview.GetGraphRequest,
				options *corerpv20250801preview.ApplicationsClientGetGraphOptions,
			) (resp azfake.Responder[corerpv20250801preview.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
				errResp.SetResponseError(http.StatusInternalServerError, "InternalServerError")
				return
			}
			return srv
		}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, graphErrorServer)
		require.NoError(t, err)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			Workspace:               workspace,
			ApplicationName:         "test-app",
			Format:                  "table",
			Output:                  &output.MockOutput{},
		}

		err = runner.Run(t.Context())
		require.Error(t, err)
	})
}

// Test_Run_EnrichedMode asserts that when a Bicep path is provided the runner
// compiles the template and attaches the extracted dependsOn edges to
// GetGraphRequest.DependsOnEdges. The transport-shape and merge-policy sides
// are covered elsewhere (ExtractDependsOnEdges and MergeDependencyEdges tests
// in pkg/cli/graph and pkg/graph/edges); this test only pins the CLI-side
// plumbing between BicepFilePath, PrepareTemplate, and the request body.
func Test_Run_EnrichedMode(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	// Template: a container that dependsOn a rabbitmq queue via
	// resourceId(). ExtractDependsOnEdges resolves both endpoints to
	// canonical Radius IDs.
	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type": "Radius.Compute/containers",
				"name": "consumer",
				"dependsOn": []any{
					"[resourceId('Radius.Messaging/rabbitMQQueues', 'queue')]",
				},
			},
			map[string]any{
				"type": "Radius.Messaging/rabbitMQQueues",
				"name": "queue",
			},
		},
	}
	const (
		consumerID = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Compute/containers/consumer"
		queueID    = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Messaging/rabbitMQQueues/queue"
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), "./app.bicep").
		Return(template, bicep.RadiusExtensionPin{}, nil).
		Times(1)

	var received corerpv20250801preview.GetGraphRequest
	observeServer := func() fake.ApplicationsServer {
		srv := test_client_factory.WithApplicationsServerNoError()
		srv.GetGraph = func(
			ctx context.Context,
			rootScope string,
			applicationName string,
			body corerpv20250801preview.GetGraphRequest,
			options *corerpv20250801preview.ApplicationsClientGetGraphOptions,
		) (resp azfake.Responder[corerpv20250801preview.ApplicationsClientGetGraphResponse], errResp azfake.ErrorResponder) {
			received = body
			resp.SetResponse(http.StatusOK, corerpv20250801preview.ApplicationsClientGetGraphResponse{}, nil)
			return
		}
		return srv
	}

	factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, observeServer)
	require.NoError(t, err)

	runner := &Runner{
		RadiusCoreClientFactory: factory,
		Bicep:                   bicepMock,
		Workspace:               workspace,
		ApplicationName:         "test-app",
		BicepFilePath:           "./app.bicep",
		Format:                  "table",
		Output:                  &output.MockOutput{},
	}

	require.NoError(t, runner.Run(t.Context()))

	require.NotNil(t, received.DependsOnEdges, "enriched mode must forward extracted edges")
	entries, ok := received.DependsOnEdges[consumerID]
	require.True(t, ok, "consumer entry missing; got keys %v", received.DependsOnEdges)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].ID)
	require.Equal(t, queueID, *entries[0].ID)
	require.NotNil(t, entries[0].Direction)
	require.Equal(t, corerpv20250801preview.DirectionOutbound, *entries[0].Direction)
	require.NotNil(t, entries[0].Kind)
	require.Equal(t, corerpv20250801preview.ConnectionKindDependency, *entries[0].Kind)
}

// Test_Run_EnrichedMode_CompileError_Wrapped asserts that a Bicep compile
// failure is surfaced as a clierrors.Message so users see the file path in the
// error rather than the raw underlying error.
func Test_Run_EnrichedMode_CompileError_Wrapped(t *testing.T) {
	workspace := &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), "./bad.bicep").
		Return(nil, bicep.RadiusExtensionPin{}, clierrors.Message("syntax error"))

	factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
	require.NoError(t, err)

	runner := &Runner{
		RadiusCoreClientFactory: factory,
		Bicep:                   bicepMock,
		Workspace:               workspace,
		ApplicationName:         "test-app",
		BicepFilePath:           "./bad.bicep",
		Format:                  "table",
		Output:                  &output.MockOutput{},
	}

	err = runner.Run(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "./bad.bicep")
}

func Test_Run_EnrichedMode_JSON(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		compileError bool
	}{
		{name: "success"},
		{name: "compile error", compileError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			workspace := &workspaces.Workspace{
				Name:  "test-workspace",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}
			factory, err := test_client_factory.NewRadiusCoreTestClientFactory(workspace.Scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
			require.NoError(t, err)

			var stdout, stderr bytes.Buffer
			writer := &output.OutputWriter{Writer: &stdout, LogWriter: &stderr}
			ctrl := gomock.NewController(t)
			bicepMock := bicep.NewMockInterface(ctrl)
			bicepMock.EXPECT().PrepareTemplate(gomock.Any(), "./app.bicep").
				DoAndReturn(func(context.Context, string) (map[string]any, bicep.RadiusExtensionPin, error) {
					writer.LogInfo("Downloading Bicep...")
					if tc.compileError {
						return nil, bicep.RadiusExtensionPin{}, errors.New("compilation failed")
					}
					return sampleTemplate(), bicep.RadiusExtensionPin{}, nil
				})
			runner := &Runner{
				RadiusCoreClientFactory: factory,
				Bicep:                   bicepMock,
				Workspace:               workspace,
				ApplicationName:         "test-app",
				BicepFilePath:           "./app.bicep",
				Format:                  output.FormatJson,
				Output:                  writer,
			}

			err = runner.Run(t.Context())
			require.Contains(t, stderr.String(), "Compiling ./app.bicep")
			require.Contains(t, stderr.String(), "Downloading Bicep...")
			if tc.compileError {
				require.ErrorContains(t, err, "compilation failed")
				require.Empty(t, stdout.String())
				return
			}
			require.NoError(t, err)
			var actual corerpv20250801preview.ApplicationGraphResponse
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
			require.Empty(t, actual.Resources)
		})
	}
}

const sampleBicepPath = "/tmp/app.bicep"

// sampleTemplate returns a minimal ARM template containing a single
// Applications.Core/containers resource.
func sampleTemplate() map[string]any {
	return map[string]any{
		"resources": []any{
			map[string]any{
				"type":       "Applications.Core/containers",
				"name":       "frontend",
				"properties": map[string]any{"image": "nginx"},
			},
		},
	}
}

func TestIsModeledGraphArg(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"":                 false,
		"my-app":           false,
		"./app.bicep":      true,
		"/abs/app.BICEP":   true,
		"./app.json":       false,
		"./app.txt":        false,
		"./nested/x.bicep": true,
	}
	for in, want := range cases {
		require.Equal(t, want, isModeledGraphArg(in), in)
	}
}

func TestRunner_RunModeled_LocalFilesystem(t *testing.T) {
	for _, backend := range []string{"", "oci", "git", "unknown"} {
		t.Run("backend="+backend, func(t *testing.T) {
			testRunModeledLocalFilesystem(t, backend)
		})
	}
}

func testRunModeledLocalFilesystem(t *testing.T, backend string) {
	t.Helper()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv(archivefactory.BackendEnvVar, backend)
	t.Setenv(archivefactory.StateRegistryEnvVar, "")
	t.Setenv(archivefactory.GraphRegistryEnvVar, "")

	store, err := archivestore.NewStore(archivestore.Options{
		Archive: archivefactory.NewGraphArchive(os.Getenv(archivefactory.GraphRegistryEnvVar)),
	})
	require.NoError(t, err)

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), sampleBicepPath).
		Return(sampleTemplate(), bicep.RadiusExtensionPin{}, nil).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    store,
	}

	err = runner.Run(t.Context())
	require.NoError(t, err)

	contents, err := os.ReadFile(defaultModeledGraphFile)
	require.NoError(t, err)
	require.Contains(t, string(contents), "frontend")
	require.Contains(t, string(contents), "Applications.Core/containers")
	var graph corerpv20250801preview.ApplicationGraphResponse
	require.NoError(t, json.Unmarshal(contents, &graph))
	require.Len(t, graph.Resources, 1)
	require.Equal(t, "frontend", *graph.Resources[0].Name)
}

func TestRunner_RunModeled_ArchivePersistence(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "feature/foo")
	t.Setenv("GITHUB_REF_NAME", "42/merge")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), sampleBicepPath).
		Return(sampleTemplate(), bicep.RadiusExtensionPin{}, nil).
		Times(1)

	storeMock := persistence.NewMockStore(ctrl)
	expectedKey := persistence.Key{Namespace: "feature%2Ffoo", Name: modeledGraphKeyName}
	storeMock.EXPECT().
		Save(gomock.Any(), expectedKey, gomock.Any(), gomock.Any()).
		DoAndReturn(saveAssertion(t, "feature%2Ffoo", "frontend")).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    storeMock,
	}

	err := runner.Run(t.Context())
	require.NoError(t, err)

	_, statErr := os.Stat(defaultModeledGraphFile)
	require.True(t, os.IsNotExist(statErr), "modeled graph must not be written locally in repo-radius mode")
}

// Exercise the real adapter's path validation and JSON I/O, not just a mock
// Store, so source-branch encoding must produce distinct, safe namespaces.
func TestRunner_RunModeled_ArchiveStore_SourceBranches(t *testing.T) {
	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REF_NAME", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), sampleBicepPath).
		Return(sampleTemplate(), bicep.RadiusExtensionPin{}, nil).
		Times(3)

	archiveDir := t.TempDir()
	session := statearchive.NewMockSession(ctrl)
	session.EXPECT().Path().Return(archiveDir).AnyTimes()
	session.EXPECT().Commit(gomock.Any(), gomock.Any()).Return(nil).Times(3)
	session.EXPECT().Close(gomock.Any()).Times(6)
	archive := statearchive.NewMockArchive(ctrl)
	archive.EXPECT().Open(gomock.Any(), archivestore.DefaultGraphArchive).Return(session, nil).Times(6)
	store, err := archivestore.NewStore(archivestore.Options{Archive: archive})
	require.NoError(t, err)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    store,
	}

	for _, branch := range []string{"feature/foo", "feature-foo", "feature%2Ffoo"} {
		t.Setenv("GITHUB_HEAD_REF", branch)
		require.NoError(t, runner.Run(t.Context()))
	}
	for _, branch := range []string{"feature/foo", "feature-foo", "feature%2Ffoo"} {
		namespace := url.QueryEscape(branch)
		got, err := store.Load(t.Context(), persistence.Key{Namespace: namespace, Name: modeledGraphKeyName})
		require.NoError(t, err)
		require.Len(t, got.Resources, 1)
		require.Equal(t, "frontend", *got.Resources[0].Name)
		contents, err := os.ReadFile(filepath.Join(archiveDir, namespace, defaultModeledGraphFile))
		require.NoError(t, err)
		require.Contains(t, string(contents), "frontend")
	}
	_, err = os.Stat(defaultModeledGraphFile)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunner_RunModeled_ArchiveConfigurationError(t *testing.T) {
	for _, tc := range []struct {
		backend string
		want    string
	}{
		{want: archivefactory.GraphRegistryEnvVar},
		{backend: "oci", want: archivefactory.GraphRegistryEnvVar},
		{backend: "git", want: "Git state archive backend has been removed"},
		{backend: "unknown", want: "invalid " + archivefactory.BackendEnvVar},
	} {
		t.Run("backend="+tc.backend, func(t *testing.T) {
			withTempCwd(t)
			t.Setenv("GITHUB_ACTIONS", "true")
			t.Setenv("GITHUB_HEAD_REF", "feature/foo")
			t.Setenv(archivefactory.BackendEnvVar, tc.backend)
			t.Setenv(archivefactory.GraphRegistryEnvVar, "")
			store, err := archivestore.NewStore(archivestore.Options{
				Archive: archivefactory.NewGraphArchive(os.Getenv(archivefactory.GraphRegistryEnvVar)),
			})
			require.NoError(t, err)

			ctrl := gomock.NewController(t)
			bicepMock := bicep.NewMockInterface(ctrl)
			bicepMock.EXPECT().PrepareTemplate(gomock.Any(), sampleBicepPath).Return(sampleTemplate(), bicep.RadiusExtensionPin{}, nil)
			runner := &Runner{
				Bicep:         bicepMock,
				Output:        &output.MockOutput{},
				BicepFilePath: sampleBicepPath,
				GraphStore:    store,
			}
			err = runner.Run(t.Context())
			require.ErrorContains(t, err, tc.want)
			require.ErrorContains(t, err, "save modeled graph to radius-graph archive")
			_, err = os.Stat(defaultModeledGraphFile)
			require.ErrorIs(t, err, os.ErrNotExist, "archive errors must not fall back to local output")
		})
	}
}

func TestRunner_RunModeled_FallsBackToRefName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "")
	t.Setenv("GITHUB_REF_NAME", "main")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), sampleBicepPath).
		Return(sampleTemplate(), bicep.RadiusExtensionPin{}, nil).
		Times(1)

	storeMock := persistence.NewMockStore(ctrl)
	storeMock.EXPECT().
		Save(gomock.Any(), persistence.Key{Namespace: "main", Name: modeledGraphKeyName}, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    storeMock,
	}

	require.NoError(t, runner.Run(t.Context()))
}

func TestRunner_RunModeled_NoBranchInEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "")
	t.Setenv("GITHUB_REF_NAME", "")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), sampleBicepPath).
		Return(sampleTemplate(), bicep.RadiusExtensionPin{}, nil).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    persistence.NewMockStore(ctrl),
	}

	err := runner.Run(t.Context())
	require.Error(t, err)
}

func TestRunner_RunModeled_BicepCompileError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), sampleBicepPath).
		Return(nil, bicep.RadiusExtensionPin{}, errors.New("syntax error")).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
	}

	err := runner.Run(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "syntax error")
}

func TestRunner_RunModeled_NilGraphStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "feature/foo")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), sampleBicepPath).
		Return(sampleTemplate(), bicep.RadiusExtensionPin{}, nil).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    nil,
	}

	err := runner.Run(t.Context())
	require.Error(t, err)
}

func TestRunner_RunModeled_StoreSaveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withTempCwd(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_HEAD_REF", "feature/foo")

	bicepMock := bicep.NewMockInterface(ctrl)
	bicepMock.EXPECT().
		PrepareTemplate(gomock.Any(), sampleBicepPath).
		Return(sampleTemplate(), bicep.RadiusExtensionPin{}, nil).
		Times(1)

	storeMock := persistence.NewMockStore(ctrl)
	storeMock.EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("push rejected")).
		Times(1)

	runner := &Runner{
		Bicep:         bicepMock,
		Output:        &output.MockOutput{},
		BicepFilePath: sampleBicepPath,
		GraphStore:    storeMock,
	}

	err := runner.Run(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "push rejected")
	require.Contains(t, err.Error(), archivestore.DefaultGraphArchive)
}

// withTempCwd switches the current working directory to a freshly-created
// temp directory and restores the original on test cleanup.
func withTempCwd(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Chdir(tmp)
	require.True(t, filepath.IsAbs(tmp))
}

// saveAssertion returns a Save implementation that asserts the inbound
// payload before recording success.
func saveAssertion(t *testing.T, wantBranch, wantResource string) func(ctx context.Context, key persistence.Key, graph *corerpv20250801preview.ApplicationGraphResponse, opts persistence.SaveOptions) error {
	t.Helper()
	return func(_ context.Context, key persistence.Key, graph *corerpv20250801preview.ApplicationGraphResponse, opts persistence.SaveOptions) error {
		require.Equal(t, wantBranch, key.Namespace)
		require.Equal(t, modeledGraphKeyName, key.Name)
		require.NotNil(t, graph)
		require.Len(t, graph.Resources, 1)
		require.Equal(t, wantResource, *graph.Resources[0].Name)
		return nil
	}
}
