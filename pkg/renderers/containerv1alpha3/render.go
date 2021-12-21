// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha3

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/volumev1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
)

const (
	ResourceType = "Container"
)

// Liveness/Readiness constants
const (
	DefaultInitialDelaySeconds   = 0
	DefaultFailureThreshold      = 3
	DefaultPeriodSeconds         = 10
	AzureKeyVaultSecretsUserRole = "Key Vault Secrets User"
	AzureKeyVaultCryptoUserRole  = "Key Vault Crypto User"
)

// Renderer is the WorkloadRenderer implementation for containerized workload.
type Renderer struct {

	// RoleAssignmentMap is an optional map of connection kind -> []Role Assignment. Used to configure managed
	// identity permissions for cloud resources. This will be nil in environments that don't support role assignments.
	RoleAssignmentMap map[radclient.ContainerConnectionKind]RoleAssignmentData
}

func (r Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) (radiusResourceIDs []azresources.ResourceID, azureResourceIDs []azresources.ResourceID, err error) {
	properties, err := r.convert(resource)
	if err != nil {
		return nil, nil, err
	}

	// Right now we only have things in connections and ports as rendering dependencies - we'll add more things
	// in the future... eg: volumes
	//
	// Anywhere we accept a resource ID in the model should have its value returned from here
	for _, connection := range properties.Connections {
		resourceID, err := azresources.Parse(*connection.Source)
		if err != nil {
			return nil, nil, err
		}

		// Non-radius Azure connections that are accessible from Radius container resource.
		if *connection.Kind == radclient.ContainerConnectionKindAzure {
			azureResourceIDs = append(azureResourceIDs, resourceID)
			continue
		}

		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}

	for _, port := range properties.Container.Ports {
		provides := to.String(port.Provides)
		if provides == "" {
			continue
		}

		resourceID, err := azresources.Parse(provides)
		if err != nil {
			return nil, nil, err
		}
		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}

	for _, volume := range properties.Container.Volumes {
		switch v := volume.(type) {
		case *radclient.PersistentVolume:
			resourceID, err := azresources.Parse(to.String(v.Source))
			if err != nil {
				return nil, nil, err
			}
			radiusResourceIDs = append(radiusResourceIDs, resourceID)
		}
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	outputResources := []outputresource.OutputResource{}
	resource := options.Resource
	dependencies := options.Dependencies

	cw, err := r.convert(resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Create the deployment as the primary workload
	deploymentOutputResources, secretData, err := r.makeDeployment(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies}, cw)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, deploymentOutputResources...)

	// If there are secrets we'll use a Kubernetes secret to hold them. This is already referenced
	// by the deployment.
	if len(secretData) > 0 {
		outputResources = append(outputResources, r.makeSecret(ctx, resource, secretData))
	}

	// Connections might require a role assignment to grant access.
	roles := []outputresource.OutputResource{}
	for _, connection := range cw.Connections {
		if connection == nil || !r.isIdentitySupported(*connection) {
			continue
		}

		rbacOutputResources, err := r.makeRoleAssignmentsForResource(ctx, *connection, dependencies)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		roles = append(roles, rbacOutputResources...)
	}

	// If we created role assigmments then we will need an identity and the mapping of the identity to AKS.
	if len(roles) > 0 {
		outputResources = append(outputResources, roles...)
		outputResources = append(outputResources, r.makeManagedIdentity(ctx, resource))
		outputResources = append(outputResources, r.makePodIdentity(ctx, resource, roles))
	}

	return renderers.RendererOutput{Resources: outputResources}, nil
}

