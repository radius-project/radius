// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/connectorrp/handlers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/mongodatabases"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/resourcemodel"

	"github.com/project-radius/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewApplicationModel(arm *armauth.ArmConfig, k8s client.Client) (ApplicationModel, error) {
	// Configure the providers supported by the appmodel
	supportedProviders := map[string]bool{
		providers.ProviderKubernetes: true,
	}
	if arm != nil {
		supportedProviders[providers.ProviderAzure] = true
		supportedProviders[providers.ProviderAzureKubernetesService] = true
	}

	radiusResourceModel := []RadiusResourceModel{
		{
			ResourceType: mongodatabases.ResourceType,
			Renderer:     &mongodatabases.Renderer{},
		},
		// {
		// 	ResourceType: daprhttproutev1alpha3.ResourceType,
		// 	Renderer:     &daprhttproutev1alpha3.Renderer{},
		// },
		// {
		// 	ResourceType: daprpubsubv1alpha3.ResourceType,
		// 	Renderer: &daprpubsubv1alpha3.Renderer{
		// 		PubSubs: daprpubsubv1alpha3.SupportedPubSubKindValues,
		// 	},
		// },
		// {
		// 	ResourceType: daprstatestorev1alpha3.ResourceType,
		// 	Renderer: &daprstatestorev1alpha3.Renderer{
		// 		StateStores: daprstatestorev1alpha3.SupportedStateStoreKindValues,
		// 	},
		// },
		// {
		// 	ResourceType: daprsecretstorev1alpha3.ResourceType,
		// 	Renderer: &daprsecretstorev1alpha3.Renderer{
		// 		SecretStores: daprsecretstorev1alpha3.SupportedSecretStoreKindValues,
		// 	},
		// },
		// {
		// 	ResourceType: microsoftsqlv1alpha3.ResourceType,
		// 	Renderer:     &microsoftsqlv1alpha3.Renderer{},
		// },
		// {
		// 	ResourceType: redisv1alpha3.ResourceType,
		// 	Renderer:     &redisv1alpha3.Renderer{},
		// },
		// {
		// 	ResourceType: rabbitmqv1alpha3.ResourceType,
		// 	Renderer:     &rabbitmqv1alpha3.Renderer{},
		// },
		// {
		// 	ResourceType: extenderv1alpha3.ResourceType,
		// 	Renderer:     &extenderv1alpha3.Renderer{},
		// },
	}

	outputResourceModel := []OutputResourceModel{
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.Kubernetes,
		// 		Provider: providers.ProviderKubernetes,
		// 	},
		// 	ResourceHandler: handlers.NewKubernetesHandler(k8s),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.Deployment,
		// 		Provider: providers.ProviderKubernetes,
		// 	},
		// 	ResourceHandler: handlers.NewKubernetesHandler(k8s),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.KubernetesHTTPRoute,
		// 		Provider: providers.ProviderKubernetes,
		// 	},
		// 	ResourceHandler: handlers.NewKubernetesHandler(k8s),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.SecretProviderClass,
		// 		Provider: providers.ProviderKubernetes,
		// 	},
		// 	ResourceHandler: handlers.NewKubernetesHandler(k8s),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.DaprStateStoreAzureStorage,
		// 		Provider: providers.ProviderKubernetes,
		// 	},
		// 	ResourceHandler: handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.DaprComponent,
		// 		Provider: providers.ProviderKubernetes,
		// 	},
		// 	ResourceHandler: handlers.NewKubernetesHandler(k8s),
		// },
	}

	azureOutputResourceModel := []OutputResourceModel{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: providers.ProviderAzure,
			},
			ResourceHandler:        handlers.NewAzureCosmosDBMongoHandler(arm),
			SecretValueTransformer: &mongodatabases.AzureTransformer{},
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: providers.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureCosmosAccountHandler(arm),
		},
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.DaprStateStoreAzureStorage,
		// 		Provider: providers.ProviderAzure,
		// 	},
		// 	ResourceHandler: handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.AzureCosmosDBSQL,
		// 		Provider: providers.ProviderAzure,
		// 	},
		// 	ResourceHandler: handlers.NewAzureCosmosDBSQLHandler(arm),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
		// 		Provider: providers.ProviderAzure,
		// 	},
		// 	ResourceHandler: handlers.NewDaprPubSubServiceBusHandler(arm, k8s),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.AzureSqlServer,
		// 		Provider: providers.ProviderAzure,
		// 	},
		// 	ResourceHandler: handlers.NewARMHandler(arm),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.AzureSqlServerDatabase,
		// 		Provider: providers.ProviderAzure,
		// 	},
		// 	ResourceHandler: handlers.NewARMHandler(arm),
		// },
		// {
		// 	ResourceType: resourcemodel.ResourceType{
		// 		Type:     resourcekinds.AzureRedis,
		// 		Provider: providers.ProviderAzure,
		// 	},
		// 	ResourceHandler: handlers.NewAzureRedisHandler(arm),
		// },
	}

	err := checkForDuplicateRegistrations(radiusResourceModel, outputResourceModel)
	if err != nil {
		return ApplicationModel{}, err
	}

	if arm != nil {
		outputResourceModel = append(outputResourceModel, azureOutputResourceModel...)
	}
	return NewModel(radiusResourceModel, outputResourceModel, supportedProviders), nil
}

// checkForDuplicateRegistrations checks for duplicate registrations with the same resource type
func checkForDuplicateRegistrations(radiusResources []RadiusResourceModel, outputResources []OutputResourceModel) error {
	rendererRegistration := make(map[string]int)
	for _, r := range radiusResources {
		rendererRegistration[r.ResourceType]++
		if rendererRegistration[r.ResourceType] > 1 {
			return fmt.Errorf("Multiple resource renderers registered for resource type: %s", r.ResourceType)
		}
	}

	outputResourceHandlerRegistration := make(map[resourcemodel.ResourceType]int)
	for _, o := range outputResources {
		outputResourceHandlerRegistration[o.ResourceType]++
		if outputResourceHandlerRegistration[o.ResourceType] > 1 {
			return fmt.Errorf("Multiple output resource handlers registered for resource type: %s", o.ResourceType)
		}
	}
	return nil
}
