// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/healthcontract"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/resourcemodel"
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

	if _, ok := m["status"]; ok {
		outputResources, err := mapDeepGetMap(m, "status", "resources")
		if err != nil {
			return nil, fmt.Errorf("cannot convert %s: %w", objRef, err)
		}

		status, err := NewRestRadiusResourceStatus(objRef, outputResources)
		if err != nil {
			return nil, fmt.Errorf("cannot convert %s: %w", objRef, err)
		}
		if properties == nil {
			properties = make(map[string]interface{})
		}
		properties["status"] = status
	}

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

func NewRestRadiusResourceStatus(objRef string, ors map[string]interface{}) (rest.ComponentStatus, error) {
	restOutputResources, err := NewRestOutputResources(objRef, ors)
	if err != nil {
		return rest.ComponentStatus{}, err
	}

	// Aggregate the resource status
	aggregateHealthState, aggregateHealthStateErrorDetails := rest.GetUserFacingHealthState(restOutputResources)
	aggregateProvisiongState := rest.GetUserFacingProvisioningState(restOutputResources)

	status := rest.ComponentStatus{
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
