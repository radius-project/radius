package aci

import (
	"context"
	"fmt"
	"sort"
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

	ngroupsclient "github.com/radius-project/radius/pkg/sdk/v20241101preview"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

type Renderer struct {
}

func (r Renderer) GetDependencyIDs(ctx context.Context, dm v1.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return nil, nil, v1.ErrInvalidModelConversion
	}
	properties := resource.Properties

	for _, connection := range properties.Connections {
		resourceID, err := resources.ParseResource(connection.Source)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid source: %s. Must be either a URL or a valid resourceID", connection.Source))
		}

		// Non-radius Azure connections that are accessible from Radius container resource.
		if connection.IAM.Kind.IsKind(datamodel.KindAzure) {
			azureResourceIDs = append(azureResourceIDs, resourceID)
			continue
		}

		if resources_radius.IsRadiusResource(resourceID) {
			radiusResourceIDs = append(radiusResourceIDs, resourceID)
			continue
		}
	}

	return radiusResourceIDs, azureResourceIDs, nil
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

	orResources := []rpv1.OutputResource{}

	// Populate environment variables in properties.container.env
	env := []*ngroupsclient.EnvironmentVariable{}
	for name, val := range properties.Container.Env {
		if val.ValueFrom != nil {
			return renderers.RendererOutput{}, fmt.Errorf("valueFrom not supported with ACI")
		}

		env = append(env, &ngroupsclient.EnvironmentVariable{
			Name:  to.Ptr(name),
			Value: val.Value,
		})
	}

	// We build the environment variable list in a stable order for testability
	envData, secretData, err := getEnvVarsAndSecretData(resource, options.Dependencies)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("failed to obtain environment variables and secret data: %w", err)
	}

	// Populate environment variables from connections
	for _, key := range getSortedKeys(envData) {
		env = append(env, &ngroupsclient.EnvironmentVariable{
			Name:  to.Ptr(envData[key].Name),
			Value: to.Ptr(envData[key].Value),
		})
	}

	// Populate secret data from connections
	for _, key := range getSortedKeys(secretData) {
		env = append(env, &ngroupsclient.EnvironmentVariable{
			Name:        to.Ptr(secretData[key].Name),
			SecureValue: to.Ptr(secretData[key].Value),
		})
	}

	containerPorts := []*ngroupsclient.ContainerPort{}
	ipAddress := &ngroupsclient.IPAddress{
		Type: to.Ptr(ngroupsclient.ContainerGroupIPAddressTypePrivate),
	}

	// Support only one port for now.
	// port 80 has to be defined
	firstPort := int32(80)
	// Set the gateway if it's defined on the container resource
	gateway := ""
	if properties.Runtimes != nil && properties.Runtimes.ACI != nil && properties.Runtimes.ACI.GatewayID != "" {
		gateway = resources.MustParse(resource.Properties.Runtimes.ACI.GatewayID).Name()
	}
	for _, v := range properties.Container.Ports {
		// exposed within container group for interacting with the container
		containerPorts = append(containerPorts, &ngroupsclient.ContainerPort{
			Port:     to.Ptr(v.ContainerPort),
			Protocol: to.Ptr(ngroupsclient.ContainerNetworkProtocolTCP),
		})
		// ports exposed to communicate with the container group -- standard is port 80
		ipAddress.Ports = append(ipAddress.Ports, &ngroupsclient.Port{
			Port:     to.Ptr(v.ContainerPort),
			Protocol: to.Ptr(ngroupsclient.ContainerGroupNetworkProtocolTCP),
		})
	}

	if len(containerPorts) > 0 {
		firstPort = to.Int32(containerPorts[0].Port)
	}
	// todo: check test scenarios on this
	if len(ipAddress.Ports) < 1 {
		ipAddress = nil
	}

	nsgID := internalLBNSGID
	if gateway != "" {
		nsgID = options.Environment.Compute.ACICompute.ResourceGroup + "/providers/Microsoft.Network/networkSecurityGroups/" + gateway + "-nsg"
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
				ID: to.Ptr(nsgID),
			},
		},
	}

	vnetSubnet := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureVirtualNetworkSubnet,
		ID:      resources.MustParse(vnetID + "/subnets/" + resource.Name),
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.Network/virtualNetworks/subnets",
				Provider: resourcemodel.ProviderAzure,
			},
			Data: subnet,
		},
	}
	orResources = append(orResources, vnetSubnet)

	var networkprofile *ngroupsclient.NetworkProfile
	appSubnetID := vnetID + "/subnets/" + resource.Name

	profileDep := []string{rpv1.LocalIDAzureVirtualNetworkSubnet}
	if gateway == "" {
		internalSubnetID := vnetID + "/subnets/internal-lb"

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
							Port:              to.Ptr(firstPort),
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
							FrontendPort:   to.Ptr(firstPort),
							BackendPort:    to.Ptr(firstPort),
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
					Type:     "Microsoft.Network/loadBalancers/applications",
					Provider: resourcemodel.ProviderAzure,
				},
				Data:         lb,
				Dependencies: []string{rpv1.LocalIDAzureVirtualNetworkSubnet},
			},
		}

		networkprofile = &ngroupsclient.NetworkProfile{
			LoadBalancer: &ngroupsclient.LoadBalancer{
				BackendAddressPools: []*ngroupsclient.LoadBalancerBackendAddressPool{
					{
						Resource: &ngroupsclient.APIEntityReference{
							ID: to.Ptr(backendAddressPoolID),
						},
					},
				},
			},
		}
		orResources = append(orResources, lbResource)
		profileDep = append(profileDep, rpv1.LocalIDAzureContainerLoadBalancer)
	} else {
		appgwID := options.Environment.Compute.ACICompute.ResourceGroup + "/providers/Microsoft.Network/applicationGateways/" + gateway
		networkprofile = &ngroupsclient.NetworkProfile{
			ApplicationGateway: &ngroupsclient.ApplicationGateway{
				Resource: &ngroupsclient.APIEntityReference{
					ID: to.Ptr(appgwID),
				},
				BackendAddressPools: []*ngroupsclient.ApplicationGatewayBackendAddressPool{
					{
						Resource: &ngroupsclient.APIEntityReference{
							ID: to.Ptr(strings.Join([]string{appgwID, "backendAddressPools", resource.Name}, "/")),
						},
					},
				},
			},
		}
	}

	profile := &ngroupsclient.ContainerGroupProfile{
		Location: to.Ptr(resource.Location),
		Name:     to.Ptr(resource.Name),
		Properties: &ngroupsclient.ContainerGroupProfileProperties{
			Containers: []*ngroupsclient.Container{
				{
					Name: to.Ptr(resource.Name),
					Properties: &ngroupsclient.ContainerProperties{
						Image:                to.Ptr(resource.Properties.Container.Image),
						EnvironmentVariables: env,
						Command:              to.SliceOfPtrs(properties.Container.Command...),
						Ports:                containerPorts,
						Resources: &ngroupsclient.ResourceRequirements{
							// Hard-coded right now!
							Requests: &ngroupsclient.ResourceRequests{
								CPU:        to.Ptr(1.0),
								MemoryInGB: to.Ptr(2.0),
							},
						},
					},
				},
			},
			IPAddress: ipAddress,
			OSType:    to.Ptr(ngroupsclient.OperatingSystemTypesLinux),
			SKU:       to.Ptr(ngroupsclient.ContainerGroupSKUStandard),
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
			Dependencies: profileDep,
		},
	}
	orResources = append(orResources, orProfile)

	// TODO: rename to ngroup
	nGroup := &ngroupsclient.NGroup{
		Name:     to.Ptr(resource.Name),
		Location: to.Ptr(resource.Location),
		Identity: ProcessNGroupIdentity(options.Environment),
		Properties: &ngroupsclient.NGroupProperties{
			UpdateProfile: &ngroupsclient.UpdateProfile{
				UpdateMode: to.Ptr(ngroupsclient.NGroupUpdateModeRolling),
			},
			ElasticProfile: &ngroupsclient.ElasticProfile{
				DesiredCount: to.Ptr[int32](1),
				ContainerGroupNamingPolicy: &ngroupsclient.ElasticProfileContainerGroupNamingPolicy{
					GUIDNamingPolicy: &ngroupsclient.ElasticProfileContainerGroupNamingPolicyGUIDNamingPolicy{
						Prefix: to.Ptr(resource.Name + "-"),
					},
				},
			},
			ContainerGroupProfiles: []*ngroupsclient.ContainerGroupProfileStub{
				{
					Resource: &ngroupsclient.APIEntityReference{}, // Updated by handler
					ContainerGroupProperties: &ngroupsclient.NGroupContainerGroupProperties{
						SubnetIDs: []*ngroupsclient.ContainerGroupSubnetID{
							{
								ID:   to.Ptr(appSubnetID),
								Name: to.Ptr(resource.Name),
							},
						},
					},
					NetworkProfile: networkprofile,
				},
			},
		},
	}

	orNGroup := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureCGNGroups,
		ID:      resources.MustParse(options.Environment.Compute.ACICompute.ResourceGroup + "/providers/Microsoft.ContainerInstance/nGroups/" + resource.Name),
		CreateResource: &rpv1.Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     "Microsoft.ContainerInstance/nGroups",
				Provider: resourcemodel.ProviderAzure,
			},
			Data:         nGroup,
			Dependencies: []string{rpv1.LocalIDAzureCGProfile},
		},
	}
	orResources = append(orResources, orNGroup)

	return renderers.RendererOutput{
		Resources:      orResources,
		RadiusResource: dm,
		ComputedValues: map[string]rpv1.ComputedValueReference{
			// Populate hostname for the frontend of load balancer.
			"hostname": {
				LocalID:           rpv1.LocalIDAzureContainerLoadBalancer,
				PropertyReference: "hostname",
			},
		},
	}, nil
}

