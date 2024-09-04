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

package container

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/handlers"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	azrenderer "github.com/radius-project/radius/pkg/corerp/renderers/container/azure"
	azvolrenderer "github.com/radius-project/radius/pkg/corerp/renderers/volume/azure"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_azure "github.com/radius-project/radius/pkg/ucp/resources/azure"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

const (
	ResourceType = "Applications.Core/containers"

	// Liveness/Readiness constants
	DefaultInitialDelaySeconds = 0
	DefaultFailureThreshold    = 3
	DefaultPeriodSeconds       = 10
	DefaultTimeoutSeconds      = 5

	AzureKeyVaultSecretsUserRole = "Key Vault Secrets User"
	AzureKeyVaultCryptoUserRole  = "Key Vault Crypto User"
)

// GetSupportedKinds returns a list of supported volume kinds.
func GetSupportedKinds() []string {
	keys := []string{}
	keys = append(keys, datamodel.AzureKeyVaultVolume)
	return keys
}

// Renderer is the WorkloadRenderer implementation for containerized workload.
type Renderer struct {
	// RoleAssignmentMap is an optional map of connection kind -> []Role Assignment. Used to configure managed
	// identity permissions for cloud resources. This will be nil in environments that don't support role assignments.
	RoleAssignmentMap map[datamodel.IAMKind]RoleAssignmentData
}