func (r Renderer) convert(resource renderers.RendererResource) (*radclient.ContainerProperties, error) {
	properties := &radclient.ContainerProperties{}
	err := resource.ConvertDefinition(properties)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (r Renderer) makeDeployment(ctx context.Context, options renderers.RenderOptions, cc *radclient.ContainerProperties) ([]outputresource.OutputResource, map[string][]byte, error) {
	// Keep track of the set of routes, we will need these to generate labels later
	routes := []struct {
		Name string
		Type string
	}{}
	resource := options.Resource
	dependencies := options.Dependencies

	ports := []corev1.ContainerPort{}
	for _, port := range cc.Container.Ports {
		if provides := to.String(port.Provides); provides != "" {
			resourceId, err := azresources.Parse(provides)
			if err != nil {
				return []outputresource.OutputResource{}, nil, err
			}
			routeName := resourceId.Name()
			routeType := resourceId.Types[len(resourceId.Types)-1].Type
			routes = append(routes, struct {
				Name string
				Type string
			}{Name: routeName, Type: routeType})

			ports = append(ports, corev1.ContainerPort{
				// Name generation logic has to match the code in HttpRoute
				Name:          kubernetes.GetShortenedTargetPortName(resource.ApplicationName + routeType + routeName),
				ContainerPort: to.Int32(port.ContainerPort),
				Protocol:      corev1.ProtocolTCP,
			})
		} else {
			ports = append(ports, corev1.ContainerPort{
				ContainerPort: to.Int32(port.ContainerPort),
				Protocol:      corev1.ProtocolTCP,
			})
		}

	}

	container := corev1.Container{
		Name:  resource.ResourceName,
		Image: to.String(cc.Container.Image),
		// TODO: use better policies than this when we have a good versioning story
		ImagePullPolicy: corev1.PullPolicy("Always"),
		Ports:           ports,
		Env:             []corev1.EnvVar{},
		VolumeMounts:    []corev1.VolumeMount{},
	}

	var err error
	if cc.Container.ReadinessProbe != nil {
		container.ReadinessProbe, err = r.makeHealthProbe(cc.Container.ReadinessProbe)
		if err != nil {
			return []outputresource.OutputResource{}, nil, fmt.Errorf("readiness probe encountered errors: %w ", err)
		}
	}
	if cc.Container.LivenessProbe != nil {
		container.LivenessProbe, err = r.makeHealthProbe(cc.Container.LivenessProbe)
		if err != nil {
			return []outputresource.OutputResource{}, nil, fmt.Errorf("liveness probe encountered errors: %w ", err)
		}
	}

	// We build the environment variable list in a stable order for testability
	env := map[string]corev1.EnvVar{}

	// For the values that come from connections we back them with secretData. We'll extract the values
	// and return them.
	secretData := map[string][]byte{}

	// Take each connection and create environment variables for each part
	for name, con := range cc.Connections {
		properties := dependencies[to.String(con.Source)]
		for key, value := range properties.ComputedValues {
			name := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), strings.ToUpper(key))

			// We'll store each value in a secret named with the same name as the resource.
			// We'll use the environment variable names as keys.
			source := corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: resource.ResourceName,
					},
					Key: name,
				},
			}
			switch v := value.(type) {
			case string:
				secretData[name] = []byte(v)
				env[name] = corev1.EnvVar{Name: name, ValueFrom: &source}
			case float64: // Float is used by the JSON serializer
				secretData[name] = []byte(strconv.Itoa(int(v)))
				env[name] = corev1.EnvVar{Name: name, ValueFrom: &source}
			case int:
				secretData[name] = []byte(strconv.Itoa(v))
				env[name] = corev1.EnvVar{Name: name, ValueFrom: &source}
			}
		}
	}

	for k, v := range cc.Container.Env {
		env[k] = corev1.EnvVar{Name: k, Value: to.String(v)}
	}

	// Append in sorted order
	for _, key := range getSortedKeys(env) {
		container.Env = append(container.Env, env[key])
	}

	outputResources := []outputresource.OutputResource{}

	podLabels := kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName)

	// Add volumes
	volumes := []corev1.Volume{}
	for volumeName, volume := range cc.Container.Volumes {
		// Based on the kind, create a persistent/ephemeral volume
		switch v := volume.(type) {
		case *radclient.EphemeralVolume:
			volumeSpec, volumeMountSpec, err := r.makeEphemeralVolume(volumeName, *v)
			if err != nil {
				return []outputresource.OutputResource{}, nil, fmt.Errorf("unable to create ephemeral volume spec for volume: %s - %w", volumeName, err)
			}
			// Add the volume mount to the Container spec
			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
			// Add the volume to the list of volumes to be added to the Volumes spec
			volumes = append(volumes, volumeSpec)
		case *radclient.PersistentVolume:
			var volumeSpec corev1.Volume
			var volumeMountSpec corev1.VolumeMount
			properties := dependencies[to.String(v.Source)]

			switch properties.Definition["kind"] {
			case volumev1alpha3.PersistentVolumeKindAzureFileShare:
				// Create spec for persistent volume
				volumeSpec, volumeMountSpec, err = r.makeAzureFileSharePersistentVolume(volumeName, *v, options)
				if err != nil {
					return []outputresource.OutputResource{}, nil, fmt.Errorf("unable to create persistent volume spec for volume: %s - %w", volumeName, err)
				}
			case volumev1alpha3.PersistentVolumeKindAzureKeyVault:
				// Make Managed Identity
				managedIdentity := r.makeManagedIdentity(ctx, resource)
				outputResources = append(outputResources, managedIdentity)

				// Make Role assignments
				roleNames := []string{}
				if properties.Definition["secrets"] != nil {
					roleNames = append(roleNames, AzureKeyVaultSecretsUserRole)
				}
				if properties.Definition["certificates"] != nil || properties.Definition["keys"] != nil {
					roleNames = append(roleNames, AzureKeyVaultCryptoUserRole)
				}
				kvID := properties.Definition["resource"].(string)
				roleAssignments, err := r.makeRoleAssignmentsForAzureKeyVaultCSIDriver(ctx, kvID, roleNames)
				if err != nil {
					return []outputresource.OutputResource{}, nil, fmt.Errorf("unable to create role assignments for volume: %s - %w", volumeName, err)
				}
				outputResources = append(outputResources, roleAssignments...)

				// Make Pod Identity
				podIdentity := r.makePodIdentity(ctx, resource, []outputresource.OutputResource{})
				outputResources = append(outputResources, podIdentity)
				// We need to add labels for pod identity created to the pod metadata
				podIDLabel := kubernetes.MakeAADPodIdentityBindingLabels(podIdentity.Resource.(map[string]string)[handlers.PodIdentityNameKey])
				podLabels = labels.Merge(podIDLabel, podLabels)

				secretProviderClass := properties.OutputResources[outputresource.LocalIDSecretProviderClass]
				secretProviderClassName := secretProviderClass.Data.(resourcemodel.KubernetesIdentity).Name
				// Create spec for secret store
				volumeSpec, volumeMountSpec, err = r.makeAzureKeyVaultPersistentVolume(volumeName, *v, secretProviderClassName, options)
				if err != nil {
					return []outputresource.OutputResource{}, nil, fmt.Errorf("unable to create secretstore volume spec for volume: %s - %w", volumeName, err)
				}
			default:
				return []outputresource.OutputResource{}, nil, fmt.Errorf("Unsupported volume kind: %s for volume: %s. Supported kinds are: %v", properties.Definition["kind"], volumeName, volumev1alpha3.GetSupportedKinds())
			}

			// Add the volume mount to the Container spec
			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
			// Add the volume to the list of volumes to be added to the Volumes spec
			volumes = append(volumes, volumeSpec)

			// Add azurestorageaccountname and azurestorageaccountkey as secrets
			// These will be added as key-value pairs to the kubernetes secret created for the container
			// The key values are as per: https://docs.microsoft.com/en-us/azure/aks/azure-files-volume
			for key, value := range properties.ComputedValues {
				if value.(string) == outputresource.LocalIDAzureFileShareStorageAccount {
					// The storage account was not created when the computed value was rendered
					// Lookup the actual storage account name from the local id
					id := properties.OutputResources[value.(string)].Data.(resourcemodel.ARMIdentity).ID
					r, err := azresources.Parse(id)
					if err != nil {
						return []outputresource.OutputResource{}, nil, err
					}
					value = r.Name()
				}
				secretData[key] = []byte(value.(string))
			}
		default:
			return []outputresource.OutputResource{}, secretData, fmt.Errorf("Only ephemeral or persistent volumes are supported. Got kind: %v", *volume.GetVolume().Kind)
		}
	}

	// In addition to the descriptive labels, we need to attach labels for each route
	// so that the generated services can find these pods
	for _, routeInfo := range routes {
		routeLabels := kubernetes.MakeRouteSelectorLabels(resource.ApplicationName, routeInfo.Type, routeInfo.Name)
		podLabels = labels.Merge(routeLabels, podLabels)
	}

	deployment := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: kubernetes.MakeSelectorLabels(resource.ApplicationName, resource.ResourceName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
					Volumes:    volumes,
				},
			},
		},
	}

	output := outputresource.NewKubernetesOutputResource(outputresource.LocalIDDeployment, &deployment, deployment.ObjectMeta)
	outputResources = append(outputResources, output)
	return outputResources, secretData, nil
}

