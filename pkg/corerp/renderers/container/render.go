// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container

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
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers/volumev1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	ResourceType = "Applications.Core/containers"

	// Liveness/Readiness constants
	DefaultInitialDelaySeconds = 0
	DefaultFailureThreshold    = 3
	DefaultPeriodSeconds       = 10

	AzureKeyVaultSecretsUserRole = "Key Vault Secrets User"
	AzureKeyVaultCryptoUserRole  = "Key Vault Crypto User"
)

// Renderer is the WorkloadRenderer implementation for containerized workload.
type Renderer struct {

	// RoleAssignmentMap is an optional map of connection kind -> []Role Assignment. Used to configure managed
	// identity permissions for cloud resources. This will be nil in environments that don't support role assignments.
	RoleAssignmentMap map[datamodel.IAMKind]RoleAssignmentData
}

func (r Renderer) GetDependencyIDs(ctx context.Context, dm conv.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return nil, nil, conv.ErrInvalidModelConversion
	}
	properties := resource.Properties

	// Right now we only have things in connections and ports as rendering dependencies - we'll add more things
	// in the future... eg: volumes
	//
	// Anywhere we accept a resource ID in the model should have its value returned from here
	for _, connection := range properties.Connections {
		resourceID, err := resources.Parse(connection.Source)
		if err != nil {
			return nil, nil, err
		}

		// Non-radius Azure connections that are accessible from Radius container resource.
		if connection.IAM.Kind.IsKind(datamodel.KindAzure) {
			azureResourceIDs = append(azureResourceIDs, resourceID)
			continue
		}

		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}

	for _, port := range properties.Container.Ports {
		provides := port.Provides
		if provides == "" {
			continue
		}

		resourceID, err := resources.Parse(provides)
		if err != nil {
			return nil, nil, err
		}
		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}

	for _, volume := range properties.Container.Volumes {
		switch volume.Kind {
		case datamodel.Persistent:
			resourceID, err := resources.Parse(volume.Persistent.Source)
			if err != nil {
				return nil, nil, err
			}
			radiusResourceIDs = append(radiusResourceIDs, resourceID)
		}
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	appId, err := resources.Parse(resource.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("invalid application id: %w ", err)
	}

	outputResources := []outputresource.OutputResource{}
	dependencies := options.Dependencies
	applicationName := appId.Name()

	// Create the deployment as the primary workload
	deploymentOutput, otherOutputResources, secretData, err := r.makeDeployment(ctx, *resource, applicationName, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputResources = append(outputResources, otherOutputResources...)

	// If there are secrets we'll use a Kubernetes secret to hold them. This is already referenced
	// by the deployment.
	if len(secretData) > 0 {
		outputResources = append(outputResources, r.makeSecret(ctx, *resource, applicationName, secretData, options))
		deploymentOutput.Dependencies = append(deploymentOutput.Dependencies, outputresource.Dependency{
			LocalID: outputresource.LocalIDSecret,
		})
	}

	outputResources = append(outputResources, deploymentOutput)

	// Connections might require a role assignment to grant access.
	roles := []outputresource.OutputResource{}
	for _, connection := range resource.Properties.Connections {
		if !r.isIdentitySupported(connection.IAM.Kind) {
			continue
		}

		rbacOutputResources, err := r.makeRoleAssignmentsForResource(ctx, &connection, dependencies)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		roles = append(roles, rbacOutputResources...)
	}

	// If we created role assigmments then we will need an identity and the mapping of the identity to AKS.
	if len(roles) > 0 {
		outputResources = append(outputResources, roles...)
		outputResources = append(outputResources, r.makeManagedIdentity(ctx, *resource, applicationName))
		outputResources = append(outputResources, r.makePodIdentity(ctx, *resource, applicationName, roles))
	}

	return renderers.RendererOutput{Resources: outputResources}, nil
}

func (r Renderer) makeDeployment(ctx context.Context, resource datamodel.ContainerResource, applicationName string, options renderers.RenderOptions) (outputresource.OutputResource, []outputresource.OutputResource, map[string][]byte, error) {
	// Keep track of the set of routes, we will need these to generate labels later
	routes := []struct {
		Name string
		Type string
	}{}

	dependencies := options.Dependencies
	cc := resource.Properties

	ports := []corev1.ContainerPort{}
	for _, port := range cc.Container.Ports {
		if provides := port.Provides; provides != "" {
			resourceId, err := resources.Parse(provides)
			if err != nil {
				return outputresource.OutputResource{}, []outputresource.OutputResource{}, nil, err
			}

			routeName := resourceId.Name()
			routeType := resourceId.TypeSegments()[len(resourceId.TypeSegments())-1].Type
			routeTypeParts := strings.Split(routeType, "/")

			routeTypeSuffix := routeTypeParts[len(routeTypeParts)-1]

			routes = append(routes, struct {
				Name string
				Type string
			}{Name: routeName, Type: routeTypeSuffix})

			ports = append(ports, corev1.ContainerPort{
				// Name generation logic has to match the code in HttpRoute
				Name:          kubernetes.GetShortenedTargetPortName(applicationName + routeTypeSuffix + routeName),
				ContainerPort: port.ContainerPort,
				Protocol:      corev1.ProtocolTCP,
			})
		} else {
			ports = append(ports, corev1.ContainerPort{
				ContainerPort: port.ContainerPort,
				Protocol:      corev1.ProtocolTCP,
			})
		}

	}

	container := corev1.Container{
		Name:  resource.Name,
		Image: cc.Container.Image,
		// TODO: use better policies than this when we have a good versioning story
		ImagePullPolicy: corev1.PullPolicy("Always"),
		Ports:           ports,
		Env:             []corev1.EnvVar{},
		VolumeMounts:    []corev1.VolumeMount{},
	}

	var err error
	if !cc.Container.ReadinessProbe.IsEmpty() {
		container.ReadinessProbe, err = r.makeHealthProbe(cc.Container.ReadinessProbe)
		if err != nil {
			return outputresource.OutputResource{}, []outputresource.OutputResource{}, nil, fmt.Errorf("readiness probe encountered errors: %w ", err)
		}
	}
	if !cc.Container.LivenessProbe.IsEmpty() {
		container.LivenessProbe, err = r.makeHealthProbe(cc.Container.LivenessProbe)
		if err != nil {
			return outputresource.OutputResource{}, []outputresource.OutputResource{}, nil, fmt.Errorf("liveness probe encountered errors: %w ", err)
		}
	}

	// We build the environment variable list in a stable order for testability
	// For the values that come from connections we back them with secretData. We'll extract the values
	// and return them.
	env, secretData := getEnvVarsAndSecretData(resource, dependencies)

	for k, v := range cc.Container.Env {
		env[k] = corev1.EnvVar{Name: k, Value: v}
	}

	// Append in sorted order
	for _, key := range getSortedKeys(env) {
		container.Env = append(container.Env, env[key])
	}

	outputResources := []outputresource.OutputResource{}

	podLabels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name)

	// Add volumes
	volumes := []corev1.Volume{}
	for volumeName, volume := range cc.Container.Volumes {
		// Based on the kind, create a persistent/ephemeral volume
		switch volume.Kind {
		case datamodel.Ephemeral:
			volumeSpec, volumeMountSpec, err := r.makeEphemeralVolume(volumeName, volume.Ephemeral)
			if err != nil {
				return outputresource.OutputResource{}, []outputresource.OutputResource{}, nil, fmt.Errorf("unable to create ephemeral volume spec for volume: %s - %w", volumeName, err)
			}
			// Add the volume mount to the Container spec
			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
			// Add the volume to the list of volumes to be added to the Volumes spec
			volumes = append(volumes, volumeSpec)
		case datamodel.Persistent:
			var volumeSpec corev1.Volume
			var volumeMountSpec corev1.VolumeMount
			properties := dependencies[volume.Persistent.Source]

			switch properties.Definition["kind"] {
			case volumev1alpha3.PersistentVolumeKindAzureFileShare:
				// Create spec for persistent volume
				volumeSpec, volumeMountSpec, err = r.makeAzureFileSharePersistentVolume(volumeName, volume.Persistent, resource.Name, options)
				if err != nil {
					return outputresource.OutputResource{}, []outputresource.OutputResource{}, nil, fmt.Errorf("unable to create persistent volume spec for volume: %s - %w", volumeName, err)
				}
			case volumev1alpha3.PersistentVolumeKindAzureKeyVault:
				// Make Managed Identity
				managedIdentity := r.makeManagedIdentity(ctx, resource, applicationName)
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
					return outputresource.OutputResource{}, []outputresource.OutputResource{}, nil, fmt.Errorf("unable to create role assignments for volume: %s - %w", volumeName, err)
				}
				outputResources = append(outputResources, roleAssignments...)

				// Make Pod Identity
				podIdentity := r.makePodIdentity(ctx, resource, applicationName, []outputresource.OutputResource{})
				outputResources = append(outputResources, podIdentity)
				// We need to add labels for pod identity created to the pod metadata
				podIDLabel := kubernetes.MakeAADPodIdentityBindingLabels(podIdentity.Resource.(map[string]string)[handlers.PodIdentityNameKey])
				podLabels = labels.Merge(podIDLabel, podLabels)

				secretProviderClass := properties.OutputResources[outputresource.LocalIDSecretProviderClass]
				secretProviderClassName := secretProviderClass.Data.(resourcemodel.KubernetesIdentity).Name
				// Create spec for secret store
				volumeSpec, volumeMountSpec, err = r.makeAzureKeyVaultPersistentVolume(volumeName, volume.Persistent, secretProviderClassName, options)
				if err != nil {
					return outputresource.OutputResource{}, []outputresource.OutputResource{}, nil, fmt.Errorf("unable to create secretstore volume spec for volume: %s - %w", volumeName, err)
				}
			default:
				return outputresource.OutputResource{}, []outputresource.OutputResource{}, nil, fmt.Errorf("Unsupported volume kind: %s for volume: %s. Supported kinds are: %v", properties.Definition["kind"], volumeName, volumev1alpha3.GetSupportedKinds())
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
					r, err := resources.Parse(id)
					if err != nil {
						return outputresource.OutputResource{}, []outputresource.OutputResource{}, nil, err
					}
					value = r.Name()
				}
				secretData[key] = []byte(value.(string))
			}
		default:
			return outputresource.OutputResource{}, []outputresource.OutputResource{}, secretData, fmt.Errorf("Only ephemeral or persistent volumes are supported. Got kind: %v", volume.Kind)
		}
	}

	// In addition to the descriptive labels, we need to attach labels for each route
	// so that the generated services can find these pods
	for _, routeInfo := range routes {
		routeLabels := kubernetes.MakeRouteSelectorLabels(applicationName, routeInfo.Type, routeInfo.Name)
		podLabels = labels.Merge(routeLabels, podLabels)
	}

	deployment := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(applicationName, resource.Name),
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(applicationName, resource.Name),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: kubernetes.MakeSelectorLabels(applicationName, resource.Name),
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

	deploymentOutput := outputresource.NewKubernetesOutputResource(resourcekinds.Deployment, outputresource.LocalIDDeployment, &deployment, deployment.ObjectMeta)
	return deploymentOutput, outputResources, secretData, nil
}

