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

package containers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	platformprovider "github.com/radius-project/radius/pkg/platform-provider"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"

	ucp_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

var _ ctrl.Controller = (*CreateOrUpdateResource)(nil)

// RoleAssignmentData describes how to configure role assignment permissions based on the kind of
// connection.
type RoleAssignmentData struct {
	// RoleNames contains the names of the IAM roles to grant.
	RoleNames    []string
	ResourceType string
}

type RoleAssignment struct {
	ResourceID resources.ID
	RoleNames  []string
}

// TODO: Move this to provider code. this is predefined roles
var roleAssignmentMap = map[datamodel.IAMKind]RoleAssignmentData{
	// Example of how to read this data:
	//
	// For a KeyVault connection...
	// - Look up the dependency based on the connection.Source (azure.com.KeyVault)
	// - Find the output resource matching LocalID of that dependency (Microsoft.KeyVault/vaults)
	// - Apply the roles in RoleNames (Key Vault Secrets User, Key Vault Crypto User)
	datamodel.KindAzureComKeyVault: {
		ResourceType: "Microsoft.KeyVault/vaults",
		RoleNames: []string{
			"Key Vault Secrets User",
			"Key Vault Crypto User",
		},
	},
	datamodel.KindAzure: {
		// RBAC for non-Radius Azure resources. Supports user specified roles.
		// More information can be found here: https://github.com/radius-project/radius/issues/1321
	},
}

// CreateOrUpdateResource is the async operation controller to create or update Applications.Core/Containers resource.
type CreateOrUpdateResource struct {
	ctrl.BaseController
}

type BaseResource struct {
	v1.BaseResource
	datamodel.PortableResourceMetadata
	Properties rpv1.BasicResourceProperties
}

// NewCreateOrUpdateResource creates a new CreateOrUpdateResource controller.
func NewCreateOrUpdateResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateResource{ctrl.NewBaseAsyncController(opts)}, nil
}

func isIdentitySupported(roles map[datamodel.IAMKind]RoleAssignmentData, kind datamodel.IAMKind) bool {
	if roles == nil || !kind.IsValid() {
		return false
	}

	_, ok := roles[kind]
	return ok
}

func getConnectedResources(container *datamodel.ContainerResource) ([]resources.ID, error) {
	properties := container.Properties
	connectedResources := []resources.ID{}

	for _, connection := range properties.Connections {
		// if source is a URL, it is valid (example: 'http://containerx:3000').
		if isURL(connection.Source) {
			continue
		}

		// If source is not a URL, it must be either resource ID, invalid string, or empty (example: containerhttproute.id).
		rID, err := resources.ParseResource(connection.Source)
		if err != nil {
			return nil, fmt.Errorf("invalid source: %s. Must be either a URL or a valid resourceID", connection.Source)
		}

		if ucp_radius.IsRadiusResource(rID) {
			connectedResources = append(connectedResources, rID)
		}
	}

	for _, port := range properties.Container.Ports {
		if port.Provides == "" {
			continue
		}

		rID, err := resources.ParseResource(port.Provides)
		if err != nil {
			return nil, err
		}

		if ucp_radius.IsRadiusResource(rID) {
			connectedResources = append(connectedResources, rID)
		}
	}

	for _, vol := range properties.Container.Volumes {
		switch vol.Kind {
		case datamodel.Persistent:
			rID, err := resources.ParseResource(vol.Persistent.Source)
			if err != nil {
				return nil, err
			}

			if ucp_radius.IsRadiusResource(rID) {
				connectedResources = append(connectedResources, rID)
			}
		}
	}

	return connectedResources, nil
}

type EnvVar struct {
	Name     string
	Value    []byte
	IsSecret bool
}

