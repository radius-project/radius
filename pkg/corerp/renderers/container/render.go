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
	"crypto/sha1"
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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	azrenderer "github.com/project-radius/radius/pkg/corerp/renderers/container/azure"
	azvolrenderer "github.com/project-radius/radius/pkg/corerp/renderers/volume/azure"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
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

	defaultServiceAccountName = "default"
)

// GetSupportedKinds returns a list of supported volume kinds
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
		resourceID, err := resources.ParseResource(connection.Source)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
		}

		// Non-radius Azure connections that are accessible from Radius container resource.
		if connection.IAM.Kind.IsKind(datamodel.KindAzure) {
			azureResourceIDs = append(azureResourceIDs, resourceID)
			continue
		}

		if resourceID.IsRadiusRPResource() {
			radiusResourceIDs = append(radiusResourceIDs, resourceID)
			continue
		}
	}

	for _, port := range properties.Container.Ports {
		provides := port.Provides
		if provides == "" {
			continue
		}

		resourceID, err := resources.ParseResource(provides)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
		}

		if resourceID.IsRadiusRPResource() {
			radiusResourceIDs = append(radiusResourceIDs, resourceID)
			continue
		}
	}

	for _, volume := range properties.Container.Volumes {
		switch volume.Kind {
		case datamodel.Persistent:
			resourceID, err := resources.ParseResource(volume.Persistent.Source)
			if err != nil {
				return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
			}

			if resourceID.IsRadiusRPResource() {
				radiusResourceIDs = append(radiusResourceIDs, resourceID)
				continue
			}
		}
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	appId, err := resources.ParseResource(resource.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid application id: %s ", err.Error()))
	}

	outputResources := []rpv1.OutputResource{}
	dependencies := options.Dependencies

	// Connections might require a role assignment to grant access.
	roles := []rpv1.OutputResource{}
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

	if len(roles) > 0 {
		outputResources = append(outputResources, roles...)
	}

	computedValues := map[string]rpv1.ComputedValueReference{}

	// Create the deployment as the primary workload
	deploymentResources, secretData, err := r.makeDeployment(ctx, appId.Name(), options, computedValues, resource, roles)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, deploymentResources...)

	// If there are secrets we'll use a Kubernetes secret to hold them. This is already referenced
	// by the deployment.
	if len(secretData) > 0 {
		outputResources = append(outputResources, r.makeSecret(ctx, *resource, appId.Name(), secretData, options))
	}

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
	}, nil
}

