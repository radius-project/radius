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

package environments

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/corerp/frontend/controller/util"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*CreateOrUpdateEnvironment)(nil)

// CreateOrUpdateEnvironments is the controller implementation to create or update environment resource.
type CreateOrUpdateEnvironment struct {
	ctrl.Operation[*datamodel.Environment, datamodel.Environment]
}

// NewCreateOrUpdateEnvironment creates a new controller for creating or updating an environment resource.
func NewCreateOrUpdateEnvironment(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateEnvironment{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Environment]{
				RequestConverter:  converter.EnvironmentDataModelFromVersioned,
				ResponseConverter: converter.EnvironmentDataModelToVersioned,
			},
		),
	}, nil
}

// Run checks if a resource with the same namespace already exists, and if not, updates the resource with the new values.
// If a resource with the same namespace already exists, a conflict response is returned.
func (e *CreateOrUpdateEnvironment) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := e.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if err := newResource.Properties.Compute.Identity.Validate(); err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// Create Query filter to query kubernetes namespace used by the other environment resources.
	namespace := newResource.Properties.Compute.KubernetesCompute.Namespace
	result, err := util.FindResources(ctx, serviceCtx.ResourceID.RootScope(), serviceCtx.ResourceID.Type(), "properties.compute.kubernetes.namespace", namespace, e.DatabaseClient())
	if err != nil {
		return nil, err
	}

	if len(result.Items) > 0 {
		env := &datamodel.Environment{}
		if err := result.Items[0].As(env); err != nil {
			return nil, err
		}

		// If a different resource has the same namespace, return a conflict
		// Otherwise, continue and update the resource
		if old == nil || env.ID != old.ID {
			return rest.NewConflictResponse(fmt.Sprintf("Environment %s with the same namespace (%s) already exists", env.ID, namespace)), nil
		}
	}

	if newResource.Properties.Compute.Kind == rpv1.ACIComputeKind {
		if err := e.createOrUpdateACIEnvironment(ctx, newResource); err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return e.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}

const (
	ApplicationAddressSpace = "10.1.0.0/16"
	LBAddressSpace          = "172.16.0.0/19"
	InternalLBSubnetName    = "internal-lb"
	InternalLBSubnet        = "172.16.1.0/24"
	AppGatewaySubnetName    = "app-gateway"
	AppGatewaySubnet        = "172.16.2.0/24"

	ResourceLocation string = "WestUS 3"
)