// GetDependencyIDs parses the connections, ports, environment variables, and volumes of a container resource to return the Radius and Azure
// resource IDs.
func (r Renderer) GetDependencyIDs(ctx context.Context, dm v1.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return nil, nil, v1.ErrInvalidModelConversion
	}
	properties := resource.Properties

	// Right now we only have things in connections and ports as rendering dependencies - we'll add more things
	// in the future... eg: volumes
	//
	// Anywhere we accept a resource ID in the model should have its value returned from here
	for _, connection := range properties.Connections {
		if isURL(connection.Source) {
			continue
		}

		// if the source is not a URL, it either a resourceID or invalid.
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

	// Environment variables can be sourced from secrets, which are resources. We need to iterate over the environment variables to handle any possible instances.
	for _, envVars := range properties.Container.Env {
		if envVars.ValueFrom != nil && envVars.ValueFrom.SecretRef != nil {
			// If the string begins with a '/', it is a radius resourceID.
			if strings.HasPrefix(envVars.ValueFrom.SecretRef.Source, "/") {
				resourceID, err := resources.ParseResource(envVars.ValueFrom.SecretRef.Source)
				if err != nil {
					return nil, nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid source: %s. Must be either a kubernetes secret name or a valid resourceID", envVars.ValueFrom.SecretRef.Source))
				}

				if resources_radius.IsRadiusResource(resourceID) {
					radiusResourceIDs = append(radiusResourceIDs, resourceID)
				}
			}
		}
	}

	for _, volume := range properties.Container.Volumes {
		switch volume.Kind {
		case datamodel.Persistent:
			resourceID, err := resources.ParseResource(volume.Persistent.Source)
			if err != nil {
				return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
			}

			if resources_radius.IsRadiusResource(resourceID) {
				radiusResourceIDs = append(radiusResourceIDs, resourceID)
				continue
			}
		}
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

// Render creates role assignments, a deployment, and a secret for a given container resource, and returns a
// RendererOutput containing the resources and computed values.
func (r Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	properties := resource.Properties

	appId, err := resources.ParseResource(properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid application id: %s ", err.Error()))
	}

	outputResources := []rpv1.OutputResource{}
	for _, rr := range properties.Resources {
		id, err := resources.Parse(rr.ID)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		outputResources = append(outputResources, rpv1.OutputResource{ID: id, RadiusManaged: to.Ptr(false)})
	}

	if properties.ResourceProvisioning == datamodel.ContainerResourceProvisioningManual {
		// Do nothing! This is a manual resource.
		return renderers.RendererOutput{Resources: outputResources}, nil
	}

	// this flag is used to indicate whether or not this resource needs a service to be generated.
	// this flag is triggered when a container has an exposed port(s), but no 'provides' field.
	var needsServiceGeneration = false

	// check if connections are valid
	for _, connection := range properties.Connections {
		// if source is a URL, it is valid (example: 'http://containerx:3000').
		if isURL(connection.Source) {
			continue
		}

		// If source is not a URL, it must be either resource ID, invalid string, or empty (example: myRedis.id).
		_, err := resources.ParseResource(connection.Source)
		if err != nil {
			return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid source: %s. Must be either a URL or a valid resourceID", connection.Source))
		}
	}

	for portName, port := range properties.Container.Ports {
		// if the container has an exposed port, note that down.
		// A single service will be generated for a container with one or more exposed ports.
		if port.ContainerPort == 0 {
			return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid ports definition: must define a ContainerPort, but ContainerPort is: %d.", port.ContainerPort))
		}

		if port.Port == 0 {
			port.Port = port.ContainerPort
			properties.Container.Ports[portName] = port
		}

		// if the container has an exposed port, it requires DNS service generation.
		needsServiceGeneration = true
	}

	dependencies := options.Dependencies

	// Connections might require a role assignment to grant access.
	roles := []rpv1.OutputResource{}
	for _, connection := range properties.Connections {
		if !r.isIdentitySupported(connection.IAM.Kind) {
			continue
		}

		rbacOutputResources, err := r.makeRoleAssignmentsForResource(&connection, dependencies)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		roles = append(roles, rbacOutputResources...)
	}

	if len(roles) > 0 {
		outputResources = append(outputResources, roles...)
	}

	// If the container has a base manifest, deserialize base manifest and validation should be done by frontend controller.
	baseManifest, err := fetchBaseManifest(resource)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid base manifest: %s", err.Error()))
	}

	computedValues := map[string]rpv1.ComputedValueReference{}

	// Create the deployment as the primary workload
	deploymentResources, secretData, err := r.makeDeployment(baseManifest, appId.Name(), options, computedValues, resource, roles)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, deploymentResources...)

	// If there are secrets we'll use a Kubernetes secret to hold them. This is already referenced
	// by the deployment.
	if len(secretData) > 0 {
		outputResources = append(outputResources, r.makeSecret(*resource, appId.Name(), secretData, options))
	}

	var servicePorts []corev1.ServicePort

	// If the container has an exposed port and uses DNS-SD, generate a service for it.
	if needsServiceGeneration {
		for portName, port := range resource.Properties.Container.Ports {
			// store portNames and portValues for use in service generation.
			servicePort := corev1.ServicePort{
				Name:       portName,
				Port:       port.Port,
				TargetPort: intstr.FromInt(int(port.ContainerPort)),
				Protocol:   corev1.ProtocolTCP,
			}
			servicePorts = append(servicePorts, servicePort)
		}

		// if a container has an exposed port, then we need to create a service for it.
		basesrv := getServiceBase(baseManifest, appId.Name(), resource, &options)
		serviceResource, err := r.makeService(basesrv, resource, servicePorts)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		outputResources = append(outputResources, serviceResource)
	}

	// Populate the remaining resources from the base manifest.
	outputResources = populateAllBaseResources(ctx, baseManifest, outputResources, options)

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
	}, nil
}

func (r Renderer) makeService(base *corev1.Service, resource *datamodel.ContainerResource, servicePorts []corev1.ServicePort) (rpv1.OutputResource, error) {
	appId, err := resources.ParseResource(resource.Properties.Application)
	if err != nil {
		return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid application id: %s. id: %s", err.Error(), resource.Properties.Application))
	}

	// Ensure that we don't have any duplicate ports.
SKIPINSERT:
	for _, newPort := range servicePorts {
		// Skip to add new port. Instead, upsert port if it already exists.
		for j, p := range base.Spec.Ports {
			if strings.EqualFold(p.Name, newPort.Name) || p.Port == newPort.Port || p.TargetPort.IntVal == newPort.TargetPort.IntVal {
				base.Spec.Ports[j] = newPort
				continue SKIPINSERT
			}
		}

		// Add new port if it doesn't exist.
		base.Spec.Ports = append(base.Spec.Ports, newPort)
	}

	base.Spec.Selector = kubernetes.MakeSelectorLabels(appId.Name(), resource.Name)
	base.Spec.Type = corev1.ServiceTypeClusterIP

	return rpv1.NewKubernetesOutputResource(rpv1.LocalIDService, base, base.ObjectMeta), nil
}

