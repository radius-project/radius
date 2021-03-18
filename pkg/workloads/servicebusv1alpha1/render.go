// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicebusv1alpha1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/workloads"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Renderer is the WorkloadRenderer implementation for the cosmos documentdb workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for servicebus workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if len(wrp) != 1 || wrp[0].Type != "azure.servicebus" {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for azure.servicebus")
	}

	properties := wrp[0].Properties
	namespaceName := properties["servicebusnamespace"]
	queueName := properties["servicebusqueue"]

	sbClient := servicebus.NewNamespacesClient(r.Arm.SubscriptionID)
	sbClient.Authorizer = r.Arm.Auth
	accessKeys, err := sbClient.ListKeys(ctx, r.Arm.ResourceGroup, namespaceName, "RootManageSharedAccessKey")

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if accessKeys.PrimaryConnectionString == nil && accessKeys.SecondaryConnectionString == nil {
		return nil, fmt.Errorf("failed to retrieve connection strings")
	}

	cs := accessKeys.PrimaryConnectionString

	values := map[string]interface{}{
		"connectionString": *cs,
		"namespace":        namespaceName,
		"queue":            queueName,
	}

	return values, nil
}

// Render is the WorkloadRenderer implementation for servicebus workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	spec, err := getSpec(w.Workload)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if !spec.Managed {
		return []workloads.WorkloadResource{}, errors.New("only 'managed=true' is supported right now")
	}

	// generate data we can use to manage a servicebus instance
	namespaceName := util.GenerateName("radius-ns")
	resource := workloads.WorkloadResource{
		Type: "azure.servicebus",
		Resource: map[string]string{
			"name":                w.Workload.GetName(),
			"servicebusnamespace": namespaceName,
			"servicebusqueue":     spec.Queue,
		},
	}

	// It's already in the correct format
	return []workloads.WorkloadResource{resource}, nil
}

type serviceBusSpec struct {
	Managed bool   `json:"managed"`
	Queue   string `json:"queue"`
}

func getSpec(item unstructured.Unstructured) (serviceBusSpec, error) {
	spec, ok := item.Object["spec"]
	if !ok {
		return serviceBusSpec{}, errors.New("workload does not contain a spec element")
	}

	b, err := json.Marshal(spec)
	if err != nil {
		return serviceBusSpec{}, err
	}

	value := serviceBusSpec{}
	err = json.Unmarshal(b, &value)
	if err != nil {
		return serviceBusSpec{}, err
	}

	return value, nil
}
