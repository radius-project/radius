/*
Copyright 2026 The Radius Authors.

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
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
)

type aggregationServingInventory struct {
	targets  []ServingCertificateTarget
	versions []string
}

func readAggregationServingInventory(ctx context.Context, kube kubernetes.Interface, namespace string) (aggregationServingInventory, error) {
	var inventory aggregationServingInventory
	if err := ctx.Err(); err != nil {
		return inventory, err
	}
	pin := func(kind string, object metav1.Object) error {
		if object.GetUID() == "" || object.GetResourceVersion() == "" || object.GetDeletionTimestamp() != nil {
			return fmt.Errorf("%s %s has no live identity", kind, object.GetName())
		}
		inventory.versions = append(inventory.versions, fmt.Sprintf("%s/%s/%s@%s", kind, object.GetName(), object.GetUID(), object.GetResourceVersion()))
		return nil
	}
	source, err := kube.CoreV1().Secrets(namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if err != nil {
		return inventory, err
	}
	if err := publicAggregationSource(source, namespace); err != nil {
		return inventory, err
	}
	if _, err := validatePublicCA(source.Data[caCertificateKey], time.Now()); err != nil {
		return inventory, err
	}
	if err := pin("Secret", source); err != nil {
		return inventory, err
	}
	service, err := kube.CoreV1().Services(namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return inventory, err
	}
	if err := pin("Service", service); err != nil {
		return inventory, err
	}
	if len(service.Spec.Selector) == 0 || service.Spec.PublishNotReadyAddresses || service.Spec.Type == corev1.ServiceTypeExternalName {
		return inventory, errors.New("aggregation requires a Ready-pod selecting Service")
	}
	port := int32(0)
	portName := ""
	for _, entry := range service.Spec.Ports {
		if entry.Port == 443 && entry.Protocol == corev1.ProtocolTCP && entry.TargetPort.Type == intstr.Int {
			port, portName = entry.TargetPort.IntVal, entry.Name
		}
	}
	if port <= 0 || port > 65535 || portName == "" {
		return inventory, errors.New("aggregation Service requires a pinned named HTTPS port with a numeric target")
	}
	deployment, err := kube.AppsV1().Deployments(namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return inventory, err
	}
	if err := pin("Deployment", deployment); err != nil {
		return inventory, err
	}
	count := int32(1)
	if deployment.Spec.Replicas != nil {
		count = *deployment.Spec.Replicas
	}
	if count < 1 || deployment.Spec.Paused || deployment.Generation <= 0 || deployment.Status.ObservedGeneration != deployment.Generation ||
		deployment.Status.Replicas != count || deployment.Status.UpdatedReplicas != count ||
		deployment.Status.ReadyReplicas != count || deployment.Status.AvailableReplicas != count {
		return inventory, errors.New("aggregation deployment has not converged")
	}
	account, err := kube.CoreV1().ServiceAccounts(namespace).Get(ctx, deployment.Spec.Template.Spec.ServiceAccountName, metav1.GetOptions{})
	if err != nil {
		return inventory, err
	}
	if err := pin("ServiceAccount", account); err != nil {
		return inventory, err
	}
	pods, err := kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(service.Spec.Selector).String(), Limit: 1000})
	if err != nil {
		return inventory, err
	}
	if pods.Continue != "" || int32(len(pods.Items)) != count {
		return inventory, errors.New("aggregation selected-pod inventory is incomplete or still rolling")
	}
	selected := map[types.UID]*corev1.Pod{}
	for index := range pods.Items {
		pod := &pods.Items[index]
		if err := pin("Pod", pod); err != nil {
			return inventory, err
		}
		if selected[pod.UID] != nil {
			return inventory, errors.New("aggregation pod identity is duplicated")
		}
		selected[pod.UID] = pod
		inventory.targets = append(inventory.targets, ServingCertificateTarget{
			TLSProofTarget: TLSProofTarget{Namespace: namespace, PodName: pod.Name, PodUID: pod.UID, ContainerName: "ucp", Port: uint16(port)},
			WorkloadUID:    deployment.UID, WorkloadGeneration: deployment.Generation,
			ServiceAccount: account.Name, ServiceAccountUID: account.UID,
			CA: source.Data[caCertificateKey], DNSName: "ucp." + namespace + ".svc",
		})
	}
	endpoints, err := kube.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{discoveryv1.LabelServiceName: service.Name}).String(), Limit: 1000})
	if err != nil {
		return inventory, err
	}
	if endpoints.Continue != "" || len(endpoints.Items) == 0 {
		return inventory, errors.New("aggregation endpoint inventory is absent or incomplete")
	}
	routed := map[types.UID]bool{}
	for index := range endpoints.Items {
		slice := &endpoints.Items[index]
		if err := pin("EndpointSlice", slice); err != nil {
			return inventory, err
		}
		owner := metav1.GetControllerOf(slice)
		if owner == nil || owner.APIVersion != "v1" || owner.Kind != "Service" || owner.Name != service.Name || owner.UID != service.UID {
			return inventory, errors.New("aggregation endpoints have foreign Service ownership")
		}
		if !slices.ContainsFunc(slice.Ports, func(value discoveryv1.EndpointPort) bool {
			return value.Name != nil && *value.Name == portName && value.Port != nil && *value.Port == port &&
				value.Protocol != nil && *value.Protocol == corev1.ProtocolTCP
		}) {
			return inventory, errors.New("aggregation endpoint port differs from the Service target")
		}
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready == nil || !*endpoint.Conditions.Ready ||
				endpoint.Conditions.Terminating != nil && *endpoint.Conditions.Terminating {
				continue
			}
			ref := endpoint.TargetRef
			if ref == nil || ref.Kind != "Pod" || ref.Namespace != namespace || selected[ref.UID] == nil ||
				selected[ref.UID].Name != ref.Name || len(endpoint.Addresses) == 0 {
				return inventory, errors.New("aggregation routes to an unobserved pod")
			}
			for _, address := range endpoint.Addresses {
				if !slices.ContainsFunc(selected[ref.UID].Status.PodIPs, func(ip corev1.PodIP) bool { return ip.IP == address }) {
					return inventory, errors.New("aggregation endpoint address differs from the observed pod")
				}
			}
			routed[ref.UID] = true
		}
	}
	if len(routed) != len(selected) {
		return inventory, errors.New("aggregation endpoints do not include every selected pod")
	}
	slices.Sort(inventory.versions)
	slices.SortFunc(inventory.targets, func(a, b ServingCertificateTarget) int {
		if a.PodName < b.PodName {
			return -1
		}
		if a.PodName > b.PodName {
			return 1
		}
		return 0
	})
	return inventory, ctx.Err()
}

func observeAggregationServing(ctx context.Context, kubeContext, namespace string, kube kubernetes.Interface) error {
	ctx, cancel := context.WithTimeout(ctx, ucpAPIServiceTimeout)
	defer cancel()
	before, err := readAggregationServingInventory(ctx, kube, namespace)
	if err != nil {
		return err
	}
	flags := genericclioptions.NewConfigFlags(false)
	flags.Context, flags.Namespace = &kubeContext, &namespace
	config, err := flags.ToRESTConfig()
	if err != nil {
		return err
	}
	for _, target := range before.targets {
		if _, err := ObserveServingCertificate(ctx, config, target); err != nil {
			return fmt.Errorf("aggregation serving pod %s: %w", target.PodName, err)
		}
	}
	after, err := readAggregationServingInventory(ctx, kube, namespace)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("aggregation serving inventory changed during observation")
	}
	return nil
}
