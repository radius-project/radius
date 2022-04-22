// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dataprovider

import (
	"context"
	"errors"
	"sync"

	"github.com/project-radius/radius/pkg/store"
	"github.com/project-radius/radius/pkg/store/cosmosdb"
)

var (
	ErrUnsupportedStorageProvider = errors.New("unsupported storage provider")
	ErrStorageNotFound            = errors.New("storage provider not found")
)

var _ DataStorageProvider = (*storageProvider)(nil)

type storageProvider struct {
	clients map[string]store.StorageClient
	options StorageProviderOptions
	lock    sync.Mutex
}

// NewStorageProvider creates new storageProvider instance.
func NewStorageProvider(opts StorageProviderOptions) DataStorageProvider {
	return &storageProvider{
		clients: map[string]store.StorageClient{},
		options: opts,
		lock:    sync.Mutex{},
	}
}

// GetStorageClient creates or gets storage client.
func (p *storageProvider) GetStorageClient(ctx context.Context, storageName string) (store.StorageClient, error) {
	if c, ok := p.clients[storageName]; ok {
		return c, nil
	}
	if err := p.init(ctx, storageName); err != nil {
		return nil, err
	}

	return p.clients[storageName], nil
}

func (p *storageProvider) init(ctx context.Context, storageName string) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	// return immediately if someone already init storage client for storageName.
	if _, ok := p.clients[storageName]; ok {
		return nil
	}

	var dbclient store.StorageClient
	var err error

	switch p.options.Provider {
	case CosmosDBProvider:
		dbclient, err = initCosmosDBProvider(ctx, p.options.CosmosDB, storageName)
	// TODO: Support the other database storage client.
	default:
		err = ErrUnsupportedStorageProvider
	}

	if err != nil {
		return err
	}

	p.clients[storageName] = dbclient

	return nil
}

func initCosmosDBProvider(ctx context.Context, opt CosmosDBOptions, storageName string) (store.StorageClient, error) {
	sopt := &cosmosdb.ConnectionOptions{
		Url:            opt.Url,
		DatabaseName:   opt.Database,
		CollectionName: storageName,
		MasterKey:      opt.MasterKey,
	}
	dbclient, err := cosmosdb.NewCosmosDBStorageClient(sopt)
	if err != nil {
		return nil, err
	}

	if err = dbclient.Init(ctx); err != nil {
		return nil, err
	}

	return dbclient, nil
}
