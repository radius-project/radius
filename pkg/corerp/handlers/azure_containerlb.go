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

package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/radius-project/radius/pkg/azure/armauth"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

func NewAzureContainerLoadBalancerHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureContainerLoadBalancerHandler{arm: arm}
}

type azureContainerLoadBalancerHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureContainerLoadBalancerHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	lb, ok := options.Resource.CreateResource.Data.(*armnetwork.LoadBalancer)
	if !ok {
		return nil, errors.New("cannot parse load balancer")
	}

	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return nil, fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	vnetID := options.Resource.ID.Truncate()

	networkClientFactory, err := armnetwork.NewClientFactory(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return nil, err
	}
	loadBalancersClient := networkClientFactory.NewLoadBalancersClient()

	var lastErr error
	for retry := 0; retry < 5; retry++ {
		lbresp, err := loadBalancersClient.Get(ctx, resourceGroupName, vnetID.Name(), nil)
		if err != nil {
			return nil, err
		}

		updateRequired := false

		found := false
		for _, conf := range lbresp.LoadBalancer.Properties.FrontendIPConfigurations {
			if to.String(conf.Name) == *lb.Properties.FrontendIPConfigurations[0].Name {
				found = true
				break
			}
		}
		if !found {
			updateRequired = true
			lbresp.LoadBalancer.Properties.FrontendIPConfigurations = append(lbresp.LoadBalancer.Properties.FrontendIPConfigurations, lb.Properties.FrontendIPConfigurations...)
		}

		found = false
		for _, pool := range lbresp.LoadBalancer.Properties.BackendAddressPools {
			if to.String(pool.Name) == *lb.Properties.BackendAddressPools[0].Name {
				found = true
				break
			}
		}
		if !found {
			updateRequired = true
			lbresp.LoadBalancer.Properties.BackendAddressPools = append(lbresp.LoadBalancer.Properties.BackendAddressPools, lb.Properties.BackendAddressPools...)
		}

		found = false
		for _, r := range lbresp.LoadBalancer.Properties.LoadBalancingRules {
			if to.String(r.Name) == *lb.Properties.LoadBalancingRules[0].Name {
				found = true
				break
			}
		}

		if !found {
			updateRequired = true
			lbresp.LoadBalancer.Properties.LoadBalancingRules = append(lbresp.LoadBalancer.Properties.LoadBalancingRules, lb.Properties.LoadBalancingRules...)
		}

		found = false
		for _, pr := range lbresp.LoadBalancer.Properties.Probes {
			if to.String(pr.Name) == *lb.Properties.Probes[0].Name {
				found = true
				break
			}
		}

		if !found {
			updateRequired = true
			lbresp.LoadBalancer.Properties.Probes = append(lbresp.LoadBalancer.Properties.Probes, lb.Properties.Probes...)
		}

		if updateRequired {
			poller, err := loadBalancersClient.BeginCreateOrUpdate(ctx, resourceGroupName, to.String(lb.Name), lbresp.LoadBalancer, nil)
			if err != nil {
				logger.Info("failed to update load balancer", "retry", retry, "error", err.Error())
				fmt.Printf("failed to update load balancer: %s, retry: %d", err.Error(), retry)
				lastErr = err
				time.Sleep(60 * time.Second)
				continue
			}

			_, err = poller.PollUntilDone(ctx, nil)
			if err != nil {
				logger.Info("failed to poll load balancer update", "retry", retry, "error", err.Error())
				fmt.Printf("failed to poll load balancer: %s, retry: %d", err.Error(), retry)
				lastErr = err
				// Retry if the request was cancelled by the following errors:
				// {\n  \"status\": \"Canceled\",\n  \"error\": {\n    \"code\": \"Canceled\",\n    \"message\": \"Operation was canceled.\",\n    \"details\": [\n      {\n        \"code\": \"CanceledAndSupersededDueToAnotherOperation\",\n        \"message\": \"Operation PutLoadBalancerOperation (52037801-74e8-4660-b359-39055aa02d7c) was canceled and superseded by operation PutLoadBalancerOperation (bb71e36a-3f38-4762-b28e-1cc2fbb44ca9).\"\n      }\n    ]\n  }\n}\n
				// This happens when multiple container deployment updates LB. Needs to have the better way.
				time.Sleep(60 * time.Second)
				continue
			}
		}
		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, lastErr
	}

	lbresp, err := loadBalancersClient.Get(ctx, resourceGroupName, vnetID.Name(), nil)
	if err != nil {
		return nil, err
	}

	hostname := ""
	if len(lb.Properties.FrontendIPConfigurations) > 0 {
		for _, conf := range lbresp.Properties.FrontendIPConfigurations {
			if to.String(conf.Name) == to.String(lb.Properties.FrontendIPConfigurations[0].Name) {
				hostname = to.String(conf.Properties.PrivateIPAddress)
			}
		}
	}

	properties := map[string]string{
		"hostname": hostname,
	}

	return properties, nil
}

func (handler *azureContainerLoadBalancerHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")

	vnetID := options.Resource.ID.Truncate()

	networkClientFactory, err := armnetwork.NewClientFactory(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return err
	}

	loadBalancersClient := networkClientFactory.NewLoadBalancersClient()

	var lastErr error
	for retry := 0; retry < 5; retry++ {
		lbresp, err := loadBalancersClient.Get(ctx, resourceGroupName, vnetID.Name(), nil)
		if err != nil {
			return err
		}

		appName := options.Resource.ID.Name()
		updateRequired := false

		deletedFrontend := []*armnetwork.FrontendIPConfiguration{}
		for _, conf := range lbresp.LoadBalancer.Properties.FrontendIPConfigurations {
			if to.String(conf.Name) == appName {
				updateRequired = true
				continue
			}

			deletedFrontend = append(deletedFrontend, conf)
		}
		lbresp.LoadBalancer.Properties.FrontendIPConfigurations = deletedFrontend

		backendaddr := []*armnetwork.BackendAddressPool{}
		for _, pool := range lbresp.LoadBalancer.Properties.BackendAddressPools {
			if to.String(pool.Name) == appName {
				updateRequired = true
				continue
			}
			backendaddr = append(backendaddr, pool)
		}
		lbresp.LoadBalancer.Properties.BackendAddressPools = backendaddr

		lbrs := []*armnetwork.LoadBalancingRule{}
		for _, lbrule := range lbresp.LoadBalancer.Properties.LoadBalancingRules {
			if to.String(lbrule.Name) == appName {
				updateRequired = true
				continue
			}
			lbrs = append(lbrs, lbrule)
		}
		lbresp.LoadBalancer.Properties.LoadBalancingRules = lbrs

		probe := []*armnetwork.Probe{}
		for _, p := range lbresp.LoadBalancer.Properties.Probes {
			if to.String(p.Name) == appName {
				updateRequired = true
				continue
			}
			probe = append(probe, p)
		}
		lbresp.LoadBalancer.Properties.Probes = probe

		if updateRequired {
			poller, err := loadBalancersClient.BeginCreateOrUpdate(ctx, resourceGroupName, to.String(lbresp.Name), lbresp.LoadBalancer, nil)
			if err != nil {
				fmt.Printf("failed to update load balancer: %s, retry: %d", err, retry)
				lastErr = err
				time.Sleep(60 * time.Second)
				continue
			}

			_, err = poller.PollUntilDone(ctx, nil)
			if err != nil {
				fmt.Printf("failed to poll load balancer: %s, retry: %d", err, retry)
				lastErr = err
				time.Sleep(60 * time.Second)
				continue
			}
		}
		lastErr = nil
		break
	}

	return lastErr
}
