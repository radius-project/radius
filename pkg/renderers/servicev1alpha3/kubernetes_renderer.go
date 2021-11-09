// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicev1alpha3

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/Azure/radius/pkg/renderers/iam"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
)

// Renderer is the WorkloadRenderer implementation for containerized workload.
type KubernetesRenderer struct {
	IAM iam.RoleAssignmentProvider
}

func (r KubernetesRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	properties, err := convert(resource)
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

	for _, port := range properties.Ports {
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
func (r KubernetesRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	outputResources := []outputresource.OutputResource{}

	properties, err := convert(options.Resource)
	if err != nil {
		return renderers.RendererOutput{Resources: outputResources}, err
	}

	if properties.Run["kind"].(string) == "executable" {
		return renderers.RendererOutput{}, errors.New("container is required right now")
	}

	b, err := json.Marshal(&properties.Run)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	container := Container{}
	err = json.Unmarshal(b, &container)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Create the deployment as the primary workload
	deployment, secretData, err := r.makeDeployment(ctx, options.Resource, properties, container, options.Dependencies)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, deployment)

	// If there are secrets we'll use a Kubernetes secret to hold them. This is already referenced
	// by the deployment.
	if len(secretData) > 0 {
		outputResources = append(outputResources, r.makeSecret(ctx, options.Resource, secretData))
	}

	// Connections might require a role assignment to grant access.
	roles := []outputresource.OutputResource{}
	for _, connection := range properties.Connections {
		if !r.IAM.IsIdentitySupported(connection.Kind) {
			continue
		}

		more, err := r.makeRoleAssignmentsForResource(ctx, options.Resource, connection, options.Dependencies)
		if err != nil {
			return renderers.RendererOutput{}, nil
		}

		roles = append(roles, more...)
	}

	// If we created role assigmments then we will need an identity and the mapping of the identity to AKS.
	if len(roles) > 0 {
		outputResources = append(outputResources, roles...)
		outputResources = append(outputResources, r.makeManagedIdentity(ctx, options.Resource))
		outputResources = append(outputResources, r.makePodIdentity(ctx, options.Resource, roles))
	}

	return renderers.RendererOutput{Resources: outputResources}, nil
}

func (r KubernetesRenderer) makeDeployment(
	ctx context.Context,
	resource renderers.RendererResource,
	properties *ServiceProperties,
	container Container,
	dependencies map[string]renderers.RendererDependency) (outputresource.OutputResource, map[string][]byte, error) {
	// Keep track of the set of routes, we will need these to generate labels later
	routes := []struct {
		Name string
		Type string
	}{}

	// For a containerized workload we can statically allocate the ports while doing rendering.
	nextPort := 49152
	portEnvVars := map[string]string{}

	ports := []corev1.ContainerPort{}
	for name, port := range properties.Ports {
		if port.Dynamic {
			chosenPort := nextPort
			port.Port = &chosenPort
			if len(properties.Ports) == 1 {
				portEnvVars["PORT"] = fmt.Sprintf("%v", chosenPort)
			}

			portEnvVars[fmt.Sprintf("%s_PORT", strings.ToUpper(name))] = fmt.Sprintf("%v", chosenPort)

			nextPort++
		}

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
				ContainerPort: int32(*port.Port),
				Protocol:      corev1.ProtocolTCP,
			})
		} else {
			ports = append(ports, corev1.ContainerPort{
				ContainerPort: int32(*port.Port),
				Protocol:      corev1.ProtocolTCP,
			})
		}

	}

	k8sContainer := corev1.Container{
		Name:  resource.ResourceName,
		Image: container.Image,
		// TODO: use better policies than this when we have a good versioning story
		ImagePullPolicy: corev1.PullPolicy("Always"),
		Ports:           ports,
		Env:             []corev1.EnvVar{},
		VolumeMounts:    []corev1.VolumeMount{},
	}

	var err error
	if properties.ReadinessProbe != nil {
		k8sContainer.ReadinessProbe, err = r.makeHealthProbe(properties.ReadinessProbe)
		if err != nil {
			return outputresource.OutputResource{}, nil, fmt.Errorf("readiness probe encountered errors: %w ", err)
		}
	}
	if properties.LivenessProbe != nil {
		k8sContainer.LivenessProbe, err = r.makeHealthProbe(properties.LivenessProbe)
		if err != nil {
			return outputresource.OutputResource{}, nil, fmt.Errorf("liveness probe encountered errors: %w ", err)
		}
	}

	// We build the environment variable list in a stable order for testability
	env := map[string]corev1.EnvVar{}
	for k, v := range portEnvVars {
		env[k] = corev1.EnvVar{
			Name:  k,
			Value: v,
		}
	}

	// For the values that come from connections we back them with secretData. We'll extract the values
	// and return them.
	secretData := map[string][]byte{}

	// Take each connection and create environment variables for each part
	for name, con := range properties.Connections {
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

	for k, v := range properties.Env {
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
	for _, key := range getSortedEnvKeys(env) {
		k8sContainer.Env = append(k8sContainer.Env, env[key])
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
					Containers: []corev1.Container{k8sContainer},
				},
			},
		},
	}

	output := outputresource.NewKubernetesOutputResource(outputresource.LocalIDDeployment, &deployment, deployment.ObjectMeta)
	return output, secretData, nil
}

func (r KubernetesRenderer) makeHealthProbe(healthProbe map[string]interface{}) (*corev1.Probe, error) {
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

func (r KubernetesRenderer) setContainerHealthProbeConfig(probeSpec *corev1.Probe, config containerHealthProbeConfig) {
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

func (r KubernetesRenderer) makeSecret(ctx context.Context, resource renderers.RendererResource, secrets map[string][]byte) outputresource.OutputResource {
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

// Builds a user-assigned managed identity output resource.
func (r KubernetesRenderer) makeManagedIdentity(ctx context.Context, resource renderers.RendererResource) outputresource.OutputResource {
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
func (r KubernetesRenderer) makePodIdentity(ctx context.Context, resource renderers.RendererResource, roles []outputresource.OutputResource) outputresource.OutputResource {

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
func (r KubernetesRenderer) makeRoleAssignmentsForResource(ctx context.Context, resource renderers.RendererResource, connection Connection, dependencies map[string]renderers.RendererDependency) ([]outputresource.OutputResource, error) {
	// We're reporting errors in this code path to avoid obscuring a bug in another layer of the system.
	// None of these error conditions should be caused by invalid user input. They should only be caused
	// by internal bugs in Radius.
	roleAssignmentData, ok := r.IAM.RoleAssignmentMap[connection.Kind]
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

func getSortedEnvKeys(env map[string]corev1.EnvVar) []string {
	keys := []string{}
	for k := range env {
		key := k
		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys
}
