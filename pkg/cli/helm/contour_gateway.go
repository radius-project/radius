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

package helm

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/kubeutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	ContourGatewayClassName        = "contour"
	ContourGatewayControllerName   = "projectcontour.io/gateway-controller"
	DefaultContourGatewayName      = "radius"
	DefaultContourGatewayNamespace = RadiusSystemNamespace

	radiusManagedByLabel = "app.kubernetes.io/managed-by"
	radiusPartOfLabel    = "app.kubernetes.io/part-of"
	radiusManagedValue   = "radius"
)

var (
	gatewayClassGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gatewayclasses",
	}
	gatewayGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gateways",
	}
)

// ContourGatewayReconciler manages the default Gateway API resources used by
// Radius.Compute/routes when Contour is installed by Radius.
type ContourGatewayReconciler interface {
	Reconcile(ctx context.Context, kubeContext string) error
	Delete(ctx context.Context, kubeContext string) error
}

type DynamicContourGatewayReconciler struct {
	Client dynamic.Interface
}

func NewContourGatewayReconciler() *DynamicContourGatewayReconciler {
	return &DynamicContourGatewayReconciler{}
}

func (r *DynamicContourGatewayReconciler) Reconcile(ctx context.Context, kubeContext string) error {
	client, err := r.client(kubeContext)
	if err != nil {
		return err
	}

	if err := reconcileGatewayClass(ctx, client); err != nil {
		return err
	}

	return reconcileGateway(ctx, client)
}

func (r *DynamicContourGatewayReconciler) Delete(ctx context.Context, kubeContext string) error {
	client, err := r.client(kubeContext)
	if err != nil {
		return err
	}

	if err := deleteManagedResource(ctx, client.Resource(gatewayGVR).Namespace(DefaultContourGatewayNamespace), DefaultContourGatewayName); err != nil {
		return err
	}

	return deleteManagedResource(ctx, client.Resource(gatewayClassGVR), ContourGatewayClassName)
}

func (r *DynamicContourGatewayReconciler) client(kubeContext string) (dynamic.Interface, error) {
	if r.Client != nil {
		return r.Client, nil
	}

	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: kubeContext,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func reconcileGatewayClass(ctx context.Context, client dynamic.Interface) error {
	resource := client.Resource(gatewayClassGVR)
	existing, err := resource.Get(ctx, ContourGatewayClassName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, newContourGatewayClass(), metav1.CreateOptions{})
		return err
	} else if err != nil {
		return err
	}

	controllerName, _, _ := unstructured.NestedString(existing.Object, "spec", "controllerName")
	if controllerName != ContourGatewayControllerName {
		return fmt.Errorf("GatewayClass %q already exists with controllerName %q, expected %q", ContourGatewayClassName, controllerName, ContourGatewayControllerName)
	}

	return nil
}

func reconcileGateway(ctx context.Context, client dynamic.Interface) error {
	resource := client.Resource(gatewayGVR).Namespace(DefaultContourGatewayNamespace)
	existing, err := resource.Get(ctx, DefaultContourGatewayName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, newContourGateway(), metav1.CreateOptions{})
		return err
	} else if err != nil {
		return err
	}

	if !isRadiusManaged(existing) {
		return fmt.Errorf("Gateway %q in namespace %q already exists and is not managed by Radius", DefaultContourGatewayName, DefaultContourGatewayNamespace)
	}

	gateway := newContourGateway()
	gateway.SetResourceVersion(existing.GetResourceVersion())
	_, err = resource.Update(ctx, gateway, metav1.UpdateOptions{})
	return err
}

func deleteManagedResource(ctx context.Context, resource dynamic.ResourceInterface, name string) error {
	existing, err := resource.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	if !isRadiusManaged(existing) {
		return nil
	}

	err = resource.Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func newContourGatewayClass() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GatewayClass",
			"metadata": map[string]any{
				"name":   ContourGatewayClassName,
				"labels": radiusManagedLabels(),
			},
			"spec": map[string]any{
				"controllerName": ContourGatewayControllerName,
			},
		},
	}
}

func newContourGateway() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":      DefaultContourGatewayName,
				"namespace": DefaultContourGatewayNamespace,
				"labels":    radiusManagedLabels(),
			},
			"spec": map[string]any{
				"gatewayClassName": ContourGatewayClassName,
				"listeners": []any{
					map[string]any{
						"name":     "http",
						"protocol": "HTTP",
						"port":     int64(80),
						"allowedRoutes": map[string]any{
							"namespaces": map[string]any{
								"from": "All",
							},
						},
					},
				},
			},
		},
	}
}

func radiusManagedLabels() map[string]any {
	return map[string]any{
		radiusManagedByLabel: radiusManagedValue,
		radiusPartOfLabel:    radiusManagedValue,
	}
}

func isRadiusManaged(resource *unstructured.Unstructured) bool {
	labels := resource.GetLabels()
	return labels[radiusManagedByLabel] == radiusManagedValue && labels[radiusPartOfLabel] == radiusManagedValue
}