func (r Renderer) makeEphemeralVolume(volumeName string, volume radclient.EphemeralVolume) (corev1.Volume, corev1.VolumeMount, error) {
	// Make volume spec
	volumeSpec := corev1.Volume{}
	volumeSpec.Name = volumeName
	volumeSpec.VolumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{}
	if volume.ManagedStore != nil && *volume.ManagedStore == radclient.EphemeralVolumeManagedStoreMemory {
		volumeSpec.VolumeSource.EmptyDir.Medium = corev1.StorageMediumMemory
	} else {
		volumeSpec.VolumeSource.EmptyDir.Medium = corev1.StorageMediumDefault
	}

	// Make volumeMount spec
	volumeMountSpec := corev1.VolumeMount{}
	volumeMountSpec.MountPath = to.String(volume.MountPath)
	volumeMountSpec.Name = volumeName

	return volumeSpec, volumeMountSpec, nil
}

func (r Renderer) makeHealthProbe(p radclient.HealthProbePropertiesClassification) (*corev1.Probe, error) {
	probeSpec := corev1.Probe{}

	switch probe := p.(type) {
	case *radclient.HTTPGetHealthProbeProperties:
		// Set the probe spec
		probeSpec.Handler.HTTPGet = &corev1.HTTPGetAction{}
		probeSpec.Handler.HTTPGet.Port = intstr.FromInt(int(to.Int32(probe.ContainerPort)))
		probeSpec.Handler.HTTPGet.Path = to.String(probe.Path)
		httpHeaders := []corev1.HTTPHeader{}
		for k, v := range probe.Headers {
			httpHeaders = append(httpHeaders, corev1.HTTPHeader{
				Name:  k,
				Value: to.String(v),
			})
		}
		probeSpec.Handler.HTTPGet.HTTPHeaders = httpHeaders
		c := containerHealthProbeConfig{
			initialDelaySeconds: probe.InitialDelaySeconds,
			failureThreshold:    probe.FailureThreshold,
			periodSeconds:       probe.PeriodSeconds,
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	case *radclient.TCPHealthProbeProperties:
		// Set the probe spec
		probeSpec.Handler.TCPSocket = &corev1.TCPSocketAction{}
		probeSpec.TCPSocket.Port = intstr.FromInt(int(to.Int32(probe.ContainerPort)))
		c := containerHealthProbeConfig{
			initialDelaySeconds: probe.InitialDelaySeconds,
			failureThreshold:    probe.FailureThreshold,
			periodSeconds:       probe.PeriodSeconds,
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	case *radclient.ExecHealthProbeProperties:
		// Set the probe spec
		probeSpec.Handler.Exec = &corev1.ExecAction{}
		probeSpec.Exec.Command = strings.Split(to.String(probe.Command), " ")
		c := containerHealthProbeConfig{
			initialDelaySeconds: probe.InitialDelaySeconds,
			failureThreshold:    probe.FailureThreshold,
			periodSeconds:       probe.PeriodSeconds,
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	default:
		return nil, fmt.Errorf("health probe kind unsupported: %v", *p.GetHealthProbeProperties().Kind)
	}
	return &probeSpec, nil
}

type containerHealthProbeConfig struct {
	initialDelaySeconds *float32
	failureThreshold    *float32
	periodSeconds       *float32
}

func (r Renderer) setContainerHealthProbeConfig(probeSpec *corev1.Probe, config containerHealthProbeConfig) {
	// Initialize with Radius defaults and overwrite if values are specified
	probeSpec.InitialDelaySeconds = DefaultInitialDelaySeconds
	probeSpec.FailureThreshold = DefaultFailureThreshold
	probeSpec.PeriodSeconds = DefaultPeriodSeconds

	if config.initialDelaySeconds != nil {
		probeSpec.InitialDelaySeconds = int32(*config.initialDelaySeconds)
	}

	if config.failureThreshold != nil {
		probeSpec.FailureThreshold = int32(*config.failureThreshold)
	}

	if config.periodSeconds != nil {
		probeSpec.PeriodSeconds = int32(*config.periodSeconds)
	}
}

func (r Renderer) makeAzureFileSharePersistentVolume(volumeName string, persistentVolume radclient.PersistentVolume, options renderers.RenderOptions) (corev1.Volume, corev1.VolumeMount, error) {
	// Make volume spec
	volumeSpec := corev1.Volume{}
	volumeSpec.Name = volumeName
	volumeSpec.VolumeSource.AzureFile = &corev1.AzureFileVolumeSource{}
	volumeSpec.AzureFile.SecretName = options.Resource.ResourceName
	resourceID, err := azresources.Parse(to.String(persistentVolume.Source))
	if err != nil {
		return corev1.Volume{}, corev1.VolumeMount{}, err
	}
	shareName := resourceID.Types[2].Name
	volumeSpec.AzureFile.ShareName = shareName
	// Make volumeMount spec
	volumeMountSpec := corev1.VolumeMount{}
	volumeMountSpec.Name = volumeName
	volumeMountSpec.MountPath = to.String(persistentVolume.MountPath)
	if persistentVolume.Rbac != nil && *persistentVolume.Rbac == radclient.PersistentVolumeRbacRead {
		volumeMountSpec.ReadOnly = true
	}
	return volumeSpec, volumeMountSpec, nil
}

func (r Renderer) makeAzureKeyVaultPersistentVolume(volumeName string, keyvaultVolume radclient.PersistentVolume, secretProviderClassName string, options renderers.RenderOptions) (corev1.Volume, corev1.VolumeMount, error) {
	// Make Volume Spec which uses the SecretProvider created above
	volumeSpec := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver: "secrets-store.csi.k8s.io",
				// We will support only Read operations
				ReadOnly: to.BoolPtr(true),
				VolumeAttributes: map[string]string{
					"secretProviderClass": secretProviderClassName,
				},
			},
		},
	}

	// Make Volume mount spec
	volumeMountSpec := corev1.VolumeMount{
		Name:      volumeName,
		MountPath: to.String(keyvaultVolume.MountPath),
		// We will support only reads to the secret store volume
		ReadOnly: true,
	}
	return volumeSpec, volumeMountSpec, nil
}

