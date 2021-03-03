// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package azurehelpers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/marstr/randname"
)

// GenerateGroupName leverages BaseGroupName() to return a more detailed name,
// helping to avoid collisions.  It appends each of the `affixes` to
// BaseGroupName() separated by dashes, and adds a 5-character random string.
func GenerateGroupName(baseGroupName string, affixes ...string) string {
	// go1.10+
	// import strings
	// var b strings.Builder
	// b.WriteString(BaseGroupName())
	b := bytes.NewBufferString(baseGroupName)
	b.WriteRune('-')
	for _, affix := range affixes {
		b.WriteString(affix)
		b.WriteRune('-')
	}
	return randname.GenerateWithPrefix(b.String(), 5)
}

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
func GetGroup(ctx context.Context, subscriptionID, groupName, userAgent string, authorizer *autorest.Authorizer) (resources.Group, error) {
	groupsClient := getGroupsClient(subscriptionID, userAgent, authorizer)
	return groupsClient.Get(ctx, groupName)
}

// ListResourcesInResourceGroup gets all resources in resource group
func ListResourcesInResourceGroup(ctx context.Context, subscriptionID, userAgent, groupName, apiVersion string, authorizer *autorest.Authorizer) (resources.ListResultPage, error) {
	resourcesClient := getResourcesClient(subscriptionID, userAgent, authorizer)
	resourcesClient.RequestInspector = WithAPIVersion(apiVersion)
	var top10 int32 = 10
	resourcesInRg, err := resourcesClient.ListByResourceGroup(ctx, groupName, "", "", &top10)
	fmt.Printf("Resources found: %v\n", resourcesInRg)
	return resourcesInRg, err
}

func getResourcesClient(subscriptionID, userAgent string, authorizer *autorest.Authorizer) resources.Client {
	resourcesClient := resources.NewClient(subscriptionID)
	resourcesClient.Authorizer = *authorizer
	resourcesClient.AddToUserAgent(userAgent)
	return resourcesClient
}

func getGroupsClient(subscriptionID, userAgent string, authorizer *autorest.Authorizer) resources.GroupsClient {
	groupsClient := resources.NewGroupsClient(subscriptionID)
	groupsClient.Authorizer = *authorizer
	groupsClient.AddToUserAgent(userAgent)
	return groupsClient
}
