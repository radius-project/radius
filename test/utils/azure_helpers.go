// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package utils

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-05-01/resources"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2015-06-15/storage"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/test/config"
)

var (
	// ResourcesPkgAPIVersion is the API version of the azure go-sdk resources package used
	ResourcesPkgAPIVersion string = "2019-05-01"
)

// WithAPIVersion returns a prepare decorator that changes the request's query for api-version
// This can be set up as a client's RequestInspector.
func WithAPIVersion(apiVersion string) autorest.PrepareDecorator {
	return func(p autorest.Preparer) autorest.Preparer {
		return autorest.PreparerFunc(func(r *http.Request) (*http.Request, error) {
			r, err := p.Prepare(r)
			if err == nil {
				v := r.URL.Query()
				d, err := url.QueryUnescape(apiVersion)
				if err != nil {
					return r, err
				}
				v.Set("api-version", d)
				r.URL.RawQuery = v.Encode()
			}
			return r, err
		})
	}
}

// GetGroup gets info on the resource group in use
func GetGroup(ctx context.Context, groupName string) (resources.Group, error) {
	groupsClient, err := config.AzureConfig.GetGroupsClient()
	if err != nil {
		return resources.Group{}, err
	}
	return groupsClient.Get(ctx, groupName)
}

// DeleteGroup deletes the resource group
func DeleteGroup(ctx context.Context, groupName string) (result resources.GroupsDeleteFuture, err error) {
	groupsClient, err := config.AzureConfig.GetGroupsClient()
	if err != nil {
		return resources.GroupsDeleteFuture{}, err
	}
	return groupsClient.Delete(ctx, groupName)
}

// ListResourcesInResourceGroup gets all resources in resource group
func ListResourcesInResourceGroup(ctx context.Context, groupName string) (resources.ListResultPage, error) {
	resourcesClient, err := config.AzureConfig.GetResourcesClient()
	if err != nil {
		return resources.ListResultPage{}, err
	}
	resourcesClient.RequestInspector = WithAPIVersion(ResourcesPkgAPIVersion)
	var top10 int32 = 10
	resourcesInRg, err := resourcesClient.ListByResourceGroup(ctx, groupName, "", "", &top10)
	return resourcesInRg, err
}

// GetContainer gets info about an existing container.
func GetContainer(ctx context.Context, accountName, accountGroupName, containerName string) (azblob.ContainerURL, error) {
	c := getContainerURL(ctx, accountName, accountGroupName, containerName)

	_, err := c.GetProperties(ctx, azblob.LeaseAccessConditions{})
	return c, err
}

var (
	blobFormatString = `https://%s.blob.core.windows.net`
)

func getContainerURL(ctx context.Context, accountName, accountGroupName, containerName string) azblob.ContainerURL {
	key := getAccountPrimaryKey(ctx, accountName, accountGroupName)
	c, _ := azblob.NewSharedKeyCredential(accountName, key)
	p := azblob.NewPipeline(c, azblob.PipelineOptions{})
	u, _ := url.Parse(fmt.Sprintf(blobFormatString, accountName))
	service := azblob.NewServiceURL(*u, p)
	container := service.NewContainerURL(containerName)
	return container
}

func getAccountPrimaryKey(ctx context.Context, accountName, accountGroupName string) string {
	response, err := GetAccountKeys(ctx, accountName, accountGroupName)
	if err != nil {
		log.Fatalf("failed to list keys: %v", err)
	}
	return *response.Key1
}

// GetAccountKeys gets the storage account keys
func GetAccountKeys(ctx context.Context, accountName, accountGroupName string) (storage.AccountKeys, error) {
	accountsClient, err := config.AzureConfig.GetStorageAccountsClient()
	if err != nil {
		return storage.AccountKeys{}, err
	}
	return accountsClient.ListKeys(ctx, accountGroupName, accountName)
}

// AcquireStorageContainerLease acquires an infinite lease on the storage container
func AcquireStorageContainerLease(ctx context.Context, accountName, accountGroupName, containerName string) error {
	container, _ := GetContainer(ctx, accountName, accountGroupName, containerName)
	_, err := container.AcquireLease(ctx, "", -1, azblob.ModifiedAccessConditions{})
	return err
}

// BreakStorageContainerLease breaks a lease on the storage container
func BreakStorageContainerLease(ctx context.Context, accountName, accountGroupName, containerName string) {
	// Break lease on the test cluster to make it available for other tests
	container, _ := GetContainer(ctx, accountName, accountGroupName, containerName)
	_, err := container.BreakLease(ctx, -1, azblob.ModifiedAccessConditions{})
	if err != nil {
		fmt.Println("Error breaking lease: " + err.Error())
	}
}