func (r Renderer) makeSecret(ctx context.Context, resource renderers.RendererResource, secrets map[string][]byte) outputresource.OutputResource {
	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resource.ResourceName,
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Type: corev1.SecretTypeOpaque,
		Data: secrets,
	}

	// Skip registration of the secret resource with the HealthService since health as a concept is not quite applicable to it
	output := outputresource.NewKubernetesOutputResource(outputresource.LocalIDSecret, &secret, secret.ObjectMeta)
	return output
}

func (r Renderer) isIdentitySupported(connection radclient.ContainerConnection) bool {
	if r.RoleAssignmentMap == nil || connection.Kind == nil {
		return false
	}

	_, ok := r.RoleAssignmentMap[*connection.Kind]
	return ok
}

// Builds a user-assigned managed identity output resource.
func (r Renderer) makeManagedIdentity(ctx context.Context, resource renderers.RendererResource) outputresource.OutputResource {
	managedIdentityName := resource.ApplicationName + "-" + resource.ResourceName + "-msi"
	identityOutputResource := outputresource.OutputResource{
		ResourceKind: resourcekinds.AzureUserAssignedManagedIdentity,
		LocalID:      outputresource.LocalIDUserAssignedManagedIdentity,
		Deployed:     false,
		Managed:      true,
		Resource: map[string]string{
			handlers.ManagedKey:                  "true",
			handlers.UserAssignedIdentityNameKey: managedIdentityName,
		},
	}

	return identityOutputResource
}

