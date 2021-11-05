// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"encoding/json"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/resourcemodel"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func NewRestApplicationResource(id azresources.ResourceID, input radiusv1alpha3.Application) (resourceprovider.ApplicationResource, error) {
	template := map[string]interface{}{}
	if input.Spec.Template != nil {
		err := json.Unmarshal(input.Spec.Template.Raw, &template)
		if err != nil {
			return resourceprovider.ApplicationResource{}, err
		}
	}

	properties := map[string]interface{}{}
	obj, ok := template["body"]
	if ok {
		body, ok := obj.(map[string]interface{})
		if ok {
			obj, ok := body["properties"]
			if ok {
				p, ok := obj.(map[string]interface{})
				if ok {
					properties = p
				}
			}
		}
	}

	return resourceprovider.ApplicationResource{
		ID:         id.ID,
		Type:       id.Type(),
		Name:       id.Name(),
		Properties: properties,
	}, nil
}

func NewKubernetesApplicationResource(id azresources.ResourceID, input resourceprovider.ApplicationResource) (radiusv1alpha3.Application, error) {
	properties := input.Properties
	if properties == nil {
		properties = map[string]interface{}{}
	}

	template := map[string]interface{}{
		"body": map[string]interface{}{
			"properties": properties,
		},
	}

	b, err := json.Marshal(template)
	if err != nil {
		return radiusv1alpha3.Application{}, err
	}

	return radiusv1alpha3.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "radius.dev/v1alpha3",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: id.Name(),
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Applciation: id.Name(),
			Template:    &runtime.RawExtension{Raw: b},
		},
	}, nil
}

func NewRestRadiusResource(id azresources.ResourceID, input radiusv1alpha3.Resource) (resourceprovider.RadiusResource, error) {
	template := map[string]interface{}{}
	if input.Spec.Template != nil {
		err := json.Unmarshal(input.Spec.Template.Raw, &template)
		if err != nil {
			return resourceprovider.RadiusResource{}, err
		}
	}

	properties := map[string]interface{}{}
	obj, ok := template["body"]
	if ok {
		body, ok := obj.(map[string]interface{})
		if ok {
			obj, ok := body["properties"]
			if ok {
				p, ok := obj.(map[string]interface{})
				if ok {
					properties = p
				}
			}
		}
	}

	if input.Status.ComputedValues != nil {
		computedValues := map[string]interface{}{}
		err := json.Unmarshal(input.Spec.Template.Raw, &computedValues)
		if err != nil {
			return resourceprovider.RadiusResource{}, err
		}

		for k, v := range computedValues {
			properties[k] = v
		}
	}

	resources := []rest.OutputResource{}
	for localID, r := range input.Status.Resources {
		identity := resourcemodel.ResourceIdentity{
			Kind: resourcemodel.IdentityKindKubernetes,
			Data: resourcemodel.KubernetesIdentity{
				Kind:       r.Kind,
				APIVersion: r.APIVersion,
				Name:       r.Name,
				Namespace:  input.Namespace,
			},
		}

		resources = append(resources, rest.OutputResource{
			LocalID:            localID,
			Managed:            true,
			ResourceKind:       "kubernetes",
			OutputResourceInfo: identity,
			Status: rest.OutputResourceStatus{
				ProvisioningState: rest.Provisioned,
				HealthState:       rest.Provisioned,
			},
		})
	}

	if input.Status.Phrase == "Ready" {
		properties["provisioningState"] = string(rest.SuccededStatus)
		properties["status"] = rest.ComponentStatus{
			ProvisioningState: rest.Provisioned,
			HealthState:       "Healthy",
			OutputResources:   resources,
		}
	} else {
		properties["provisioningState"] = string(rest.DeployingStatus)
		properties["status"] = rest.ComponentStatus{
			ProvisioningState: rest.Provisioning,
			HealthState:       "Pending",
			OutputResources:   resources,
		}
	}

	return resourceprovider.RadiusResource{
		ID:         id.ID,
		Type:       id.Type(),
		Name:       id.Name(),
		Properties: properties,
	}, nil
}

func NewKubernetesRadiusResource(id azresources.ResourceID, input resourceprovider.RadiusResource) (unstructured.Unstructured, error) {
	properties := input.Properties
	if properties == nil {
		properties = map[string]interface{}{}
	}

	template := map[string]interface{}{
		"body": map[string]interface{}{
			"properties": properties,
		},
	}

	b, err := json.Marshal(template)
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	return unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": kubernetes.MakeResourceName(id.Types[len(id.Types)-2].Name, id.Types[len(id.Types)-1].Name),
				"annotations": map[string]string{
					kubernetes.LabelRadiusResourceType: id.Types[len(id.Types)-1].Type,
				},
				"labels": kubernetes.MakeResourceCRDLabels(id.Types[len(id.Types)-2].Name, id.Types[len(id.Types)-1].Type, id.Types[len(id.Types)-1].Name),
			},
			"spec": radiusv1alpha3.ResourceSpec{
				Application: id.Types[len(id.Types)-2].Name,
				Resource:    id.Types[len(id.Types)-1].Name,
				Template:    &runtime.RawExtension{Raw: b},
			},
		},
	}, nil
}
