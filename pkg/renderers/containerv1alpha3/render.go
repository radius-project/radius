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

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
)

// Volume constants
const (
	VolumeKindEphemeral  = "ephemeral"
	VolumeKindPersistent = "persistent"
	ManagedStoreDisk     = "disk"
	ManagedStoreMemory   = "memory"
	RbacPermissionsRead  = "read"
	RbacPermissionsWrite = "write"
	StorageAccountName   = "storageAccount"
)

// Liveness/Readiness constants
const (
	DefaultInitialDelaySeconds = 0
	DefaultFailureThreshold    = 3
	DefaultPeriodSeconds       = 10
	HTTPGet                    = "httpGet"
	TCP                        = "tcp"
	Exec                       = "exec"
)

// Renderer is the WorkloadRenderer implementation for containerized workload.
type Renderer struct {

	// RoleAssignmentMap is an optional map of connection kind -> []Role Assignment. Used to configure managed
	// identity permissions for cloud resources. This will be nil in environments that don't support role assignments.
	RoleAssignmentMap map[string]RoleAssignmentData
}

func (r Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	properties, err := r.convert(resource)
	if err != nil {
		return nil, err
	}

	// Right now we only have things in connections and ports as rendering dependencies - we'll add more things
	// in the future... eg: volumes
	//
	// Anywhere we accept a resource ID in the model should have its value returned from here
	deps := []azresources.ResourceID{}
	for _, connection := range properties.Connections {
		resourceId, err := azresources.Parse(connection.Source)
		if err != nil {
			return nil, err
		}
		deps = append(deps, resourceId)
	}

	for _, port := range properties.Container.Ports {
		if port.Provides == "" {
			continue
		}

		resourceId, err := azresources.Parse(port.Provides)
		if err != nil {
			return nil, err
		}
		deps = append(deps, resourceId)
	}

	for _, volume := range properties.Container.Volumes {
		if volume[kindProperty] == VolumeKindPersistent {
			persistentVolume, err := asPersistentVolume(volume)
			if err != nil {
				return nil, err
			}
			resourceID, err := azresources.Parse(persistentVolume.Source)
			if err != nil {
				return nil, err
			}
			deps = append(deps, resourceID)
		} else {
			continue
		}
	}

	return deps, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	outputResources := []outputresource.OutputResource{}

	cw, err := r.convert(resource)
	if err != nil {
		return renderers.RendererOutput{Resources: outputResources}, err
	}

	// Create the deployment as the primary workload
	deployment, secretData, err := r.makeDeployment(ctx, resource, dependencies, cw)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, deployment)

	// If there are secrets we'll use a Kubernetes secret to hold them. This is already referenced
	// by the deployment.
	if len(secretData) > 0 {
		outputResources = append(outputResources, r.makeSecret(ctx, resource, secretData))
	}

	// Connections might require a role assignment to grant access.
	roles := []outputresource.OutputResource{}
	for _, connection := range cw.Connections {
		if !r.isIdentitySupported(connection) {
			continue
		}

		more, err := r.makeRoleAssignmentsForResource(ctx, resource, connection, dependencies)
		if err != nil {
			return renderers.RendererOutput{}, nil
		}

		roles = append(roles, more...)
	}

	// If we created role assigmments then we will need an identity and the mapping of the identity to AKS.
	if len(roles) > 0 {
		outputResources = append(outputResources, roles...)
		outputResources = append(outputResources, r.makeManagedIdentity(ctx, resource))
		outputResources = append(outputResources, r.makePodIdentity(ctx, resource, roles))
	}

	return renderers.RendererOutput{Resources: outputResources}, nil
}

