package httproutev1alpha3

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/model/resourcesv1alpha3"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloadsv1alpha3"
)

type Renderer struct {
}

func (r Renderer) ProvideBindings(ctx context.Context, workload workloadsv1alpha3.InstantiatedWorkload, resources []workloadsv1alpha3.WorkloadResourceProperties) (map[string]resourcesv1alpha3.BindingState, error) {
	return nil, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, w workloadsv1alpha3.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	// This should return a service as an output resource
	outputResources := []outputresource.OutputResource{}

	route := &HttpRoute{}
	err := w.Workload.AsRequired(Kind, route)
	if err != nil {
		return nil, err
	}

	service := &corev1.Service{
		TypeMeta: v1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name,
			Namespace: w.Application, // TODO why is this a different namespace
			Labels:    kubernetes.MakeDescriptiveLabels(w.Application, w.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeSelectorLabels(w.Application, w.Name),
			Type:     corev1.ServiceTypeClusterIP,
			Ports:    []corev1.ServicePort{},
		},
	}

	// TODO set protocol and host
	port := corev1.ServicePort{
		Name:       w.Name,
		Port:       int32(route.GetEffectivePort()),
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(route.ID)),
		Protocol:   corev1.ProtocolTCP,
	}

	service.Spec.Ports = append(service.Spec.Ports, port)

	res := outputresource.OutputResource{
		Kind:     outputresource.KindKubernetes,
		LocalID:  outputresource.LocalIDService,
		Deployed: false,
		Managed:  true,
		Type:     outputresource.TypeKubernetes,
		Info: outputresource.K8sInfo{
			Kind:       service.TypeMeta.Kind,
			APIVersion: service.TypeMeta.APIVersion,
			Name:       service.ObjectMeta.Name,
			Namespace:  service.ObjectMeta.Namespace,
		},
		Resource: service,
		AdditionalProperties: map[string]interface{}{ // TODO make this accept jsonpointer
			"host":   w.Name,
			"port":   fmt.Sprint(route.GetEffectivePort()),
			"url":    "",
			"scheme": "http",
		},
	}

	outputResources = append(outputResources, res)

	return outputResources, nil
}
