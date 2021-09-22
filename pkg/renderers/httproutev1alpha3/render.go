package httproutev1alpha3

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
)

type Renderer struct {
}

// Need a step to take rendered routes to be usable by component
func (r Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, w renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	// This should return a service as an output resource
	outputResources := []outputresource.OutputResource{}

	route := &HttpRoute{}
	err := w.AsRequired(Kind, route)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	service := &corev1.Service{
		TypeMeta: v1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.ResourceName,
			Namespace: w.ApplicationName, // TODO why is this a different namespace
			Labels:    kubernetes.MakeDescriptiveLabels(w.ApplicationName, w.ResourceName),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeSelectorLabels(w.ApplicationName, w.ResourceName),
			Type:     corev1.ServiceTypeClusterIP,
			Ports:    []corev1.ServicePort{},
		},
	}

	// TODO set protocol and host
	port := corev1.ServicePort{
		Name:       w.ResourceName,
		Port:       int32(route.GetEffectivePort()),
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(route.ResourceID.ID)),
		Protocol:   corev1.ProtocolTCP,
	}

	service.Spec.Ports = append(service.Spec.Ports, port)

	res := outputresource.OutputResource{
		Kind:     resourcekinds.Kubernetes,
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
	}

	computedValues := map[string]renderers.ComputedValue{ // TODO make this accept jsonpointer
		"host": {
			LocalID: outputresource.LocalIDService,
			Value:   w.ResourceName, // TODO the url isn't stored on the output resource atm?
		},
		"port": {
			LocalID: outputresource.LocalIDService,
			Value:   route.GetEffectivePort(), // TODO the url isn't stored on the output resource atm?
		},
		"url": {
			LocalID: outputresource.LocalIDService,
			Value:   fmt.Sprintf("http://%s:%d", w.ResourceName, route.GetEffectivePort()), // TODO the url isn't stored on the output resource atm?
		},
		"scheme": {
			LocalID: outputresource.LocalIDService,
			Value:   "http", // TODO the url isn't stored on the output resource atm?
		},
	}

	outputResources = append(outputResources, res)

	return renderers.RendererOutput{Resources: outputResources, ComputedValues: computedValues}, nil
}
