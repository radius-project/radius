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

package gateway

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// hard-coded for PoC.
	aciLocation = "West US 3"
)

type Renderer struct {
}

func (r Renderer) GetDependencyIDs(ctx context.Context, dm v1.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	// no ops for aci
	return radiusResourceIDs, azureResourceIDs, nil
}

// Render creates a gateway object and http route objects based on the given parameters, and returns them along
// with a computed value for the gateway's public endpoint.
func (r Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	outputResources := []rpv1.OutputResource{}
	gateway, ok := dm.(*datamodel.Gateway)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	resourceGroupID := options.Environment.Compute.ACICompute.ResourceGroup

	if len(gateway.Properties.Routes) == 0 {
		return renderers.RendererOutput{}, errors.New("gateway must have at least one route")
	}

	// Extract the target container.
	_, containerName, targetPort, _ := parseURL(gateway.Properties.Routes[0].Destination)

	// Generate dns prefix for gateway public ip
	suffix := fmt.Sprintf("%x", sha1.Sum([]byte(resourceGroupID)))
	dnsPrefix := fmt.Sprintf("%s-%s", gateway.Name, suffix[:10])

	publicIP := &armnetwork.PublicIPAddress{
		Name:     to.Ptr(gateway.Name),
		Location: to.Ptr(aciLocation),
		SKU: &armnetwork.PublicIPAddressSKU{
			Name: to.Ptr(armnetwork.PublicIPAddressSKUNameStandard),
			Tier: to.Ptr(armnetwork.PublicIPAddressSKUTierRegional),
		},
		Properties: &armnetwork.PublicIPAddressPropertiesFormat{
			PublicIPAddressVersion:   to.Ptr(armnetwork.IPVersionIPv4),
			PublicIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodStatic),
			DNSSettings: &armnetwork.PublicIPAddressDNSSettings{
				DomainNameLabel: to.Ptr(dnsPrefix),
			},
		},
	}

	publicIPResource := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzurePublicIP,
		ID:      resources.MustParse(resourceGroupID + "/providers/Microsoft.Network/publicIPAddresses/" + gateway.Name),
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.Network/publicIPAddresses",
				Provider: resourcemodel.ProviderAzure,
			},
			Data: publicIP,
		},
	}

	outputResources = append(outputResources, publicIPResource)

	nsgName := gateway.Name + "-nsg"
	nsg := &armnetwork.SecurityGroup{
		Name:     to.Ptr(nsgName),
		Location: to.Ptr(aciLocation),
		Properties: &armnetwork.SecurityGroupPropertiesFormat{
			SecurityRules: []*armnetwork.SecurityRule{
				{
					Name: to.Ptr("AppGatewayV2ProbeInbound"),
					Properties: &armnetwork.SecurityRulePropertiesFormat{
						Description:              to.Ptr("Allow traffic from GatewayManager. This rule is needed for application gateway probes to work."),
						Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
						SourceAddressPrefix:      to.Ptr("GatewayManager"),
						SourcePortRange:          to.Ptr("*"),
						DestinationAddressPrefix: to.Ptr("*"),
						DestinationPortRange:     to.Ptr("65200-65535"),
						Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
						Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
						Priority:                 to.Ptr[int32](100),
					},
				},
				{
					Name: to.Ptr("AllowHTTPInbound"),
					Properties: &armnetwork.SecurityRulePropertiesFormat{
						Description:              to.Ptr("Allow Internet traffic on port 80."),
						Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
						SourceAddressPrefix:      to.Ptr("Internet"),
						SourcePortRange:          to.Ptr("*"),
						DestinationAddressPrefix: to.Ptr("*"),
						DestinationPortRange:     to.Ptr(fmt.Sprintf("%d", targetPort)),
						Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
						Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
						Priority:                 to.Ptr[int32](110),
					},
				},
				{
					Name: to.Ptr("AllowPublicIPAddress"),
					Properties: &armnetwork.SecurityRulePropertiesFormat{
						Description:              to.Ptr("Allow traffic from public ip address."),
						Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
						SourceAddressPrefix:      to.Ptr("Internet"),
						SourcePortRange:          to.Ptr("*"),
						DestinationAddressPrefix: to.Ptr("placeholder"),
						DestinationPortRange:     to.Ptr(fmt.Sprintf("%d", targetPort)),
						Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
						Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
						Priority:                 to.Ptr[int32](111),
					},
				},
				{
					Name: to.Ptr("AllowVirtualNetworkInbound"),
					Properties: &armnetwork.SecurityRulePropertiesFormat{
						Description:              to.Ptr("Allow Internet traffic to Virtual network."),
						Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
						SourceAddressPrefix:      to.Ptr("*"),
						SourcePortRange:          to.Ptr("*"),
						DestinationAddressPrefix: to.Ptr("VirtualNetwork"),
						DestinationPortRange:     to.Ptr(fmt.Sprintf("%d", targetPort)),
						Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
						Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
						Priority:                 to.Ptr[int32](112),
					},
				},
			},
		},
	}

	nsgResource := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureAppGWNetworkSecurityGroup,
		ID:      resources.MustParse(resourceGroupID + "/providers/Microsoft.Network/networkSecurityGroups/" + nsgName),
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.Network/networkSecurityGroups",
				Provider: resourcemodel.ProviderAzure,
			},
			Data:         nsg,
			Dependencies: []string{rpv1.LocalIDAzurePublicIP},
		},
	}

	outputResources = append(outputResources, nsgResource)

	appgwID := resourceGroupID + "/providers/Microsoft.Network/applicationGateways/" + gateway.Name

	subnet := &armnetwork.Subnet{
		Name: to.Ptr(gateway.Name),
		Type: to.Ptr("Microsoft.Network/virtualNetworks/subnets"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr("172.16.2.0/29"),
			ApplicationGatewayIPConfigurations: []*armnetwork.ApplicationGatewayIPConfiguration{
				{
					ID: to.Ptr(strings.Join([]string{appgwID, "gatewayIPConfigurations", gateway.Name}, "/")),
				},
			},
			NetworkSecurityGroup: &armnetwork.SecurityGroup{
				ID: to.Ptr(nsgResource.ID.String()),
			},
		},
	}

	envID := options.Environment.Resource
	vnetID := options.Environment.Compute.ACICompute.ResourceGroup + "/providers/Microsoft.Network/virtualNetworks/" + envID.Name()

	vnetSubnet := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureVirtualNetworkSubnet,
		ID:      resources.MustParse(vnetID + "/subnets/" + gateway.Name),
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.Network/virtualNetworks/subnets",
				Provider: resourcemodel.ProviderAzure,
			},
			Data:         subnet,
			Dependencies: []string{rpv1.LocalIDAzureAppGWNetworkSecurityGroup},
		},
	}

	outputResources = append(outputResources, vnetSubnet)

	// AppGateway
	frontendPortName := fmt.Sprintf("port_%d", targetPort)
	appgw := &armnetwork.ApplicationGateway{
		Name:     to.Ptr(gateway.Name),
		Location: to.Ptr(aciLocation),
		Properties: &armnetwork.ApplicationGatewayPropertiesFormat{
			SKU: &armnetwork.ApplicationGatewaySKU{
				Name: to.Ptr(armnetwork.ApplicationGatewaySKUNameStandardV2),
				Tier: to.Ptr(armnetwork.ApplicationGatewayTierStandardV2),
			},
			GatewayIPConfigurations: []*armnetwork.ApplicationGatewayIPConfiguration{
				{
					Name: to.Ptr(gateway.Name),
					Properties: &armnetwork.ApplicationGatewayIPConfigurationPropertiesFormat{
						Subnet: &armnetwork.SubResource{
							ID: to.Ptr(vnetID + "/subnets/" + gateway.Name),
						},
					},
				},
			},
			FrontendIPConfigurations: []*armnetwork.ApplicationGatewayFrontendIPConfiguration{
				{
					Name: to.Ptr(gateway.Name),
					Properties: &armnetwork.ApplicationGatewayFrontendIPConfigurationPropertiesFormat{
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						PublicIPAddress: &armnetwork.SubResource{
							ID: to.Ptr(publicIPResource.ID.String()),
						},
					},
				},
			},
			FrontendPorts: []*armnetwork.ApplicationGatewayFrontendPort{
				{
					Name: to.Ptr(frontendPortName),
					Properties: &armnetwork.ApplicationGatewayFrontendPortPropertiesFormat{
						Port: to.Ptr(targetPort),
					},
				},
			},
			BackendAddressPools: []*armnetwork.ApplicationGatewayBackendAddressPool{
				{
					Name: to.Ptr(containerName),
				},
			},
			BackendHTTPSettingsCollection: []*armnetwork.ApplicationGatewayBackendHTTPSettings{
				{
					Name: to.Ptr(containerName),
					Properties: &armnetwork.ApplicationGatewayBackendHTTPSettingsPropertiesFormat{
						Port:                to.Ptr(targetPort),
						Protocol:            to.Ptr(armnetwork.ApplicationGatewayProtocolHTTP),
						CookieBasedAffinity: to.Ptr(armnetwork.ApplicationGatewayCookieBasedAffinityDisabled),
						RequestTimeout:      to.Ptr[int32](60),
						Probe: &armnetwork.SubResource{
							ID: to.Ptr(strings.Join([]string{appgwID, "probes", containerName}, "/")),
						},
					},
				},
			},
			HTTPListeners: []*armnetwork.ApplicationGatewayHTTPListener{
				{
					Name: to.Ptr(containerName),
					Properties: &armnetwork.ApplicationGatewayHTTPListenerPropertiesFormat{
						FrontendIPConfiguration: &armnetwork.SubResource{
							ID: to.Ptr(strings.Join([]string{appgwID, "frontendIPConfigurations", gateway.Name}, "/")),
						},
						FrontendPort: &armnetwork.SubResource{
							ID: to.Ptr(strings.Join([]string{appgwID, "frontendPorts", frontendPortName}, "/")),
						},
						Protocol:                    to.Ptr(armnetwork.ApplicationGatewayProtocolHTTP),
						RequireServerNameIndication: to.Ptr(false),
					},
				},
			},
			RequestRoutingRules: []*armnetwork.ApplicationGatewayRequestRoutingRule{
				{
					Name: to.Ptr(containerName),
					Properties: &armnetwork.ApplicationGatewayRequestRoutingRulePropertiesFormat{
						Priority: to.Ptr[int32](1),
						HTTPListener: &armnetwork.SubResource{
							ID: to.Ptr(strings.Join([]string{appgwID, "httpListeners", containerName}, "/")),
						},
						BackendAddressPool: &armnetwork.SubResource{
							ID: to.Ptr(strings.Join([]string{appgwID, "backendAddressPools", containerName}, "/")),
						},
						BackendHTTPSettings: &armnetwork.SubResource{
							ID: to.Ptr(strings.Join([]string{appgwID, "backendHttpSettingsCollection", containerName}, "/")),
						},
						RuleType: to.Ptr(armnetwork.ApplicationGatewayRequestRoutingRuleTypeBasic),
					},
				},
			},
			Probes: []*armnetwork.ApplicationGatewayProbe{
				{
					Name: to.Ptr(containerName),
					Properties: &armnetwork.ApplicationGatewayProbePropertiesFormat{
						Protocol:                            to.Ptr(armnetwork.ApplicationGatewayProtocolHTTP),
						Host:                                to.Ptr("localhost"),
						Path:                                to.Ptr("/"),
						Interval:                            to.Ptr[int32](3600),
						Timeout:                             to.Ptr[int32](3600),
						UnhealthyThreshold:                  to.Ptr[int32](3),
						PickHostNameFromBackendHTTPSettings: to.Ptr(false),
					},
				},
			},
			AutoscaleConfiguration: &armnetwork.ApplicationGatewayAutoscaleConfiguration{
				MinCapacity: to.Ptr[int32](0),
				MaxCapacity: to.Ptr[int32](3),
			},
		},
	}
	appGWResource := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureApplicationGateway,
		ID:      resources.MustParse(appgwID),
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.Network/applicationGateways",
				Provider: resourcemodel.ProviderAzure,
			},
			Data:         appgw,
			Dependencies: []string{rpv1.LocalIDAzurePublicIP, rpv1.LocalIDAzureVirtualNetworkSubnet},
		},
	}

	outputResources = append(outputResources, appGWResource)

	return renderers.RendererOutput{
		Resources: outputResources,
		ComputedValues: map[string]rpv1.ComputedValueReference{
			"url": {
				LocalID:           rpv1.LocalIDAzurePublicIP,
				PropertyReference: "publicIPFQDN",
			},
		},
	}, nil
}

func parseURL(sourceURL string) (scheme string, hostname string, port int32, err error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", "", 0, err
	}

	scheme = u.Scheme
	host := u.Host

	hostname, strPort, err := net.SplitHostPort(host)
	_, ok := err.(*net.AddrError)
	if ok {
		strPort = ""
		hostname = host
	} else if err != nil {
		return "", "", 0, err
	}

	if scheme == "http" && strPort == "" {
		strPort = "80"
	}

	if scheme == "https" && strPort == "" {
		strPort = "443"
	}

	// bound check port
	portInt, err := strconv.Atoi(strPort)
	if err != nil {
		return "", "", 0, err
	}

	if portInt < 0 || portInt > 65535 {
		return "", "", 0, fmt.Errorf("port %d is out of range", port)
	}

	port = int32(portInt)

	return scheme, hostname, port, nil
}