func getEnvVarsAndSecretData(resource datamodel.ContainerResource, dependencies map[string]renderers.RendererDependency) (map[string]corev1.EnvVar, map[string][]byte) {
	env := map[string]corev1.EnvVar{}
	secretData := map[string][]byte{}
	cc := resource.Properties

	// Take each connection and create environment variables for each part
	// We'll store each value in a secret named with the same name as the resource.
	// We'll use the environment variable names as keys.
	// Float is used by the JSON serializer
	for name, con := range cc.Connections {
		properties := dependencies[con.Source]
		for key, value := range properties.ComputedValues {
			name := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), strings.ToUpper(key))

			source := corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: resource.Name,
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

	return env, secretData
}

func (r Renderer) makeEphemeralVolume(volumeName string, volume *datamodel.EphemeralVolume) (corev1.Volume, corev1.VolumeMount, error) {
	// Make volume spec
	volumeSpec := corev1.Volume{}
	volumeSpec.Name = volumeName
	volumeSpec.VolumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{}
	if volume != nil && volume.ManagedStore == datamodel.ManagedStoreMemory {
		volumeSpec.VolumeSource.EmptyDir.Medium = corev1.StorageMediumMemory
	} else {
		volumeSpec.VolumeSource.EmptyDir.Medium = corev1.StorageMediumDefault
	}

	// Make volumeMount spec
	volumeMountSpec := corev1.VolumeMount{}
	volumeMountSpec.MountPath = volume.MountPath
	volumeMountSpec.Name = volumeName

	return volumeSpec, volumeMountSpec, nil
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
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	default:
		return nil, fmt.Errorf("health probe kind unsupported: %v", p.Kind)
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

func (r Renderer) makeAzureFileSharePersistentVolume(volumeName string, persistentVolume *datamodel.PersistentVolume, applicationName string, options renderers.RenderOptions) (corev1.Volume, corev1.VolumeMount, error) {
	// Make volume spec
	volumeSpec := corev1.Volume{}
	volumeSpec.Name = volumeName
	volumeSpec.VolumeSource.AzureFile = &corev1.AzureFileVolumeSource{}
	volumeSpec.AzureFile.SecretName = applicationName
	resourceID, err := resources.Parse(persistentVolume.Source)
	if err != nil {
		return corev1.Volume{}, corev1.VolumeMount{}, err
	}
	shareName := resourceID.TypeSegments()[2].Name
	volumeSpec.AzureFile.ShareName = shareName
	// Make volumeMount spec
	volumeMountSpec := corev1.VolumeMount{}
	volumeMountSpec.Name = volumeName
	if persistentVolume != nil && persistentVolume.Rbac == datamodel.VolumeRbacRead {
		volumeMountSpec.MountPath = persistentVolume.MountPath
		volumeMountSpec.ReadOnly = true
	}
	return volumeSpec, volumeMountSpec, nil
}

func (r Renderer) makeAzureKeyVaultPersistentVolume(volumeName string, keyvaultVolume *datamodel.PersistentVolume, secretProviderClassName string, options renderers.RenderOptions) (corev1.Volume, corev1.VolumeMount, error) {
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
		MountPath: keyvaultVolume.MountPath,
		// We will support only reads to the secret store volume
		ReadOnly: true,
	}
	return volumeSpec, volumeMountSpec, nil
}

