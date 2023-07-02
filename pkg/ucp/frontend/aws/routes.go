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

package aws

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	ucp_aws "github.com/project-radius/radius/pkg/ucp/aws"
	sdk_cred "github.com/project-radius/radius/pkg/ucp/credentials"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	awsproxy_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/awsproxy"
	aws_credential_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/credentials/aws"
	"github.com/project-radius/radius/pkg/ucp/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/validator"
)

const (
	planeScope             = "/planes/aws/{planeName}"
	resourcePath           = "/accounts/{accountId}/regions/{region}/providers/{providerNamespace}/{resourceType}/{resourceName}"
	resourceCollectionPath = "/accounts/{accountId}/regions/{region}/providers/{providerNamespace}/{resourceType}"
	operationResultsPath   = "/accounts/{accountId}/regions/{region}/providers/{providerNamespace}/locations/{location}/operationResults/{operationId}"
	operationStatusesPath  = "/accounts/{accountId}/regions/{region}/providers/{providerNamespace}/locations/{location}/operationStatuses/{operationId}"

	credentialResourcePath   = "/providers/System.AWS/credentials/{credentialName}"
	credentialCollectionPath = "/providers/System.AWS/credentials"

	// OperationTypeAWSResource is the operation type for CRUDL operations on AWS resources.
	OperationTypeAWSResource = "AWSRESOURCE"
)

