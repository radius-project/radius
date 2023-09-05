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

package httproute

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/kubernetes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

type Renderer struct {
}

// GetDependencyIDs returns nils for the resourceIDs, radiusResourceIDs and an error.
func (r Renderer) GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) (radiusResourceIDs []resources.ID, resourceIDs []resources.ID, err error) {
	return nil, nil, nil
}

// Render checks if the DataModelInterface is a valid HTTP Route, sets default port if none is provided, creates a
// ComputedValueReference map, creates a service resource and returns a RendererOutput with the resources and
// computed values. It returns an error if the service resource creation fails.
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

	service, err := r.makeService(ctx, route, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, service)

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
	}, nil
}

func (r *Renderer) makeService(ctx context.Context, route *datamodel.HTTPRoute, options renderers.RenderOptions) (rpv1.OutputResource, error) {
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
			Name:        kubernetes.NormalizeResourceName(route.Name),
			Namespace:   options.Environment.Namespace,
			Labels:      renderers.GetLabels(options, appId.Name(), route.Name, route.ResourceTypeName()),
			Annotations: renderers.GetAnnotations(options),
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

	return rpv1.NewKubernetesOutputResource(rpv1.LocalIDService, service, service.ObjectMeta), nil
}
