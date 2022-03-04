// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenderv1alpha3

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ renderers.Renderer = (*AzureRenderer)(nil)

type KubernetesRenderer struct {
}

type KubernetesOptions struct {
	DescriptiveLabels map[string]string
	SelectorLabels    map[string]string
	Namespace         string
	Name              string
}

func (r *KubernetesRenderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *KubernetesRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.ExtenderProperties{}
	err := options.Resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues := map[string]renderers.ComputedValueReference{}
	for k, v := range properties.Properties {
		computedValues[k] = renderers.ComputedValueReference{
			Value: v,
		}
	}

	secretValues := map[string]renderers.SecretValueReference{}
	for k := range properties.Secrets {
		secretValues[k] = renderers.SecretValueReference{
			LocalID:       outputresource.LocalIDScrapedSecret,
			ValueSelector: k,
		}
	}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{},
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func (r KubernetesRenderer) MakeSecrets(options KubernetesOptions, secrets map[string]interface{}) *corev1.Secret {
	secretData := make(map[string][]byte)
	for k, v := range secrets {
		secretData[k] = []byte(v.(string))
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: options.Namespace,
			Labels:    options.DescriptiveLabels,
		},
		Type: corev1.SecretTypeOpaque,

		Data: secretData,
	}
}
