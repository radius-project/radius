// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/authorization/mgmt/authorization"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/mgmt/keyvault"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/azure/roleassignment"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
)

// keyVaultSecretsResourceType declares the resource type for Azure KeyVault Secrets.
const keyVaultSecretsResourceType = azresources.KeyVaultVaults + "/" + azresources.KeyVaultVaultsSecrets

// Permissions granted on the key vault
// Role description: https://docs.microsoft.com/en-us/azure/key-vault/general/rbac-guide?tabs=azure-cli
const keyVaultSecretsReadRole = "Key Vault Secrets User"
const keyVaultCryptoOperationsRole = "Key Vault Crypto User"

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

// Creates a user assigned managed identity
func (r Renderer) createManagedIdentity(ctx context.Context, applicationName string, componentName string, keyVaultName string) (*msi.Identity, []outputresource.OutputResource, error) {
	logger := radlogger.GetLogger(ctx)

	managedIdentityName := keyVaultName + "-" + componentName + "-msi"

	rgLocation, err := clients.GetResourceGroupLocation(ctx, r.Arm)
	if err != nil {
		return nil, []outputresource.OutputResource{}, err
	}

	msiClient := clients.NewUserAssignedIdentitiesClient(r.Arm.SubscriptionID, r.Arm.Auth)
	identity, err := msiClient.CreateOrUpdate(context.Background(), r.Arm.ResourceGroup, managedIdentityName, msi.Identity{
		Location: to.StringPtr(*rgLocation),
		Tags:     keys.MakeTagsForRadiusComponent(applicationName, componentName),
	})
	if err != nil {
		return nil, []outputresource.OutputResource{}, fmt.Errorf("failed to create user assigned managed identity: %w", err)
	}

	logger.WithValues(
		radlogger.LogFieldResourceID, *identity.ID,
		radlogger.LogFieldLocalID, outputresource.LocalIDUserAssignedManagedIdentityKV).Info("Created managed identity for KeyVault access")

	identityOutputResource := outputresource.OutputResource{
		Type:     outputresource.TypeARM,
		Kind:     resourcekinds.AzureUserAssignedManagedIdentity,
		LocalID:  outputresource.LocalIDUserAssignedManagedIdentityKV,
		Deployed: true,
		Managed:  true,
		Info: outputresource.ARMInfo{
			ID:           *identity.ID,
			ResourceType: *identity.Type,
			APIVersion:   msi.Version(),
		},
	}

	return &identity, []outputresource.OutputResource{identityOutputResource}, nil
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

// Assigns secrets user and cryto user roles to the managed identity for access to the keyvault
func (r Renderer) assignRoleToManagedIdentity(ctx context.Context, keyVaultName string, managedIdentityName string, managedIdentityID string,
	managedIdentityPrincipalID string) ([]outputresource.OutputResource, error) {
	logger := radlogger.GetLogger(ctx)
	outputResources := []outputresource.OutputResource{}

	keyVaultClient := clients.NewVaultsClient(r.Arm.SubscriptionID, r.Arm.Auth)
	keyVault, err := keyVaultClient.Get(ctx, r.Arm.ResourceGroup, keyVaultName)
	if err != nil {
		return outputResources, fmt.Errorf("failed to get key vault information: %w", err)
	}

	// Assign Key Vault Secrets User role to grant managed identity read-only access to the keyvault for secrets.
	readSecretsRA, err := roleassignment.Create(ctx, r.Arm.Auth, r.Arm.SubscriptionID, r.Arm.ResourceGroup, managedIdentityPrincipalID, *keyVault.ID, keyVaultSecretsReadRole)
	if err != nil {
		return outputResources,
			fmt.Errorf("Failed to assign '%s' role to the managed identity '%s' within keyvault '%s' scope : %w", keyVaultSecretsReadRole, managedIdentityName, keyVaultName, err)
	}

	readSecretsRAOutputResource := outputresource.OutputResource{
		Kind:     resourcekinds.AzureRoleAssignment,
		LocalID:  outputresource.LocalIDRoleAssignmentKVSecretsCerts,
		Deployed: true,
		Managed:  true,
		Type:     outputresource.TypeARM,
		Info: outputresource.ARMInfo{
			ID:           *readSecretsRA.ID,
			ResourceType: *readSecretsRA.Type,
			APIVersion:   authorization.Version(),
		},
	}
	outputResources = append(outputResources, readSecretsRAOutputResource)
	logger.WithValues(radlogger.LogFieldLocalID, outputresource.LocalIDRoleAssignmentKVSecretsCerts).Info(fmt.Sprintf("Created %s role assignment for %v to access %v", keyVaultSecretsReadRole, managedIdentityID, *keyVault.ID))

	// Assign Key Vault Crypto User role to grant managed identity permissions to perform operations using encryption keys.
	cryptoOperationsRA, err := roleassignment.Create(ctx, r.Arm.Auth, r.Arm.SubscriptionID, r.Arm.ResourceGroup, managedIdentityPrincipalID, *keyVault.ID, keyVaultCryptoOperationsRole)
	if err != nil {
		return outputResources, fmt.Errorf("Failed to assign '%s' role to the managed identity '%s' within keyvault '%s' scope : %w", keyVaultCryptoOperationsRole, managedIdentityName, keyVaultName, err)
	}

	cryptoOperationsRAOutputResource := outputresource.OutputResource{
		Kind:     resourcekinds.AzureRoleAssignment,
		LocalID:  outputresource.LocalIDRoleAssignmentKVKeys,
		Deployed: true,
		Managed:  true,
		Type:     outputresource.TypeARM,
		Info: outputresource.ARMInfo{
			ID:           *cryptoOperationsRA.ID,
			ResourceType: *cryptoOperationsRA.Type,
			APIVersion:   authorization.Version(),
		},
	}
	outputResources = append(outputResources, cryptoOperationsRAOutputResource)
	logger.WithValues(radlogger.LogFieldLocalID, outputresource.LocalIDRoleAssignmentKVKeys).Info(fmt.Sprintf("Created %s role assignment for %s to access %s", keyVaultCryptoOperationsRole, managedIdentityID, *keyVault.ID))

	return outputResources, nil
}

func (r Renderer) createPodIdentityResource(ctx context.Context, w workloads.InstantiatedWorkload, component *ContainerComponent) (outputresource.AADPodIdentity, []outputresource.OutputResource, error) {
	logger := radlogger.GetLogger(ctx)
	var podIdentity outputresource.AADPodIdentity

	outputResources := []outputresource.OutputResource{}
	for _, dependency := range component.Uses {
		binding, err := dependency.Binding.GetMatchingBinding(w.BindingValues)
		if err != nil {
			return podIdentity, outputResources, err
		}

		// If the container depends on a KeyVault, create a pod identity.
		// The list of dependency kinds to check might grow in the future
		if binding.Kind == "azure.com/KeyVault" {
			keyVaultName, err := r.getKeyVaultName(binding)
			if err != nil {
				return podIdentity, outputResources, err
			}

			// Create a user assigned managed identity
			userAssignedIdentity, or, err := r.createManagedIdentity(ctx, w.Application, w.Name, keyVaultName)
			outputResources = append(outputResources, or...)
			if err != nil {
				return podIdentity, outputResources, err
			}

			// RBAC on managed identity to access the KeyVault
			roleAssignmentOutputResources, err := r.assignRoleToManagedIdentity(ctx, keyVaultName,
				*userAssignedIdentity.Name, *userAssignedIdentity.ID, userAssignedIdentity.PrincipalID.String())
			outputResources = append(outputResources, roleAssignmentOutputResources...)
			if err != nil {
				return podIdentity, outputResources, err
			}

			// Create pod identity
			podIdentity, podIdentityOutputResource, err := r.createPodIdentity(ctx, *userAssignedIdentity, component.Name, w.Application)
			outputResources = append(outputResources, podIdentityOutputResource)
			if err != nil {
				return podIdentity, outputResources, err
			}

			logger.WithValues(radlogger.LogFieldLocalID, outputresource.LocalIDAADPodIdentity).Info(fmt.Sprintf("Created pod identity %v to bind %v", podIdentity.Name, *userAssignedIdentity.ID))
			return podIdentity, outputResources, nil
		}
	}

	return podIdentity, outputResources, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	outputResources := []outputresource.OutputResource{}
	cw, err := r.convert(w)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	deployment, or, err := r.makeDeployment(ctx, w, cw)
	if err != nil {
		return or, err
	}
	// Append the output resources created during the makeDeployment phase to the final set
	outputResources = append(outputResources, or...)

	service, err := r.makeService(ctx, w, cw)
	if err != nil {
		return outputResources, err
	}

	podIdentity, or, err := r.createPodIdentityResource(ctx, w, cw)
	if err != nil {
		return or, fmt.Errorf("unable to add pod identity: %w", err)
	}
	if podIdentity.Name != "" {
		// Add the aadpodidbinding label to the k8s spec for the container
		deployment.Spec.Template.ObjectMeta.Labels["aadpodidbinding"] = podIdentity.Name
	}

	// Append the output resources created for podid creation to the final set
	outputResources = append(outputResources, or...)

	res := outputresource.OutputResource{
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
		Resource: deployment,
	}
	outputResources = append(outputResources, res)

	if service != nil {
		res = outputresource.OutputResource{
			Kind:     resourcekinds.Kubernetes,
			LocalID:  outputresource.LocalIDService,
			Deployed: false,
			Managed:  true,
			Type:     outputresource.TypeKubernetes,
			Info: outputresource.K8sInfo{
				Kind:       deployment.TypeMeta.Kind,
				APIVersion: deployment.TypeMeta.APIVersion,
				Name:       deployment.ObjectMeta.Name,
				Namespace:  deployment.ObjectMeta.Namespace,
			},
			Resource: service,
		}
		outputResources = append(outputResources, res)
	}

	return outputResources, nil
}

func (r Renderer) convert(w workloads.InstantiatedWorkload) (*ContainerComponent, error) {
	container := &ContainerComponent{}
	err := w.Workload.AsRequired(Kind, container)
	if err != nil {
		return nil, err
	}

	return container, nil
}

func (r Renderer) makeDeployment(ctx context.Context, w workloads.InstantiatedWorkload, cc *ContainerComponent) (*appsv1.Deployment, []outputresource.OutputResource, error) {
	outputResources := []outputresource.OutputResource{}
	container := corev1.Container{
		Name:  cc.Name,
		Image: cc.Run.Container.Image,

		// TODO: use better policies than this when we have a good versioning story
		ImagePullPolicy: corev1.PullPolicy("Always"),
		Env:             []corev1.EnvVar{},
	}

	for k, v := range cc.Run.Container.Env {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	for _, dep := range cc.Uses {
		// Set environment variables in the container
		for k, v := range dep.Env {
			str, err := v.EvaluateString(w.BindingValues)
			if err != nil {
				return nil, outputResources, err
			}
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  k,
				Value: str,
			})
		}
	}

	for _, dep := range cc.Uses {
		if dep.Secrets == nil {
			continue
		}

		store, err := dep.Secrets.Store.GetMatchingBinding(w.BindingValues)
		if err != nil {
			return nil, outputResources, err
		}
		value, ok := store.Properties[handlers.KeyVaultURIKey]
		if !ok {
			return nil, outputResources, fmt.Errorf("cannot find a keyvault URI for secret store binding %s from component %s", store.Binding, store.Component)
		}

		uri, ok := value.(string)
		if !ok {
			return nil, outputResources, fmt.Errorf("value %s for binding for binding %s from component %s is not a string", handlers.KeyVaultURIKey, store.Binding, store.Component)
		}

		secrets := map[string]string{}
		for k, v := range dep.Secrets.Keys {
			value, err := v.EvaluateString(w.BindingValues)
			if err != nil {
				return nil, outputResources, err
			}

			secrets[k] = value
		}

		// Create secrets in the specified keyvault
		for secretName, secretValue := range secrets {
			or, err := r.createSecret(ctx, uri, secretName, secretValue)
			if err != nil {
				return nil, outputResources, fmt.Errorf("could not create secret: %v: %w", secretName, err)
			}
			outputResources = append(outputResources, or)
		}
	}

	for name, generic := range w.Workload.Bindings {
		if generic.Kind == KindHTTP {
			httpBinding := HTTPBinding{}
			err := generic.AsRequired(KindHTTP, &httpBinding)
			if err != nil {
				return nil, outputResources, err
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
			Name:      cc.Name,
			Namespace: w.Application,
			Labels:    kubernetes.MakeDescriptiveLabels(w.Application, w.Name),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: kubernetes.MakeSelectorLabels(w.Application, w.Name),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: kubernetes.MakeDescriptiveLabels(w.Application, w.Name),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
				},
			},
		},
	}

	return &deployment, outputResources, nil
}

