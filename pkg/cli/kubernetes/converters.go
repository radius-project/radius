// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"fmt"
	"strings"

	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/azure/radclientv3"
	"github.com/Azure/radius/pkg/kubernetes"
)

func ConvertK8sApplicationToARM(input radiusv1alpha3.Application) (*radclient.ApplicationResource, error) {
	result := radclient.ApplicationResource{}
	result.Name = to.StringPtr(input.Annotations[kubernetes.AnnotationsApplication])

	// There's nothing in properties for an application
	result.Properties = &radclient.ApplicationProperties{}

	return &result, nil
}

func ConvertK8sApplicationToARMV3(input radiusv1alpha3.Application) (*radclientv3.ApplicationResource, error) {
	result := radclientv3.ApplicationResource{}
	result.Name = to.StringPtr(input.Annotations[kubernetes.AnnotationsApplication])

	// There's nothing in properties for an application
	result.Properties = &radclientv3.ApplicationProperties{}

	return &result, nil
}

func ConvertK8sResourceToARM(input radiusv1alpha3.Resource) (*radclient.ComponentResource, error) {
	result := radclient.ComponentResource{}

	// TODO fix once we deal with client simplification and have RP changes
	return &result, nil
}

func ConvertK8sResourceToARMV3(input unstructured.Unstructured) (*radclientv3.RadiusResource, error) {
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
	result := radclientv3.RadiusResource{
		ProxyResource: radclientv3.ProxyResource{
			Resource: radclientv3.Resource{
				Name: to.StringPtr(nameparts[len(nameparts)-1]),
				ID:   to.StringPtr(id),
				Type: to.StringPtr(resourceType),
			},
		},
		Properties: properties,
	}
	return &result, nil
}

func ConvertK8sDeploymentToARM(input bicepv1alpha3.DeploymentTemplate) (*radclient.DeploymentResource, error) {
	result := radclient.DeploymentResource{}
	result.Properties = &radclient.DeploymentProperties{}

	// TODO remove once we deal with client simplification and have RP changes
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
