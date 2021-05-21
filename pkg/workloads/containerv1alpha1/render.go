// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

import (
	"context"
	"errors"
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
	kvclient "github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/keyvault"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/mgmt/keyvault"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/roleassignment"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/keyvaultv1alpha1"
)

// Renderer is the WorkloadRenderer implementation for containerized workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	values := []map[string]interface{}{}
	for _, generic := range w.Workload.Provides {
		if generic.Name != service.Name {
			continue
		}

		// we've got a match
		if service.Kind != KindHTTP || generic.Kind != KindHTTP {
			// TODO this just does the most basic thing - in theory we could define lots of different
			// types here. This is good enough for a prototype.
			return nil, fmt.Errorf("port cannot fulfill service kind: %v", service.Kind)
		}

		if len(values) > 0 {
			return nil, errors.New("more than one value source was found for this service")
		}

		http := HTTPProvidesService{}
		err := generic.AsRequired(KindHTTP, &http)
		if err != nil {
			return nil, err
		}

		uri := url.URL{
			Scheme: service.Kind,
			Host:   fmt.Sprintf("%v.%v.svc.cluster.local", w.Name, w.Application),
		}

		if http.GetEffectivePort() != 80 {
			uri.Host = uri.Host + fmt.Sprintf(":%d", http.GetEffectivePort())
		}

		mapping := map[string]interface{}{}

		mapping["uri"] = uri.String()
		mapping["scheme"] = uri.Scheme
		mapping["host"] = uri.Hostname()
		mapping["port"] = fmt.Sprintf("%d", http.GetEffectivePort())

		values = append(values, mapping)

		// keep going even after first success so we can find errors
	}

	if len(values) == 1 {
		return values[0], nil
	}

	return map[string]interface{}{}, nil
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

func (r Renderer) readKeyVaultURI(dep ContainerDependsOn, w workloads.InstantiatedWorkload) (string, error) {
	if dep.SetEnv == nil {
		return "", errors.New("unable to find keyvault uri. invalid spec")
	}

	for _, v := range dep.SetEnv {
		if v != keyvaultv1alpha1.VaultURI {
			continue
		}

		service, ok := w.ServiceValues[dep.Name]
		if !ok {
			return "", fmt.Errorf("cannot resolve service %v", dep.Name)
		}

		value, ok := service[v]
		if !ok {
			return "", fmt.Errorf("cannot resolve value %v for service %v", v, dep.Name)
		}

		str, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("value %v for service %v is not a string", v, dep.Name)
		}

		return str, nil
	}

	return "", errors.New("unable to find keyvault uri. invalid spec")
}

func (r Renderer) readKeyVaultRPRoleAssignmentID(dep ContainerDependsOn, w workloads.InstantiatedWorkload) (string, error) {
	if dep.Kind != "azure.com/KeyVault" {
		return "", nil
	}

	service, ok := w.ServiceValues[dep.Name]
	if !ok {
		return "", fmt.Errorf("cannot resolve service %v", dep.Name)
	}

	value, ok := service[KeyVaultRPRoleID]
	if !ok {
		return "", fmt.Errorf("cannot resolve value %v for service %v", KeyVaultRPRoleID, dep.Name)
	}

	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("value %v for service %v is not a string", KeyVaultRPRoleID, dep.Name)
	}

	return str, nil
}

