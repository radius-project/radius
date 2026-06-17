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

package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/davecgh/go-spew/spew"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/testcontext"
	shared "github.com/radius-project/radius/test/ucp/storetest"
)

func Test_PostgresClient(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	// You can get the right value for this by running the command: make db-init
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL is not set.")
		return
	}

	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)

	logger := postgresLogger{t: t, pool: pool}
	client := NewPostgresClient(&logger)

	clear := func(t *testing.T) {
		tag, err := pool.Exec(ctx, "DELETE FROM resources")
		require.NoError(t, err)
		t.Logf("Database reset ... %d rows deleted", tag.RowsAffected())
	}

	// The actual test logic lives in a shared package, we're just doing the setup here.
	shared.RunTest(t, client, clear)
}

var _ PostgresAPI = (*postgresLogger)(nil)

type postgresLogger struct {
	t    *testing.T
	pool *pgxpool.Pool
}

// Exec implements PostgresAPI.
func (l *postgresLogger) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	l.t.Logf("Executing: %s", sql)
	l.t.Logf("Args:\n%s", spew.Sdump(args...))
	return l.pool.Exec(ctx, sql, args...)
}

// Query implements PostgresAPI.
func (l *postgresLogger) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	l.t.Logf("Executing: %s", sql)
	l.t.Logf("Args:\n%s", spew.Sdump(args...))
	return l.pool.Query(ctx, sql, args...)
}

// QueryRow implements PostgresAPI.
func (l *postgresLogger) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	l.t.Logf("Executing: %s", sql)
	l.t.Logf("Args:\n%s", spew.Sdump(args...))
	return l.pool.QueryRow(ctx, sql, args...)
}

// Test_PostgresClient_Pagination_NonUTC_Timezone is a regression test for a bug
// where the pagination token (RFC3339Nano UTC) was cast to TIMESTAMP rather
// than TIMESTAMPTZ in the Query SQL. Under a non-UTC session timezone the cast
// silently reinterpreted the value in the local zone, shifting the comparison
// boundary and causing page N+1 to drop rows. This manifested in production as
// LIST responses returning only the first page (e.g. Applications.Core
// containers list returning 10/12 results and hiding two from getGraph).
func Test_PostgresClient_Pagination_NonUTC_Timezone(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL is not set.")
		return
	}

	cfg, err := pgxpool.ParseConfig(url)
	require.NoError(t, err)
	// Force a non-UTC session timezone to reproduce the bug. PostgreSQL applies
	// this on every connection borrowed from the pool.
	cfg.ConnConfig.RuntimeParams["timezone"] = "America/Los_Angeles"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	_, err = pool.Exec(ctx, "DELETE FROM resources")
	require.NoError(t, err)

	client := NewPostgresClient(&postgresLogger{t: t, pool: pool})

	const total = 5
	saved := make([]database.Object, 0, total)
	for i := range total {
		idStr := fmt.Sprintf("%s/providers/%s/page%d", shared.ResourceGroup1Scope, shared.ResourceType1, i)
		id, err := resources.Parse(idStr)
		require.NoError(t, err)
		obj := database.Object{
			Metadata: database.Metadata{ID: id.String()},
			Data:     map[string]any{"value": fmt.Sprintf("p%d", i)},
		}
		require.NoError(t, client.Save(ctx, &obj))
		saved = append(saved, obj)
	}

	collected := []database.Object{}
	token := ""
	for page := 0; page < total+1; page++ {
		result, err := client.Query(
			ctx,
			database.Query{RootScope: shared.ResourceGroup1Scope, ResourceType: shared.ResourceType1},
			database.WithMaxQueryItemCount(2),
			database.WithPaginationToken(token),
		)
		require.NoError(t, err)
		collected = append(collected, result.Items...)
		if result.PaginationToken == "" {
			break
		}
		token = result.PaginationToken
	}

	shared.CompareObjectLists(t, saved, collected)
}
