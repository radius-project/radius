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
	"time"

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/kubeutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

const (
	ContourGatewayClassName        = "contour"
	ContourGatewayControllerName   = "projectcontour.io/gateway-controller"
	DefaultContourGatewayName      = "radius"
	DefaultContourGatewayNamespace = RadiusSystemNamespace

	radiusManagedValue = "radius"

	defaultContourGatewayRetryInterval = 2 * time.Second
	defaultContourGatewayRetryTimeout  = 2 * time.Minute
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

func ensureDefaultContourGateway(ctx context.Context, kubeContext string) error {
	client, err := newDynamicClient(kubeContext)
	if err != nil {
		return err
	}

	return waitForDefaultContourGateway(ctx, client, defaultContourGatewayRetryInterval, defaultContourGatewayRetryTimeout)
}

func waitForDefaultContourGateway(ctx context.Context, client dynamic.Interface, interval time.Duration, timeout time.Duration) error {
	var lastErr error
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		err := reconcileDefaultContourGateway(ctx, client)
		if err == nil {
			return true, nil
		}

		if isRetryableGatewayAPIError(err) {
			lastErr = err
			return false, nil
		}

		return false, err
	})
	if err != nil && lastErr != nil {
		return fmt.Errorf("timed out waiting for Gateway API resources to become available: %w", lastErr)
	}

	return err
}

func isRetryableGatewayAPIError(err error) bool {
	return apierrors.IsNotFound(err)
}

func reconcileDefaultContourGateway(ctx context.Context, client dynamic.Interface) error {
	if err := reconcileGatewayClass(ctx, client); err != nil {
		return err
	}

	return reconcileGateway(ctx, client)
}

func deleteDefaultContourGateway(ctx context.Context, kubeContext string) error {
	client, err := newDynamicClient(kubeContext)
	if err != nil {
		return err
	}

	return deleteDefaultContourGatewayResources(ctx, client)
}

func deleteDefaultContourGatewayResources(ctx context.Context, client dynamic.Interface) error {
	if err := deleteManagedResource(ctx, client.Resource(gatewayGVR).Namespace(DefaultContourGatewayNamespace), DefaultContourGatewayName); err != nil {
		return err
	}

	return deleteManagedResource(ctx, client.Resource(gatewayClassGVR), ContourGatewayClassName)
}

func newDynamicClient(kubeContext string) (dynamic.Interface, error) {
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
		return fmt.Errorf("gatewayClass %q already exists with controllerName %q, expected %q", ContourGatewayClassName, controllerName, ContourGatewayControllerName)
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
		return fmt.Errorf("gateway %q in namespace %q already exists and is not managed by Radius", DefaultContourGatewayName, DefaultContourGatewayNamespace)
	}

	gatewayLabels := existing.GetLabels()
	if gatewayLabels == nil {
		gatewayLabels = map[string]string{}
	}
	gatewayLabels[kubernetes.LabelManagedBy] = radiusManagedValue
	gatewayLabels[kubernetes.LabelPartOf] = radiusManagedValue
	existing.SetLabels(gatewayLabels)

	desired := newContourGateway()
	if err := unstructured.SetNestedField(existing.Object, desired.Object["spec"], "spec"); err != nil {
		return err
	}

	_, err = resource.Update(ctx, existing, metav1.UpdateOptions{})
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
					map[string]any{
						"name":     "https",
						"protocol": "HTTPS",
						"port":     int64(443),
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
		kubernetes.LabelManagedBy: radiusManagedValue,
		kubernetes.LabelPartOf:    radiusManagedValue,
	}
}

func isRadiusManaged(resource *unstructured.Unstructured) bool {
	labels := resource.GetLabels()
	return labels[kubernetes.LabelManagedBy] == radiusManagedValue && labels[kubernetes.LabelPartOf] == radiusManagedValue
}