// Builds an AKS pod-identity output resource.
func (r Renderer) makePodIdentity(ctx context.Context, resource renderers.RendererResource, roles []outputresource.OutputResource) outputresource.OutputResource {

	// Note: Pod Identity name cannot have camel case
	podIdentityName := fmt.Sprintf("podid-%s-%s", strings.ToLower(resource.ApplicationName), strings.ToLower(resource.ResourceName))

	// Managed identity with required role assignments should be created first
	dependencies := []outputresource.Dependency{
		{
			LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
		},
	}

	for _, role := range roles {
		dependencies = append(dependencies, outputresource.Dependency{LocalID: role.LocalID})
	}

	outputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAADPodIdentity,
		ResourceKind: resourcekinds.AzurePodIdentity,
		Managed:      true,
		Deployed:     false,
		Resource: map[string]string{
			handlers.ManagedKey:         "true",
			handlers.PodIdentityNameKey: podIdentityName,
			handlers.PodNamespaceKey:    resource.ApplicationName,
		},
		Dependencies: dependencies,
	}

	return outputResource
}

// Assigns roles/permissions to a specific resource for the managed identity resource.
func (r Renderer) makeRoleAssignmentsForResource(ctx context.Context, connection radclient.ContainerConnection, dependencies map[string]renderers.RendererDependency) ([]outputresource.OutputResource, error) {
	var roleNames []string
	var armResourceIdentifier string
	if *connection.Kind == radclient.ContainerConnectionKindAzure {
		if len(connection.Roles) < 1 {
			return nil, fmt.Errorf("rbac permissions are required to access Azure connections")
		}
		for _, role := range connection.Roles {
			roleNames = append(roleNames, *role)
		}
		armResourceIdentifier = *connection.Source
	} else {
		// We're reporting errors in this code path to avoid obscuring a bug in another layer of the system.
		// None of these error conditions should be caused by invalid user input. They should only be caused
		// by internal bugs in Radius.
		roleAssignmentData, ok := r.RoleAssignmentMap[*connection.Kind]
		if !ok {
			return nil, fmt.Errorf("RBAC is not supported for connection kind %q", *connection.Kind)
		}

		// The dependency will have already been fetched by the system.
		dependency, ok := dependencies[*connection.Source]
		if !ok {
			return nil, fmt.Errorf("connection source %q was not found in the dependencies collection", *connection.Source)
		}

		// Find the matching output resource based on LocalID
		target, ok := dependency.OutputResources[roleAssignmentData.LocalID]
		if !ok {
			return nil, fmt.Errorf("output resource %q was not found in the outputs of dependency %q", roleAssignmentData.LocalID, *connection.Source)
		}

		// Now we know the resource ID to assign roles against.
		arm, ok := target.Data.(resourcemodel.ARMIdentity)
		if !ok {
			return nil, fmt.Errorf("output resource %q must be an ARM resource to support role assignments. Was: %+v", roleAssignmentData.LocalID, target)
		}
		armResourceIdentifier = arm.ID

		roleNames = roleAssignmentData.RoleNames
	}

	outputResources := []outputresource.OutputResource{}
	for _, roleName := range roleNames {
		localID := outputresource.GenerateLocalIDForRoleAssignment(armResourceIdentifier, roleName)
		roleAssignment := outputresource.OutputResource{
			ResourceKind: resourcekinds.AzureRoleAssignment,
			LocalID:      localID,
			Managed:      true,
			Deployed:     false,
			Resource: map[string]string{
				handlers.RoleNameKey:             roleName,
				handlers.RoleAssignmentTargetKey: armResourceIdentifier,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
				},
			},
		}

		outputResources = append(outputResources, roleAssignment)
	}

	return outputResources, nil
}

// Assigns roles/permissions to a specific resource for the managed identity resource.
func (r Renderer) makeRoleAssignmentsForAzureKeyVaultCSIDriver(ctx context.Context, keyVaultID string, roleNames []string) ([]outputresource.OutputResource, error) {
	roleAssignmentData := RoleAssignmentData{
		RoleNames: roleNames,
		LocalID:   outputresource.LocalIDKeyVault,
	}

	outputResources := []outputresource.OutputResource{}
	for _, roleName := range roleAssignmentData.RoleNames {
		localID := outputresource.GenerateLocalIDForRoleAssignment(keyVaultID, roleName)
		roleAssignment := outputresource.OutputResource{
			ResourceKind: resourcekinds.AzureRoleAssignment,
			LocalID:      localID,
			Managed:      true,
			Deployed:     false,
			Resource: map[string]string{
				handlers.RoleNameKey:             roleName,
				handlers.RoleAssignmentTargetKey: keyVaultID,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
				},
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
