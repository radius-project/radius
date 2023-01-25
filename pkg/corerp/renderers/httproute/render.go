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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

type Renderer struct {
}

func (r Renderer) GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) (radiusResourceIDs []resources.ID, resourceIDs []resources.ID, err error) {
	return nil, nil, nil
}

func (r Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	route, ok := dm.(*datamodel.HTTPRoute)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}
	outputResources := []rpv1.OutputResource{}

	if route.Properties.Port == 0 {
		route.Properties.Port = renderers.DefaultPort
	}

	computedValues := map[string]rpv1.ComputedValueReference{
		"hostname": {
			Value: kubernetes.NormalizeResourceName(route.Name),
		},
		"port": {
			Value: route.Properties.Port,
		},
		"url": {
			Value: fmt.Sprintf("http://%s:%d", kubernetes.NormalizeResourceName(route.Name), route.Properties.Port),
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

func (r *Renderer) makeService(route *datamodel.HTTPRoute, options renderers.RenderOptions) (rpv1.OutputResource, error) {
	appId, err := resources.ParseResource(route.Properties.Application)
	if err != nil {
		return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid application id: %s. id: %s", err.Error(), route.Properties.Application))
	}

	typeParts := strings.Split(ResourceType, "/")
	resourceTypeSuffix := typeParts[len(typeParts)-1]

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.NormalizeResourceName(route.Name),
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(appId.Name(), route.Name, route.ResourceTypeName()),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeRouteSelectorLabels(appId.Name(), resourceTypeSuffix, route.Name),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       route.Name,
					Port:       route.Properties.Port,
					TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(resourceTypeSuffix + route.Name)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return rpv1.NewKubernetesOutputResource(resourcekinds.Service, rpv1.LocalIDService, service, service.ObjectMeta), nil
}