func (r Renderer) makeDeployment(
	manifest kubeutil.ObjectManifest,
	applicationName string,
	options renderers.RenderOptions,
	computedValues map[string]rpv1.ComputedValueReference,
	resource *datamodel.ContainerResource,
	roles []rpv1.OutputResource) ([]rpv1.OutputResource, map[string][]byte, error) {

	// If the container requires azure role, it needs to configure workload identity (aka federated identity).
	identityRequired := len(roles) > 0

	dependencies := options.Dependencies
	properties := resource.Properties

	normalizedName := kubernetes.NormalizeResourceName(resource.Name)

	deployment := getDeploymentBase(manifest, applicationName, resource, &options)
	podSpec := &deployment.Spec.Template.Spec

	container := &podSpec.Containers[0]
	for i, c := range podSpec.Containers {
		if strings.EqualFold(c.Name, normalizedName) {
			container = &podSpec.Containers[i]
			break
		}
	}

	ports := []corev1.ContainerPort{}
	for _, port := range properties.Container.Ports {
		ports = append(ports, corev1.ContainerPort{
			ContainerPort: port.ContainerPort,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	container.Image = properties.Container.Image
	container.Ports = append(container.Ports, ports...)
	container.Command = properties.Container.Command
	container.Args = properties.Container.Args
	container.WorkingDir = properties.Container.WorkingDir

	// If the user has specified an image pull policy, use it. Else, we will use Kubernetes default.
	if properties.Container.ImagePullPolicy != "" {
		container.ImagePullPolicy = corev1.PullPolicy(properties.Container.ImagePullPolicy)
	}

	var err error
	if !properties.Container.ReadinessProbe.IsEmpty() {
		container.ReadinessProbe, err = r.makeHealthProbe(properties.Container.ReadinessProbe)
		if err != nil {
			return []rpv1.OutputResource{}, nil, fmt.Errorf("readiness probe encountered errors: %w ", err)
		}
	}
	if !properties.Container.LivenessProbe.IsEmpty() {
		container.LivenessProbe, err = r.makeHealthProbe(properties.Container.LivenessProbe)
		if err != nil {
			return []rpv1.OutputResource{}, nil, fmt.Errorf("liveness probe encountered errors: %w ", err)
		}
	}

	// We build the environment variable list in a stable order for testability
	// For the values that come from connections we back them with secretData. We'll extract the values
	// and return them.
	env, secretData, err := getEnvVarsAndSecretData(resource, dependencies)
	if err != nil {
		return []rpv1.OutputResource{}, nil, fmt.Errorf("failed to obtain environment variables and secret data: %w", err)
	}

	for k, v := range properties.Container.Env {
		env[k], err = convertEnvVar(k, v, options)
		if err != nil {
			return []rpv1.OutputResource{}, nil, fmt.Errorf("failed to convert environment variable: %w", err)
		}
	}

	// Append in sorted order
	for _, key := range getSortedKeys(env) {
		container.Env = append(container.Env, env[key])
	}

	outputResources := []rpv1.OutputResource{}
	deps := []string{}

	podLabels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())

	// Add volumes
	volumes := []corev1.Volume{}

	// Create Kubernetes resource name scoped in Kubernetes namespace
	kubeIdentityName := normalizedName
	podSpec.ServiceAccountName = normalizedName

	// Create Azure resource name for managed/federated identity-scoped in resource group specified by Environment resource.
	// To avoid the naming conflicts, we add the application name prefix to resource name.
	azIdentityName := azrenderer.MakeResourceName(applicationName, resource.Name, azrenderer.Separator)

	for volumeName, volumeProperties := range properties.Container.Volumes {
		// Based on the kind, create a persistent/ephemeral volume
		switch volumeProperties.Kind {
		case datamodel.Ephemeral:
			volumeSpec, volumeMountSpec, err := makeEphemeralVolume(volumeName, volumeProperties.Ephemeral)
			if err != nil {
				return []rpv1.OutputResource{}, nil, fmt.Errorf("unable to create ephemeral volume spec for volume: %s - %w", volumeName, err)
			}
			// Add the volume mount to the Container spec
			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
			// Add the volume to the list of volumes to be added to the Volumes spec
			volumes = append(volumes, volumeSpec)
		case datamodel.Persistent:
			var volumeSpec corev1.Volume
			var volumeMountSpec corev1.VolumeMount

			properties, ok := dependencies[volumeProperties.Persistent.Source]
			if !ok {
				return []rpv1.OutputResource{}, nil, errors.New("volume dependency resource not found")
			}

			vol, ok := properties.Resource.(*datamodel.VolumeResource)
			if !ok {
				return []rpv1.OutputResource{}, nil, errors.New("invalid dependency resource")
			}

			switch vol.Properties.Kind {
			case datamodel.AzureKeyVaultVolume:
				// This will add the required managed identity resources.
				identityRequired = true

				// Prepare role assignments
				roleNames := []string{}
				if len(vol.Properties.AzureKeyVault.Secrets) > 0 {
					roleNames = append(roleNames, AzureKeyVaultSecretsUserRole)
				}
				if len(vol.Properties.AzureKeyVault.Certificates) > 0 || len(vol.Properties.AzureKeyVault.Keys) > 0 {
					roleNames = append(roleNames, AzureKeyVaultCryptoUserRole)
				}

				// Build RoleAssignment output.resource
				kvID := vol.Properties.AzureKeyVault.Resource
				roleAssignments, raDeps := azrenderer.MakeRoleAssignments(kvID, roleNames)
				outputResources = append(outputResources, roleAssignments...)
				deps = append(deps, raDeps...)

				// Create Per-Pod SecretProviderClass for the selected volume
				// csiobjectspec must be generated when volume is updated.
				objectSpec, err := handlers.GetMapValue[string](properties.ComputedValues, azvolrenderer.SPCVolumeObjectSpecKey)
				if err != nil {
					return []rpv1.OutputResource{}, nil, err
				}

				spcName := kubernetes.NormalizeResourceName(vol.Name)
				secretProvider, err := azrenderer.MakeKeyVaultSecretProviderClass(applicationName, spcName, vol, objectSpec, &options.Environment)
				if err != nil {
					return []rpv1.OutputResource{}, nil, err
				}
				outputResources = append(outputResources, *secretProvider)
				deps = append(deps, rpv1.LocalIDSecretProviderClass)

				// Create volume spec which associated with secretProviderClass.
				volumeSpec, volumeMountSpec, err = azrenderer.MakeKeyVaultVolumeSpec(volumeName, volumeProperties.Persistent.MountPath, spcName)
				if err != nil {
					return []rpv1.OutputResource{}, nil, fmt.Errorf("unable to create secretstore volume spec for volume: %s - %w", volumeName, err)
				}
			default:
				return []rpv1.OutputResource{}, nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported volume kind: %s for volume: %s. Supported kinds are: %v", vol.Properties.Kind, volumeName, GetSupportedKinds()))
			}

			// Add the volume mount to the Container spec
			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
			// Add the volume to the list of volumes to be added to the Volumes spec
			volumes = append(volumes, volumeSpec)

			// Add azurestorageaccountname and azurestorageaccountkey as secrets
			// These will be added as key-value pairs to the kubernetes secret created for the container
			// The key values are as per: https://docs.microsoft.com/en-us/azure/aks/azure-files-volume
			for key, value := range properties.ComputedValues {
				if value.(string) == rpv1.LocalIDAzureFileShareStorageAccount {
					// The storage account was not created when the computed value was rendered
					// Lookup the actual storage account name from the local id
					id := properties.OutputResources[value.(string)]
					value = id.Name()
				}
				secretData[key] = []byte(value.(string))
			}
		default:
			return []rpv1.OutputResource{}, secretData, v1.NewClientErrInvalidRequest(fmt.Sprintf("Only ephemeral or persistent volumes are supported. Got kind: %v", volumeProperties.Kind))
		}
	}

	serviceAccountBase := getServiceAccountBase(manifest, applicationName, resource, &options)
	// In order to enable per-container identity, it creates user-assigned managed identity, federated identity, and service account.
	if identityRequired {
		// 1. Create Per-Container managed identity.
		managedIdentity, err := azrenderer.MakeManagedIdentity(azIdentityName, options.Environment.CloudProviders)
		if err != nil {
			return []rpv1.OutputResource{}, nil, err
		}
		outputResources = append(outputResources, *managedIdentity)

		// 2. Create Per-container federated identity resource.
		fedIdentity, err := azrenderer.MakeFederatedIdentity(kubeIdentityName, &options.Environment)
		if err != nil {
			return []rpv1.OutputResource{}, nil, err
		}
		outputResources = append(outputResources, *fedIdentity)

		// 3. Create Per-container service account.
		saAccount := azrenderer.SetWorkloadIdentityServiceAccount(serviceAccountBase)
		outputResources = append(outputResources, *saAccount)
		deps = append(deps, rpv1.LocalIDServiceAccount)

		// This is required to enable workload identity.
		podLabels[azrenderer.AzureWorkloadIdentityUseKey] = "true"

		// 4. Add RBAC resources to the dependencies.
		for _, role := range roles {
			deps = append(deps, role.LocalID)
		}

		computedValues[handlers.IdentityProperties] = rpv1.ComputedValueReference{
			Value: options.Environment.Identity,
			Transformer: func(r v1.DataModelInterface, cv map[string]any) error {
				ei, err := handlers.GetMapValue[*rpv1.IdentitySettings](cv, handlers.IdentityProperties)
				if err != nil {
					return err
				}
				res, ok := r.(*datamodel.ContainerResource)
				if !ok {
					return errors.New("resource must be ContainerResource")
				}
				if res.Properties.Identity == nil {
					res.Properties.Identity = &rpv1.IdentitySettings{}
				}
				res.Properties.Identity.Kind = ei.Kind
				res.Properties.Identity.OIDCIssuer = ei.OIDCIssuer
				return nil
			},
		}

		computedValues[handlers.UserAssignedIdentityIDKey] = rpv1.ComputedValueReference{
			LocalID:           rpv1.LocalIDUserAssignedManagedIdentity,
			PropertyReference: handlers.UserAssignedIdentityIDKey,
			Transformer: func(r v1.DataModelInterface, cv map[string]any) error {
				resourceID, err := handlers.GetMapValue[string](cv, handlers.UserAssignedIdentityIDKey)
				if err != nil {
					return err
				}
				res, ok := r.(*datamodel.ContainerResource)
				if !ok {
					return errors.New("resource must be ContainerResource")
				}
				if res.Properties.Identity == nil {
					res.Properties.Identity = &rpv1.IdentitySettings{}
				}
				res.Properties.Identity.Resource = resourceID
				return nil
			},
		}
	} else {
		// If the container doesn't require identity, we'll use the default service account
		or := rpv1.NewKubernetesOutputResource(rpv1.LocalIDServiceAccount, serviceAccountBase, serviceAccountBase.ObjectMeta)
		outputResources = append(outputResources, or)
		deps = append(deps, rpv1.LocalIDServiceAccount)
	}

	// Create the role and role bindings for SA.
	role := makeRBACRole(applicationName, kubeIdentityName, options.Environment.Namespace, resource)
	outputResources = append(outputResources, *role)
	deps = append(deps, rpv1.LocalIDKubernetesRole)

	roleBinding := makeRBACRoleBinding(applicationName, kubeIdentityName, podSpec.ServiceAccountName, options.Environment.Namespace, resource)
	outputResources = append(outputResources, *roleBinding)
	deps = append(deps, rpv1.LocalIDKubernetesRoleBinding)

	deployment.Spec.Template.ObjectMeta = mergeObjectMeta(deployment.Spec.Template.ObjectMeta, metav1.ObjectMeta{
		Labels: podLabels,
	})

	deployment.Spec.Selector = mergeLabelSelector(deployment.Spec.Selector, &metav1.LabelSelector{
		MatchLabels: kubernetes.MakeSelectorLabels(applicationName, resource.Name),
	})

	podSpec.Volumes = append(podSpec.Volumes, volumes...)

	// See: https://github.com/kubernetes/kubernetes/issues/92226 and
	// 		https://github.com/radius-project/radius/issues/3002
	//
	// Service links are a flawed and Kubernetes-only feature that we don't
	// want to leak into Radius containers.
	podSpec.EnableServiceLinks = to.Ptr(false)

	// If the user has specified a restart policy, use it. Else, it will use the Kubernetes default.
	if properties.RestartPolicy != "" {
		podSpec.RestartPolicy = corev1.RestartPolicy(properties.RestartPolicy)
	}

	// If we have a secret to reference we need to ensure that the deployment will trigger a new revision
	// when the secret changes. Normally referencing an environment variable from a secret will **NOT** cause
	// a new revision when the secret changes.
	//
	// see: https://stackoverflow.com/questions/56711894/does-k8-update-environment-variables-when-secrets-change
	//
	// The solution to this is to embed the hash of the secret as an annotation in the deployment. This way when the
	// secret changes we also change the content of the deployment and thus trigger a new revision. This is a very
	// common solution to this problem, and not a bizarre workaround that we invented.
	if len(secretData) > 0 {
		hash := kubernetes.HashSecretData(secretData)
		deployment.Spec.Template.ObjectMeta.Annotations[kubernetes.AnnotationSecretHash] = hash
		deps = append(deps, rpv1.LocalIDSecret)
	}

	// Patching Runtimes.Kubernetes.Pod to the PodSpec in deployment resource.
	if properties.Runtimes != nil && properties.Runtimes.Kubernetes != nil && properties.Runtimes.Kubernetes.Pod != "" {
		patchedPodSpec, err := patchPodSpec(podSpec, []byte(properties.Runtimes.Kubernetes.Pod))
		if err != nil {
			return []rpv1.OutputResource{}, nil, fmt.Errorf("failed to patch PodSpec: %w", err)
		}
		deployment.Spec.Template.Spec = *patchedPodSpec
	}

	deploymentOutput := rpv1.NewKubernetesOutputResource(rpv1.LocalIDDeployment, deployment, deployment.ObjectMeta)
	deploymentOutput.CreateResource.Dependencies = deps

	outputResources = append(outputResources, deploymentOutput)
	return outputResources, secretData, nil
}

// convertEnvVar function to convert from map[string]EnvironmentVariable to map[string]corev1.EnvVar
func convertEnvVar(key string, env datamodel.EnvironmentVariable, options renderers.RenderOptions) (corev1.EnvVar, error) {
	if env.Value != nil {
		return corev1.EnvVar{Name: key, Value: *env.Value}, nil
	} else if env.ValueFrom != nil {
		// There are two cases to handle here:
		// 1. The value comes from a kubernetes secret
		// 2. The value comes from a Applications.Core/SecretStore resource id.

		// If the value comes from a kubernetes secret, we'll reference it.
		if strings.HasPrefix(env.ValueFrom.SecretRef.Source, "/") {
			secretStore, ok := options.Dependencies[env.ValueFrom.SecretRef.Source].Resource.(*datamodel.SecretStore)
			if !ok {
				return corev1.EnvVar{}, fmt.Errorf("failed to find source in dependencies: %s", env.ValueFrom.SecretRef.Source)
			}

			// The format may be <namespace>/<name> or <name>, as an example "default/my-secret" or "my-secret". We split the string on '/'
			// and take the second part if the secret is namespace qualified.
			var name string
			if strings.Contains(secretStore.Properties.Resource, "/") {
				parts := strings.Split(secretStore.Properties.Resource, "/")
				if len(parts) == 2 {
					name = parts[1]
				} else {
					name = secretStore.Properties.Resource
				}
			} else {
				name = env.ValueFrom.SecretRef.Source
			}

			return corev1.EnvVar{
				Name: key,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: name,
						},
						Key: env.ValueFrom.SecretRef.Key,
					},
				},
			}, nil

		} else {
			return corev1.EnvVar{
				Name: key,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: env.ValueFrom.SecretRef.Source,
						},
						Key: env.ValueFrom.SecretRef.Key,
					},
				},
			}, nil
		}

	} else {
		return corev1.EnvVar{}, fmt.Errorf("failed to convert environment variable: %s, both value and valueFrom cannot be nil", key)
	}
}

