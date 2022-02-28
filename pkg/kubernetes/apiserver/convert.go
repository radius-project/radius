// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/project-radius/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/project-radius/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/resourcemodel"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
)

// Converts K8s Application to a REST Application
func NewRestApplicationResource(id azresources.ResourceID, input radiusv1alpha3.Application, status rest.ApplicationStatus) (resourceprovider.ApplicationResource, error) {
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

	properties["status"] = status

	return resourceprovider.ApplicationResource{
		ID:         id.ID,
		Type:       id.Type(),
		Name:       id.Name(),
		Properties: properties,
	}, nil
}

// Converts REST Application to a K8s Application
func NewKubernetesApplicationResource(id azresources.ResourceID, input resourceprovider.ApplicationResource, namespace string) (radiusv1alpha3.Application, error) {
	properties := input.Properties
	var raw *runtime.RawExtension
	if len(properties) == 0 {
		raw = nil
	} else {
		template := map[string]interface{}{
			"body": map[string]interface{}{
				"properties": properties,
			},
		}

		b, err := json.Marshal(template)
		if err != nil {
			return radiusv1alpha3.Application{}, err
		}
		raw = &runtime.RawExtension{Raw: b}
	}

	return radiusv1alpha3.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "radius.dev/v1alpha3",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      id.Name(),
			Namespace: namespace,
			Annotations: map[string]string{
				kubernetes.LabelRadiusApplication: id.Name(),
			},
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Application: id.Name(),
			Template:    raw,
		},
		Status: radiusv1alpha3.ApplicationStatus{},
	}, nil
}

// Converts K8s Resource to a REST Resource
func NewRestRadiusResource(input radiusv1alpha3.Resource) (resourceprovider.RadiusResource, error) {
	unstMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&input)
	if err != nil {
		return resourceprovider.RadiusResource{}, err
	}
	unst := unstructured.Unstructured{Object: unstMap}
	return NewRestRadiusResourceFromUnstructured(unst)
}

func NewRestRadiusResourceFromUnstructured(input unstructured.Unstructured) (resourceprovider.RadiusResource, error) {
	objRef := fmt.Sprintf("%s/%s/%s", input.GetKind(), input.GetNamespace(), input.GetName())
	m := input.UnstructuredContent()
	template, err := mapDeepGetMap(m, "spec", "template")
	if err != nil {
		return resourceprovider.RadiusResource{}, fmt.Errorf("cannot convert %s: %w", objRef, err)
	}
	name, err := mapDeepGetString(template, "name")
	if err != nil {
		return resourceprovider.RadiusResource{}, fmt.Errorf("cannot convert %s: %w", objRef, err)
	}
	nameparts := strings.Split(name, "/")
	id, err := mapDeepGetString(template, "id")
	if err != nil {
		return resourceprovider.RadiusResource{}, fmt.Errorf("cannot convert %s: %w", objRef, err)
	}
	resourceType, err := mapDeepGetString(template, "type")
	if err != nil {
		return resourceprovider.RadiusResource{}, fmt.Errorf("cannot convert %s: %w", objRef, err)
	}
	// It is ok for "properties" to be empty
	properties, _ := mapDeepGetMap(template, "body", "properties")
	if properties == nil {
		properties = make(map[string]interface{})
	}

	if statusMap, ok := m["status"]; ok {
		// Check if there any resources
		if _, ok := statusMap.(map[string]interface{})["resources"]; !ok {
			properties["status"] = map[string]interface{}{}
		} else {
			outputResources, err := mapDeepGetMap(m, "status", "resources")
			if err != nil {
				return resourceprovider.RadiusResource{}, fmt.Errorf("cannot convert %s: %w", objRef, err)
			}

			status, err := NewRestRadiusResourceStatus(objRef, outputResources)
			if err != nil {
				return resourceprovider.RadiusResource{}, fmt.Errorf("cannot convert %s: %w", objRef, err)
			}
			properties["status"] = status
		}

		// Merge computed values
		if _, ok := statusMap.(map[string]interface{})["computedValues"]; ok {
			computedValues, err := mapDeepGetMap(m, "status", "computedValues")
			if err != nil {
				return resourceprovider.RadiusResource{}, fmt.Errorf("cannot convert %s: %w", objRef, err)
			}

			for k := range computedValues {
				value, err := mapDeepGet(computedValues, k, "Value")
				if err != nil {
					return resourceprovider.RadiusResource{}, fmt.Errorf("cannot convert %s: %w", objRef, err)
				}

				properties[k] = value
			}
		}
	}

	result := resourceprovider.RadiusResource{
		Name:       nameparts[len(nameparts)-1],
		ID:         id,
		Type:       resourceType,
		Properties: properties,
	}
	return result, nil
}

