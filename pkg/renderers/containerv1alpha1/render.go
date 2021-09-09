// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for containerized workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate allocates bindings for containerized workload
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	// For containers we only *natively* expose HTTP as a binding type - other binding types
	// might be handled by traits (decorators), so don't error out on those.
	//
	// The calling infrastructure will validate that any bindings specified by the user have a matching
	// binding in the outputs.
	bindings := map[string]components.BindingState{}

	for name, binding := range workload.Workload.Bindings {
		if binding.Kind != KindHTTP {
			continue
		}

		http := HTTPBinding{}
		err := binding.AsRequired(KindHTTP, &http)
		if err != nil {
			return nil, err
		}

		namespace := workload.Namespace
		if namespace == "" {
			namespace = workload.Application
		}

		uri := url.URL{
			Scheme: binding.Kind,
			Host:   fmt.Sprintf("%v.%v.svc.cluster.local", workload.Name, namespace),
		}

		if http.GetEffectivePort() != 80 {
			uri.Host = uri.Host + fmt.Sprintf(":%d", http.GetEffectivePort())
		}

		bindings[name] = components.BindingState{
			Component: workload.Name,
			Binding:   name,
			Kind:      KindHTTP,
			Properties: map[string]interface{}{
				"uri":    uri.String(),
				"scheme": uri.Scheme,
				"host":   uri.Hostname(),
				"port":   fmt.Sprintf("%d", http.GetEffectivePort()),
			},
		}
	}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, workload workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := &ContainerComponent{}
	err := workload.Workload.AsRequired(Kind, component)
	if err != nil {
		return nil, err
	}

	podIdentityName, outputResources, err := r.getPodIdentityAndDependencies(ctx, workload, component)
	if err != nil {
		return nil, err
	}

	deployment, deploymentOutputResources, err := r.makeDeployment(ctx, workload, component, podIdentityName)
	if err != nil {
		return nil, err
	}
	outputResources = append(outputResources, deploymentOutputResources...)

	serviceOutputResources, err := r.makeService(ctx, workload, component, deployment)
	if err != nil {
		return nil, err
	}
	outputResources = append(outputResources, serviceOutputResources...)

	return outputResources, nil
}

// Builds a user assigned managed identity output resource
func (r Renderer) getManagedIdentityOutput(ctx context.Context, applicationName string, componentName string, keyVaultName string) outputresource.OutputResource {

	managedIdentityName := keyVaultName + "-" + componentName + "-msi"
	identityOutputResource := outputresource.OutputResource{
		Type:     outputresource.TypeARM,
		Kind:     resourcekinds.AzureUserAssignedManagedIdentity,
		LocalID:  outputresource.LocalIDUserAssignedManagedIdentityKV,
		Deployed: false,
		Managed:  true,
		Resource: map[string]string{
			handlers.ManagedKey:                  "true",
			handlers.KeyVaultNameKey:             keyVaultName,
			handlers.UserAssignedIdentityNameKey: managedIdentityName,
		},
	}

	return identityOutputResource
}

func (r Renderer) getKeyVaultName(keyVaultBinding components.BindingState) (string, error) {
	value, ok := keyVaultBinding.Properties[handlers.KeyVaultURIKey]
	if !ok {
		return "", fmt.Errorf("failed to read keyvault uri from binding properties")
	}

	kvURI, ok := value.(string)
	if !ok || kvURI == "" {
		return "", fmt.Errorf("failed to read keyvault uri")
	}
	kvName := strings.Replace(strings.Split(kvURI, ".")[0], "https://", "", -1)

	return kvName, nil
}

// Builds output resources to assigns secrets user and cryto user roles to the managed identity for access to the keyvault
func (r Renderer) getRoleAssignmentOutputResources(ctx context.Context, keyVaultName string) []outputresource.OutputResource {
	// Role description: https://docs.microsoft.com/en-us/azure/key-vault/general/rbac-guide?tabs=azure-cli
	keyVaultSecretsReadRole := "Key Vault Secrets User"
	keyVaultCryptoOperationsRole := "Key Vault Crypto User"

	roleAssignmentDependencies := []outputresource.Dependency{
		{
			LocalID: outputresource.LocalIDUserAssignedManagedIdentityKV,
		},
	}
	readSecretsRAOutputResource := outputresource.OutputResource{
		Kind:     resourcekinds.AzureRoleAssignment,
		LocalID:  outputresource.LocalIDRoleAssignmentKVSecretsCerts,
		Managed:  true,
		Deployed: false,
		Type:     outputresource.TypeARM,
		Resource: map[string]string{
			handlers.ManagedKey:      "true",
			handlers.RoleNameKey:     keyVaultSecretsReadRole,
			handlers.KeyVaultNameKey: keyVaultName,
		},
		Dependencies: roleAssignmentDependencies,
	}
	outputResources := []outputresource.OutputResource{readSecretsRAOutputResource}

	cryptoOperationsRAOutputResource := outputresource.OutputResource{
		Kind:     resourcekinds.AzureRoleAssignment,
		LocalID:  outputresource.LocalIDRoleAssignmentKVKeys,
		Managed:  true,
		Deployed: false,
		Type:     outputresource.TypeARM,
		Resource: map[string]string{
			handlers.ManagedKey:      "true",
			handlers.RoleNameKey:     keyVaultCryptoOperationsRole,
			handlers.KeyVaultNameKey: keyVaultName,
		},
		Dependencies: roleAssignmentDependencies,
	}
	outputResources = append(outputResources, cryptoOperationsRAOutputResource)

	return outputResources
}