func (r Renderer) makeSecret(ctx context.Context, resource datamodel.ContainerResource, applicationName string, secrets map[string][]byte, options renderers.RenderOptions) outputresource.OutputResource {
	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resource.Name,
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(applicationName, resource.Name),
		},
		Type: corev1.SecretTypeOpaque,
		Data: secrets,
	}

	// Skip registration of the secret resource with the HealthService since health as a concept is not quite applicable to it
	output := outputresource.NewKubernetesOutputResource(resourcekinds.Secret, outputresource.LocalIDSecret, &secret, secret.ObjectMeta)
	return output
}

func (r Renderer) isIdentitySupported(kind datamodel.IAMKind) bool {
	if r.RoleAssignmentMap == nil || !kind.IsValid() {
		return false
	}

	_, ok := r.RoleAssignmentMap[kind]
	return ok
}

// Builds a user-assigned managed identity output resource.
func (r Renderer) makeManagedIdentity(ctx context.Context, resource datamodel.ContainerResource, applicationName string) outputresource.OutputResource {
	managedIdentityName := applicationName + "-" + resource.Name + "-msi"
	identityOutputResource := outputresource.OutputResource{
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureUserAssignedManagedIdentity,
			Provider: providers.ProviderAzure,
		},
		LocalID:  outputresource.LocalIDUserAssignedManagedIdentity,
		Deployed: false,
		Resource: map[string]string{
			handlers.UserAssignedIdentityNameKey: managedIdentityName,
		},
	}

	return identityOutputResource
}