func (r Renderer) createPodIdentity(ctx context.Context, msi msi.Identity, containerName string, podNamespace string) (outputresource.AADPodIdentity, outputresource.OutputResource, error) {
	logger := radlogger.GetLogger(ctx)
	var podIdentity outputresource.AADPodIdentity

	if r.Arm.K8sSubscriptionID == "" || r.Arm.K8sResourceGroup == "" || r.Arm.K8sClusterName == "" {
		return podIdentity, outputresource.OutputResource{}, errors.New("pod identity is not supported because the RP is not configured for AKS")
	}

	// Get AKS cluster name in current resource group and update it to add pod identity
	clustersClient := clients.NewManagedClustersClient(r.Arm.K8sSubscriptionID, r.Arm.Auth)
	managedCluster, err := clustersClient.Get(ctx, r.Arm.K8sResourceGroup, r.Arm.K8sClusterName)
	if err != nil {
		return podIdentity, outputresource.OutputResource{}, fmt.Errorf("failed to get managed cluster details for cluster %s in the resource group %s: %w", r.Arm.K8sClusterName, r.Arm.K8sResourceGroup, err)
	}

	managedCluster.PodIdentityProfile.Enabled = to.BoolPtr(true)
	managedCluster.PodIdentityProfile.AllowNetworkPluginKubenet = to.BoolPtr(false)

	// Note: Pod Identity name cannot have camel case
	podIdentityName := "podid-" + strings.ToLower(containerName)
	clusterPodIdentity := containerservice.ManagedClusterPodIdentity{
		Name:      &podIdentityName,
		Namespace: &podNamespace, // Note: The pod identity namespace specified here has to match the namespace in which the application is deployed
		Identity: &containerservice.UserAssignedIdentity{
			ResourceID: msi.ID,
			ClientID:   to.StringPtr(msi.ClientID.String()),
			ObjectID:   to.StringPtr(msi.PrincipalID.String()),
		},
	}

	var identities []containerservice.ManagedClusterPodIdentity
	if managedCluster.ManagedClusterProperties.PodIdentityProfile.UserAssignedIdentities != nil {
		identities = *managedCluster.PodIdentityProfile.UserAssignedIdentities
	}
	identities = append(identities, clusterPodIdentity)

	MaxRetries := 100
	var resultFuture containerservice.ManagedClustersCreateOrUpdateFuture
	for i := 0; i <= MaxRetries; i++ {
		// Retry to wait for the managed identity to propagate
		if i >= MaxRetries {
			return podIdentity, outputresource.OutputResource{}, fmt.Errorf("failed to add pod identity on the cluster %s: %w", r.Arm.K8sClusterName, err)
		}

		resultFuture, err = clustersClient.CreateOrUpdate(ctx, r.Arm.K8sResourceGroup, r.Arm.K8sClusterName, containerservice.ManagedCluster{
			ManagedClusterProperties: &containerservice.ManagedClusterProperties{
				PodIdentityProfile: &containerservice.ManagedClusterPodIdentityProfile{
					Enabled:                   to.BoolPtr(true),
					AllowNetworkPluginKubenet: to.BoolPtr(false),
					UserAssignedIdentities:    &identities,
				},
			},
			Location: managedCluster.Location,
		})

		if err == nil {
			break
		}

		// Check the error and determine if it is retryable
		detailed, ok := clients.ExtractDetailedError(err)
		if !ok {
			return podIdentity, outputresource.OutputResource{}, err
		}

		// Sometimes, the managed identity takes a while to propagate and the pod identity creation fails with status code = 0
		// For other reasons, fail
		if detailed.StatusCode != 0 {
			return podIdentity, outputresource.OutputResource{}, fmt.Errorf("failed to add pod identity on the cluster with error: %v, status code: %v", detailed.Message, detailed.StatusCode)
		}

		logger.V(radlogger.Verbose).Info("failed to add pod identity. Retrying...")
		time.Sleep(5 * time.Second)
		continue
	}

	err = resultFuture.WaitForCompletionRef(ctx, clustersClient.Client)
	if err != nil {
		return podIdentity, outputresource.OutputResource{}, fmt.Errorf("failed to add pod identity on the cluster: %w", err)
	}

	podIdentity = outputresource.AADPodIdentity{
		AKSClusterName: r.Arm.K8sClusterName,
		Name:           podIdentityName,
		Namespace:      podNamespace,
	}

	outputResource := outputresource.OutputResource{
		Deployed: true,
		Kind:     resourcekinds.AzurePodIdentity,
		Type:     outputresource.TypeAADPodIdentity,
		LocalID:  outputresource.LocalIDAADPodIdentity,
		Managed:  true,
		Info:     podIdentity,
		Resource: map[string]string{
			handlers.PodIdentityNameKey:    podIdentity.Name,
			handlers.PodIdentityClusterKey: podIdentity.AKSClusterName,
		},
	}

	return podIdentity, outputResource, nil
}