func (r Renderer) makeDeployment(ctx context.Context, applicationName string, options renderers.RenderOptions, computedValues map[string]rpv1.ComputedValueReference, resource *datamodel.ContainerResource, roles []rpv1.OutputResource) ([]rpv1.OutputResource, map[string][]byte, error) {
	// Keep track of the set of routes, we will need these to generate labels later
	routes := []struct {
		Name string
		Type string
	}{}

	identityRequired := len(roles) > 0

	dependencies := options.Dependencies
	cc := resource.Properties

	ports := []corev1.ContainerPort{}
	for _, port := range cc.Container.Ports {
		if provides := port.Provides; provides != "" {
			resourceId, err := resources.ParseResource(provides)
			if err != nil {
				return []rpv1.OutputResource{}, nil, v1.NewClientErrInvalidRequest(err.Error())
			}

			routeName := kubernetes.NormalizeResourceName(resourceId.Name())
			routeType := resourceId.TypeSegments()[len(resourceId.TypeSegments())-1].Type
			routeTypeParts := strings.Split(routeType, "/")

			routeTypeSuffix := kubernetes.NormalizeResourceName(routeTypeParts[len(routeTypeParts)-1])

			routes = append(routes, struct {
				Name string
				Type string
			}{Name: routeName, Type: routeTypeSuffix})

			ports = append(ports, corev1.ContainerPort{
				// Name generation logic has to match the code in HttpRoute
				Name:          kubernetes.GetShortenedTargetPortName(routeTypeSuffix + routeName),
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
		Name:  kubernetes.NormalizeResourceName(resource.Name),
		Image: cc.Container.Image,
		// TODO: use better policies than this when we have a good versioning story
		ImagePullPolicy: corev1.PullPolicy("Always"),
		Ports:           ports,
		Env:             []corev1.EnvVar{},
		VolumeMounts:    []corev1.VolumeMount{},
		Command:         cc.Container.Command,
		Args:            cc.Container.Args,
		WorkingDir:      cc.Container.WorkingDir,
	}

	var err error
	if !cc.Container.ReadinessProbe.IsEmpty() {
		container.ReadinessProbe, err = r.makeHealthProbe(cc.Container.ReadinessProbe)
		if err != nil {
			return []rpv1.OutputResource{}, nil, fmt.Errorf("readiness probe encountered errors: %w ", err)
		}
	}
	if !cc.Container.LivenessProbe.IsEmpty() {
		container.LivenessProbe, err = r.makeHealthProbe(cc.Container.LivenessProbe)
		if err != nil {
			return []rpv1.OutputResource{}, nil, fmt.Errorf("liveness probe encountered errors: %w ", err)
		}
	}

	// We build the environment variable list in a stable order for testability
	// For the values that come from connections we back them with secretData. We'll extract the values
	// and return them.
	env, secretData := getEnvVarsAndSecretData(resource, applicationName, dependencies)

	for k, v := range cc.Container.Env {
		env[k] = corev1.EnvVar{Name: k, Value: v}
	}

	// Append in sorted order
	for _, key := range getSortedKeys(env) {
		container.Env = append(container.Env, env[key])
	}

	outputResources := []rpv1.OutputResource{}

	deps := []rpv1.Dependency{}

	podLabels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())

	// This is the default service account name. If a volume is associated with federated identity, new service account
	// will be created and set for container pods.
	serviceAccountName := defaultServiceAccountName

	// Add volumes
	volumes := []corev1.Volume{}

	// Create Kubernetes resource name scoped in Kubernetes namespace
	kubeIdentityName := kubernetes.NormalizeResourceName(resource.Name)

	// Create Azure resource name for managed/federated identity-scoped in resource group specified by Environment resource.
	// To avoid the naming conflicts, we add the application name prefix to resource name.
	azIdentityName := azrenderer.MakeResourceName(applicationName, resource.Name, azrenderer.Separator)

	for volumeName, volumeProperties := range cc.Container.Volumes {
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
				deps = append(deps, rpv1.Dependency{LocalID: rpv1.LocalIDSecretProviderClass})

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
					id := properties.OutputResources[value.(string)].Data.(resourcemodel.ARMIdentity).ID
					r, err := resources.ParseResource(id)
					if err != nil {
						return []rpv1.OutputResource{}, nil, v1.NewClientErrInvalidRequest(err.Error())
					}
					value = r.Name()
				}
				secretData[key] = []byte(value.(string))
			}
		default:
			return []rpv1.OutputResource{}, secretData, v1.NewClientErrInvalidRequest(fmt.Sprintf("Only ephemeral or persistent volumes are supported. Got kind: %v", volumeProperties.Kind))
		}
	}

	// In addition to the descriptive labels, we need to attach labels for each route
	// so that the generated services can find these pods
	for _, routeInfo := range routes {
		routeLabels := kubernetes.MakeRouteSelectorLabels(applicationName, routeInfo.Type, routeInfo.Name)
		podLabels = labels.Merge(routeLabels, podLabels)
	}

	// In order to enable per-container identity, it creates user-assigned managed identity, federated identity, and service account.
	if identityRequired {
		// 1. Create Per-Container managed identity.
		managedIdentity, err := azrenderer.MakeManagedIdentity(azIdentityName, options.Environment.CloudProviders)
		if err != nil {
			return []rpv1.OutputResource{}, nil, err
		}
		outputResources = append(outputResources, *managedIdentity)

		// 2. Create Per-container federated identity resource.
		serviceAccountName = kubeIdentityName
		fedIdentity, err := azrenderer.MakeFederatedIdentity(serviceAccountName, &options.Environment)
		if err != nil {
			return []rpv1.OutputResource{}, nil, err
		}
		outputResources = append(outputResources, *fedIdentity)

		// 3. Create Per-container service account.
		saAccount := azrenderer.MakeFederatedIdentitySA(applicationName, serviceAccountName, options.Environment.Namespace, resource)
		outputResources = append(outputResources, *saAccount)

		// This is required to enable workload identity.
		podLabels[azrenderer.AzureWorkloadIdentityUseKey] = "true"

		deps = append(deps, rpv1.Dependency{LocalID: rpv1.LocalIDServiceAccount})

		// 4. Add RBAC resources to the dependencies.
		for _, role := range roles {
			deps = append(deps, rpv1.Dependency{LocalID: role.LocalID})
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
	}

	// Create the role and role bindings for SA.
	role := makeRBACRole(applicationName, kubeIdentityName, options.Environment.Namespace, resource)
	outputResources = append(outputResources, *role)
	deps = append(deps, rpv1.Dependency{LocalID: rpv1.LocalIDKubernetesRole})

	roleBinding := makeRBACRoleBinding(applicationName, kubeIdentityName, serviceAccountName, options.Environment.Namespace, resource)
	outputResources = append(outputResources, *roleBinding)
	deps = append(deps, rpv1.Dependency{LocalID: rpv1.LocalIDKubernetesRoleBinding})

	deployment := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.NormalizeResourceName(resource.Name),
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName()),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: kubernetes.MakeSelectorLabels(applicationName, resource.Name),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					// See: https://github.com/kubernetes/kubernetes/issues/92226 and
					// 		https://github.com/project-radius/radius/issues/3002
					//
					// Service links are a flawed and Kubernetes-only feature that we don't
					// want to leak into Radius containers.
					EnableServiceLinks: to.Ptr(false),

					ServiceAccountName: serviceAccountName,
					Containers:         []corev1.Container{container},
					Volumes:            volumes,
				},
			},
		},
	}

	// If we have a secret to reference we need to ensure that the deployment will trigger a new revision
	// when the secret changes. Normally referencing an environment variable from a secret will **NOT** cause
	// a new revision when the secret changes.
	//
	// see: https://stackoverflow.com/questions/56711894/does-k8-update-environment-variables-when-secrets-change
	//
	// The solution to this is to embed the hash of the secret as an annotation in the deployment. This way when the
	// secret changes we also change the content of the deployment and thus trigger a new revision. This is a very
	// common solution to this problem, and not a bizzare workaround that we invented.
	if len(secretData) > 0 {
		hash := r.hashSecretData(secretData)
		deployment.Spec.Template.ObjectMeta.Annotations[kubernetes.AnnotationSecretHash] = hash
		deps = append(deps, rpv1.Dependency{
			LocalID: rpv1.LocalIDSecret,
		})
	}

	deploymentOutput := rpv1.NewKubernetesOutputResource(resourcekinds.Deployment, rpv1.LocalIDDeployment, &deployment, deployment.ObjectMeta)
	deploymentOutput.Dependencies = deps

	outputResources = append(outputResources, deploymentOutput)
	return outputResources, secretData, nil
}

