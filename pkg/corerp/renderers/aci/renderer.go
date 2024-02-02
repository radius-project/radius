package aci

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"

	cs2client "github.com/radius-project/azure-cs2/client/v20230515preview"
)

const (
	// hard-coded for PoC.
	aciLocation = "West US 3"
)

type Renderer struct {
}

func (r Renderer) GetDependencyIDs(ctx context.Context, dm v1.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	return nil, nil, nil
}

func (r Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	properties := resource.Properties

	envID := options.Environment.Resource
	vnetID := options.Environment.Compute.ACICompute.ResourceGroup + "/providers/Microsoft.Network/virtualNetworks/" + envID.Name()
	internalLBName := envID.Name() + "-ilb"
	internalLBID := options.Environment.Compute.ACICompute.ResourceGroup + "/providers/Microsoft.Network/loadBalancers/" + internalLBName
	internalLBNSGID := options.Environment.Compute.ACICompute.ResourceGroup + "/providers/Microsoft.Network/networkSecurityGroups/" + envID.Name() + "-nsg"

	// Build ContainerGroupProfile and ContainerScaleSet resources
	env := []*cs2client.EnvironmentVariable{}

	for name, val := range properties.Container.Env {
		if val.ValueFrom != nil {
			return renderers.RendererOutput{}, fmt.Errorf("valueFrom not supported with ACI")
		}

		env = append(env, &cs2client.EnvironmentVariable{
			Name:  to.Ptr(name),
			Value: val.Value,
		})
	}

	containerPorts := []*cs2client.ContainerPort{}
	ipAddress := &cs2client.IPAddress{Type: to.Ptr(cs2client.ContainerGroupIPAddressTypePrivate)}

	// Support only one port for now.
	firstPort := int32(80)
	for _, v := range properties.Container.Ports {
		containerPorts = append(containerPorts, &cs2client.ContainerPort{
			Port:     to.Ptr[int32](v.ContainerPort),
			Protocol: to.Ptr(cs2client.ContainerNetworkProtocolTCP),
		})
		ipAddress.Ports = append(ipAddress.Ports, &cs2client.Port{
			Port:     to.Ptr[int32](v.ContainerPort),
			Protocol: to.Ptr(cs2client.ContainerGroupNetworkProtocolTCP),
		})
	}

	if len(containerPorts) > 0 {
		firstPort = to.Int32(containerPorts[0].Port)
	}

	// Build Subnet for this container
	subnet := &armnetwork.Subnet{
		Name: to.Ptr(resource.Name),
		Type: to.Ptr("Microsoft.Network/virtualNetworks/subnets"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: nil, // updated by handler
			Delegations: []*armnetwork.Delegation{
				{
					Name: to.Ptr("Microsoft.ContainerInstance.containerGroups"),
					Type: to.Ptr("Microsoft.Network/virtualNetworks/subnets/delegations"),
					Properties: &armnetwork.ServiceDelegationPropertiesFormat{
						ServiceName: to.Ptr("Microsoft.ContainerInstance/containerGroups"),
					},
				},
			},
			NetworkSecurityGroup: &armnetwork.SecurityGroup{
				ID: to.Ptr(internalLBNSGID),
			},
		},
	}

	vnetSubnet := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureVirtualNetworkSubnet,
		ID:      resources.MustParse(vnetID + "/subnets/" + resource.Name),
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.Network/vitualNetworks/subnets",
				Provider: resourcemodel.ProviderAzure,
			},
			Data: subnet,
		},
	}

	internalSubnetID := vnetID + "/subnets/internal-lb"
	appSubnetID := vnetID + "/subnets/" + resource.Name

	frontendIPConfID := internalLBID + "/frontendIPConfigurations/" + resource.Name
	backendAddressPoolID := internalLBID + "/backendAddressPools/" + resource.Name
	probeID := internalLBID + "/probes/" + resource.Name

	// Build internal loadBalaner configuration for this container
	lb := &armnetwork.LoadBalancer{
		Name: to.Ptr(internalLBName),
		Type: to.Ptr("Microsoft.Network/loadBalancers"),
		Properties: &armnetwork.LoadBalancerPropertiesFormat{
			FrontendIPConfigurations: []*armnetwork.FrontendIPConfiguration{
				{
					Name: to.Ptr(resource.Name),
					Properties: &armnetwork.FrontendIPConfigurationPropertiesFormat{
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						Subnet: &armnetwork.Subnet{
							ID: to.Ptr(internalSubnetID),
						},
					},
				},
			},
			BackendAddressPools: []*armnetwork.BackendAddressPool{
				{
					Name: to.Ptr(resource.Name),
				},
			},
			Probes: []*armnetwork.Probe{
				{
					Name: to.Ptr(resource.Name),
					Properties: &armnetwork.ProbePropertiesFormat{
						Protocol:          to.Ptr(armnetwork.ProbeProtocolTCP),
						Port:              to.Ptr[int32](firstPort),
						IntervalInSeconds: to.Ptr[int32](15),
						NumberOfProbes:    to.Ptr[int32](2),
					},
				},
			},
			LoadBalancingRules: []*armnetwork.LoadBalancingRule{
				{
					Name: to.Ptr(resource.Name),
					Properties: &armnetwork.LoadBalancingRulePropertiesFormat{
						Protocol:       to.Ptr(armnetwork.TransportProtocolTCP),
						FrontendPort:   to.Ptr[int32](firstPort),
						BackendPort:    to.Ptr[int32](firstPort),
						EnableTCPReset: to.Ptr(true),
						FrontendIPConfiguration: &armnetwork.SubResource{
							ID: to.Ptr(frontendIPConfID),
						},
						BackendAddressPool: &armnetwork.SubResource{
							ID: to.Ptr(backendAddressPoolID),
						},
						Probe: &armnetwork.SubResource{
							ID: to.Ptr(probeID),
						},
					},
				},
			},
		},
	}

	lbResource := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureContainerLoadBalancer,
		ID:      resources.MustParse(internalLBID + "/applications/" + resource.Name),
		AdditionalProperties: map[string]string{
			"appName": resource.Name,
		},
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.Network/loadBalancers",
				Provider: resourcemodel.ProviderAzure,
			},
			Data:         lb,
			Dependencies: []string{rpv1.LocalIDAzureVirtualNetworkSubnet},
		},
	}

	profile := &cs2client.ContainerGroupProfile{
		Location: to.Ptr(aciLocation),
		Name:     to.Ptr(resource.Name),
		Properties: &cs2client.ContainerGroupProfilePropertiesProperties{
			Containers: []*cs2client.Container{
				{
					Name: to.Ptr(resource.Name),
					Properties: &cs2client.ContainerProperties{
						Image:                to.Ptr(resource.Properties.Container.Image),
						EnvironmentVariables: env,
						Command:              to.SliceOfPtrs[string](properties.Container.Command...),
						Ports:                containerPorts,
						Resources: &cs2client.ResourceRequirements{
							// Hard-coded right now!
							Requests: &cs2client.ResourceRequests{
								CPU:        to.Ptr[float64](1.0),
								MemoryInGB: to.Ptr[float64](2.0),
							},
						},
					},
				},
			},
			IPAddress: ipAddress,
			OSType:    to.Ptr(cs2client.OperatingSystemTypesLinux),
			SKU:       to.Ptr(cs2client.ContainerGroupSKUStandard),
		},
	}

	orProfile := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureCGProfile,
		ID:      resources.MustParse(options.Environment.Compute.ACICompute.ResourceGroup + "/providers/Microsoft.ContainerInstance/containerGroupProfiles/" + resource.Name),
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.ContainerInstance/containerGroupProfiles",
				Provider: resourcemodel.ProviderAzure,
			},
			Data:         profile,
			Dependencies: []string{rpv1.LocalIDAzureContainerLoadBalancer},
		},
	}

	scaleSet := &cs2client.ContainerScaleSet{
		Name:     to.Ptr(resource.Name),
		Location: to.Ptr(aciLocation),
		Properties: &cs2client.ContainerScaleSetProperties{
			UpdateProfile: &cs2client.UpdateProfile{
				UpdateMode: to.Ptr(cs2client.UpdateModeRolling),
			},
			ElasticProfile: &cs2client.ElasticProfile{
				DesiredCount: to.Ptr[int32](1),
				ContainerGroupNamingPolicy: &cs2client.ContainerGroupNamingPolicy{
					GUIDNamingPolicy: &cs2client.GUIDNamingPolicy{
						Prefix: to.Ptr(resource.Name + "-"),
					},
				},
			},
			ContainerGroupProfiles: []*cs2client.ContainerGroupProfileAutoGenerated{
				{
					Resource: &cs2client.APIEntityReference{}, // Updated by handler
					ContainerGroupProperties: &cs2client.ContainerGroupProperties{
						SubnetIDs: []*cs2client.Subnet{
							{
								ID:   to.Ptr(appSubnetID),
								Name: to.Ptr(resource.Name),
							},
						},
					},
					NetworkProfile: &cs2client.NetworkProfile{
						LoadBalancer: &cs2client.LoadBalancer{
							BackendAddressPools: []*cs2client.LoadBalancerBackendAddressPool{
								{
									Resource: &cs2client.APIEntityReference{
										ID: to.Ptr(backendAddressPoolID),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	orScaleSet := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureCGScaleSet,
		ID:      resources.MustParse(options.Environment.Compute.ACICompute.ResourceGroup + "/providers/Microsoft.ContainerInstance/containerScaleSets/" + resource.Name),
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.ContainerInstance/containerScaleSets",
				Provider: resourcemodel.ProviderAzure,
			},
			Data:         scaleSet,
			Dependencies: []string{rpv1.LocalIDAzureCGProfile},
		},
	}

	return renderers.RendererOutput{
		Resources:      []rpv1.OutputResource{vnetSubnet, lbResource, orProfile, orScaleSet},
		RadiusResource: dm,
	}, nil
}