// Create secret in the Key Vault using ARM since ARM has write permissions to create secrets
// and no special role assignment is needed.
func (r Renderer) createSecret(ctx context.Context, kvURI, secretName string, secretValue string) (outputresource.OutputResource, error) {
	logger := radlogger.GetLogger(ctx)

	// UserAgent() returns a string of format: Azure-SDK-For-Go/v52.2.0 keyvault/2019-09-01 profiles/latest
	keyVaultAPIVersion := strings.Split(strings.Split(keyvault.UserAgent(), "keyvault/")[1], " ")[0]

	// KeyVault URI has the format: "https://<kv name>.vault.azure.net"
	vaultName := strings.Split(strings.Split(kvURI, "https://")[1], ".vault.azure.net")[0]
	secretFullName := vaultName + "/" + secretName
	template := map[string]interface{}{
		"$schema":        "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
		"contentVersion": "1.0.0.0",
		"parameters":     map[string]interface{}{},
		"resources": []interface{}{
			map[string]interface{}{
				"type":       keyVaultSecretsResourceType,
				"name":       secretFullName,
				"apiVersion": keyVaultAPIVersion,
				"properties": map[string]interface{}{
					"contentType": "text/plain",
					"value":       secretValue,
				},
			},
		},
	}

	dc := clients.NewDeploymentsClient(r.Arm.SubscriptionID, r.Arm.Auth)
	parameters := map[string]interface{}{}
	deploymentProperties := &resources.DeploymentProperties{
		Parameters: parameters,
		Mode:       resources.DeploymentModeIncremental,
		Template:   template,
	}
	deploymentName := "create-secret-" + vaultName + "-" + secretName
	op, err := dc.CreateOrUpdate(context.Background(), r.Arm.ResourceGroup, deploymentName, resources.Deployment{
		Properties: deploymentProperties,
	})
	if err != nil {
		return outputresource.OutputResource{}, fmt.Errorf("unable to create secret: %w", err)
	}

	err = op.WaitForCompletionRef(context.Background(), dc.Client)
	if err != nil {
		return outputresource.OutputResource{}, fmt.Errorf("could not create secret: %w", err)
	}

	_, err = op.Result(dc)
	if err != nil {
		return outputresource.OutputResource{}, fmt.Errorf("could not create secret: %w", err)
	}

	secretResource := azure.Resource{
		SubscriptionID: r.Arm.SubscriptionID,
		ResourceGroup:  r.Arm.ResourceGroup,
		Provider:       "Microsoft.KeyVault",
		ResourceType:   keyVaultSecretsResourceType,
		ResourceName:   secretFullName,
	}
	or := outputresource.OutputResource{
		Kind:     resourcekinds.AzureKeyVaultSecret,
		LocalID:  outputresource.LocalIDKeyVaultSecret,
		Deployed: true,
		Managed:  true,
		Type:     outputresource.TypeARM,
		Info: outputresource.ARMInfo{
			ID:           secretResource.String(),
			ResourceType: keyVaultSecretsResourceType,
			APIVersion:   keyVaultAPIVersion,
		},
	}
	logger.WithValues(radlogger.LogFieldLocalID, outputresource.LocalIDKeyVaultSecret).Info(fmt.Sprintf("Created secret: %s in Key Vault: %s successfully", secretName, vaultName))

	return or, nil
}

func (r Renderer) makeService(ctx context.Context, w workloads.InstantiatedWorkload, cc *ContainerComponent) (*corev1.Service, error) {
	service := corev1.Service{
		TypeMeta: v1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cc.Name,
			Namespace: w.Application,
			Labels:    kubernetes.MakeDescriptiveLabels(w.Application, w.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeSelectorLabels(w.Application, w.Name),
			Type:     corev1.ServiceTypeClusterIP,
			Ports:    []corev1.ServicePort{},
		},
	}

	for name, generic := range w.Workload.Bindings {
		if generic.Kind == KindHTTP {
			httpBinding := HTTPBinding{}
			err := generic.AsRequired(KindHTTP, &httpBinding)
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

	return &service, nil
}