func getEnvVarsAndSecretData(resource *datamodel.ContainerResource, dependencies map[string]renderers.RendererDependency) (map[string]corev1.EnvVar, map[string][]byte, error) {
	env := map[string]corev1.EnvVar{}
	secretData := map[string][]byte{}
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

			// handles case where container has source field structured as a URL.
			if isURL(source) {
				// parse source into scheme, hostname, and port.
				scheme, hostname, port, err := parseURL(source)
				if err != nil {
					return map[string]corev1.EnvVar{}, map[string][]byte{}, fmt.Errorf("failed to parse source URL: %w", err)
				}

				schemeKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "SCHEME")
				hostnameKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "HOSTNAME")
				portKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "PORT")

				env[schemeKey] = corev1.EnvVar{Name: schemeKey, Value: scheme}
				env[hostnameKey] = corev1.EnvVar{Name: hostnameKey, Value: hostname}
				env[portKey] = corev1.EnvVar{Name: portKey, Value: port}

				continue
			}

			// handles case where container has source field structured as a resourceID.
			for key, value := range properties.ComputedValues {
				name := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), strings.ToUpper(key))

				source := corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: kubernetes.NormalizeResourceName(resource.Name),
						},
						Key: name,
					},
				}
				switch v := value.(type) {
				case string:
					secretData[name] = []byte(v)
					env[name] = corev1.EnvVar{Name: name, ValueFrom: &source}
				case float64:
					secretData[name] = []byte(strconv.Itoa(int(v)))
					env[name] = corev1.EnvVar{Name: name, ValueFrom: &source}
				case int:
					secretData[name] = []byte(strconv.Itoa(v))
					env[name] = corev1.EnvVar{Name: name, ValueFrom: &source}
				}
			}
		}
	}

	return env, secretData, nil
}

