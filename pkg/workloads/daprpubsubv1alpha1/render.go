// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/workloads"
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

	if len(wrp) != 1 || wrp[0].Type != workloads.ResourceKindDaprPubSubTopicAzureServiceBus {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for %s", workloads.ResourceKindDaprPubSubTopicAzureServiceBus)
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
	component := DaprPubSubComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if !component.Config.Managed {
		return []workloads.WorkloadResource{}, errors.New("only 'managed=true' is supported right now")
	}

	// generate data we can use to manage a servicebus instance

	resource := workloads.WorkloadResource{
		Type: workloads.ResourceKindDaprPubSubTopicAzureServiceBus,
		Resource: map[string]string{
			"name":                 w.Workload.Name,
			"namespace":            w.Application,
			"apiVersion":           "dapr.io/v1alpha1",
			"kind":                 "Component",
			"servicebuspubsubname": component.Config.Name,
			"servicebustopic":      component.Config.Topic,
		},
	}

	// It's already in the correct format
	return []workloads.WorkloadResource{resource}, nil
}