func (r Renderer) convert(resource renderers.RendererResource) (*ContainerProperties, error) {
	properties := &ContainerProperties{}
	err := resource.ConvertDefinition(properties)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (r Renderer) makeDeployment(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency, cc *ContainerProperties) (outputresource.OutputResource, map[string][]byte, error) {
	// Keep track of the set of routes, we will need these to generate labels later
	routes := []struct {
		Name string
		Type string
	}{}

	ports := []corev1.ContainerPort{}
	for _, port := range cc.Container.Ports {
		if port.Provides != "" {
			resourceId, err := azresources.Parse(port.Provides)
			if err != nil {
				return outputresource.OutputResource{}, nil, err
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
				ContainerPort: int32(*port.ContainerPort),
				Protocol:      corev1.ProtocolTCP,
			})
		} else {
			ports = append(ports, corev1.ContainerPort{
				ContainerPort: int32(*port.ContainerPort),
				Protocol:      corev1.ProtocolTCP,
			})
		}

	}

	container := corev1.Container{
		Name:  resource.ResourceName,
		Image: cc.Container.Image,
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
			return outputresource.OutputResource{}, nil, fmt.Errorf("readiness probe encountered errors: %w ", err)
		}
	}
	if cc.Container.LivenessProbe != nil {
		container.LivenessProbe, err = r.makeHealthProbe(cc.Container.LivenessProbe)
		if err != nil {
			return outputresource.OutputResource{}, nil, fmt.Errorf("liveness probe encountered errors: %w ", err)
		}
	}

	// We build the environment variable list in a stable order for testability
	env := map[string]corev1.EnvVar{}

	// For the values that come from connections we back them with secretData. We'll extract the values
	// and return them.
	secretData := map[string][]byte{}

	// Take each connection and create environment variables for each part
	for name, con := range cc.Connections {
		properties := dependencies[con.Source]
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
		switch val := v.(type) {
		case string:
			env[k] = corev1.EnvVar{Name: k, Value: val}
		case float64: // Float is used by the JSON serializer
			env[k] = corev1.EnvVar{Name: k, Value: strconv.Itoa(int(val))}
		case int:
			env[k] = corev1.EnvVar{Name: k, Value: strconv.Itoa(val)}
		}
	}

	// Append in sorted order
	for _, key := range getSortedKeys(env) {
		container.Env = append(container.Env, env[key])
	}

	// Add volumes
	volumes := []corev1.Volume{}
	for volumeName, volume := range cc.Container.Volumes {
		// Based on the kind, create a persistent/ephemeral volume
		if volume[kindProperty] == VolumeKindEphemeral {
			volumeSpec, volumeMountSpec, err := r.makeEphemeralVolume(volumeName, volume)
			if err != nil {
				return outputresource.OutputResource{}, nil, fmt.Errorf("unable to create ephemeral volume spec for volume: %s - %w", volumeName, err)
			}
			// Add the volume mount to the Container spec
			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
			// Add the volume to the list of volumes to be added to the Volumes spec
			volumes = append(volumes, volumeSpec)
		} else if volume[kindProperty] == VolumeKindPersistent {
			persistentVolume, err := asPersistentVolume(volume)
			if err != nil {
				return outputresource.OutputResource{}, nil, fmt.Errorf("unable to deserialize properties for persistent volume: %s - %w", volumeName, err)
			}

			// Create spec for persistent volume
			volumeSpec, volumeMountSpec, err := r.makePersistentVolume(volumeName, *persistentVolume, resource, dependencies)
			if err != nil {
				return outputresource.OutputResource{}, nil, fmt.Errorf("unable to create persistent volume spec for volume: %s - %w", volumeName, err)
			}
			// Add the volume mount to the Container spec
			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
			// Add the volume to the list of volumes to be added to the Volumes spec
			volumes = append(volumes, volumeSpec)
		} else if volume[kindProperty] == VolumeKindPersistent {
			persistentVolume, err := asPersistentVolume(volume)
			if err != nil {
				return outputresource.OutputResource{}, nil, fmt.Errorf("unable to deserialize properties for persistent volume: %s - %w", volumeName, err)
			}

			// Create spec for persistent volume
			volumeSpec, volumeMountSpec, err := r.makePersistentVolume(volumeName, *persistentVolume, resource, dependencies)
			if err != nil {
				return outputresource.OutputResource{}, nil, fmt.Errorf("unable to create persistent volume spec for volume: %s - %w", volumeName, err)
			}
			// Add the volume mount to the Container spec
			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
			// Add the volume to the list of volumes to be added to the Volumes spec
			volumes = append(volumes, volumeSpec)

			// Add azurestorageaccountname and azurestorageaccountkey as secrets
			// These will be added as key-value pairs to the kubernetes secret created for the container
			// The key values are as per: https://docs.microsoft.com/en-us/azure/aks/azure-files-volume
			properties := dependencies[persistentVolume.Source]
			for key, value := range properties.ComputedValues {
				secretData[key] = []byte(value.(string))
			}
		} else {
			return outputresource.OutputResource{}, secretData, fmt.Errorf("Only ephemeral or persistent volumes are supported. Got kind: %v", volume[kindProperty])
		}
	}

	// In addition to the descriptive labels, we need to attach labels for each route
	// so that the generated services can find these pods
	podLabels := kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName)
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
	return output, secretData, nil
}

