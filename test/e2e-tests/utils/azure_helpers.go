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
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/radius/test/e2e-tests/config"
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
	groupsClient := getGroupsClient()
	return groupsClient.Get(ctx, groupName)
}

// DeleteGroup deletes the resource group
func DeleteGroup(ctx context.Context, groupName string) (resources.GroupsDeleteFuture, error) {
	groupsClient := getGroupsClient()
	return groupsClient.Delete(ctx, groupName)
}

// ListResourcesInResourceGroup gets all resources in resource group
func ListResourcesInResourceGroup(ctx context.Context, groupName string, apiVersion string) (resources.ListResultPage, error) {
	resourcesClient := getResourcesClient()
	resourcesClient.RequestInspector = WithAPIVersion(apiVersion)
	var top10 int32 = 10
	resourcesInRg, err := resourcesClient.ListByResourceGroup(ctx, groupName, "", "", &top10)
	fmt.Printf("Resources found: %v\n", resourcesInRg)
	return resourcesInRg, err
}

func getResourcesClient() resources.Client {
	resourcesClient := resources.NewClient(config.SubscriptionID())
	// a, _ := iam.GetResourceManagementAuthorizer()
	a, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		fmt.Println("Failed to init authorizer")
		return resources.Client{}
	}
	resourcesClient.Authorizer = a
	// _ := resourcesClient.AddToUserAgent(config.UserAgent())
	return resourcesClient
}

func getGroupsClient() resources.GroupsClient {
	groupsClient := resources.NewGroupsClient(config.SubscriptionID())
	a, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		log.Fatalf("failed to initialize authorizer: %v\n", err)
	}
	groupsClient.Authorizer = a
	// groupsClient.AddToUserAgent(config.UserAgent())
	return groupsClient
}
