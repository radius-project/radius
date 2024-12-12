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

package dataprovider

import (
	context "context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/radius-project/radius/pkg/kubeutil"
	store "github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/pkg/ucp/store/apiserverstore"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/ucp/store/etcdstore"
	"github.com/radius-project/radius/pkg/ucp/store/inmemory"
	"github.com/radius-project/radius/pkg/ucp/store/postgres"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type storageFactoryFunc func(ctx context.Context, options StorageProviderOptions) (store.StorageClient, error)

var storageClientFactory = map[StorageProviderType]storageFactoryFunc{
	TypeAPIServer:  initAPIServerClient,
	TypeETCD:       InitETCDClient,
	TypeInMemory:   initInMemoryClient,
	TypePostgreSQL: initPostgreSQLClient,
}

func initAPIServerClient(ctx context.Context, opt StorageProviderOptions) (store.StorageClient, error) {
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

// InitETCDClient checks if the ETCD client is in memory and if the client is not nil, then it initializes the storage
// client and returns an ETCDClient. If either of these conditions are not met, an error is returned.
func InitETCDClient(ctx context.Context, opt StorageProviderOptions) (store.StorageClient, error) {
	if !opt.ETCD.InMemory {
		return nil, errors.New("failed to initialize etcd client: inmemory is the only supported mode for now")
	}
	if opt.ETCD.Client == nil {
		return nil, errors.New("failed to initialize etcd client: ETCDOptions.Client is nil, this is a bug")
	}

	// Initialize the storage client once the storage service has started
	client, err := opt.ETCD.Client.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize etcd client: %w", err)
	}

	etcdClient := etcdstore.NewETCDClient(client)
	return etcdClient, nil
}

// initInMemoryClient creates a new in-memory store client.
func initInMemoryClient(ctx context.Context, opt StorageProviderOptions) (store.StorageClient, error) {
	return inmemory.NewClient(), nil
}

// initPostgreSQLClient creates a new PostgreSQL store client.
func initPostgreSQLClient(ctx context.Context, opt StorageProviderOptions) (store.StorageClient, error) {
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
