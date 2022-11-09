// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprinvokehttproutes"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprpubsubbrokers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprsecretstores"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprstatestores"
	"github.com/project-radius/radius/pkg/linkrp/renderers/extenders"
	"github.com/project-radius/radius/pkg/linkrp/renderers/mongodatabases"
	"github.com/project-radius/radius/pkg/linkrp/renderers/rabbitmqmessagequeues"
	"github.com/project-radius/radius/pkg/linkrp/renderers/rediscaches"
	"github.com/project-radius/radius/pkg/linkrp/renderers/sqldatabases"
	"github.com/project-radius/radius/pkg/resourcemodel"

	"github.com/project-radius/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewApplicationModel(arm *armauth.ArmConfig, k8s client.Client) (ApplicationModel, error) {
	// Configure the providers supported by the appmodel
	supportedProviders := map[string]bool{
		resourcemodel.ProviderKubernetes: true,
	}
	if arm != nil {
		supportedProviders[resourcemodel.ProviderAzure] = true
	}

	radiusResourceModel := []RadiusResourceModel{
		{
			ResourceType: mongodatabases.ResourceType,
			Renderer:     &mongodatabases.Renderer{},
		},
		{
			ResourceType: sqldatabases.ResourceType,
			Renderer:     &sqldatabases.Renderer{},
		},
		{
			ResourceType: rediscaches.ResourceType,
			Renderer:     &rediscaches.Renderer{},
		},
		{
			ResourceType: rabbitmqmessagequeues.ResourceType,
			Renderer:     &rabbitmqmessagequeues.Renderer{},
		},
		{
			ResourceType: daprinvokehttproutes.ResourceType,
			Renderer:     &daprinvokehttproutes.Renderer{},
		},
		{
			ResourceType: daprpubsubbrokers.ResourceType,
			Renderer: &daprpubsubbrokers.Renderer{
				PubSubs: daprpubsubbrokers.SupportedPubSubKindValues,
			},
		},
		{
			ResourceType: daprsecretstores.ResourceType,
			Renderer: &daprsecretstores.Renderer{
				SecretStores: daprsecretstores.SupportedSecretStoreKindValues,
			},
		},
		{
			ResourceType: daprstatestores.ResourceType,
			Renderer: &daprstatestores.Renderer{
				StateStores: daprstatestores.SupportedStateStoreKindValues,
			},
		},
		{
			ResourceType: extenders.ResourceType,
			Renderer:     &extenders.Renderer{},
		},
	}

	outputResourceModel := []OutputResourceModel{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureStorage,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprComponent,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewDaprComponentHandler(k8s),
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
				Type:     resourcekinds.DaprStateStoreAzureStorage,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewDaprPubSubServiceBusHandler(arm, k8s),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRedis,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler:        handlers.NewARMHandler(arm),
			SecretValueTransformer: &rediscaches.AzureTransformer{},
		},
	}

	recipeModel := RecipeModel{
		RecipeHandler: handlers.NewRecipeHandler(arm),
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
