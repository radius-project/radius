// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"fmt"
	"strings"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/kubernetes"
)

func ConvertK8sApplicationToARM(input radiusv1alpha3.Application) (*radclient.ApplicationResource, error) {
	result := radclient.ApplicationResource{}
	result.Name = to.StringPtr(input.Annotations[kubernetes.LabelRadiusApplication])

	// There's nothing in properties for an application
	result.Properties = &radclient.ApplicationProperties{}

	return &result, nil
}

func ConvertK8sResourceToARM(input unstructured.Unstructured) (*radclient.RadiusResource, error) {
	objRef := fmt.Sprintf("%s/%s/%s", input.GetKind(), input.GetNamespace(), input.GetName())
	m := input.UnstructuredContent()
	template, err := mapDeepGetMap(m, "spec", "template")
	if err != nil {
		return nil, fmt.Errorf("cannot convert %s: %w", objRef, err)
	}
	name, err := mapDeepGetString(template, "name")
	if err != nil {
		return nil, fmt.Errorf("cannot convert %s: %w", objRef, err)
	}
	nameparts := strings.Split(name, "/")
	id, err := mapDeepGetString(template, "id")
	if err != nil {
		return nil, fmt.Errorf("cannot convert %s: %w", objRef, err)
	}
	resourceType, err := mapDeepGetString(template, "type")
	if err != nil {
		return nil, fmt.Errorf("cannot convert %s: %w", objRef, err)
	}
	// It is ok for "properties" to be empty
	properties, _ := mapDeepGetMap(template, "body", "properties")
	result := radclient.RadiusResource{
		ProxyResource: radclient.ProxyResource{
			Resource: radclient.Resource{
				Name: to.StringPtr(nameparts[len(nameparts)-1]),
				ID:   to.StringPtr(id),
				Type: to.StringPtr(resourceType),
			},
		},
		Properties: properties,
	}
	return &result, nil
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
