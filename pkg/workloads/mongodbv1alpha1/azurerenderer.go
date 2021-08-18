// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha1

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/cosmosdbmongov1alpha1"
)

type AzureRenderer struct {
	Arm armauth.ArmConfig
}

var _ workloads.WorkloadRenderer = (*AzureRenderer)(nil)

func (r AzureRenderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	logger := radlogger.GetLogger(ctx)
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	databaseResource, err := workloads.FindByLocalID(resources, outputresource.LocalIDAzureCosmosDBMongo)
	if err != nil {
		return nil, err
	}

	accountName := databaseResource.Properties[handlers.CosmosDBAccountNameKey]
	databaseName := databaseResource.Properties[handlers.CosmosDBDatabaseNameKey]
	logger.Info(fmt.Sprintf("fulfilling binding for account: %v db: %v", accountName, databaseName))

	connectionString, err := cosmosdbmongov1alpha1.GetConnectionString(ctx, r.Arm, accountName, databaseName)
	if err != nil {
		return nil, err
	}

	bindings := map[string]components.BindingState{
		BindingMongo: {
			Component: workload.Name,
			Binding:   BindingMongo,
			Kind:      "mongodb.com/Mongo",
			Properties: map[string]interface{}{
				"connectionString": connectionString,
				"database":         databaseName,
			},
		},
	}

	return bindings, nil
}

func (r AzureRenderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := MongoDBComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	converted := r.convertToCosmosDBMongo(component)
	if converted.Config.Managed {
		return cosmosdbmongov1alpha1.RenderManaged(converted)
	} else {
		return cosmosdbmongov1alpha1.RenderUnmanaged(converted)
	}
}

func (r AzureRenderer) convertToCosmosDBMongo(input MongoDBComponent) cosmosdbmongov1alpha1.CosmosDBMongoComponent {
	return cosmosdbmongov1alpha1.CosmosDBMongoComponent{
		Name: input.Name,
		Kind: input.Kind,
		Config: cosmosdbmongov1alpha1.CosmosDBMongoConfig{
			Managed:  input.Config.Managed,
			Resource: input.Config.Resource,
		},
		Run:      input.Run,
		Uses:     input.Uses,
		Bindings: input.Bindings,
		Traits:   input.Traits,
	}
}
