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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/rad/azcli"
	"github.com/Azure/radius/pkg/workloads"
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
	resources = append(resources, workloads.NewKubernetesResource("Deployment", deployment))
	if service != nil {
		resources = append(resources, workloads.NewKubernetesResource("Service", service))
	}

	fmt.Printf("Container depends on: %v", w.Workload.DependsOn)
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

	var podID string
	var err error
	for _, dep := range cc.DependsOn {
		// If the container depends on a KeyVault, create a pod identity
		if dep.Kind == "azure.com/KeyVault" {
			setVars := make(map[string]string)
			if dep.Set != nil {
				for k, v := range dep.Set {
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

					setVars[k] = str
				}
			}
			podID, err = r.createPodIdentity(ctx, w, cc, setVars)
			if err != nil {
				return nil, errors.New("unable to create pod identity")
			}
		}

		if dep.SetEnv == nil {
			continue
		}

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
						"aadpodidbinding":              podID,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
				},
			},
		},
	}

	// if podID != "" {
	// 	fmt.Println("@@@@@ Adding podid to the deployment spec")
	// 	deployment.Spec.Template.ObjectMeta.Labels["aadpodidbinding"] = podID
	// 	fmt.Printf("@@@@ Deployment spec: %v\n", deployment)
	// }

	return &deployment, nil
}

func (r Renderer) createPodIdentity(ctx context.Context, w workloads.InstantiatedWorkload, cc *ContainerComponent, setVars map[string]string) (string, error) {

	dc := resources.NewDeploymentsClient(r.Arm.SubscriptionID)
	dc.Authorizer = r.Arm.Auth

	// TODO Use deployment templates/SDK
	// For now using az cli

	// Get AKS cluster name in current resource group
	mcc := containerservice.NewManagedClustersClient(r.Arm.SubscriptionID)
	mcc.Authorizer = r.Arm.Auth

	var cluster *containerservice.ManagedCluster
	for list, err := mcc.ListByResourceGroupComplete(ctx, r.Arm.ResourceGroup); list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return "", fmt.Errorf("cannot read AKS clusters: %w", err)
		}

		// For SOME REASON the value 'true' in a tag gets normalized to 'True'
		tag, ok := list.Value().Tags["rad-environment"]
		if ok && strings.EqualFold(*tag, "true") {
			temp := list.Value()
			cluster = &temp
			break
		}
	}

	if cluster == nil {
		return "", fmt.Errorf("could not find an AKS instance in resource group '%v'", r.Arm.ResourceGroup)
	}

	msiResourceID := setVars["MSI_ID"]
	// Note: Pod Identity name cannot have camel case
	podIdentityName := "podid-" + cc.Name
	// Note: The pod identity namespace specified here has to match the namespace in which the application pod is deployed
	out, err := azcli.RunCLICommandWithOutput("aks", "pod-identity", "add", "--resource-group", r.Arm.ResourceGroup, "--cluster-name", *cluster.Name, "--namespace", w.Application, "--name", podIdentityName, "--identity-resource-id", msiResourceID)
	if err != nil {
		return "", fmt.Errorf("unable to create pod identity")
	}
	fmt.Println("@@@@@@ created pod identity...")
	fmt.Println(string(out))
	return podIdentityName, nil
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
