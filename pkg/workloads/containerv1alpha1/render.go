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

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/mgmt/keyvault"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/authorization/mgmt/authorization"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/keyvaultv1alpha1"
	"github.com/gofrs/uuid"
)

// Renderer is the WorkloadRenderer implementation for containerized workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	cw, err := r.convert(w)
	if err != nil {
		return nil, err
	}

	values := []map[string]interface{}{}
	for _, p := range cw.Provides {
		if p.Name == service.Name {
			// we've got a match
			if service.Kind != "http" {
				// TODO this just does the most basic thing - in theory we could define lots of different
				// types here. This is good enough for a prototype.
				return nil, fmt.Errorf("port cannot fulfil service kind: %v", service.Kind)
			}

			if len(values) > 0 {
				return nil, errors.New("more than one value source was found for this service")
			}

			uri := url.URL{
				Scheme: service.Kind,
				Host:   fmt.Sprintf("%v.%v.svc.cluster.local", w.Name, w.Application),
			}

			if p.Port != nil && *p.Port != 80 {
				uri.Host = uri.Host + fmt.Sprintf(":%d", *p.Port)
			}

			mapping := map[string]interface{}{}

			mapping["uri"] = uri.String()
			mapping["scheme"] = uri.Scheme
			mapping["host"] = uri.Hostname()
			if p.Port != nil {
				mapping["port"] = fmt.Sprintf("%d", *p.Port)
			} else {
				mapping["port"] = "80"
			}

			values = append(values, mapping)

			// keep going even after first success so we can find errors
		}
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

func (r Renderer) createRoleAssignment(ctx context.Context, managedIdentity msi.Identity, kvID string) error {
	// Assign KeyVault Reader permissions to the managed identity for the pod
	rdc := authorization.NewRoleDefinitionsClient(r.Arm.SubscriptionID)
	rdc.Authorizer = r.Arm.Auth

	// By default grant Key Vault Reader role with scope = KeyVault which provides read-only access to the Keyvault for secrets, keys and certificates
	roleList, err := rdc.List(ctx, kvID, "roleName eq 'Key Vault Reader'")
	if err != nil || !roleList.NotDone() {
		return fmt.Errorf("failed to create role assignment for user assigned managed identity: %w", err)
	}

	rac := authorization.NewRoleAssignmentsClient(r.Arm.SubscriptionID)
	rac.Authorizer = r.Arm.Auth
	raName, _ := uuid.NewV4()

	MaxRetries := 100
	for i := 0; i <= MaxRetries; i++ {

		// Retry to wait for the managed identity to propagate
		if i >= MaxRetries {
			return fmt.Errorf("failed to create role assignment for user assigned managed identity after retries: %w", err)
		}

		_, err = rac.Create(
			ctx,
			kvID,
			raName.String(),
			authorization.RoleAssignmentCreateParameters{
				RoleAssignmentProperties: &authorization.RoleAssignmentProperties{
					PrincipalID:      to.StringPtr(managedIdentity.PrincipalID.String()),
					RoleDefinitionID: to.StringPtr(*roleList.Values()[0].ID),
				},
			})

		if err == nil {
			return nil
		}

		// Check the error and determine if it is ignorable/retryable
		detailed, ok := util.ExtractDetailedError(err)
		if !ok {
			return err
		}
		// StatusCode = 409 indicates that the role assignment already exists. Ignore that error
		if detailed.StatusCode == 409 {
			return nil
		}

		// Sometimes, the managed identity takes a while to propagate and the role assignment creation fails with status code = 400
		// For other reasons, fail.
		if detailed.StatusCode != 400 {
			return fmt.Errorf("failed to create role assignment with error: %v, statuscode: %v", detailed.Message, detailed.StatusCode)
		}

		log.Println("Failed to create role assignment. Retrying...")
		time.Sleep(5 * time.Second)
		continue
	}

	return nil
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

	err = r.createRoleAssignment(ctx, msi, *kv.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment for user assigned managed identity: %w", err)
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

	// Fixup ports so that port and container port are always both assigned or neither are.
	for i := range container.Provides {
		if container.Provides[i].ContainerPort != nil && container.Provides[i].Port == nil {
			container.Provides[i].Port = container.Provides[i].ContainerPort
		}

		if container.Provides[i].Port != nil && container.Provides[i].ContainerPort == nil {
			container.Provides[i].ContainerPort = container.Provides[i].Port
		}
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

	for _, dep := range cc.DependsOn {
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
		}
	}

	for _, p := range cc.Provides {
		if p.ContainerPort != nil {
			port := corev1.ContainerPort{
				Name:          p.Name,
				ContainerPort: int32(*p.ContainerPort),
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

	for _, provides := range cc.Provides {
		if provides.ContainerPort != nil {
			port := corev1.ServicePort{
				Name:     provides.Name,
				Port:     int32(*provides.ContainerPort),
				Protocol: corev1.ProtocolTCP,
			}

			service.Spec.Ports = append(service.Spec.Ports, port)
		}
	}

	if len(service.Spec.Ports) == 0 {
		return nil, nil
	}

	return &service, nil
}
