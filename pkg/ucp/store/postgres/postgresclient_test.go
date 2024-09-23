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
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/davecgh/go-spew/spew"
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