func (e *CreateOrUpdateEnvironment) createOrUpdateACIEnvironment(ctx context.Context, resource *datamodel.Environment) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// 1. Create NSG
	// 2. VNET - Address space. 10.1.0.0/16 for application , 172.16.0.0/16 for load balancer
	// 3. Subnets - 172.16.0.0/24 for internal load balancer, 172.16.1.0/24 for app gateway
	// 4. Internal load balancer

	rgID, err := resources.Parse(resource.Properties.Compute.ACICompute.ResourceGroup)
	if err != nil {
		return err
	}

	resourceGroupName := rgID.FindScope("resourceGroups")
	subscriptionID := rgID.FindScope("subscriptions")

	// Ensure resource group is created.
	err = clientv2.EnsureResourceGroupIsCreated(ctx, subscriptionID, resourceGroupName, ResourceLocation, &e.Options().Arm.ClientOptions)
	if err != nil {
		return err
	}

	networkClientFactory, err := armnetwork.NewClientFactory(subscriptionID, e.Options().Arm.ClientOptions.Cred, nil)
	if err != nil {
		return err
	}

	virtualNetworksClient := networkClientFactory.NewVirtualNetworksClient()

	envName := resource.Name
	vnetName := envName

	logger.Info("Creating VNET...", "VNET", vnetName, "SubscriptionID", subscriptionID, "ResourceGroup", resourceGroupName)

	_, err = virtualNetworksClient.Get(ctx, resourceGroupName, vnetName, nil)
	if err != nil {
		pollerResp, err := virtualNetworksClient.BeginCreateOrUpdate(
			ctx,
			resourceGroupName,
			vnetName,
			armnetwork.VirtualNetwork{
				Location: to.Ptr(ResourceLocation),
				Properties: &armnetwork.VirtualNetworkPropertiesFormat{
					AddressSpace: &armnetwork.AddressSpace{
						AddressPrefixes: []*string{
							to.Ptr(ApplicationAddressSpace),
							to.Ptr(LBAddressSpace),
						},
					},
				},
			},
			nil)

		if err != nil {
			return err
		}

		vnetResp, err := pollerResp.PollUntilDone(ctx, nil)
		if err != nil {
			return err
		}
		logger.Info("Created VNET.", "ID", vnetResp.ID)
	}

	// Create NSG for applications
	nsgName := envName + "-nsg"
	logger.Info("Creating NSG...", "NSG", nsgName, "SubscriptionID", subscriptionID, "ResourceGroup", resourceGroupName)

	securityGroupsClient := networkClientFactory.NewSecurityGroupsClient()
	nsgPoller, err := securityGroupsClient.BeginCreateOrUpdate(
		ctx,
		resourceGroupName,
		nsgName,
		armnetwork.SecurityGroup{
			Location: to.Ptr(ResourceLocation),
			Properties: &armnetwork.SecurityGroupPropertiesFormat{
				SecurityRules: []*armnetwork.SecurityRule{
					{
						Name: to.Ptr("AllowHTTPInbound"),
						Properties: &armnetwork.SecurityRulePropertiesFormat{
							Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
							SourceAddressPrefix:      to.Ptr("*"),
							SourcePortRange:          to.Ptr("*"),
							DestinationAddressPrefix: to.Ptr("*"),
							DestinationPortRange:     to.Ptr("443"),
							Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
							Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
							Priority:                 to.Ptr[int32](110),
						},
					},
				},
			},
		},
		nil)

	if err != nil {
		return err
	}

	internalSubnetNSG, err := nsgPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	logger.Info("Created internal subnet NSG.", "ID", internalSubnetNSG.ID)

	logger.Info("Creating subnet for internal load balancer...", "Subnet", InternalLBSubnet, "SubscriptionID", subscriptionID, "ResourceGroup", resourceGroupName)
	subnetsClient := networkClientFactory.NewSubnetsClient()
	subnetPoll, err := subnetsClient.BeginCreateOrUpdate(
		ctx,
		resourceGroupName,
		vnetName,
		InternalLBSubnetName,
		armnetwork.Subnet{
			Properties: &armnetwork.SubnetPropertiesFormat{
				AddressPrefix: to.Ptr(InternalLBSubnet),
				NetworkSecurityGroup: &armnetwork.SecurityGroup{
					ID: internalSubnetNSG.ID,
				},
			},
		},
		nil,
	)

	if err != nil {
		return err
	}

	subnetResp, err := subnetPoll.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	logger.Info("Created subnet for internal load balancer.", "ID", subnetResp.ID)

	internalLBName := envName + "-ilb"
	loadBalancersClient := networkClientFactory.NewLoadBalancersClient()
	lbPoller, err := loadBalancersClient.BeginCreateOrUpdate(ctx,
		resourceGroupName,
		internalLBName,
		armnetwork.LoadBalancer{
			SKU: &armnetwork.LoadBalancerSKU{
				Name: to.Ptr(armnetwork.LoadBalancerSKUNameStandard),
				Tier: to.Ptr(armnetwork.LoadBalancerSKUTierRegional),
			},
			Location:   to.Ptr(ResourceLocation),
			Properties: &armnetwork.LoadBalancerPropertiesFormat{
				// Following resources are not deleted if we do not specify etag
				// FrontendIPConfigurations: []*armnetwork.FrontendIPConfiguration{},
				// BackendAddressPools:      []*armnetwork.BackendAddressPool{},
				// Probes:                   []*armnetwork.Probe{},
				// LoadBalancingRules:       []*armnetwork.LoadBalancingRule{},
				// InboundNatRules:          []*armnetwork.InboundNatRule{},
			},
		}, nil)

	if err != nil {
		return err
	}

	lbResp, err := lbPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	logger.Info("Created internal load balancer.", "ID", lbResp.ID)

	return nil
}