func (r Renderer) makeEphemeralVolume(volumeName string, volume map[string]interface{}) (corev1.Volume, corev1.VolumeMount, error) {
	ephemeralVolume, err := asEphemeralVolume(volume)
	if err != nil {
		return corev1.Volume{}, corev1.VolumeMount{}, fmt.Errorf("unable to deserialize properties for ephemeral volume: %s - %w", volumeName, err)
	}
	// Make volume spec
	volumeSpec := corev1.Volume{}
	volumeSpec.Name = volumeName
	volumeSpec.VolumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{}
	if ephemeralVolume.ManagedStore == ManagedStoreMemory {
		volumeSpec.VolumeSource.EmptyDir.Medium = corev1.StorageMediumMemory
	} else {
		volumeSpec.VolumeSource.EmptyDir.Medium = corev1.StorageMediumDefault
	}

	// Make volumeMount spec
	volumeMountSpec := corev1.VolumeMount{}
	volumeMountSpec.MountPath = ephemeralVolume.MountPath
	volumeMountSpec.Name = volumeName

	return volumeSpec, volumeMountSpec, nil
}

func (r Renderer) makeHealthProbe(healthProbe map[string]interface{}) (*corev1.Probe, error) {
	probeSpec := corev1.Probe{}

	if strings.EqualFold(healthProbe[kindProperty].(string), HTTPGet) {
		// httpGet probe has been specified. Read the readiness probe properties as httpGet probe
		var httpGetProbe HTTPGetHealthProbe
		data, err := json.Marshal(healthProbe)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(data, &httpGetProbe)
		if err != nil {
			return nil, err
		}

		// Set the probe spec
		probeSpec.Handler.HTTPGet = &corev1.HTTPGetAction{}
		probeSpec.Handler.HTTPGet.Port = intstr.FromInt(httpGetProbe.Port)
		probeSpec.Handler.HTTPGet.Path = httpGetProbe.Path
		httpHeaders := []corev1.HTTPHeader{}
		for k, v := range httpGetProbe.Headers {
			httpHeaders = append(httpHeaders, corev1.HTTPHeader{
				Name:  k,
				Value: v,
			})
		}
		probeSpec.Handler.HTTPGet.HTTPHeaders = httpHeaders
		c := containerHealthProbeConfig{
			initialDelaySeconds: httpGetProbe.InitialDelaySeconds,
			failureThreshold:    httpGetProbe.FailureThreshold,
			periodSeconds:       httpGetProbe.PeriodSeconds,
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	} else if strings.EqualFold(healthProbe[kindProperty].(string), TCP) {
		// tcp probe has been specified. Read the readiness probe properties as tcp probe
		var tcpProbe TCPHealthProbe
		data, err := json.Marshal(healthProbe)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(data, &tcpProbe)
		if err != nil {
			return nil, err
		}

		// Set the probe spec
		probeSpec.Handler.TCPSocket = &corev1.TCPSocketAction{}
		probeSpec.TCPSocket.Port = intstr.FromInt(tcpProbe.Port)
		c := containerHealthProbeConfig{
			initialDelaySeconds: tcpProbe.InitialDelaySeconds,
			failureThreshold:    tcpProbe.FailureThreshold,
			periodSeconds:       tcpProbe.PeriodSeconds,
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	} else if strings.EqualFold(healthProbe[kindProperty].(string), Exec) {
		// exec probe has been specified. Read the readiness probe properties as exec probe
		var execProbe ExecHealthProbe
		data, err := json.Marshal(healthProbe)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(data, &execProbe)
		if err != nil {
			return nil, err
		}

		// Set the probe spec
		probeSpec.Handler.Exec = &corev1.ExecAction{}
		probeSpec.Exec.Command = strings.Split(execProbe.Command, " ")
		c := containerHealthProbeConfig{
			initialDelaySeconds: execProbe.InitialDelaySeconds,
			failureThreshold:    execProbe.FailureThreshold,
			periodSeconds:       execProbe.PeriodSeconds,
		}
		r.setContainerHealthProbeConfig(&probeSpec, c)
	}

	return &probeSpec, nil
}

type containerHealthProbeConfig struct {
	initialDelaySeconds *int
	failureThreshold    *int
	periodSeconds       *int
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

func (r Renderer) makePersistentVolume(volumeName string, persistentVolume PersistentVolume, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (corev1.Volume, corev1.VolumeMount, error) {
	// Make volume spec
	volumeSpec := corev1.Volume{}
	volumeSpec.Name = volumeName
	volumeSpec.VolumeSource.AzureFile = &corev1.AzureFileVolumeSource{}
	volumeSpec.AzureFile.SecretName = resource.ResourceName
	resourceID, err := azresources.Parse(persistentVolume.Source)
	if err != nil {
		return corev1.Volume{}, corev1.VolumeMount{}, err
	}
	shareName := resourceID.Types[2].Name
	volumeSpec.AzureFile.ShareName = shareName
	// Make volumeMount spec
	volumeMountSpec := corev1.VolumeMount{}
	volumeMountSpec.Name = volumeName
	volumeMountSpec.MountPath = persistentVolume.MountPath
	if strings.EqualFold(persistentVolume.Rbac, RbacPermissionsRead) {
		volumeMountSpec.ReadOnly = true
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

	output := outputresource.NewKubernetesOutputResource(outputresource.LocalIDSecret, &secret, secret.ObjectMeta)
	return output
}

func (r Renderer) isIdentitySupported(connection ContainerConnection) bool {
	if r.RoleAssignmentMap == nil {
		return false
	}

	_, ok := r.RoleAssignmentMap[connection.Kind]
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
func (r Renderer) makeRoleAssignmentsForResource(ctx context.Context, resource renderers.RendererResource, connection ContainerConnection, dependencies map[string]renderers.RendererDependency) ([]outputresource.OutputResource, error) {
	// We're reporting errors in this code path to avoid obscuring a bug in another layer of the system.
	// None of these error conditions should be caused by invalid user input. They should only be caused
	// by internal bugs in Radius.
	roleAssignmentData, ok := r.RoleAssignmentMap[connection.Kind]
	if !ok {
		return nil, fmt.Errorf("connection kind %q does not support managed identity", connection.Kind)
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

	outputResources := []outputresource.OutputResource{}
	for _, roleName := range roleAssignmentData.RoleNames {
		localID := outputresource.GenerateLocalIDForRoleAssignment(arm.ID, roleName)
		roleAssignment := outputresource.OutputResource{
			ResourceKind: resourcekinds.AzureRoleAssignment,
			LocalID:      localID,
			Managed:      true,
			Deployed:     false,
			Resource: map[string]string{
				handlers.RoleNameKey:             roleName,
				handlers.RoleAssignmentTargetKey: arm.ID,
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