func (r Renderer) getPodIdentityAndDependencies(ctx context.Context, workload workloads.InstantiatedWorkload, component *ContainerComponent) (podIdentityName string, outputResources []outputresource.OutputResource, err error) {
	for _, dependency := range component.Uses {
		binding, err := dependency.Binding.GetMatchingBinding(workload.BindingValues)
		if err != nil {
			return "", nil, err
		}

		// If the container depends on a KeyVault, create a pod identity.
		// The list of dependency kinds to check might grow in the future
		if binding.Kind == "azure.com/KeyVault" {
			keyVaultName, err := r.getKeyVaultName(binding)
			if err != nil {
				return "", nil, err
			}

			// Create a user assigned managed identity
			identityOutputResource := r.getManagedIdentityOutput(ctx, workload.Application, workload.Name, keyVaultName)
			outputResources := []outputresource.OutputResource{identityOutputResource}

			// RBAC on managed identity to access the KeyVault
			roleAssignmentOutputResources := r.getRoleAssignmentOutputResources(ctx, keyVaultName)
			outputResources = append(outputResources, roleAssignmentOutputResources...)

			// Create pod identity
			podIdentityName, podIdentityOutputResource := r.getPodIdentityOutputResource(ctx, component.Name, workload.Application)
			outputResources = append(outputResources, podIdentityOutputResource)

			return podIdentityName, outputResources, nil
		}
	}

	return "", nil, nil
}

func (r Renderer) makeDeployment(ctx context.Context, workload workloads.InstantiatedWorkload, component *ContainerComponent, podIdentityName string) (*appsv1.Deployment, []outputresource.OutputResource, error) {
	outputResources := []outputresource.OutputResource{}
	deploymentDependencies := []outputresource.Dependency{}

	container := corev1.Container{
		Name:            component.Name,
		Image:           component.Run.Container.Image,
		ImagePullPolicy: corev1.PullPolicy("Always"), // https://github.com/Azure/radius/issues/734
		Env:             []corev1.EnvVar{},
	}

	for k, v := range component.Run.Container.Env {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	for _, dep := range component.Uses {
		// Set environment variables in the container
		for k, v := range dep.Env {
			str, err := v.EvaluateString(workload.BindingValues)
			if err != nil {
				return nil, nil, err
			}
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  k,
				Value: str,
			})
		}

		// Evaluate dependencies on keyvault secrets, generate output resource
		if dep.Secrets == nil {
			continue
		}

		deploymentDependencies = append(deploymentDependencies, outputresource.Dependency{LocalID: outputresource.LocalIDKeyVaultSecret})

		store, err := dep.Secrets.Store.GetMatchingBinding(workload.BindingValues)
		if err != nil {
			return nil, nil, err
		}

		keyVaultName, err := r.getKeyVaultName(store)
		if err != nil {
			return nil, nil, err
		}

		secrets := map[string]string{}
		for k, v := range dep.Secrets.Keys {
			value, err := v.EvaluateString(workload.BindingValues)
			if err != nil {
				return nil, nil, err
			}
			secrets[k] = value
		}

		// Create secrets in the specified keyvault
		for secretName, secretValue := range secrets {
			secretsOutputResource, err := r.getKeyVaultSecretOutputResource(ctx, keyVaultName, secretName, secretValue)
			if err != nil {
				return nil, nil, fmt.Errorf("could not create secret: %v: %w", secretName, err)
			}
			outputResources = append(outputResources, secretsOutputResource)
		}
	}

	for name, genericBinding := range workload.Workload.Bindings {
		if genericBinding.Kind == KindHTTP {
			httpBinding := HTTPBinding{}
			err := genericBinding.AsRequired(KindHTTP, &httpBinding)
			if err != nil {
				return nil, nil, err
			}

			port := corev1.ContainerPort{
				Name:          name,
				ContainerPort: int32(httpBinding.GetEffectiveContainerPort()),
			}

			port.Protocol = "TCP"
			container.Ports = append(container.Ports, port)
		}
	}

	deployment := appsv1.Deployment{
		TypeMeta: v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      component.Name,
			Namespace: workload.Application,
			Labels:    kubernetes.MakeDescriptiveLabels(workload.Application, workload.Name),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: kubernetes.MakeSelectorLabels(workload.Application, workload.Name),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: kubernetes.MakeDescriptiveLabels(workload.Application, workload.Name),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
				},
			},
		},
	}

	if podIdentityName != "" {
		// Add the aadpodidbinding label to the k8s spec for the container
		deployment.Spec.Template.ObjectMeta.Labels["aadpodidbinding"] = podIdentityName

		deploymentDependencies = append(deploymentDependencies, outputresource.Dependency{LocalID: outputresource.LocalIDAADPodIdentity})
	}

	deploymentOutputResource := outputresource.OutputResource{
		Kind:     resourcekinds.Kubernetes,
		LocalID:  outputresource.LocalIDDeployment,
		Deployed: false,
		Managed:  true,
		Type:     outputresource.TypeKubernetes,
		Info: outputresource.K8sInfo{
			Kind:       deployment.TypeMeta.Kind,
			APIVersion: deployment.TypeMeta.APIVersion,
			Name:       deployment.ObjectMeta.Name,
			Namespace:  deployment.ObjectMeta.Namespace,
		},
		Resource:     &deployment,
		Dependencies: deploymentDependencies,
	}
	outputResources = append(outputResources, deploymentOutputResource)

	return &deployment, outputResources, nil
}