func (r Renderer) createManagedIdentityForKeyVault(ctx context.Context, dep ContainerDependsOn, w workloads.InstantiatedWorkload, cw *ContainerComponent) (*msi.Identity, error) {
	// Read KV_URI
	kvURI, err := r.readKeyVaultURI(dep, w)
	if err != nil || kvURI == "" {
		return nil, fmt.Errorf("failed to read keyvault uri: %w", err)
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

	for _, dep := range cw.DependsOn {
		// If the container depends on a KeyVault, create a pod identity.
		// The list of dependency kinds to check might grow in the future
		if dep.Kind == "azure.com/KeyVault" {
			// Create a user assigned managed identity and assign it the right permissions to access the KeyVault
			msi, err := r.createManagedIdentityForKeyVault(ctx, dep, w, cw)
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

	keyvaults := make(map[string]string)
	for _, dep := range cc.DependsOn {
		// Set environment variables in the container
		for k, v := range dep.SetEnv {
			service, ok := w.ServiceValues[dep.Name]
			if !ok {
				return nil, fmt.Errorf("cannot resolve service %v", dep.Name)
			}

			value, ok := service[v]
			if !ok {
				return nil, fmt.Errorf("cannot resolve value %v for service %v", v, dep.Name)
			}

			str, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("value %v for service %v is not a string", v, dep.Name)
			}

			container.Env = append(container.Env, corev1.EnvVar{
				Name:  k,
				Value: str,
			})

			if dep.Kind == "azure.com/KeyVault" && v == KeyVaultURIIdentifier {
				// Cache the KV URIs and use it for setting secrets, if any
				keyvaults[dep.Name] = str
			}
		}
	}

	// By now, all keyvault URIs have been cached
	for _, dep := range cc.DependsOn {
		secrets := make(map[string]string)
		var kvURI string
		for k, v := range dep.SetSecret {
			service, ok := w.ServiceValues[dep.Name]
			if !ok {
				return nil, fmt.Errorf("cannot resolve service %v", dep.Name)
			}

			if k == SecretStoreIdentifier {
				// Read the keyvault name and then look up the keyvault dependency to get the URI
				kvURI = keyvaults[v.(string)]
			} else if k == SecretKeysIdentifier {
				// Read the secrets
				setSecrets := v.(map[string]interface{})
				for sn, sv := range setSecrets {
					var str string
					value, ok := service[sv.(string)]
					if !ok {
						return nil, fmt.Errorf("cannot resolve value %v for service %v", v, dep.Name)
					}

					str, ok = value.(string)
					if !ok {
						return nil, fmt.Errorf("value %v for service %v is not a string", v, dep.Name)
					}
					secrets[sn] = str
				}
			}
		}

		// Create secrets in the specified keyvault
		for s, sv := range secrets {
			var secretValue kvclient.SecretSetParameters
			secretValue.Value = &sv
			err := r.createSecret(ctx, kvURI, s, secretValue)
			if err != nil {
				fmt.Printf("err: %v", err.Error())
				return nil, fmt.Errorf("Could not create secret: %v: %w", s, err)
			}
		}
	}

	// By now all secrets for all KeyVaults have been created.
	// Now delete RP role assignment for writing secrets to the KeyVault
	for _, dep := range cc.DependsOn {
		if dep.Kind != "azure.com/KeyVault" {
			raID, err := r.readKeyVaultRPRoleAssignmentID(dep, w)
			if err != nil {
				return nil, err
			}
			err = roleassignment.Delete(ctx, r.Arm.Auth, r.Arm.SubscriptionID, raID)
			if err != nil {
				return nil, fmt.Errorf("Unable to delete role assignment to RP for write secrets: %w", err)
			}
		}
	}

	for _, generic := range w.Workload.Provides {
		if generic.Kind == KindHTTP {
			httpProvides := HTTPProvidesService{}
			err := generic.AsRequired(KindHTTP, &httpProvides)
			if err != nil {
				return nil, err
			}
		}
	}

	for _, p := range cc.Provides {
		if p.ContainerPort != nil {
			port := corev1.ContainerPort{
				Name:          httpProvides.Name,
				ContainerPort: int32(httpProvides.GetEffectiveContainerPort()),
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

	dc := resources.NewDeploymentsClient(r.Arm.SubscriptionID)
	dc.Authorizer = r.Arm.Auth

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

// getAuthorizerForResource returns an authorizer over the specified scope based on the auth method
func (r Renderer) getAuthorizerForResource(ctx context.Context, scope string) (autorest.Authorizer, error) {
	authMethod := armauth.GetAuthMethod()
	if authMethod == armauth.ServicePrincipalAuth {
		return auth.NewAuthorizerFromEnvironmentWithResource(scope)
	} else if authMethod == armauth.ManagedIdentityAuth {
		msiKeyConfig := &auth.MSIConfig{
			Resource: scope,
			ClientID: r.Arm.ClientID,
		}
		return msiKeyConfig.Authorizer()
	} else {
		return auth.NewAuthorizerFromCLIWithResource(scope)
	}
}

func (r Renderer) createSecret(ctx context.Context, kvURI, secretName string, secretValue kvclient.SecretSetParameters) error {
	// Get a token for the RP system assigned identity for the Key Vault resource
	// The RP has previously been granted permission earlier to create secrets
	kvauth, err := r.getAuthorizerForResource(ctx, "https://vault.azure.net")
	if err != nil {
		return err
	}

	kvc := kvclient.New()
	kvc.Authorizer = kvauth
	_, err = kvc.SetSecret(ctx, kvURI, secretName, secretValue)
	if err != nil {
		return err
	}
	log.Printf("Created secret: %v in KeyVault: %v", secretName, kvURI)

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

	for _, generic := range w.Workload.Provides {
		if generic.Kind == KindHTTP {
			httpProvides := HTTPProvidesService{}
			err := generic.AsRequired(KindHTTP, &httpProvides)
			if err != nil {
				return nil, err
			}

			port := corev1.ServicePort{
				Name:       httpProvides.Name,
				Port:       int32(httpProvides.GetEffectivePort()),
				TargetPort: intstr.FromInt(httpProvides.GetEffectiveContainerPort()),
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
