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

	"github.com/project-radius/radius/pkg/kubeutil"
	store "github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/store/apiserverstore"
	ucpv1alpha1 "github.com/project-radius/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/project-radius/radius/pkg/ucp/store/cosmosdb"
	"github.com/project-radius/radius/pkg/ucp/store/etcdstore"
	"k8s.io/apimachinery/pkg/runtime"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type storageFactoryFunc func(context.Context, StorageProviderOptions, string) (store.StorageClient, error)

var storageClientFactory = map[StorageProviderType]storageFactoryFunc{
	TypeAPIServer: initAPIServerClient,
	TypeCosmosDB:  initCosmosDBClient,
	TypeETCD:      InitETCDClient,
}

func initAPIServerClient(ctx context.Context, opt StorageProviderOptions, _ string) (store.StorageClient, error) {
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

	// We only need to interact with UCP's store types.
	scheme := runtime.NewScheme()

	// Safe to ignore, this will only fail for duplicates, which there clearly won't be.
	_ = ucpv1alpha1.AddToScheme(scheme)

	options := runtimeclient.Options{
		Scheme: scheme,

		// The client will log info the console that we don't really care about.
		Opts: runtimeclient.WarningHandlerOptions{
			SuppressWarnings: true,
		},
	}

	rc, err := runtimeclient.New(cfg, options)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize APIServer client: %w", err)
	}

	client := apiserverstore.NewAPIServerClient(rc, opt.APIServer.Namespace)
	return client, nil
}

func initCosmosDBClient(ctx context.Context, opt StorageProviderOptions, collectionName string) (store.StorageClient, error) {
	sopt := &cosmosdb.ConnectionOptions{
		Url:                  opt.CosmosDB.Url,
		DatabaseName:         opt.CosmosDB.Database,
		CollectionName:       collectionName,
		MasterKey:            opt.CosmosDB.MasterKey,
		CollectionThroughput: opt.CosmosDB.CollectionThroughput,
	}
	dbclient, err := cosmosdb.NewCosmosDBStorageClient(sopt)
	if err != nil {
		return nil, fmt.Errorf("failed to create CosmosDB client - configuration may be invalid: %w", err)
	}

	if err = dbclient.Init(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize CosmosDB client - configuration may be invalid: %w", err)
	}

	return dbclient, nil
}

// InitETCDClient initializes a new etcd client.
func InitETCDClient(ctx context.Context, opt StorageProviderOptions, _ string) (store.StorageClient, error) {
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