func (r Renderer) makeHealthProbe(p datamodel.HealthProbeProperties) (*corev1.Probe, error) {
	probeSpec := corev1.Probe{}

	switch p.Kind {
	case datamodel.HTTPGetHealthProbe:
		// Set the probe spec
		probeSpec.ProbeHandler.HTTPGet = &corev1.HTTPGetAction{}
		probeSpec.ProbeHandler.HTTPGet.Port = intstr.FromInt(int(p.HTTPGet.ContainerPort))
		probeSpec.ProbeHandler.HTTPGet.Path = p.HTTPGet.Path
		httpHeaders := []corev1.HTTPHeader{}
		for k, v := range p.HTTPGet.Headers {
			httpHeaders = append(httpHeaders, corev1.HTTPHeader{
				Name:  k,
				Value: v,
			})
		}
		probeSpec.ProbeHandler.HTTPGet.HTTPHeaders = httpHeaders
		c := containerHealthProbeConfig{
			initialDelaySeconds: p.HTTPGet.InitialDelaySeconds,
			failureThreshold:    p.HTTPGet.FailureThreshold,
			periodSeconds:       p.HTTPGet.PeriodSeconds,
			timeoutSeconds:      p.HTTPGet.TimeoutSeconds,
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	case datamodel.TCPHealthProbe:
		// Set the probe spec
		probeSpec.ProbeHandler.TCPSocket = &corev1.TCPSocketAction{}
		probeSpec.TCPSocket.Port = intstr.FromInt(int(p.TCP.ContainerPort))
		c := containerHealthProbeConfig{
			initialDelaySeconds: p.TCP.InitialDelaySeconds,
			failureThreshold:    p.TCP.FailureThreshold,
			periodSeconds:       p.TCP.PeriodSeconds,
			timeoutSeconds:      p.TCP.TimeoutSeconds,
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	case datamodel.ExecHealthProbe:
		// Set the probe spec
		probeSpec.ProbeHandler.Exec = &corev1.ExecAction{}
		probeSpec.Exec.Command = strings.Split(p.Exec.Command, " ")
		c := containerHealthProbeConfig{
			initialDelaySeconds: p.Exec.InitialDelaySeconds,
			failureThreshold:    p.Exec.FailureThreshold,
			periodSeconds:       p.Exec.PeriodSeconds,
			timeoutSeconds:      p.Exec.TimeoutSeconds,
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	default:
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("health probe kind unsupported: %v", p.Kind))
	}
	return &probeSpec, nil
}

type containerHealthProbeConfig struct {
	initialDelaySeconds *float32
	failureThreshold    *float32
	periodSeconds       *float32
	timeoutSeconds      *float32
}

func (r Renderer) setContainerHealthProbeConfig(probeSpec *corev1.Probe, config containerHealthProbeConfig) {
	// Initialize with Radius defaults and overwrite if values are specified
	probeSpec.InitialDelaySeconds = DefaultInitialDelaySeconds
	probeSpec.FailureThreshold = DefaultFailureThreshold
	probeSpec.PeriodSeconds = DefaultPeriodSeconds
	probeSpec.TimeoutSeconds = DefaultTimeoutSeconds

	if config.initialDelaySeconds != nil {
		probeSpec.InitialDelaySeconds = int32(*config.initialDelaySeconds)
	}

	if config.failureThreshold != nil {
		probeSpec.FailureThreshold = int32(*config.failureThreshold)
	}

	if config.periodSeconds != nil {
		probeSpec.PeriodSeconds = int32(*config.periodSeconds)
	}

	if config.timeoutSeconds != nil {
		probeSpec.TimeoutSeconds = int32(*config.timeoutSeconds)
	}
}

func (r Renderer) makeSecret(resource datamodel.ContainerResource, applicationName string, secrets map[string][]byte, options renderers.RenderOptions) rpv1.OutputResource {
	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.NormalizeResourceName(resource.Name),
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName()),
		},
		Type: corev1.SecretTypeOpaque,
		Data: secrets,
	}

	output := rpv1.NewKubernetesOutputResource(rpv1.LocalIDSecret, &secret, secret.ObjectMeta)
	return output
}