type EnvVar struct {
	Name  string
	Value string
}

func getEnvVarsAndSecretData(resource *datamodel.ContainerResource, dependencies map[string]renderers.RendererDependency) (map[string]EnvVar, map[string]EnvVar, error) {
	env := map[string]EnvVar{}
	secretData := map[string]EnvVar{}
	properties := resource.Properties

	// Take each connection and create environment variables for each part
	// We'll store each value in a secret named with the same name as the resource.
	// We'll use the environment variable names as keys.
	// Float is used by the JSON serializer
	for name, con := range properties.Connections {
		properties := dependencies[con.Source]
		if !con.GetDisableDefaultEnvVars() {
			source := con.Source
			if source == "" {
				continue
			}

			for key, value := range properties.ComputedValues {
				name := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), strings.ToUpper(key))

				switch v := value.(type) {
				case string:
					secretData[name] = EnvVar{Name: name, Value: v}
				case float64:
					secretData[name] = EnvVar{Name: name, Value: strconv.Itoa(int(v))}
				case int:
					secretData[name] = EnvVar{Name: name, Value: strconv.Itoa(v)}
				}
			}
		}
	}

	return env, secretData, nil
}

func getSortedKeys(env map[string]EnvVar) []string {
	keys := []string{}
	for k := range env {
		key := k
		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys
}

func ProcessNGroupIdentity(envOptions renderers.EnvironmentOptions) *ngroupsclient.NGroupIdentity {
	identity := &ngroupsclient.NGroupIdentity{
		Type: ConvertToManagedIdentityTypes(envOptions.Identity),
	}

	if envOptions.Compute.Identity != nil && envOptions.Compute.Identity.ManagedIdentity != nil {
		identity.UserAssignedIdentities = ConvertToUserAssignedIdentity(envOptions.Compute.Identity.ManagedIdentity)
	}
	return identity
}

func ConvertToUserAssignedIdentity(urls []string) map[string]*ngroupsclient.UserAssignedIdentities {
	identities := make(map[string]*ngroupsclient.UserAssignedIdentities)

	for _, url := range urls {
		identities[url] = &ngroupsclient.UserAssignedIdentities{}
	}

	return identities
}

func ConvertToManagedIdentityTypes(is *rpv1.IdentitySettings) *ngroupsclient.ResourceIdentityType {
	identityType := ngroupsclient.ResourceIdentityTypeSystemAssigned

	// Only override default if we have settings with a specific non-default kind
	if is != nil {
		switch is.Kind {
		case rpv1.UserAssigned:
			identityType = ngroupsclient.ResourceIdentityTypeUserAssigned
		case rpv1.SystemAssignedUserAssigned:
			identityType = ngroupsclient.ResourceIdentityTypeSystemAssignedUserAssigned
		}
	}

	return to.Ptr(identityType)
}
