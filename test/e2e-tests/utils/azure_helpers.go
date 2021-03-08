// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package utils

import (
	"context"
	"net/http"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/test/e2e-tests/config"
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