func (r Renderer) isIdentitySupported(kind datamodel.IAMKind) bool {
	if r.RoleAssignmentMap == nil || !kind.IsValid() {
		return false
	}

	_, ok := r.RoleAssignmentMap[kind]
	return ok
}

// Assigns roles/permissions to a specific resource for the managed identity resource.
func (r Renderer) makeRoleAssignmentsForResource(connection *datamodel.ConnectionProperties, dependencies map[string]renderers.RendererDependency) ([]rpv1.OutputResource, error) {
	var roleNames []string
	var armResourceIdentifier string
	if connection.IAM.Kind.IsKind(datamodel.KindAzure) {
		roleNames = append(roleNames, connection.IAM.Roles...)
		armResourceIdentifier = connection.Source
	} else {
		// We're reporting errors in this code path to avoid obscuring a bug in another layer of the system.
		// None of these error conditions should be caused by invalid user input. They should only be caused
		// by internal bugs in Radius.
		roleAssignmentData, ok := r.RoleAssignmentMap[connection.IAM.Kind]
		if !ok {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("RBAC is not supported for connection kind %q", connection.IAM.Kind))
		}

		// The dependency will have already been fetched by the system.
		dependency, ok := dependencies[connection.Source]
		if !ok {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("connection source %q was not found in the dependencies collection", connection.Source))
		}

		// Find the matching output resource based on LocalID
		target, ok := dependency.OutputResources[roleAssignmentData.LocalID]
		if !ok {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("output resource %q was not found in the outputs of dependency %q", roleAssignmentData.LocalID, connection.Source))
		}

		if !resources_azure.IsAzureResource(target) {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("output resource %q must be an ARM resource to support role assignments. Was: %+v", roleAssignmentData.LocalID, target))
		}
		armResourceIdentifier = target.String()

		roleNames = roleAssignmentData.RoleNames
	}

	outputResources := []rpv1.OutputResource{}
	for _, roleName := range roleNames {
		localID := rpv1.NewLocalID(rpv1.LocalIDRoleAssignmentPrefix, armResourceIdentifier, roleName)
		roleAssignment := rpv1.OutputResource{

			LocalID: localID,
			CreateResource: &rpv1.Resource{
				Data: map[string]string{
					handlers.RoleNameKey:         roleName,
					handlers.RoleAssignmentScope: armResourceIdentifier,
				},
				ResourceType: resourcemodel.ResourceType{
					Type:     resources_azure.ResourceTypeAuthorizationRoleAssignment,
					Provider: resourcemodel.ProviderAzure,
				},
				Dependencies: []string{rpv1.LocalIDUserAssignedManagedIdentity},
			},
		}

		outputResources = append(outputResources, roleAssignment)
	}

	return outputResources, nil
}

func getSortedKeys(env map[string]corev1.EnvVar) []string {
	keys := []string{}
	for k := range env {
		key := k
		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys
}

func isURL(input string) bool {
	_, err := url.ParseRequestURI(input)

	// if first character is a slash, it's not a URL. It's a path.
	if input == "" || err != nil || input[0] == '/' {
		return false
	}
	return true
}

func parseURL(sourceURL string) (scheme, hostname, port string, err error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", "", "", err
	}

	scheme = u.Scheme
	host := u.Host

	hostname, port, err = net.SplitHostPort(host)
	if err != nil {
		return "", "", "", err
	}

	return scheme, hostname, port, nil
}
