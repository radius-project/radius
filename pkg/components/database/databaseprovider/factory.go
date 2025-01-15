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
	"regexp"

	"github.com/jackc/pgx/v5/pgxpool"
	store "github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/apiserverstore"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/database/postgres"
	"github.com/radius-project/radius/pkg/kubeutil"
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

// initPostgreSQLClient creates a new PostgreSQL store client.
func initPostgreSQLClient(ctx context.Context, opt Options) (store.Client, error) {
	if opt.PostgreSQL.URL == "" {
		return nil, errors.New("failed to initialize PostgreSQL client: URL is required")
	}

	url := opt.PostgreSQL.URL
	regex := regexp.MustCompile(`$\{([a-zA-Z_]+)\}`)
	matches := regex.FindSubmatch([]byte(opt.PostgreSQL.URL))
	if len(matches) > 1 {
		// Extract the captured expression.
		url = string(matches[1])
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL client: %w", err)
	}

	return postgres.NewPostgresClient(pool), nil
}
