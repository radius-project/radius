// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproute

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

type Renderer struct {
}

func (r Renderer) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) (radiusResourceIDs []resources.ID, resourceIDs []resources.ID, err error) {
	return nil, nil, nil
}

func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {

	route, ok := dm.(*datamodel.HTTPRoute)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}
	outputResources := []outputresource.OutputResource{}
	appId, err := resources.Parse(route.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("invalid application id: %w. id: %s", err, route.Properties.Application)
	}
	applicationName := appId.Name()

	if route.Properties.Port == 0 {
		defaultPort := kubernetes.GetDefaultPort()
		route.Properties.Port = defaultPort
	}

	computedValues := map[string]rp.ComputedValueReference{
		"hostname": {
			Value: kubernetes.MakeResourceName(applicationName, route.Name),
		},
		"port": {
			Value: route.Properties.Port,
		},
		"url": {
			Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(applicationName, route.Name), route.Properties.Port),
		},
		"scheme": {
			Value: "http",
		},
	}

	service, err := r.makeService(route, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, service)

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
	}, nil
}

func (r *Renderer) makeService(route *datamodel.HTTPRoute, options renderers.RenderOptions) (outputresource.OutputResource, error) {
	appId, err := resources.Parse(route.Properties.Application)

	if err != nil {
		return outputresource.OutputResource{}, fmt.Errorf("invalid application id: %w. id: %s", err, route.Properties.Application)
	}
	applicationName := appId.Name()

	typeParts := strings.Split(ResourceType, "/")
	resourceTypeSuffix := typeParts[len(typeParts)-1]

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(applicationName, route.Name),
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(applicationName, route.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeRouteSelectorLabels(applicationName, resourceTypeSuffix, route.Name),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       route.Name,
					Port:       route.Properties.Port,
					TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(applicationName + resourceTypeSuffix + route.Name)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.Service, outputresource.LocalIDService, service, service.ObjectMeta), nil
}
