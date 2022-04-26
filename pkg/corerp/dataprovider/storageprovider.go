// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dataprovider

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode"

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

// NewStorageProvider creates new DataStorageProvider instance.
func NewStorageProvider(opts StorageProviderOptions) DataStorageProvider {
	return &storageProvider{
		clients: map[string]store.StorageClient{},
		options: opts,
	}
}

// GetStorageClient creates or gets storage client.
func (p *storageProvider) GetStorageClient(ctx context.Context, resourceType string) (store.StorageClient, error) {
	cn := normalizeResourceType(resourceType)
	if c, ok := p.clients[cn]; ok {
		return c, nil
	}
	if err := p.init(ctx, resourceType); err != nil {
		return nil, err
	}

	return p.clients[cn], nil
}

func (p *storageProvider) init(ctx context.Context, resourceType string) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	cn := normalizeResourceType(resourceType)

	// return immediately if someone already init storage client for storageName.
	if _, ok := p.clients[cn]; ok {
		return nil
	}

	var dbclient store.StorageClient
	var err error

	switch p.options.Provider {
	case CosmosDBProvider:
		dbclient, err = initCosmosDBProvider(ctx, p.options.CosmosDB, cn)
	// TODO: Support the other database storage client.
	default:
		err = ErrUnsupportedStorageProvider
	}

	if err != nil {
		return err
	}

	p.clients[cn] = dbclient

	return nil
}

func initCosmosDBProvider(ctx context.Context, opt CosmosDBOptions, collectionName string) (store.StorageClient, error) {
	sopt := &cosmosdb.ConnectionOptions{
		Url:            opt.Url,
		DatabaseName:   opt.Database,
		CollectionName: collectionName,
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

// normalizeResourceType converts resourcetype to safe string by removing non digit and non letter and replace '/' with '-'
func normalizeResourceType(s string) string {
	if s == "" {
		return s
	}

	sb := strings.Builder{}
	for _, ch := range s {
		if ch == '/' {
			sb.WriteString("-")
		} else if unicode.IsDigit(ch) || unicode.IsLetter(ch) {
			sb.WriteRune(ch)
		}
	}

	return strings.ToLower(sb.String())
}