func NewRestOutputResources(objRef string, original map[string]interface{}) ([]rest.OutputResource, error) {
	rrs := []rest.OutputResource{}
	for id, r := range original {
		o := r.(map[string]interface{})

		status, err := mapDeepGetMap(o, "status")
		if err != nil {
			// Skipping status since it is an optional field
			rr := rest.OutputResource{
				LocalID:            id,
				OutputResourceType: string(resourcemodel.IdentityKindKubernetes),
				Status: rest.OutputResourceStatus{
					HealthState:        healthcontract.HealthStateUnknown,
					HealthErrorDetails: "Status not found",
				},
			}
			rrs = append(rrs, rr)
		}

		// Ignoring err intentionally here since these fields might be empty and therefore omitted
		healthState, err := mapDeepGetString(status, "healthState")
		if err != nil {
			healthState = healthcontract.HealthStateUnknown
		}
		healthStateErrorDetails, _ := mapDeepGetString(status, "healthStateErrorDetails")
		provisioningState, err := mapDeepGetString(status, "provisioningState")
		if err != nil {
			provisioningState = kubernetes.ProvisioningStateNotProvisioned
		}
		provisioningStateErrorDetails, _ := mapDeepGetString(status, "provisioningStateErrorDetails")

		rr := rest.OutputResource{
			LocalID:            id,
			OutputResourceType: string(resourcemodel.IdentityKindKubernetes),
			Status: rest.OutputResourceStatus{
				HealthState:              healthState,
				HealthErrorDetails:       healthStateErrorDetails,
				ProvisioningState:        provisioningState,
				ProvisioningErrorDetails: provisioningStateErrorDetails,
			},
		}
		rrs = append(rrs, rr)
	}
	return rrs, nil
}

func NewRestRadiusResourceStatus(objRef string, ors map[string]interface{}) (rest.ResourceStatus, error) {
	restOutputResources, err := NewRestOutputResources(objRef, ors)
	if err != nil {
		return rest.ResourceStatus{}, err
	}

	// Aggregate the resource status
	aggregateHealthState, aggregateHealthStateErrorDetails := rest.GetUserFacingResourceHealthState(restOutputResources)
	aggregateProvisiongState := rest.GetUserFacingResourceProvisioningState(restOutputResources)

	status := rest.ResourceStatus{
		ProvisioningState:  aggregateProvisiongState,
		HealthState:        aggregateHealthState,
		HealthErrorDetails: aggregateHealthStateErrorDetails,
		OutputResources:    restOutputResources,
	}
	return status, nil
}

func mapDeepGetMap(input map[string]interface{}, fields ...string) (map[string]interface{}, error) {
	i, err := mapDeepGet(input, fields...)
	if err != nil {
		return nil, err
	}
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s is not a map, but %T", strings.Join(fields, "."), i)
	}
	return m, nil
}

func mapDeepGetString(input map[string]interface{}, fields ...string) (string, error) {
	i, err := mapDeepGet(input, fields...)
	if err != nil {
		return "", err
	}
	s, ok := i.(string)
	if !ok {
		return "", fmt.Errorf("%s is not a string, but %T", strings.Join(fields, "."), i)
	}
	return s, nil
}

func mapDeepGet(input map[string]interface{}, fields ...string) (interface{}, error) {
	var obj interface{} = input
	for i, field := range fields {
		m, ok := obj.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s is not map", strings.Join(fields[:i], "."))
		}
		obj, ok = m[field]
		if !ok {
			return nil, fmt.Errorf("cannot find %s", strings.Join(fields[:i+1], "."))
		}
	}
	return obj, nil
}

// NewKubernetesRadiusResource converts a radius resource to a kubernetes resource.
// Ignores the status field.
func NewKubernetesRadiusResource(id azresources.ResourceID, input resourceprovider.RadiusResource, namespace string, gvk k8sschema.GroupVersionKind) (unstructured.Unstructured, map[string]string, error) {
	properties := input.Properties
	if properties == nil {
		properties = map[string]interface{}{}
	}

	template := map[string]interface{}{
		"name": id.Types[len(id.Types)-1].Name,
		"id":   id.ID,
		"type": id.Types[len(id.Types)-1].Type,
		"body": map[string]interface{}{
			"properties": properties,
		},
	}

	// In the conversion we remove secrets from the resource body and store them
	// as a separate secret object.
	secrets := map[string]string{}
	obj, hasSecrets := properties["secrets"]
	if hasSecrets {
		rawSecrets, rightType := obj.(map[string]interface{})
		if !rightType {
			return unstructured.Unstructured{}, nil, fmt.Errorf("the secrets field should be a map, got %T", obj)
		}

		for k, v := range rawSecrets {
			str, isString := v.(string)
			if !isString {
				return unstructured.Unstructured{}, nil, fmt.Errorf("secrets must be strings, got %T", v)
			}
			secrets[k] = str
			// Redact the secrets but keep original keys in the template
			rawSecrets[k] = "***"
		}

		// Redact the secrets
		delete(properties, "secrets")
	}

	b, err := json.Marshal(template)
	if err != nil {
		return unstructured.Unstructured{}, nil, err
	}

	application := id.Types[len(id.Types)-2].Name

	unst := unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":              kubernetes.MakeResourceName(application, id.Types[len(id.Types)-1].Name),
				"annotations":       kubernetes.MakeResourceCRDLabels(application, id.Types[len(id.Types)-1].Type, id.Types[len(id.Types)-1].Name),
				"namespace":         namespace,
				"creationTimestamp": nil,
				"labels":            kubernetes.MakeResourceCRDLabels(application, id.Types[len(id.Types)-1].Type, id.Types[len(id.Types)-1].Name),
			},
			"spec": radiusv1alpha3.ResourceSpec{
				Application: application,
				Resource:    id.Types[len(id.Types)-1].Name,
				Template:    &runtime.RawExtension{Raw: b},
			},
			"status": map[string]interface{}{},
		},
	}

	unst.SetGroupVersionKind(gvk)
	return unst, secrets, err
}
