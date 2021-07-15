// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/preview/sql/mgmt/2015-05-01-preview/sql"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/gofrs/uuid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Properties keys
	serverNameKey = "servername"

	// Server properties
	dbLogin = "radiusUser"
	port    = 1433
)

func NewDaprStateStoreSQLServerHandler(arm armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprStateStoreSQLServerHandler{
		kubernetesHandler: kubernetesHandler{k8s: k8s},
		arm:               arm,
		k8s:               k8s,
	}
}

type daprStateStoreSQLServerHandler struct {
	kubernetesHandler
	arm armauth.ArmConfig
	k8s client.Client
}

func (handler *daprStateStoreSQLServerHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	location, err := getResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	dbName := properties[ComponentNameKey]

	// Generate password
	passwordConditions := &PasswordConditions{16, 2, 1, 1}
	password := generatePassword(passwordConditions)

	// Generate server name and create server
	serverName, err := handler.createServer(ctx, location, dbName, password, options)
	if err != nil {
		return nil, err
	}
	properties[serverNameKey] = serverName

	// Create database
	err = handler.createSQLDB(ctx, location, serverName, dbName, options)
	if err != nil {
		return nil, err
	}

	// Generate connection string
	connectionString := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%d;database=%s;",
		serverName, dbLogin, password, port, dbName)

	err = handler.PatchNamespace(ctx, properties[KubernetesNamespaceKey])
	if err != nil {
		return nil, err
	}

	// Translate into Dapr SQL Server component schema
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"name":      properties[KubernetesNameKey],
				"namespace": properties[KubernetesNamespaceKey],
				"labels": map[string]string{
					keys.LabelRadiusApplication:   options.Application,
					keys.LabelRadiusComponent:     options.Component,
					keys.LabelKubernetesName:      options.Component,
					keys.LabelKubernetesPartOf:    options.Application,
					keys.LabelKubernetesManagedBy: keys.LabelKubernetesManagedByRadiusRP,
				},
			},
			"spec": map[string]interface{}{
				"type":    "state.sqlserver",
				"version": "v1",
				"metadata": []interface{}{
					map[string]interface{}{
						"name":  "connectionString",
						"value": connectionString,
					},
					map[string]interface{}{
						"name":  "tableName",
						"value": "dapr",
					},
				},
			},
		},
	}

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: keys.FieldManager})
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (handler *daprStateStoreSQLServerHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"name":      properties[KubernetesNameKey],
				"namespace": properties[KubernetesNamespaceKey],
			},
		},
	}

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return err
	}

	serverName := properties[serverNameKey]
	databaseName := properties[ComponentNameKey]

	// Delete database
	sqlDBClient := sql.NewDatabasesClient(handler.arm.SubscriptionID)
	sqlDBClient.Authorizer = handler.arm.Auth
	sqlDBClient.PollingDuration = 0
	response, err := sqlDBClient.Delete(ctx, handler.arm.ResourceGroup, serverName, databaseName)
	if err != nil && response.StatusCode != 404 {
		return fmt.Errorf("failed to delete sql database `%s`: %w", databaseName, err)
	}

	// Delete the server
	sqlServerClient := sql.NewServersClient(handler.arm.SubscriptionID)
	sqlServerClient.Authorizer = handler.arm.Auth
	sqlServerClient.PollingDuration = 0
	future, err := sqlServerClient.Delete(ctx, handler.arm.ResourceGroup, serverName)
	if err != nil && future.Response().StatusCode != 404 {
		return fmt.Errorf("failed to delete sql server `%s`: %w", serverName, err)
	}

	err = future.WaitForCompletionRef(ctx, sqlServerClient.Client)
	if err != nil && !util.IsAutorest404Error(err) {
		return fmt.Errorf("failed to delete sql server `%s`: %w", serverName, err)
	}

	serverDeleteResponse, err := future.Result(sqlServerClient)
	if err != nil && serverDeleteResponse.StatusCode != 404 {
		return fmt.Errorf("failed to delete sql server `%s`: %w", serverName, err)
	}

	return nil
}

// createServer creates SQL server instance with a generated unique server name prefixed with specified database name
func (handler *daprStateStoreSQLServerHandler) createServer(ctx context.Context, location *string, databaseName string, password string, options PutOptions) (string, error) {
	logger := radlogger.GetLogger(ctx)

	sqlServerClient := sql.NewServersClient(handler.arm.SubscriptionID)
	sqlServerClient.Authorizer = handler.arm.Auth
	sqlServerClient.PollingDuration = 0

	var serverName = ""
	retryAttempts := 10

	// Generate unique server name
	// This logic is repeated all over the place in the code today
	// and it doesn't consider max length allowed for the resource or the length of the base string,
	// tracking issue to improve this: https://github.com/Azure/radius/issues/467
	for i := 0; i < retryAttempts; i++ {
		// 3-24 characters - all alphanumeric
		uid, err := uuid.NewV4()
		if err != nil {
			return "", fmt.Errorf("failed to generate sql server name: %w", err)
		}
		serverName = databaseName + strings.ReplaceAll(uid.String(), "-", "")
		serverName = serverName[0:24]

		result, err := sqlServerClient.CheckNameAvailability(ctx, sql.CheckNameAvailabilityRequest{
			Name: to.StringPtr(serverName),
			Type: to.StringPtr("Microsoft.Sql/servers"),
		})
		if err != nil {
			return "", fmt.Errorf("failed to query sql server name: %w", err)
		}

		if result.Available != nil && *result.Available {
			break
		}

		logger.Info(fmt.Sprintf("sql server name generation failed after %d attempts: %v %v", i, result.Reason, result.Message))
	}

	// Create server
	future, err := sqlServerClient.CreateOrUpdate(ctx, handler.arm.ResourceGroup, serverName, sql.Server{
		Location: location,
		Tags:     keys.MakeTagsForRadiusComponent(options.Application, options.Component),
		ServerProperties: &sql.ServerProperties{
			AdministratorLogin:         to.StringPtr(dbLogin),
			AdministratorLoginPassword: to.StringPtr(password),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create sql server: %w", err)
	}

	err = future.WaitForCompletionRef(ctx, sqlServerClient.Client)
	if err != nil {
		return "", fmt.Errorf("failed to create sql server: %w", err)
	}

	_, err = future.Result(sqlServerClient)
	if err != nil {
		return "", fmt.Errorf("failed to create sql server: %w", err)
	}

	return serverName, nil
}

func (handler *daprStateStoreSQLServerHandler) createSQLDB(ctx context.Context, location *string, serverName string, dbName string, options PutOptions) error {
	sqlDBClient := sql.NewDatabasesClient(handler.arm.SubscriptionID)
	sqlDBClient.Authorizer = handler.arm.Auth
	sqlDBClient.PollingDuration = 0

	future, err := sqlDBClient.CreateOrUpdate(
		ctx,
		handler.arm.ResourceGroup,
		serverName,
		dbName,
		sql.Database{
			Location: location,
			Tags:     keys.MakeTagsForRadiusComponent(options.Application, options.Component),
		})
	if err != nil {
		return fmt.Errorf("failed to create sql database: %w", err)
	}

	err = future.WaitForCompletionRef(ctx, sqlDBClient.Client)
	if err != nil {
		return fmt.Errorf("failed to create sql database: %w", err)
	}

	return nil
}