func getEnvVarsAndSecretData(resource *datamodel.ContainerResource, applicationName string, dependencies map[string]renderers.RendererDependency) (map[string]corev1.EnvVar, map[string][]byte) {
	env := map[string]corev1.EnvVar{}
	secretData := map[string][]byte{}
	cc := resource.Properties

	// Take each connection and create environment variables for each part
	// We'll store each value in a secret named with the same name as the resource.
	// We'll use the environment variable names as keys.
	// Float is used by the JSON serializer
	for name, con := range cc.Connections {
		properties := dependencies[con.Source]
		if !con.GetDisableDefaultEnvVars() {
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

	return env, secretData
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

func (r Renderer) makeSecret(ctx context.Context, resource datamodel.ContainerResource, applicationName string, secrets map[string][]byte, options renderers.RenderOptions) rpv1.OutputResource {
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

	// Skip registration of the secret resource with the HealthService since health as a concept is not quite applicable to it
	output := rpv1.NewKubernetesOutputResource(resourcekinds.Secret, rpv1.LocalIDSecret, &secret, secret.ObjectMeta)
	return output
}

func (r Renderer) hashSecretData(secretData map[string][]byte) string {
	// Sort keys so we can hash deterministically
	keys := []string{}
	for k := range secretData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	hash := sha1.New()

	for _, k := range keys {
		// Using | as a delimiter
		_, _ = hash.Write([]byte("|" + k + "|"))
		_, _ = hash.Write(secretData[k])
	}

	sum := hash.Sum(nil)
	return fmt.Sprintf("%x", sum)
}

func (r Renderer) isIdentitySupported(kind datamodel.IAMKind) bool {
	if r.RoleAssignmentMap == nil || !kind.IsValid() {
		return false
	}

	_, ok := r.RoleAssignmentMap[kind]
	return ok
}

// Assigns roles/permissions to a specific resource for the managed identity resource.
func (r Renderer) makeRoleAssignmentsForResource(ctx context.Context, connection *datamodel.ConnectionProperties, dependencies map[string]renderers.RendererDependency) ([]rpv1.OutputResource, error) {
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

		// Now we know the resource ID to assign roles against.
		arm, ok := target.Data.(resourcemodel.ARMIdentity)
		if !ok {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("output resource %q must be an ARM resource to support role assignments. Was: %+v", roleAssignmentData.LocalID, target))
		}
		armResourceIdentifier = arm.ID

		roleNames = roleAssignmentData.RoleNames
	}

	outputResources := []rpv1.OutputResource{}
	for _, roleName := range roleNames {
		localID := rpv1.GenerateLocalIDForRoleAssignment(armResourceIdentifier, roleName)
		roleAssignment := rpv1.OutputResource{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: resourcemodel.ProviderAzure,
			},
			LocalID:  localID,
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         roleName,
				handlers.RoleAssignmentScope: armResourceIdentifier,
			},
			Dependencies: []rpv1.Dependency{
				{
					LocalID: rpv1.LocalIDUserAssignedManagedIdentity,
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
