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

package databaseprovider

import (
	context "context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	store "github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/apiserverstore"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/database/postgres"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type databaseClientFactoryFunc func(ctx context.Context, options Options) (store.Client, error)

var databaseClientFactory = map[DatabaseProviderType]databaseClientFactoryFunc{
	TypeAPIServer:  initAPIServerClient,
	TypeInMemory:   initInMemoryClient,
	TypePostgreSQL: initPostgreSQLClient,
}

func initAPIServerClient(ctx context.Context, opt Options) (store.Client, error) {
	if opt.APIServer.Namespace == "" {
		return nil, errors.New("failed to initialize APIServer client: namespace is required")
	}

	cfg, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: opt.APIServer.Context,
		QPS:         kubeutil.DefaultServerQPS,
		Burst:       kubeutil.DefaultServerBurst,
	})
	if err != nil {
		return nil, err
	}

	// The client will log info the console that we don't really care about.
	cfg.WarningHandler = rest.NoWarnings{}

	// We only need to interact with UCP's store types.
	scheme := runtime.NewScheme()

	// Safe to ignore, this will only fail for duplicates, which there clearly won't be.
	_ = ucpv1alpha1.AddToScheme(scheme)

	options := runtimeclient.Options{
		Scheme: scheme,
	}

	rc, err := runtimeclient.New(cfg, options)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize APIServer client: %w", err)
	}

	client := apiserverstore.NewAPIServerClient(rc, opt.APIServer.Namespace)
	return client, nil
}

// initInMemoryClient creates a new in-memory store client.
func initInMemoryClient(ctx context.Context, opt Options) (store.Client, error) {
	return inmemory.NewClient(), nil
}

// envVarPattern matches ${VAR_NAME} references for expansion against environment variables.
var envVarPattern = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// expandEnvURL replaces ${VAR} references in the connection URL with the values of the
// corresponding environment variables. It returns an error if any referenced variable is unset,
// so a missing or mistyped variable fails fast with a clear message instead of silently producing
// a malformed connection string.
func expandEnvURL(url string) (string, error) {
	var missing []string
	expanded := envVarPattern.ReplaceAllStringFunc(url, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv(varName)
		if !ok {
			missing = append(missing, varName)
		}
		return val
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("required environment variable(s) not set: %s", strings.Join(missing, ", "))
	}
	return expanded, nil
}

// initPostgreSQLClient creates a new PostgreSQL store client.
func initPostgreSQLClient(ctx context.Context, opt Options) (store.Client, error) {
	if opt.PostgreSQL.URL == "" {
		return nil, errors.New("failed to initialize PostgreSQL client: URL is required")
	}

	url, err := expandEnvURL(opt.PostgreSQL.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL client: %w", err)
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL client: %w", err)
	}

	// pgxpool.New connects lazily. On a fresh install the database pod usually starts after the
	// control plane, so wait for it here instead of failing the first query and crash-looping the
	// process into kubelet's exponential restart back-off.
	if err := waitForPostgreSQL(ctx, pool.Ping, postgresStartupTimeout, postgresStartupInitialDelay, postgresStartupMaxDelay); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to initialize PostgreSQL client: %w", err)
	}

	return postgres.NewPostgresClient(pool), nil
}

const (
	// postgresStartupTimeout bounds how long startup waits for PostgreSQL to accept connections.
	postgresStartupTimeout = 3 * time.Minute

	// postgresStartupInitialDelay is the first delay between connection attempts.
	postgresStartupInitialDelay = 250 * time.Millisecond

	// postgresStartupMaxDelay caps the delay between connection attempts.
	postgresStartupMaxDelay = 2 * time.Second

	// postgresCannotConnectNow is the SQLSTATE PostgreSQL returns while it is starting up.
	postgresCannotConnectNow = "57P03"
)

// waitForPostgreSQL calls ping until it succeeds, a non-retryable error is returned, the timeout
// elapses, or ctx is cancelled.
func waitForPostgreSQL(ctx context.Context, ping func(context.Context) error, timeout, initialDelay, maxDelay time.Duration) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	delay := initialDelay
	for attempt := 1; ; attempt++ {
		err := ping(ctx)
		if err == nil {
			if attempt > 1 {
				logger.Info("PostgreSQL is accepting connections", "attempts", attempt)
			}
			return nil
		}

		if !isRetryablePostgreSQLError(err) {
			return err
		}

		if attempt == 1 {
			logger.Info("Waiting for PostgreSQL to accept connections", "error", err.Error())
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for PostgreSQL to accept connections: %w", err)
		case <-time.After(delay):
		}

		delay = min(delay*2, maxDelay)
	}
}

// isRetryablePostgreSQLError reports whether err may resolve on its own while the database starts.
// Errors reported by the server itself (for example, authentication failures) are not retried,
// except for "cannot_connect_now", which PostgreSQL returns while it is still starting up.
func isRetryablePostgreSQLError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == postgresCannotConnectNow
	}

	return true
}