func getEnvironmentVariables(resource *datamodel.ContainerResource, resourceMap map[string]*BaseResource) ([]EnvVar, error) {
	env := []EnvVar{}
	properties := resource.Properties

	// Take each connection and create environment variables for each part
	// We'll store each value in a secret named with the same name as the resource.
	// We'll use the environment variable names as keys.
	// Float is used by the JSON serializer
	for name, con := range properties.Connections {
		dep := resourceMap[con.Source]

		if !con.GetDisableDefaultEnvVars() {
			source := con.Source
			if source == "" {
				continue
			}

			// handles case where container has source field structured as a URL.
			if isURL(source) {
				// parse source into scheme, hostname, and port.
				scheme, hostname, port, err := parseURL(source)
				if err != nil {
					return nil, fmt.Errorf("failed to parse source URL: %w", err)
				}

				schemeKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "SCHEME")
				hostnameKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "HOSTNAME")
				portKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "PORT")

				env = append(env, EnvVar{Name: schemeKey, Value: []byte(scheme)})
				env = append(env, EnvVar{Name: hostnameKey, Value: []byte(hostname)})
				env = append(env, EnvVar{Name: portKey, Value: []byte(port)})
				continue
			}

			// handles case where container has source field structured as a resourceID.
			for key, value := range dep.ComputedValues {
				name := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), strings.ToUpper(key))
				switch v := value.(type) {
				case string:
					env = append(env, EnvVar{Name: name, Value: []byte(v)})
				case float64:
					env = append(env, EnvVar{Name: name, Value: []byte(strconv.Itoa(int(v)))})
				case int:
					env = append(env, EnvVar{Name: name, Value: []byte(strconv.Itoa(v))})
				}
			}
		}
	}

	return env, nil
}

