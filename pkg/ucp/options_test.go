// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package ucp

import (
	"fmt"
	"testing"

	authapi "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
)

func TestNewOptionsUserAuthorization(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			config, err := LoadConfig([]byte(fmt.Sprintf(`
databaseProvider:
  provider: inmemory
queueProvider:
  provider: inmemory
  name: test
ucp:
  kind: direct
  direct:
    endpoint: http://localhost:9000
userAuthorization:
  enabled: %t
`, enabled)))
			require.NoError(t, err)
			options, err := NewOptions(t.Context(), config)
			require.NoError(t, err)
			if !enabled {
				require.Nil(t, options.UserAuthorization)
				require.Equal(t, authorization.RuntimeNotConfigured, options.UserAuthorization.State().Status)
				return
			}

			require.NotNil(t, options.UserAuthorization)
			state := options.UserAuthorization.State()
			require.Equal(t, authorization.RuntimeUnavailable, state.Status)
			require.False(t, state.Running)
			require.Empty(t, state.LastKnownMode)
		})
	}
}

func TestNewOptionsKubernetesSettingsRequiresRuntime(t *testing.T) {
	_, err := NewOptions(t.Context(), &Config{UserAuthorization: authorization.RuntimeOptions{KubernetesSettings: true}})
	require.ErrorContains(t, err, "requires the user authorization runtime")
}

func TestNewOptionsInstallationBootstrap(t *testing.T) {
	config, err := LoadConfig([]byte(`
databaseProvider:
  provider: inmemory
queueProvider:
  provider: inmemory
  name: test
ucp:
  kind: direct
  direct:
    endpoint: http://localhost:9000
kubernetesCallerAuthentication:
  kubernetesIssuer: kubernetes:cluster-a
userAuthorization:
  enabled: true
  installation:
    mode: DryRun
    administrators:
      - type: group
        issuer: kubernetes:cluster-a
        subject: operators
`))
	require.NoError(t, err)
	require.NotNil(t, config.UserAuthorization.Installation)
	require.Len(t, config.UserAuthorization.Installation.Administrators, 1)
	options, err := NewOptions(t.Context(), config)
	require.NoError(t, err)
	client, err := options.DatabaseProvider.GetClient(t.Context())
	require.NoError(t, err)
	catalog, err := authorization.NewCatalog(nil)
	require.NoError(t, err)
	store, err := authorization.NewPolicyStore(client, catalog, nil, nil)
	require.NoError(t, err)
	snapshot, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	policy, err := snapshot.Policy()
	require.NoError(t, err)
	require.Equal(t, authapi.UserAuthorizationModeDryRun, *policy.Settings.Properties.Mode)
	require.Len(t, policy.Assignments, 1)
	require.Equal(t, "operators", *policy.Assignments[0].Properties.Principal.Subject)
	require.Equal(t, authapi.PrincipalTypeGroup, *policy.Assignments[0].Properties.Principal.Type)
	require.Equal(t, authapi.AssignmentManagementKindBootstrap, *policy.Assignments[0].Properties.ManagedBy.Kind)
	require.Equal(t, authorization.RuntimeUnavailable, options.UserAuthorization.State().Status)
}
