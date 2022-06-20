// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/cli/clients"

	"golang.org/x/sync/errgroup"
)

const (
	AzureConnectionARMResourceType = "AzureConnection"
)

type LegacyARMManagementClient struct {
	Connection      *arm.Connection
	ResourceGroup   string
	SubscriptionID  string
	EnvironmentName string
}

var _ clients.LegacyManagementClient = (*LegacyARMManagementClient)(nil)

func (dm *LegacyARMManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) (*radclient.RadiusResourceList, error) {
	radiusResourceClient := radclient.NewRadiusResourceClient(dm.Connection, dm.SubscriptionID)

	response, err := radiusResourceClient.List(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Resources not found in application '%s' and environment '%s'", applicationName, dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}
	result, err := SortResourceList(response.RadiusResourceList)
	if err != nil {
		return nil, err
	}

	return &result, err
}

func SortResourceList(list radclient.RadiusResourceList) (radclient.RadiusResourceList, error) {
	sort.Slice(list.Value, func(i, j int) bool {
		t1 := *list.Value[i].Resource.Type
		t2 := *list.Value[j].Resource.Type

		n1 := *list.Value[i].Resource.Name
		n2 := *list.Value[j].Resource.Name

		if t1 != t2 {
			return t1 < t2
		}

		return n1 < n2

	})
	return list, nil
}

func (dm *LegacyARMManagementClient) ListApplications(ctx context.Context) (*radclient.ApplicationList, error) {
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)
	response, err := ac.List(ctx, dm.ResourceGroup, nil)
	if err != nil {
		if isEnvNotFound(err) {
			errorMessage := fmt.Sprintf("Environment '%s' not found ", dm.EnvironmentName)
			return nil, radclient.NewRadiusError("EnvironmentNotFound", errorMessage)
		}
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Applications not found in environment '%s'", dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}
	return &response.ApplicationList, nil
}

func (dm *LegacyARMManagementClient) ShowApplication(ctx context.Context, applicationName string) (*radclient.ApplicationResource, error) {
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)
	response, err := ac.Get(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Application '%s' not found in environment '%s'", applicationName, dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}
	return &response.ApplicationResource, err
}

func (dm *LegacyARMManagementClient) DeleteApplication(ctx context.Context, appName string) error {
	con, sub, rg := dm.Connection, dm.SubscriptionID, dm.ResourceGroup
	radiusResourceClient := radclient.NewRadiusResourceClient(con, sub)
	resp, err := radiusResourceClient.List(ctx, dm.ResourceGroup, appName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Application  %q not found in environment %q", appName, dm.EnvironmentName)
			return radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return err
	}

	g, errGroupCtx := errgroup.WithContext(ctx)
	for _, resource := range resp.RadiusResourceList.Value {

		if !strings.HasPrefix(*resource.Type, "Microsoft.CustomProviders") {
			// Only Radius resource types are deleted by Radius RP. Example of a Radius resource type: Microsoft.CustomProviders/mongo.com.MongoDatabase
			// Connection to Azure resources is returned as a part of radius resource list response, but lifecycle of these resources is not managed by Radius RP and should be explicitly deleted separately.
			// TODO: "Microsoft.CustomProviders" should be updated to reflect Radius RP name once we move out of custom RP mode:
			// https://github.com/project-radius/radius/issues/1637
			continue
		}

		r := *resource // prevent loopclouse issues (see https://pkg.go.dev/cmd/vet for more info)
		g.Go(func() error {
			types := strings.Split(*r.Type, "/")
			resourceType := types[len(types)-1]
			poller, err := radclient.NewRadiusResourceClient(con, sub).BeginDelete(
				errGroupCtx, rg, appName, resourceType, *r.Name, nil)
			if err != nil {
				return err
			}

			_, err = poller.PollUntilDone(errGroupCtx, radclient.PollInterval)
			if err != nil {
				if isNotFound(err) {
					errorMessage := fmt.Sprintf("Resource %s/%s not found in application '%s' environment '%s'",
						resourceType, *r.Name, appName, dm.EnvironmentName)
					return radclient.NewRadiusError("ResourceNotFound", errorMessage)
				}
				return err
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	poller, err := radclient.NewApplicationClient(con, sub).BeginDelete(ctx, rg, appName, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(ctx, radclient.PollInterval)
	if isNotFound(err) {
		errorMessage := fmt.Sprintf("Application  %q not found in environment %q", appName, dm.EnvironmentName)
		return radclient.NewRadiusError("ResourceNotFound", errorMessage)
	}
	return err
}

func (dm *LegacyARMManagementClient) ShowResource(ctx context.Context, appName, resourceType, resourceName, resourceGroup, resourceSubscriptionID string) (interface{}, error) {
	var options radclient.RadiusResourceGetOptions

	if KnownAzureResourceType(resourceType) {
		options = radclient.RadiusResourceGetOptions{
			ResourceGroup:          to.StringPtr(resourceGroup),
			ResourceType:           to.StringPtr(resourceType),
			ResourceSubscriptionID: to.StringPtr(resourceSubscriptionID),
		}

		// For Azure resources full resource type is passed in properties. "Azure" is used as generic type to route all azure resource types requests to Radius RP.
		resourceType = AzureConnectionARMResourceType
	}

	client := radclient.NewRadiusResourceClient(dm.Connection, dm.SubscriptionID)
	result, err := client.Get(ctx, dm.ResourceGroup, appName, resourceType, resourceName, &options)
	if err != nil {
		return nil, err
	}
	return result.RadiusResource, nil
}

func isNotFound(err error) bool {
	var httpresp azcore.HTTPResponse
	ok := errors.As(err, &httpresp)
	return ok && httpresp.RawResponse().StatusCode == http.StatusNotFound
}

func isEnvNotFound(err error) bool {
	if err, ok := err.(*net.OpError); ok {
		if err, ok := err.Err.(*net.DNSError); ok && err.IsNotFound {
			return true
		}
	}
	return false
}

func KnownAzureResourceType(resourceType string) bool {
	if strings.HasPrefix(resourceType, "Microsoft.") && !strings.HasPrefix(resourceType, "Microsoft.CustomProviders") {
		return true
	}

	return false
}
