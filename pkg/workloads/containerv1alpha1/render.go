// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/mgmt/keyvault"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/curp/components"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/roleassignment"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for containerized workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for containerized workload.
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

		uri := url.URL{
			Scheme: binding.Kind,
			Host:   fmt.Sprintf("%v.%v.svc.cluster.local", workload.Name, workload.Application),
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

func (r Renderer) createManagedIdentity(ctx context.Context, identityName, location string) (msi.Identity, error) {
	// Create a user assigned managed identity
	msiClient := msi.NewUserAssignedIdentitiesClient(r.Arm.SubscriptionID)
	msiClient.Authorizer = r.Arm.Auth
	id, err := msiClient.CreateOrUpdate(context.Background(), r.Arm.ResourceGroup, identityName, msi.Identity{
		Location: to.StringPtr(location),
	})
	if err != nil {
		return msi.Identity{}, fmt.Errorf("failed to create user assigned managed identity: %w", err)
	}

	log.Printf("Created managed identity for KeyVault access: %v", *id.ID)

	return id, nil
}

func (r Renderer) createManagedIdentityForKeyVault(ctx context.Context, store components.BindingState, w workloads.InstantiatedWorkload, cw *ContainerComponent) (*msi.Identity, error) {
	// Read the keyvault URI so we can get the keyvault name for permissions
	value, ok := store.Properties[KeyVaultURIIdentifier]
	if !ok {
		return nil, fmt.Errorf("failed to read keyvault uri")
	}

	kvURI, ok := value.(string)
	if !ok || kvURI == "" {
		return nil, fmt.Errorf("failed to read keyvault uri")
	}

	kvName := strings.Replace(strings.Split(kvURI, ".")[0], "https://", "", -1)
	// Create user assigned managed identity
	managedIdentityName := kvName + "-" + cw.Name + "-msi"

	g := resources.NewGroupsClient(r.Arm.SubscriptionID)
	g.Authorizer = r.Arm.Auth
	rg, err := g.Get(ctx, r.Arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("could not find resource group: %w", err)
	}
	msi, err := r.createManagedIdentity(ctx, managedIdentityName, *rg.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to create user assigned managed identity: %w", err)
	}

	kvc := keyvault.NewVaultsClient(r.Arm.SubscriptionID)
	kvc.Authorizer = r.Arm.Auth
	if err != nil {
		return nil, err
	}
	kv, err := kvc.Get(ctx, r.Arm.ResourceGroup, kvName)
	if err != nil {
		return nil, fmt.Errorf("unable to find keyvault: %w", err)
	}

	// Create Role Assignment to grant the managed identity appropriate access permissions to the Key Vault
	// By default grant Key Vault Secrets User role with scope which provides read-only access to the Keyvault for secrets and certificates
	_, err = roleassignment.Create(ctx, r.Arm.Auth, r.Arm.SubscriptionID, r.Arm.ResourceGroup, msi.PrincipalID.String(), *kv.ID, "Key Vault Secrets User")
	if err != nil {
		return nil, fmt.Errorf("Failed to create role assignment to assign Key Vault Secrets User permissions to managed identity: %v: %w", msi.Name, err)
	}
	// By default grant Key Vault Secrets User role with scope which provides read-only access to the Keyvault for encryption keys
	_, err = roleassignment.Create(ctx, r.Arm.Auth, r.Arm.SubscriptionID, r.Arm.ResourceGroup, msi.PrincipalID.String(), *kv.ID, "Key Vault Crypto User")
	if err != nil {
		return nil, fmt.Errorf("Failed to create role assignment to assign Key Vault Crypto User permissions to managed identity: %v: %w", msi.Name, err)
	}

	log.Printf("Created role assignment for %v to access %v", *msi.ID, *kv.ID)

	return &msi, nil
}

func (r Renderer) createPodIdentityResource(ctx context.Context, w workloads.InstantiatedWorkload, cw *ContainerComponent) (AADPodIdentity, error) {
	var podIdentity AADPodIdentity

	for _, dependency := range cw.Uses {
		binding, err := dependency.Binding.GetMatchingBinding(w.BindingValues)
		if err != nil {
			return AADPodIdentity{}, err
		}

		// If the container depends on a KeyVault, create a pod identity.
		// The list of dependency kinds to check might grow in the future
		if binding.Kind == "azure.com/KeyVault" {
			// Create a user assigned managed identity and assign it the right permissions to access the KeyVault
			msi, err := r.createManagedIdentityForKeyVault(ctx, binding, w, cw)
			if err != nil {
				return AADPodIdentity{}, err
			}

			// Create pod identity
			podIdentity, err := r.createPodIdentity(ctx, *msi, cw.Name, w.Application)
			if err != nil {
				return AADPodIdentity{}, fmt.Errorf("failed to create pod identity: %w", err)
			}

			log.Printf("Created pod identity %v to bind %v", podIdentity.Name, *msi.ID)
			return podIdentity, nil
		}
	}

	return podIdentity, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	cw, err := r.convert(w)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	deployment, err := r.makeDeployment(ctx, w, cw)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	service, err := r.makeService(ctx, w, cw)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	resources := []workloads.WorkloadResource{}

	podIdentity, err := r.createPodIdentityResource(ctx, w, cw)
	if err != nil {
		return []workloads.WorkloadResource{}, fmt.Errorf("unable to add pod identity: %w", err)
	}
	if podIdentity.Name != "" {
		// Add the aadpodidbinding label to the k8s spec for the container
		deployment.Spec.Template.ObjectMeta.Labels["aadpodidbinding"] = podIdentity.Name

		// Append the Pod identity created to the list of resources
		resources = append(resources, workloads.WorkloadResource{
			Type: workloads.ResourceKindAzurePodIdentity,
			Resource: map[string]string{
				PodIdentityName:    podIdentity.Name,
				PodIdentityCluster: podIdentity.ClusterName,
			},
		})
	}

	resources = append(resources, workloads.NewKubernetesResource("Deployment", deployment))
	if service != nil {
		resources = append(resources, workloads.NewKubernetesResource("Service", service))
	}

	return resources, nil
}

func (r Renderer) convert(w workloads.InstantiatedWorkload) (*ContainerComponent, error) {
	container := &ContainerComponent{}
	err := w.Workload.AsRequired(Kind, container)
	if err != nil {
		return nil, err
	}

	return container, nil
}

func (r Renderer) makeDeployment(ctx context.Context, w workloads.InstantiatedWorkload, cc *ContainerComponent) (*appsv1.Deployment, error) {
	container := corev1.Container{
		Name:  cc.Name,
		Image: cc.Run.Container.Image,

		// TODO: use better policies than this when we have a good versioning story
		ImagePullPolicy: corev1.PullPolicy("Always"),
		Env:             []corev1.EnvVar{},
	}

	for _, e := range cc.Run.Container.Environment {
		if e.Value != nil {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  e.Name,
				Value: *e.Value,
			})
			continue
		}
	}

	for _, dep := range cc.Uses {
		// Set environment variables in the container
		for k, v := range dep.Env {
			str, err := v.EvaluateString(w.BindingValues)
			if err != nil {
				return nil, err
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
			return nil, err
		}
		value, ok := store.Properties[KeyVaultURIIdentifier]
		if !ok {
			return nil, fmt.Errorf("cannot find a keyvault URI for secret store binding %s from component %s", store.Binding, store.Component)
		}

		uri, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("value %s for binding for binding %s from component %s is not a string", KeyVaultURIIdentifier, store.Binding, store.Component)
		}

		secrets := map[string]string{}
		for k, v := range dep.Secrets.Keys {
			value, err := v.EvaluateString(w.BindingValues)
			if err != nil {
				return nil, err
			}

			secrets[k] = value
		}

		// Create secrets in the specified keyvault
		for secretName, secretValue := range secrets {
			err := r.createSecret(ctx, uri, secretName, secretValue)
			if err != nil {
				return nil, fmt.Errorf("could not create secret: %v: %w", secretName, err)
			}
		}
	}

	for name, generic := range w.Workload.Bindings {
		if generic.Kind == KindHTTP {
			httpBinding := HTTPBinding{}
			err := generic.AsRequired(KindHTTP, &httpBinding)
			if err != nil {
				return nil, err
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
			Labels: map[string]string{
				workloads.LabelRadiusApplication: w.Application,
				workloads.LabelRadiusComponent:   cc.Name,
				// TODO get the component revision here...
				"app.kubernetes.io/name":       cc.Name,
				"app.kubernetes.io/part-of":    w.Application,
				"app.kubernetes.io/managed-by": "radius-rp",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					workloads.LabelRadiusApplication: w.Application,
					workloads.LabelRadiusComponent:   cc.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						workloads.LabelRadiusApplication: w.Application,
						workloads.LabelRadiusComponent:   cc.Name,
						// TODO get the component revision here...
						"app.kubernetes.io/name":       cc.Name,
						"app.kubernetes.io/part-of":    w.Application,
						"app.kubernetes.io/managed-by": "radius-rp",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
				},
			},
		},
	}

	return &deployment, nil
}

