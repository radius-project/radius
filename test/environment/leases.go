// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environment

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/go-autorest/autorest"
)

var (
	blobFormatString = `https://%s.blob.core.windows.net`
)

// AcquireStorageContainerLease acquires an infinite lease on the storage container
func AcquireStorageContainerLease(ctx context.Context, auth autorest.Authorizer, subscriptionID string, accountName, accountGroupName, containerName string) error {
	container, err := GetContainer(ctx, auth, subscriptionID, accountName, accountGroupName, containerName)
	if err != nil {
		return err
	}

	_, err = container.AcquireLease(ctx, "", -1, azblob.ModifiedAccessConditions{})
	return err
}

// BreakStorageContainerLease breaks a lease on the storage container
func BreakStorageContainerLease(ctx context.Context, auth autorest.Authorizer, subscriptionID string, accountName, accountGroupName, containerName string) error {
	// Break lease on the test cluster to make it available for other tests
	container, err := GetContainer(ctx, auth, subscriptionID, accountName, accountGroupName, containerName)
	if err != nil {
		return nil
	}
	_, err = container.BreakLease(ctx, -1, azblob.ModifiedAccessConditions{})
	return err
}

// GetContainer gets info about an existing container.
func GetContainer(ctx context.Context, auth autorest.Authorizer, subscriptionID string, accountName, accountGroupName, containerName string) (azblob.ContainerURL, error) {
	c, err := getContainerURL(ctx, auth, subscriptionID, accountName, accountGroupName, containerName)
	if err != nil {
		return azblob.ContainerURL{}, err
	}

	_, err = c.GetProperties(ctx, azblob.LeaseAccessConditions{})
	return c, err
}

func getContainerURL(ctx context.Context, auth autorest.Authorizer, subscriptionID string, accountName, accountGroupName, containerName string) (azblob.ContainerURL, error) {
	key, err := getAccountPrimaryKey(ctx, auth, subscriptionID, accountName, accountGroupName)
	if err != nil {
		return azblob.ContainerURL{}, err
	}

	c, err := azblob.NewSharedKeyCredential(accountName, key)
	if err != nil {
		return azblob.ContainerURL{}, err
	}

	p := azblob.NewPipeline(c, azblob.PipelineOptions{})
	u, err := url.Parse(fmt.Sprintf(blobFormatString, accountName))
	if err != nil {
		return azblob.ContainerURL{}, err
	}

	service := azblob.NewServiceURL(*u, p)
	container := service.NewContainerURL(containerName)
	return container, nil
}

func getAccountPrimaryKey(ctx context.Context, auth autorest.Authorizer, subscriptionID string, accountName, accountGroupName string) (string, error) {
	keys, err := GetAccountKeys(ctx, auth, subscriptionID, accountName, accountGroupName)
	if err != nil {
		return "", err
	}

	var key *storage.AccountKey
	for _, k := range keys {
		if strings.EqualFold(string(k.Permissions), string(storage.Full)) {
			key = &k
			break
		}
	}

	if key == nil {
		return "", fmt.Errorf("listkeys contained keys, but none of them have full access")
	}

	return *key.Value, nil
}

// GetAccountKeys gets the storage account keys
func GetAccountKeys(ctx context.Context, auth autorest.Authorizer, subscriptionID string, accountName, accountGroupName string) ([]storage.AccountKey, error) {
	accountsClient := storage.NewAccountsClient(subscriptionID)
	accountsClient.Authorizer = auth

	keys, err := accountsClient.ListKeys(ctx, accountGroupName, accountName, "")
	if err != nil {
		return []storage.AccountKey{}, fmt.Errorf("failed to query storage keys: %w", err)
	}

	// We don't expect this to happen, but just being defensive
	if keys.Keys == nil || len(*keys.Keys) == 0 {
		return nil, fmt.Errorf("listkeys returned an empty or nil list of keys")
	}

	return *keys.Keys, nil
}