// Run checks if the resource exists, renders the resource, deploys the resource, applies the
// deployment output to the resource, deletes any resources that are no longer needed, and saves the resource.
func (c *CreateOrUpdateResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	var platform platformprovider.Provider

	obj, err := c.StorageClient().Get(ctx, request.ResourceID)
	if err != nil && !errors.Is(&store.ErrNotFound{ID: request.ResourceID}, err) {
		return ctrl.Result{}, err
	}

	container, ok := obj.Data.(*datamodel.ContainerResource)
	if !ok {
		return ctrl.NewFailedResult(v1.ErrorDetails{Code: "IntervalError", Message: "invalid datamodel"}), nil
	}

	properties := container.Properties
	appId, err := resources.ParseResource(properties.Application)
	if err != nil {
		return ctrl.Result{}, err
	}

	// TODO: Get environment / application resources

	// TODO: Moved the following code to frontend.
	// Preserve outputresources.
	outputResources := []rpv1.OutputResource{}
	for _, rr := range properties.Resources {
		id, err := resources.Parse(rr.ID)
		if err != nil {
			return ctrl.Result{}, err
		}

		outputResources = append(outputResources, rpv1.OutputResource{ID: id, RadiusManaged: to.Ptr(false)})
	}

	// TODO: It must check if the old resource is ContainerResourceProvisioningManual
	if properties.ResourceProvisioning == datamodel.ContainerResourceProvisioningManual {
		// Do nothing! This is a manual resource.
		return ctrl.Result{}, nil
	}

	// Get all connected resources.
	connectedResources, err := getConnectedResources(container)
	if err != nil {
		return ctrl.Result{}, err
	}

	connectedResourceMap := map[string]*BaseResource{}
	for _, id := range connectedResources {
		sc, err := c.DataProvider().GetStorageClient(ctx, id.Type())
		if err != nil {
			return ctrl.Result{}, err
		}

		obj, err := sc.Get(ctx, id.String())
		r := &BaseResource{}
		if err = obj.As(r); err != nil {
			return ctrl.Result{}, err
		}
		connectedResourceMap[id.String()] = r
	}

	// Original code: Collect radius resource from container.Volumes and container.Ports[0].Provides

	// This is the flag to create the service route.
	isRouteRequired := false

	for portName, port := range properties.Container.Ports {
		// if the container has an exposed port, note that down.
		// A single service will be generated for a container with one or more exposed ports.
		if port.ContainerPort == 0 {
			return ctrl.Result{}, fmt.Errorf("invalid ports definition: must define a ContainerPort, but ContainerPort is: %d.", port.ContainerPort)
		}

		if port.Port == 0 {
			port.Port = port.ContainerPort
			properties.Container.Ports[portName] = port
		}

		// if the container has an exposed port, but no 'provides' field, it requires DNS service generation.
		if port.Provides == "" {
			isRouteRequired = true
		}
	}

	// Get role assignments for each connections.
	roles := []RoleAssignment{}
	for _, connection := range properties.Connections {
		sourceID, err := resources.Parse(connection.Source)
		if err != nil {
			return ctrl.Result{}, err
		}

		predefined, ok := roleAssignmentMap[connection.IAM.Kind]
		if ok {
			dependency, ok := connectedResourceMap[connection.Source]
			if !ok {
				return ctrl.Result{}, fmt.Errorf("dependency not found: %s", connection.Source)
			}

			var foundRoles *RoleAssignment
			for _, outputResource := range dependency.Properties.Status.OutputResources {
				if outputResource.ID.Type() == predefined.ResourceType {
					foundRoles = &RoleAssignment{
						ResourceID: sourceID,
						RoleNames:  predefined.RoleNames,
					}
					break
				}
			}

			if foundRoles != nil {
				roles = append(roles, *foundRoles)
			}
		} else {
			if len(connection.IAM.Roles) > 0 && connection.Source != "" {
				roles = append(roles, RoleAssignment{
					ResourceID: sourceID,
					RoleNames:  connection.IAM.Roles,
				})
			}
		}
	}

	// Get env vars and secret data
	envVars, err := getEnvironmentVariables(container, connectedResourceMap)

	// Get volumes and if Azure Keyvault volume is found, add AzureKeyVaultSecretsUserRole, AzureKeyVaultCryptoUserRole to "roles".
	for volName, volProperties := range properties.Container.Volumes {
		if volProperties.Kind == datamodel.Persistent {
			volumeResource, ok := connectedResourceMap[volProperties.Persistent.Source]
			if !ok {
				return ctrl.Result{}, fmt.Errorf("volume resource not found: %s", volProperties.Persistent.Source)
			}

			if volumeResource.Properties.Kind == datamodel.AzureKeyVaultVolume {
				// Add the roles to the roles list.
				roles = append(roles, RoleAssignment{
					ResourceID: resources.ID(volumeProperties.Persistent.Source),
					RoleNames:  []string{"AzureKeyVaultSecretsUserRole", "AzureKeyVaultCryptoUserRole"},
				})
			}
		}
	}

	// Call secretstore to create secret for connections' secrets.
	secretStoreProvider, err := platform.SecretStore()
	err = secretStoreProvider.CreateOrUpdateSecretStore(ctx, &datamodel.SecretStoreProperties{})

	for _, role := range roles {
		if role.ResourceID.PlaneNamespace() == "azure" {
			// CreateOrUpdate identity for cloud iam. (Azure managed identity)
			identity, err := platform.Identity()
			rID, err := identity.CreateOrUpdateIdentity(ctx, &datamodel.IdentityProperties{})
			if err != nil {
				return ctrl.Result{}, err
			}
			// CreateOrUpdate identity role binding for cloud iam. (Azure managed identity) for connection resources.
			err = identity.AssignRoleToIdentity(ctx, rID, role.ResourceID, role.RoleNames)
		}
	}

	// CreateOrUpdate identity for cloud iam. (Azure managed identity)
	identity, err := platform.Identity()
	rID, err := identity.CreateOrUpdateIdentity(ctx, &datamodel.IdentityProperties{})
	if err != nil {
		return ctrl.Result{}, err
	}

	// CreateOrUpdate identity role binding for container platform. (Service Account)
	err = identity.AssignRoleToIdentity(ctx, rID, []string{"Service Account"})

	// CreateOrUpdate the container.
	containerProvider, err := platform.Container()
	if err != nil {
		return ctrl.Result{}, err
	}

	err = containerProvider.CreateOrUpdateContainer(ctx, container)
	if err != nil {
		return ctrl.Result{}, err
	}

	routeProvider, err := platform.Route()
	if err != nil {
		return ctrl.Result{}, err
	}

	if isRouteRequired {
		err = routeProvider.CreateOrUpdateRoute(ctx, properties.Container.Ports)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// CreateOrUpdate the routes in the container.

	/*
		# Save output resource.

		nr := &store.Object{
			Metadata: store.Metadata{
				ID: request.ResourceID,
			},
			Data: deploymentDataModel,
		}
		err = c.StorageClient().Save(ctx, nr, store.WithETag(obj.ETag))
		if err != nil {
			return ctrl.Result{}, err
		}
	*/

	return ctrl.Result{}, nil
}
