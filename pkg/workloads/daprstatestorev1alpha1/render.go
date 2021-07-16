// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var supportedStateStoreKindValues = [3]string{"any", "state.azure.tablestorage", "state.sqlserver"}

// Renderer is the WorkloadRenderer implementation for the dapr statestore workload.
type Renderer struct {
	SupportsArm        bool
	SupportsKubernetes bool
}

// Allocate is the WorkloadRenderer implementation for dapr statestore workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	bindings := map[string]components.BindingState{
		"default": {
			Component: workload.Name,
			Binding:   "default",
			Properties: map[string]interface{}{
				"stateStoreName": workload.Name,
			},
		},
	}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for dapr statestore workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.OutputResource, error) {
	component := DaprStateStoreComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.OutputResource{}, err
	}
	if r.SupportsArm {
		resourceKind := ""
		localID := ""
		if component.Config.Kind == "any" || component.Config.Kind == "state.azure.tablestorage" {
			resourceKind = workloads.ResourceKindDaprStateStoreAzureStorage
			localID = workloads.LocalIDDaprStateStoreAzureStorage
		} else if component.Config.Kind == "state.sqlserver" {
			resourceKind = workloads.ResourceKindDaprStateStoreSQLServer
			localID = workloads.LocalIDDaprStateStoreSQLServer
		} else {
			return []workloads.OutputResource{}, fmt.Errorf("%s is not supported. Supported kind values: %s", component.Config.Kind, supportedStateStoreKindValues)
		}

		if component.Config.Managed {
			if component.Config.Resource != "" {
				return nil, workloads.ErrResourceSpecifiedForManagedResource
			}

			resource := workloads.OutputResource{
				LocalID:            localID,
				ResourceKind:       resourceKind,
				OutputResourceType: workloads.OutputResourceTypeArm,
				Resource: map[string]string{
					handlers.ManagedKey:              "true",
					handlers.KubernetesNameKey:       w.Name,
					handlers.KubernetesNamespaceKey:  w.Application,
					handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
					handlers.KubernetesKindKey:       "Component",
					handlers.ComponentNameKey:        w.Name,
				},
			}

			return []workloads.OutputResource{resource}, nil
		} else {
			if component.Config.Kind == "state.sqlserver" {
				return nil, errors.New("only Radius managed resources are supported for Dapr SQL Server")
			}

			if component.Config.Resource == "" {
				return nil, workloads.ErrResourceMissingForUnmanagedResource
			}

			accountID, err := workloads.ValidateResourceID(component.Config.Resource, StorageAccountResourceType, "Storage Account")
			if err != nil {
				return nil, err
			}

			// generate data we can use to connect to a Storage Account
			resource := workloads.OutputResource{
				LocalID:            localID,
				ResourceKind:       workloads.ResourceKindDaprStateStoreAzureStorage,
				OutputResourceType: workloads.OutputResourceTypeArm,
				Resource: map[string]string{
					handlers.ManagedKey:              "false",
					handlers.KubernetesNameKey:       w.Name,
					handlers.KubernetesNamespaceKey:  w.Application,
					handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
					handlers.KubernetesKindKey:       "Component",

					handlers.StorageAccountIDKey:   accountID.ID,
					handlers.StorageAccountNameKey: accountID.Types[0].Name,
				},
			}
			return []workloads.OutputResource{resource}, nil
		}
	}

	if r.SupportsKubernetes {
		if component.Config.Kind != "any" {
			return []workloads.OutputResource{}, errors.New("only kind 'any' is supported right now")
		}
		if !component.Config.Managed {
			return []workloads.OutputResource{}, errors.New("only 'managed=true' is supported right now")
		}
		resources := []workloads.OutputResource{}
		deployment := appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: appsv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      component.Name,
				Namespace: w.Namespace,
				Labels: map[string]string{
					keys.LabelRadiusApplication: w.Application,
					keys.LabelRadiusComponent:   component.Name,
					// TODO get the component revision here...
					"app.kubernetes.io/name":       component.Name,
					"app.kubernetes.io/part-of":    w.Application,
					"app.kubernetes.io/managed-by": "radius-rp",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						keys.LabelRadiusApplication: w.Application,
						keys.LabelRadiusComponent:   component.Name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							keys.LabelRadiusApplication: w.Application,
							keys.LabelRadiusComponent:   component.Name,
							// TODO get the component revision here...
							"app.kubernetes.io/name":       component.Name,
							"app.kubernetes.io/part-of":    w.Application,
							"app.kubernetes.io/managed-by": "radius-rp",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "redis",
								Image: "redis",
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 6379,
									},
								},
							},
						},
					},
				},
			},
		}
		resources = append(resources, workloads.NewKubernetesResource("RedisDeployment", &deployment))

		service := corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      component.Name,
				Namespace: w.Namespace,
				Labels: map[string]string{
					keys.LabelRadiusApplication: w.Application,
					keys.LabelRadiusComponent:   component.Name,
					// TODO get the component revision here...
					"app.kubernetes.io/name":       component.Name,
					"app.kubernetes.io/part-of":    w.Application,
					"app.kubernetes.io/managed-by": "radius-rp",
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					keys.LabelRadiusApplication: w.Application,
					keys.LabelRadiusComponent:   component.Name,
				},
				Type: corev1.ServiceTypeClusterIP,
				Ports: []corev1.ServicePort{
					{
						Name:       "redis",
						Port:       6379,
						TargetPort: intstr.FromInt(6379),
						Protocol:   corev1.ProtocolTCP,
					},
				},
			},
		}
		resources = append(resources, workloads.NewKubernetesResource("RedisService", &service))

		statestore := unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "dapr.io/v1alpha1",
				"kind":       "Component",
				"metadata": map[string]interface{}{
					"name":      component.Name,
					"namespace": w.Namespace,
					"labels": map[string]string{
						keys.LabelRadiusApplication: w.Application,
						keys.LabelRadiusComponent:   component.Name,
						// TODO get the component revision here...
						"app.kubernetes.io/name":       component.Name,
						"app.kubernetes.io/part-of":    w.Application,
						"app.kubernetes.io/managed-by": "radius-rp",
					},
				},
				"spec": map[string]interface{}{
					"type":    "state.redis",
					"version": "v1",
					"metadata": []interface{}{
						map[string]interface{}{
							"name":  "redisHost",
							"value": fmt.Sprintf("%s.%s.svc.cluster.local:6379", component.Name, w.Namespace),
						},
						map[string]interface{}{
							"name":  "redisPassword",
							"value": "",
						},
					},
				},
			},
		}
		resources = append(resources, workloads.NewKubernetesResource("StateStore", &statestore))

		return resources, nil
	}
	return []workloads.OutputResource{}, errors.New("must support either kubernetes or ARM")
}
