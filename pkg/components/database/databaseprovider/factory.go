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
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"syscall"
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
	if err := waitForPostgreSQL(ctx, pool.Ping, defaultPostgresWaitOptions); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to initialize PostgreSQL client: %w", err)
	}

	return postgres.NewPostgresClient(pool), nil
}

// postgresWaitOptions controls how startup waits for PostgreSQL to accept connections.
type postgresWaitOptions struct {
	// timeout bounds the total wait.
	timeout time.Duration
	// attemptTimeout bounds each connection attempt so a dropped packet cannot consume the whole wait.
	attemptTimeout time.Duration
	// initialDelay is the first delay between attempts. It doubles up to maxDelay.
	initialDelay time.Duration
	maxDelay     time.Duration
}

var defaultPostgresWaitOptions = postgresWaitOptions{
	timeout:        3 * time.Minute,
	attemptTimeout: 5 * time.Second,
	initialDelay:   250 * time.Millisecond,
	maxDelay:       2 * time.Second,
}

// postgresCannotConnectNow is the SQLSTATE PostgreSQL returns while it is starting up.
const postgresCannotConnectNow = "57P03"

// waitForPostgreSQL calls ping until it succeeds, a non-retryable error is returned, the timeout
// elapses, or ctx is cancelled.
func waitForPostgreSQL(ctx context.Context, ping func(context.Context) error, opts postgresWaitOptions) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	waitCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	delay := opts.initialDelay
	for attempt := 1; ; attempt++ {
		attemptCtx, cancelAttempt := context.WithTimeout(waitCtx, opts.attemptTimeout)
		err := ping(attemptCtx)
		cancelAttempt()
		if err == nil {
			if attempt > 1 {
				logger.Info("PostgreSQL is accepting connections", "attempts", attempt)
			}
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
		if waitCtx.Err() != nil {
			return fmt.Errorf("timed out waiting for PostgreSQL to accept connections: %w", err)
		}
		if !isRetryablePostgreSQLError(err) {
			return err
		}

		if attempt == 1 {
			logger.Info("Waiting for PostgreSQL to accept connections", "error", err.Error())
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCtx.Done():
			return fmt.Errorf("timed out waiting for PostgreSQL to accept connections: %w", err)
		case <-time.After(delay):
		}

		delay = min(delay*2, opts.maxDelay)
	}
}

// isRetryablePostgreSQLError reports whether err is a transient connectivity error that may resolve
// on its own while the database starts. Everything else, including authentication, authorization,
// TLS and certificate errors, fails fast so misconfiguration is reported immediately.
func isRetryablePostgreSQLError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// "cannot_connect_now" is returned while PostgreSQL is still starting up.
		return pgErr.Code == postgresCannotConnectNow
	}

	// The per-attempt timeout elapsed. The caller has already checked the overall deadline.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// The server closed the connection, for example while it is restarting.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Nothing is listening yet, or the pod network is not routable yet.
	for _, errno := range []syscall.Errno{syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED, syscall.EHOSTUNREACH, syscall.ENETUNREACH} {
		if errors.Is(err, errno) {
			return true
		}
	}

	// The service name is not resolvable yet.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
