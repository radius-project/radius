// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/apiserverstore"
	"github.com/radius-project/radius/pkg/components/database/postgres"
	"github.com/radius-project/radius/test/ucp/kubeenv"
	"github.com/stretchr/testify/require"
)

func runPolicyBackend(t *testing.T, newClient func(*testing.T) database.Client) {
	t.Helper()
	t.Run("initialization", func(t *testing.T) { runPolicyStoreInitialization(t, newClient(t)) })
	t.Run("contract", func(t *testing.T) { runPolicyStoreContract(t, newClient(t)) })
	t.Run("crashes", func(t *testing.T) { runPolicyStoreCrashRecovery(t, newClient) })
	t.Run("delayed_recoverer", func(t *testing.T) { runPolicyStoreDelayedRecovery(t, newClient(t)) })
	t.Run("role_races", func(t *testing.T) { runPolicyRoleRaces(t, newClient) })
	t.Run("deployment", func(t *testing.T) { runDeploymentPolicyContract(t, newClient(t)) })
	t.Run("deployment_crashes", func(t *testing.T) { runDeploymentPolicyCrashRecovery(t, newClient) })
}

func TestPolicyStoreAPIServer(t *testing.T) {
	if testing.Short() {
		t.Skip("real Kubernetes API server integration; omit -short to run")
	}
	client, env, err := kubeenv.StartEnvironment([]string{filepath.Join("..", "..", "..", "deploy", "Chart", "crds", "ucpd")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, env.Stop()) })
	runPolicyBackend(t, func(t *testing.T) database.Client {
		t.Helper()
		namespace := "rbac-" + uuid.NewString()
		require.NoError(t, kubeenv.EnsureNamespace(t.Context(), client, namespace))
		return apiserverstore.NewAPIServerClient(client, namespace)
	})
}

func TestPolicyStorePostgres(t *testing.T) {
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL is not set; real Postgres integration not executed")
	}
	if testing.Short() {
		t.Skip("real Postgres integration; omit -short to run")
	}
	admin, err := pgxpool.New(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	runPolicyBackend(t, func(t *testing.T) database.Client {
		t.Helper()
		// Each test owns an isolated schema, never truncates a shared table.
		schema := "rbac_" + uuid.NewString()
		quoted := `"` + schema + `"`
		_, err := admin.Exec(t.Context(), "CREATE SCHEMA "+quoted)
		require.NoError(t, err)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
			defer cancel()
			_, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
			require.NoError(t, err)
		})
		cfg, err := pgxpool.ParseConfig(url)
		require.NoError(t, err)
		cfg.ConnConfig.RuntimeParams["search_path"] = quoted
		pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
		require.NoError(t, err)
		t.Cleanup(pool.Close)
		_, err = pool.Exec(t.Context(), `CREATE TABLE resources (
			id TEXT PRIMARY KEY, original_id TEXT NOT NULL, resource_type TEXT NOT NULL,
			root_scope TEXT NOT NULL, routing_scope TEXT NOT NULL,
			etag TEXT NOT NULL, resource_data JSONB NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW())`)
		require.NoError(t, err)
		return postgres.NewPostgresClient(pool)
	})
}
