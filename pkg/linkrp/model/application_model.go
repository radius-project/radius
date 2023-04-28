/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package model

import (
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprinvokehttproutes"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprpubsubbrokers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprsecretstores"
	"github.com/project-radius/radius/pkg/linkrp/renderers/extenders"
	"github.com/project-radius/radius/pkg/linkrp/renderers/mongodatabases"
	"github.com/project-radius/radius/pkg/linkrp/renderers/rabbitmqmessagequeues"
	"github.com/project-radius/radius/pkg/linkrp/renderers/sqldatabases"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/sdk"

	"github.com/project-radius/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewApplicationModel(arm *armauth.ArmConfig, k8s client.Client, connection sdk.Connection) (ApplicationModel, error) {
	// Configure the providers supported by the appmodel
	supportedProviders := map[string]bool{
		resourcemodel.ProviderKubernetes: true,
		resourcemodel.ProviderAWS:        true,
	}
	if arm != nil {
		supportedProviders[resourcemodel.ProviderAzure] = true
	}

	radiusResourceModel := []RadiusResourceModel{
		{
			ResourceType: linkrp.MongoDatabasesResourceType,
			Renderer:     &mongodatabases.Renderer{},
		},
		{
			ResourceType: linkrp.SqlDatabasesResourceType,
			Renderer:     &sqldatabases.Renderer{},
		},
		{
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Renderer:     &rabbitmqmessagequeues.Renderer{},
		},
		{
			ResourceType: linkrp.DaprInvokeHttpRoutesResourceType,
			Renderer:     &daprinvokehttproutes.Renderer{},
		},
		{
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Renderer: &daprpubsubbrokers.Renderer{
				PubSubs: daprpubsubbrokers.SupportedPubSubModes,
			},
		},
		{
			ResourceType: linkrp.DaprSecretStoresResourceType,
			Renderer: &daprsecretstores.Renderer{
				SecretStores: daprsecretstores.SupportedSecretStoreModes,
			},
		},
		{
			ResourceType: linkrp.ExtendersResourceType,
			Renderer:     &extenders.Renderer{},
		},
	}

	outputResourceModel := []OutputResourceModel{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprComponent,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewDaprComponentHandler(k8s),
		},

		{
			// Handles any Kubernetes resource type
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AnyResourceType,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8s),
		},

		{
			// Handles any AWS resource type
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AnyResourceType,
				Provider: resourcemodel.ProviderAWS,
			},
			ResourceHandler: handlers.NewAWSHandler(connection),
		},
	}

	azureOutputResourceModel := []OutputResourceModel{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler:        handlers.NewARMHandler(arm),
			SecretValueTransformer: &mongodatabases.AzureTransformer{},
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewARMHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureSqlServer,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewARMHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureSqlServerDatabase,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewARMHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureTableService,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewARMHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureTable,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewARMHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewDaprPubSubServiceBusHandler(arm, k8s),
		},
	}

	recipeModel := RecipeModel{
		RecipeHandler: handlers.NewRecipeHandler(connection),
	}

	err := checkForDuplicateRegistrations(radiusResourceModel, outputResourceModel)
	if err != nil {
		return ApplicationModel{}, err
	}
	if arm != nil {
		outputResourceModel = append(outputResourceModel, azureOutputResourceModel...)
	}
	return NewModel(recipeModel, radiusResourceModel, outputResourceModel, supportedProviders), nil
}

// checkForDuplicateRegistrations checks for duplicate registrations with the same resource type
func checkForDuplicateRegistrations(radiusResources []RadiusResourceModel, outputResources []OutputResourceModel) error {
	rendererRegistration := make(map[string]int)
	for _, r := range radiusResources {
		rendererRegistration[r.ResourceType]++
		if rendererRegistration[r.ResourceType] > 1 {
			return fmt.Errorf("multiple resource renderers registered for resource type: %s", r.ResourceType)
		}
	}

	outputResourceHandlerRegistration := make(map[resourcemodel.ResourceType]int)
	for _, o := range outputResources {
		outputResourceHandlerRegistration[o.ResourceType]++
		if outputResourceHandlerRegistration[o.ResourceType] > 1 {
			return fmt.Errorf("multiple output resource handlers registered for resource type: %s", o.ResourceType)
		}
	}
	return nil
}
