// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"math/rand"
	"time"

	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/preview/sql/mgmt/2015-05-01-preview/sql"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/resourcemodel"
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

func (handler *daprStateStoreSQLServerHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	location, err := clients.GetResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	dbName := properties[ResourceName]

	// Generate password
	passwordConditions := &PasswordConditions{16, 2, 1, 1}
	password := generatePassword(passwordConditions)

	// Generate server name and create server
	serverName, err := handler.createServer(ctx, location, dbName, password, *options)
	if err != nil {
		return nil, err
	}
	properties[serverNameKey] = serverName

	// Create database
	database, err := handler.createSQLDB(ctx, location, serverName, dbName, *options)
	if err != nil {
		return nil, err
	}

	// Use the identity of the database as the thing to monitor.
	options.Resource.Identity = resourcemodel.NewARMIdentity(*database.ID, clients.GetAPIVersionFromUserAgent(sql.UserAgent()))

	// Generate connection string
	connectionString := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%d;database=%s;",
		serverName, dbLogin, password, port, dbName)

	err = handler.PatchNamespace(ctx, properties[KubernetesNamespaceKey])
	if err != nil {
		return nil, err
	}

	// Translate into Dapr SQL Server schema
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"name":      properties[KubernetesNameKey],
				"namespace": properties[KubernetesNamespaceKey],
				"labels":    kubernetes.MakeDescriptiveLabels(options.ApplicationName, options.ResourceName),
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

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (handler *daprStateStoreSQLServerHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties
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
	databaseName := properties[ResourceName]

	// Delete database
	sqlDBClient := clients.NewDatabasesClient(handler.arm.SubscriptionID, handler.arm.Auth)
	response, err := sqlDBClient.Delete(ctx, handler.arm.ResourceGroup, serverName, databaseName)
	if err != nil && response.StatusCode != 404 {
		return fmt.Errorf("failed to delete sql database `%s`: %w", databaseName, err)
	}

	// Delete the server
	sqlServerClient := clients.NewServersClient(handler.arm.SubscriptionID, handler.arm.Auth)
	future, err := sqlServerClient.Delete(ctx, handler.arm.ResourceGroup, serverName)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "sql server", err)
	}

	err = future.WaitForCompletionRef(ctx, sqlServerClient.Client)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "sql server", err)
	}

	return nil
}

// createServer creates SQL server instance with a generated unique server name prefixed with specified database name
func (handler *daprStateStoreSQLServerHandler) createServer(ctx context.Context, location *string, databaseName string, password string, options PutOptions) (string, error) {
	logger := radlogger.GetLogger(ctx)

	sqlServerClient := clients.NewServersClient(handler.arm.SubscriptionID, handler.arm.Auth)

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
		Tags:     keys.MakeTagsForRadiusResource(options.ApplicationName, options.ResourceName),
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

func (handler *daprStateStoreSQLServerHandler) createSQLDB(ctx context.Context, location *string, serverName string, dbName string, options PutOptions) (sql.Database, error) {
	sqlDBClient := clients.NewDatabasesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	future, err := sqlDBClient.CreateOrUpdate(
		ctx,
		handler.arm.ResourceGroup,
		serverName,
		dbName,
		sql.Database{
			Location: location,
			Tags:     keys.MakeTagsForRadiusResource(options.ApplicationName, options.ResourceName),
		})
	if err != nil {
		return sql.Database{}, fmt.Errorf("failed to create sql database: %w", err)
	}

	err = future.WaitForCompletionRef(ctx, sqlDBClient.Client)
	if err != nil {
		return sql.Database{}, fmt.Errorf("failed to create sql database: %w", err)
	}

	return future.Result(sqlDBClient)
}

type PasswordConditions struct {
	Lower       int
	Upper       int
	Digits      int
	SpecialChar int
}

func generatePassword(conditions *PasswordConditions) string {
	pwd := generateString(conditions.Digits, "1234567890") +
		generateString(conditions.Lower, "abcdefghijklmnopqrstuvwxyz") +
		generateString(conditions.Upper, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") +
		generateString(conditions.SpecialChar, "-")

	pwdArray := strings.Split(pwd, "")
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(pwdArray), func(i, j int) {
		pwdArray[i], pwdArray[j] = pwdArray[j], pwdArray[i]
	})
	pwd = strings.Join(pwdArray, "")

	return pwd
}

func generateString(length int, allowedCharacters string) string {
	str := ""
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	for len(str) < length {
		str += string(allowedCharacters[rnd.Intn(len(allowedCharacters))])
	}
	return str
}

func NewDaprStateStoreSQLServerHealthHandler(arm armauth.ArmConfig, k8s client.Client) HealthHandler {
	return &daprStateStoreSQLServerHealthHandler{
		arm: arm,
		k8s: k8s,
	}
}

type daprStateStoreSQLServerHealthHandler struct {
	arm armauth.ArmConfig
	k8s client.Client
}

func (handler *daprStateStoreSQLServerHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