// AADPodIdentity represents the AAD pod identity added to a Kubernetes cluster
type AADPodIdentity struct {
	Name        string
	Namespace   string
	ClusterName string
}

func (r Renderer) createPodIdentity(ctx context.Context, msi msi.Identity, containerName, podNamespace string) (AADPodIdentity, error) {

	// Get AKS cluster name in current resource group
	mcc := containerservice.NewManagedClustersClient(r.Arm.SubscriptionID)
	mcc.Authorizer = r.Arm.Auth

	var cluster *containerservice.ManagedCluster
	for list, err := mcc.ListByResourceGroupComplete(ctx, r.Arm.ResourceGroup); list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return AADPodIdentity{}, fmt.Errorf("cannot read AKS clusters: %w", err)
		}

		if radresources.HasRadiusEnvironmentTag(list.Value().Tags) {
			temp := list.Value()
			cluster = &temp
			break
		}
	}

	if cluster == nil {
		return AADPodIdentity{}, fmt.Errorf("could not find an AKS instance in resource group '%v'", r.Arm.ResourceGroup)
	}

	// Note: Pod Identity name cannot have camel case
	podIdentityName := "podid-" + strings.ToLower(containerName)

	// Get the cluster and modify it to add pod identity
	managedCluster, err := mcc.Get(ctx, r.Arm.ResourceGroup, *cluster.Name)
	if err != nil {
		return AADPodIdentity{}, fmt.Errorf("failed to get managed cluster: %w", err)
	}
	managedCluster.PodIdentityProfile.Enabled = to.BoolPtr(true)
	managedCluster.PodIdentityProfile.AllowNetworkPluginKubenet = to.BoolPtr(false)
	podID := containerservice.ManagedClusterPodIdentity{
		Name: &podIdentityName,
		// Note: The pod identity namespace specified here has to match the namespace in which the application is deployed
		Namespace: &podNamespace,
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
	identities = append(identities, podID)

	MaxRetries := 100
	var mcFuture containerservice.ManagedClustersCreateOrUpdateFuture
	for i := 0; i <= MaxRetries; i++ {
		// Retry to wait for the managed identity to propagate
		if i >= MaxRetries {
			return AADPodIdentity{}, fmt.Errorf("failed to add pod identity on the cluster after retries: %w", err)
		}

		mcFuture, err = mcc.CreateOrUpdate(ctx, r.Arm.ResourceGroup, *cluster.Name, containerservice.ManagedCluster{
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
		detailed, ok := util.ExtractDetailedError(err)
		if !ok {
			return AADPodIdentity{}, err
		}

		// Sometimes, the managed identity takes a while to propagate and the pod identity creation fails with status code = 0
		// For other reasons, fail
		if detailed.StatusCode != 0 {
			return AADPodIdentity{}, fmt.Errorf("failed to add pod identity on the cluster with error: %v, status code: %v", detailed.Message, detailed.StatusCode)
		}

		fmt.Println("failed to add pod identity. Retrying...")
		time.Sleep(5 * time.Second)
		continue
	}

	err = mcFuture.WaitForCompletionRef(ctx, mcc.Client)
	if err != nil {
		return AADPodIdentity{}, fmt.Errorf("failed to add pod identity on the cluster: %w", err)
	}

	podIdentity := AADPodIdentity{
		Name:        podIdentityName,
		Namespace:   podNamespace,
		ClusterName: *cluster.Name,
	}
	return podIdentity, nil
}

func (r Renderer) createSecret(ctx context.Context, kvURI, secretName string, secretValue string) error {
	// Create secret in the Key Vault using ARM since ARM has write permissions to create secrets
	// and no special role assignment is needed.

	// UserAgent() returns a string of format: Azure-SDK-For-Go/v52.2.0 keyvault/2019-09-01 profiles/latest
	kvAPIVersion := strings.Split(strings.Split(keyvault.UserAgent(), "keyvault/")[1], " ")[0]

	// KeyVault URI has the format: "https://<kv name>.vault.azure.net"
	vaultName := strings.Split(strings.Split(kvURI, "https://")[1], ".vault.azure.net")[0]
	secretFullName := vaultName + "/" + secretName
	template := map[string]interface{}{
		"$schema":        "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
		"contentVersion": "1.0.0.0",
		"parameters":     map[string]interface{}{},
		"resources": []interface{}{
			map[string]interface{}{
				"type":       "Microsoft.KeyVault/vaults/secrets",
				"name":       secretFullName,
				"apiVersion": kvAPIVersion,
				"properties": map[string]interface{}{
					"contentType": "text/plain",
					"value":       secretValue,
				},
			},
		},
	}

	dc := resources.NewDeploymentsClient(r.Arm.SubscriptionID)
	dc.Authorizer = r.Arm.Auth
	parameters := map[string]interface{}{}
	deploymentProperties := &resources.DeploymentProperties{
		Parameters: parameters,
		Mode:       resources.Incremental,
		Template:   template,
	}
	deploymentName := "create-secret-" + vaultName + "-" + secretName
	op, err := dc.CreateOrUpdate(context.Background(), r.Arm.ResourceGroup, deploymentName, resources.Deployment{
		Properties: deploymentProperties,
	})
	if err != nil {
		return fmt.Errorf("unable to create secret: %w", err)
	}

	err = op.WaitForCompletionRef(context.Background(), dc.Client)
	if err != nil {
		return fmt.Errorf("could not create secret: %w", err)
	}

	_, err = op.Result(dc)
	if err != nil {
		return fmt.Errorf("could not create secret: %w", err)
	}
	log.Printf("Created secret: %s in Key Vault: %s successfully", secretName, vaultName)

	return nil
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
			Labels: map[string]string{
				workloads.LabelRadiusApplication: w.Application,
				workloads.LabelRadiusComponent:   cc.Name,
				// TODO get the component revision here...
				"app.kubernetes.io/name":       cc.Name,
				"app.kubernetes.io/part-of":    w.Application,
				"app.kubernetes.io/managed-by": "radius-rp",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				workloads.LabelRadiusApplication: w.Application,
				workloads.LabelRadiusComponent:   cc.Name,
			},
			Type:  corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{},
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
