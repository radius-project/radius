// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/workloads"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Renderer is the WorkloadRenderer implementation for the dapr pubsub workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for dapr pubsub workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if service.Kind != "dapr.io/PubSubTopic" {
		return nil, fmt.Errorf("cannot fulfill service kind: %v", service.Kind)
	}

	if len(wrp) != 1 || wrp[0].Type != "dapr.pubsubtopic.azureservicebus" {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for dapr.pubsubtopic.azureservicebus")
	}

	properties := wrp[0].Properties
	namespaceName := properties["servicebusnamespace"]
	pubsubName := properties["servicebuspubsubname"]
	topicName := properties["servicebustopic"]

	values := map[string]interface{}{
		"namespace":  namespaceName,
		"pubsubName": pubsubName,
		"topic":      topicName,
	}

	return values, nil
}

// Render is the WorkloadRenderer implementation for dapr pubsub workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	spec, err := getSpec(w.Workload)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if spec.Kind != "any" && spec.Kind != "pubsub.azure.servicebus" {
		return []workloads.WorkloadResource{}, errors.New("only kind 'any' and 'pubsub.azure.servicebus' is supported right now")
	}

	if !spec.Managed {
		return []workloads.WorkloadResource{}, errors.New("only 'managed=true' is supported right now")
	}

	// generate data we can use to manage a pubsub
	resource := workloads.WorkloadResource{
		Type: "dapr.pubsubtopic.azureservicebus",
		Resource: map[string]string{
			"name":                 w.Workload.GetName(),
			"namespace":            w.Workload.GetNamespace(),
			"apiVersion":           "dapr.io/v1alpha1",
			"kind":                 "Component",
			"servicebuspubsubname": spec.Name,
			"servicebustopic":      spec.Topic,
		},
	}

	// It's already in the correct format
	return []workloads.WorkloadResource{resource}, nil
}

type pubsubSpec struct {
	Kind    string `json:"kind"`
	Managed bool   `json:"managed"`
	Name    string `json:"name"`
	Topic   string `json:"topic"`
}

func getSpec(item unstructured.Unstructured) (pubsubSpec, error) {
	spec, ok := item.Object["spec"]
	if !ok {
		return pubsubSpec{}, errors.New("workload does not contain a spec element")
	}

	b, err := json.Marshal(spec)
	if err != nil {
		return pubsubSpec{}, err
	}

	pubsub := pubsubSpec{}
	err = json.Unmarshal(b, &pubsub)
	if err != nil {
		return pubsubSpec{}, err
	}

	return pubsub, nil
}