// Initialize initializes the AWS module.
func (m *Module) Initialize(ctx context.Context) (http.Handler, error) {
	secretClient, err := m.options.SecretProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	// Support override of AWS Clients for testing.
	if m.AWSClients.CloudControl == nil || m.AWSClients.CloudFormation == nil {
		awsConfig, err := m.newAWSConfig(ctx)
		if err != nil {
			return nil, err
		}

		if m.AWSClients.CloudControl == nil {
			m.AWSClients.CloudControl = cloudcontrol.NewFromConfig(awsConfig)
		}

		if m.AWSClients.CloudFormation == nil {
			m.AWSClients.CloudFormation = cloudformation.NewFromConfig(awsConfig)
		}
	}

	baseRouter := server.NewSubrouter(m.router, m.options.PathBase+planeScope)

	handlerOptions := []server.HandlerOptions{
		{
			// URLS for standard UCP resource async status result.
			ParentRouter:  baseRouter,
			Path:          operationResultsPath,
			Method:        v1.OperationGetOperationResult,
			OperationType: &v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetOperationResult},
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return awsproxy_ctrl.NewGetAWSOperationResults(opt, m.AWSClients)
			},
		},
		{
			// URLS for standard UCP resource async status.
			ParentRouter:  baseRouter,
			Path:          operationStatusesPath,
			Method:        v1.OperationGetOperationStatuses,
			OperationType: &v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetOperationStatuses},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return awsproxy_ctrl.NewGetAWSOperationStatuses(opts, m.AWSClients)
			},
		},
	}

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter:  baseRouter,
			Path:          resourcePath,
			Method:        v1.OperationPut,
			OperationType: &v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationPut},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return awsproxy_ctrl.NewCreateOrUpdateAWSResource(opts, m.AWSClients)
			},
		},
		{
			ParentRouter:  baseRouter,
			Path:          resourcePath,
			Method:        v1.OperationDelete,
			OperationType: &v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationDelete},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return awsproxy_ctrl.NewDeleteAWSResource(opts, m.AWSClients)
			},
		},
		{
			ParentRouter:  baseRouter,
			Path:          resourcePath,
			Method:        v1.OperationGet,
			OperationType: &v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGet},
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return awsproxy_ctrl.NewGetAWSResource(opt, m.AWSClients)
			},
		},
	}...)

	// URLS for "non-idempotent" resource lifecycle operations. These are extensions to the UCP spec that are needed when
	// a resource has a non-idempotent lifecyle and a computed name.
	//
	// The normal UCP lifecycle operations have a user-specified resource name which must be part of the URL. These
	// operations are structured so that the resource name is not part of the URL.
	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			// URLs for standard UCP resource lifecycle operations.
			ParentRouter:  baseRouter,
			Path:          resourceCollectionPath,
			Method:        v1.OperationList,
			OperationType: &v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationList},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return awsproxy_ctrl.NewListAWSResources(opts, m.AWSClients)
			},
		},
		{
			ParentRouter:  baseRouter,
			Path:          fmt.Sprintf("%s/:%s", resourceCollectionPath, "put"),
			Method:        v1.OperationPutImperative,
			OperationType: &v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationPutImperative},
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return awsproxy_ctrl.NewCreateOrUpdateAWSResourceWithPost(opt, m.AWSClients)
			},
		},
		{
			ParentRouter:  baseRouter,
			Path:          fmt.Sprintf("%s/:%s", resourceCollectionPath, "get"),
			Method:        v1.OperationGetImperative,
			OperationType: &v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetImperative},
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return awsproxy_ctrl.NewGetAWSResourceWithPost(opt, m.AWSClients)
			},
		},
		{
			ParentRouter:  baseRouter,
			Path:          fmt.Sprintf("%s/:%s", resourceCollectionPath, "delete"),
			Method:        v1.OperationDeleteImperative,
			OperationType: &v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationDeleteImperative},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return awsproxy_ctrl.NewDeleteAWSResourceWithPost(opts, m.AWSClients)
			},
		},
	}...)

	// URLs for operations on AWS credential resources.
	//
	// These use the OpenAPI spec validator. General AWS operations DO NOT use the spec validator
	// because we rely on CloudControl's validation.
	apiValidator := validator.APIValidatorUCP(m.options.SpecLoader)

	credentialCollectionRouter := server.NewSubrouter(baseRouter, credentialCollectionPath, apiValidator)
	credentialResourceRouter := server.NewSubrouter(baseRouter, credentialResourcePath, apiValidator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: credentialCollectionRouter,
			ResourceType: v20220901privatepreview.AWSCredentialType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewListResources(opt,
					controller.ResourceOptions[datamodel.AWSCredential]{
						RequestConverter:  converter.AWSCredentialDataModelFromVersioned,
						ResponseConverter: converter.AWSCredentialDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: credentialResourceRouter,
			ResourceType: v20220901privatepreview.AWSCredentialType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					controller.ResourceOptions[datamodel.AWSCredential]{
						RequestConverter:  converter.AWSCredentialDataModelFromVersioned,
						ResponseConverter: converter.AWSCredentialDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: credentialResourceRouter,
			Method:       v1.OperationPut,
			ResourceType: v20220901privatepreview.AWSCredentialType,
			ControllerFactory: func(o controller.Options) (controller.Controller, error) {
				return aws_credential_ctrl.NewCreateOrUpdateAWSCredential(o, secretClient)
			},
		},
		{
			ParentRouter: credentialResourceRouter,
			Method:       v1.OperationDelete,
			ResourceType: v20220901privatepreview.AWSCredentialType,
			ControllerFactory: func(o controller.Options) (controller.Controller, error) {
				return aws_credential_ctrl.NewDeleteAWSCredential(o, secretClient)
			},
		},
	}...)

	ctrlOpts := controller.Options{
		Address:      m.options.Address,
		PathBase:     m.options.PathBase,
		DataProvider: m.options.DataProvider,
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return nil, err
		}
	}

	return m.router, nil
}

func (m *Module) newAWSConfig(ctx context.Context) (aws.Config, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	credProviders := []func(*config.LoadOptions) error{}

	switch m.options.Config.Identity.AuthMethod {
	case hostoptions.AuthUCPCredential:
		provider, err := sdk_cred.NewAWSCredentialProvider(m.options.SecretProvider, m.options.UCPConnection, &aztoken.AnonymousCredential{})
		if err != nil {
			return aws.Config{}, err
		}
		p := ucp_aws.NewUCPCredentialProvider(provider, ucp_aws.DefaultExpireDuration)
		credProviders = append(credProviders, config.WithCredentialsProvider(p))
		logger.Info("Configuring 'UCPCredential' authentication mode using UCP Credential API")

	default:
		logger.Info("Configuring default authentication mode with environment variable.")
	}

	awscfg, err := config.LoadDefaultConfig(ctx, credProviders...)
	if err != nil {
		return aws.Config{}, err
	}

	return awscfg, nil
}