func (r Renderer) getPodIdentityOutputResource(ctx context.Context, containerName string, podNamespace string) (string, outputresource.OutputResource) {
	// Note: Pod Identity name cannot have camel case
	podIdentityName := "podid-" + strings.ToLower(containerName)

	// Managed identity with required role assignments should be created first
	podIdentityDependencies := []outputresource.Dependency{
		{
			LocalID: outputresource.LocalIDUserAssignedManagedIdentityKV,
		},
		{
			LocalID: outputresource.LocalIDRoleAssignmentKVSecretsCerts,
		},
		{
			LocalID: outputresource.LocalIDRoleAssignmentKVKeys,
		},
	}

	outputResource := outputresource.OutputResource{
		LocalID:  outputresource.LocalIDAADPodIdentity,
		Type:     outputresource.TypeAADPodIdentity,
		Kind:     resourcekinds.AzurePodIdentity,
		Managed:  true,
		Deployed: false,
		Resource: map[string]string{
			handlers.ManagedKey:            "true",
			handlers.PodIdentityNameKey:    podIdentityName,
			handlers.PodIdentityClusterKey: r.Arm.K8sClusterName,
			handlers.PodNamespaceKey:       podNamespace,
		},
		Dependencies: podIdentityDependencies,
	}

	return podIdentityName, outputResource
}

func (r Renderer) getKeyVaultSecretOutputResource(ctx context.Context, keyVaultName string, secretName string, secretValue string) (outputresource.OutputResource, error) {
	keyVaultSecretOutputResource := outputresource.OutputResource{
		LocalID:  outputresource.LocalIDKeyVaultSecret,
		Type:     outputresource.TypeARM,
		Kind:     resourcekinds.AzureKeyVaultSecret,
		Deployed: false,
		Managed:  true,
		Resource: map[string]string{
			handlers.ManagedKey:             "true",
			handlers.KeyVaultNameKey:        keyVaultName,
			handlers.KeyVaultSecretNameKey:  secretName,
			handlers.KeyVaultSecretValueKey: secretValue,
		},
	}

	return keyVaultSecretOutputResource, nil
}

func (r Renderer) makeService(ctx context.Context, workload workloads.InstantiatedWorkload, component *ContainerComponent, deployment *appsv1.Deployment) ([]outputresource.OutputResource, error) {
	service := corev1.Service{
		TypeMeta: v1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      component.Name,
			Namespace: workload.Application,
			Labels:    kubernetes.MakeDescriptiveLabels(workload.Application, workload.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeSelectorLabels(workload.Application, workload.Name),
			Type:     corev1.ServiceTypeClusterIP,
			Ports:    []corev1.ServicePort{},
		},
	}

	for name, binding := range workload.Workload.Bindings {
		if binding.Kind == KindHTTP {
			httpBinding := HTTPBinding{}
			err := binding.AsRequired(KindHTTP, &httpBinding)
			if err != nil {
				return nil, err
			}

			port := corev1.ServicePort{
				Name:       name,
				Port:       int32(httpBinding.GetEffectivePort()),
				TargetPort: intstr.FromInt(httpBinding.GetEffectiveContainerPort()),
				Protocol:   corev1.ProtocolTCP,
			}

			service.Spec.Ports = append(service.Spec.Ports, port)
		}
	}

	if len(service.Spec.Ports) == 0 {
		return nil, nil
	}

	serviceDependencies := []outputresource.Dependency{
		{
			LocalID: outputresource.LocalIDDeployment,
		},
	}
	serviceOutputResource := outputresource.OutputResource{
		Kind:     resourcekinds.Kubernetes,
		LocalID:  outputresource.LocalIDService,
		Managed:  true,
		Deployed: false,
		Type:     outputresource.TypeKubernetes,
		Info: outputresource.K8sInfo{
			Kind:       deployment.TypeMeta.Kind,
			APIVersion: deployment.TypeMeta.APIVersion,
			Name:       deployment.ObjectMeta.Name,
			Namespace:  deployment.ObjectMeta.Namespace,
		},
		Resource:     &service,
		Dependencies: serviceDependencies,
	}

	return []outputresource.OutputResource{serviceOutputResource}, nil
}
