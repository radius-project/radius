// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha3

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

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
				return outputresource.OutputResource{}, nil, err
			}
			// Add the volume mount to the Container spec
			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
			// Add the volume to the list of volumes to be added to the Volumes spec
			volumes = append(volumes, volumeSpec)
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
			Name:      resource.ResourceName,
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

func asEphemeralVolume(volume map[string]interface{}) (*EphemeralVolume, error) {
	data, err := json.Marshal(volume)
	if err != nil {
		return nil, err
	}
	var ephemeralVolume EphemeralVolume
	json.Unmarshal(data, &ephemeralVolume)
	return &ephemeralVolume, nil
}

func (r Renderer) makeEphemeralVolume(volumeName string, volume map[string]interface{}) (corev1.Volume, corev1.VolumeMount, error) {
	ephemeralVolume, err := asEphemeralVolume(volume)
	if err != nil {
		return corev1.Volume{}, corev1.VolumeMount{}, err
	}
	// Make volume spec
	volumeSpec := corev1.Volume{}
	volumeSpec.Name = volumeName
	volumeSpec.VolumeSource = corev1.VolumeSource{}
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