// Builds an AKS pod-identity output resource.
func (r Renderer) makePodIdentity(ctx context.Context, resource datamodel.ContainerResource, applicationName string, roles []outputresource.OutputResource) outputresource.OutputResource {

	// Note: Pod Identity name cannot have camel case
	podIdentityName := fmt.Sprintf("podid-%s-%s", strings.ToLower(applicationName), strings.ToLower(resource.Name))

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
		LocalID: outputresource.LocalIDAADPodIdentity,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzurePodIdentity,
			Provider: providers.ProviderAzureKubernetesService,
		},
		Deployed: false,
		Resource: map[string]string{
			handlers.PodIdentityNameKey: podIdentityName,
			handlers.PodNamespaceKey:    applicationName,
		},
		Dependencies: dependencies,
	}

	return outputResource
}

// Assigns roles/permissions to a specific resource for the managed identity resource.
func (r Renderer) makeRoleAssignmentsForResource(ctx context.Context, connection *datamodel.ConnectionProperties, dependencies map[string]renderers.RendererDependency) ([]outputresource.OutputResource, error) {
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
			return nil, fmt.Errorf("RBAC is not supported for connection kind %q", connection.IAM.Kind)
		}

		// The dependency will have already been fetched by the system.
		dependency, ok := dependencies[connection.Source]
		if !ok {
			return nil, fmt.Errorf("connection source %q was not found in the dependencies collection", connection.Source)
		}

		// Find the matching output resource based on LocalID
		target, ok := dependency.OutputResources[roleAssignmentData.LocalID]
		if !ok {
			return nil, fmt.Errorf("output resource %q was not found in the outputs of dependency %q", roleAssignmentData.LocalID, connection.Source)
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
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: providers.ProviderAzure,
			},
			LocalID:  localID,
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         roleName,
				handlers.RoleAssignmentScope: armResourceIdentifier,
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
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: providers.ProviderAzure,
			},
			LocalID:  localID,
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         roleName,
				handlers.RoleAssignmentScope: keyVaultID,
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
