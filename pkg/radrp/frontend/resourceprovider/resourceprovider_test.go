// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceprovider

import (
	context "context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/backend/deployment"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/schema"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const (
	testLocation    = "test-location"
	testID          = "test-id"
	subscriptionID  = "test-subscription"
	resourceGroup   = "test-resource-group"
	providerName    = "radiusv3"
	applicationName = "test-application"
	resourceType    = "Container" // Need to use a real resource type
	resourceName    = "test-resource"
	operationName   = "test-operation"
)

type testcase struct {
	description string
	verb        string
	invoke      func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error)
	setupDB     func(database *db.MockRadrpDB, err error)
	id          azresources.ResourceID
}

// Cases where we want to implement functionality consistently (like validation)
//
// In generate we can data-drive all of the negative testing and a lot of the positive testing.
var testcases = []testcase{
	{
		description: "ListApplications",
		verb:        "List",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.ListApplications(ctx, id)
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			database.EXPECT().ListV3Applications(gomock.Any(), gomock.Any()).
				Times(1).DoAndReturn(func(interface{}, interface{}) ([]db.ApplicationResource, error) {
				return nil, err
			})
		},
		id: parseOrPanic(applicationListID()),
	},
	{
		description: "GetApplication",
		verb:        "Get",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.GetApplication(ctx, id)
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			database.EXPECT().GetV3Application(gomock.Any(), gomock.Any()).
				Times(1).DoAndReturn(func(interface{}, interface{}) (db.ApplicationResource, error) {
				return db.ApplicationResource{}, err
			})
		},
		id: parseOrPanic(applicationID(applicationName)),
	},
	{
		description: "UpdateApplication",
		verb:        "Update",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.UpdateApplication(ctx, id, []byte("{}"))
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			database.EXPECT().UpdateV3ApplicationDefinition(gomock.Any(), gomock.Any()).
				Times(1).DoAndReturn(func(interface{}, interface{}) (bool, error) {
				return false, err
			})
		},
		id: parseOrPanic(applicationID(applicationName)),
	},
	{
		description: "DeleteApplication",
		verb:        "Delete",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.DeleteApplication(ctx, id)
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			database.EXPECT().DeleteV3Application(gomock.Any(), gomock.Any()).
				Times(1).DoAndReturn(func(interface{}, interface{}) error {
				return err
			})
		},
		id: parseOrPanic(applicationID(applicationName)),
	},
	{
		description: "ListAllResourcesByApplication",
		verb:        "List",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.ListAllV3ResourcesByApplication(ctx, id)
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			database.EXPECT().GetV3Application(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(interface{}, interface{}) ([]db.RadiusResource, error) {
				return nil, err
			})
		},
		id: parseOrPanic(fmt.Sprintf("%s/%s", applicationID(applicationName), schema.GenericResourceType)),
	},
	{
		description: "ListResources",
		verb:        "List",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.ListResources(ctx, id)
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			database.EXPECT().ListV3Resources(gomock.Any(), gomock.Any()).
				Times(1).DoAndReturn(func(interface{}, interface{}) ([]db.RadiusResource, error) {
				return nil, err
			})
		},
		id: parseOrPanic(resourceListID(applicationName, resourceType)),
	},
	{
		description: "GetResource",
		verb:        "Get",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.GetResource(ctx, id)
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			database.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).
				Times(1).DoAndReturn(func(interface{}, interface{}) (db.RadiusResource, error) {
				return db.RadiusResource{}, err
			})
		},
		id: parseOrPanic(resourceID(applicationName, resourceType, resourceName)),
	},
	{
		description: "UpdateResource",
		verb:        "Update",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.UpdateResource(ctx, id, []byte("{}"))
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			database.EXPECT().UpdateV3ResourceDefinition(gomock.Any(), gomock.Any(), gomock.Any()).
				Times(1).DoAndReturn(func(interface{}, interface{}, interface{}) (bool, error) {
				return false, err
			})
		},
		id: parseOrPanic(resourceID(applicationName, resourceType, resourceName)),
	},
	{
		description: "DeleteResource",
		verb:        "Delete",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.DeleteResource(ctx, id)
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			// First database call is actually a get here
			database.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).
				Times(1).DoAndReturn(func(interface{}, interface{}) (db.RadiusResource, error) {
				return db.RadiusResource{}, err
			})
		},
		id: parseOrPanic(resourceID(applicationName, resourceType, resourceName)),
	},
	{
		description: "GetOperation",
		verb:        "Get",
		invoke: func(rp ResourceProvider, ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
			return rp.GetOperation(ctx, id)
		},
		setupDB: func(database *db.MockRadrpDB, err error) {
			database.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).
				Times(1).DoAndReturn(func(interface{}, interface{}) ([]db.ApplicationResource, error) {
				return nil, err
			})
		},
		id: parseOrPanic(operationID(applicationName, resourceType, resourceName, operationName)),
	},
}

func Test_AllEndpoints_RejectInvalidApplicationType(t *testing.T) {
	ctx := createContext(t)

	id := parseOrPanic(fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/%s/%s/%s/%s",
		subscriptionID,
		resourceGroup,
		azresources.CustomProvidersResourceProviders,
		providerName,
		"InvalidApplicationType", applicationName))

	for _, testcase := range testcases {
		t.Run(testcase.description, func(t *testing.T) {
			test := createRPTest(t)

			response, err := testcase.invoke(test.rp, ctx, id)
			require.NoError(t, err)

			expected := armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    armerrors.Invalid,
					Message: "unsupported resource type",
				},
			}
			require.Equal(t, rest.NewBadRequestARMResponse(expected), response)
		})
	}
}

func Test_AllEndpoints_RejectInvalidResourceID(t *testing.T) {
	ctx := createContext(t)

	// None of our endpoints will support this ID.
	id := parseOrPanic(resourceID(applicationName, "InvalidResourceType", resourceName))

	for _, testcase := range testcases {
		if testcase.description == "ListAllResourcesByApplication" {
			continue
		}

		t.Run(testcase.description, func(t *testing.T) {
			test := createRPTest(t)

			response, err := testcase.invoke(test.rp, ctx, id)
			require.NoError(t, err)

			expected := armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    armerrors.Invalid,
					Message: "unsupported resource type",
				},
			}
			require.Equal(t, rest.NewBadRequestARMResponse(expected), response)
		})
	}
}

func Test_AllEndpoints_AllEndpoints_ReadonlyEndpoints_HandleDBNotFound(t *testing.T) {
	ctx := createContext(t)

	for _, testcase := range testcases {
		if testcase.verb == "Update" || testcase.verb == "Delete" || testcase.description == "ListApplications" {
			continue
		}

		t.Run(testcase.description, func(t *testing.T) {
			test := createRPTest(t)

			// configure the mock to return not found
			testcase.setupDB(test.db, db.ErrNotFound)

			response, err := testcase.invoke(test.rp, ctx, testcase.id)
			require.NoError(t, err)

			require.Equal(t, rest.NewNotFoundResponse(testcase.id), response)
		})
	}
}

func Test_AllEndpoints_AllEndpoints_DeleteEndpoints_AllowDBNotFound(t *testing.T) {
	ctx := createContext(t)

	for _, testcase := range testcases {
		if testcase.verb != "Delete" {
			continue
		}

		t.Run(testcase.description, func(t *testing.T) {
			test := createRPTest(t)

			// configure the mock to return not found
			testcase.setupDB(test.db, db.ErrNotFound)

			response, err := testcase.invoke(test.rp, ctx, testcase.id)
			require.NoError(t, err)

			require.Equal(t, rest.NewNoContentResponse(), response)
		})
	}
}

func Test_AllEndpoints_PropagateUnexpectedError(t *testing.T) {
	ctx := createContext(t)

	for _, testcase := range testcases {
		t.Run(testcase.description, func(t *testing.T) {
			test := createRPTest(t)

			// configure the mock to return not found
			testcase.setupDB(test.db, errors.New("some other error"))

			response, err := testcase.invoke(test.rp, ctx, testcase.id)
			require.Error(t, err)
			require.Nil(t, response)
		})
	}
}

func Test_ListApplications_Success(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(applicationListID())
	appID := "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app"
	data := []db.ApplicationResource{
		{
			ID:              appID,
			Type:            id.Type(),
			SubscriptionID:  subscriptionID,
			ResourceGroup:   resourceGroup,
			ApplicationName: applicationName,
			Tags: map[string]string{
				"tag": "value",
			},
			Location: testLocation,
		},
	}

	resource := []db.RadiusResource{
		{
			ID:                testID,
			Type:              id.Type(),
			SubscriptionID:    subscriptionID,
			ResourceGroup:     resourceGroup,
			ApplicationName:   applicationName,
			ResourceName:      resourceName,
			ProvisioningState: "string(rest.SuccededStatus)",
			Status: db.RadiusResourceStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
			},
			Definition: map[string]interface{}{
				"data": true,
			},
		},
	}

	test.db.EXPECT().ListV3Applications(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)
	test.db.EXPECT().ListAllV3ResourcesByApplication(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(resource, nil)
	test.db.EXPECT().ListAllAzureResourcesForApplication(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]db.AzureResource{}, nil)

	response, err := test.rp.ListApplications(ctx, id)
	require.NoError(t, err)

	expected := ApplicationResourceList{
		Value: []ApplicationResource{
			{
				ID:   appID,
				Type: id.Type(),
				Name: applicationName,
				Tags: map[string]string{
					"tag": "value",
				},
				Location: testLocation,
				Properties: map[string]interface{}{
					"status": rest.ApplicationStatus{
						ProvisioningState: "Provisioned",
						HealthState:       "Healthy",
					},
				},
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_GetApplication_Success(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(applicationListID())
	data := db.ApplicationResource{
		ID:              testID,
		Type:            id.Type(),
		SubscriptionID:  subscriptionID,
		ResourceGroup:   resourceGroup,
		ApplicationName: applicationName,
		Tags: map[string]string{
			"tag": "value",
		},
		Location: testLocation,
	}

	resource := []db.RadiusResource{
		{
			ID:                testID,
			Type:              id.Type(),
			SubscriptionID:    subscriptionID,
			ResourceGroup:     resourceGroup,
			ApplicationName:   applicationName,
			ResourceName:      resourceName,
			ProvisioningState: "string(rest.SuccededStatus)",
			Status: db.RadiusResourceStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
			},
			Definition: map[string]interface{}{
				"data": true,
			},
		},
	}

	test.db.EXPECT().GetV3Application(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)
	test.db.EXPECT().ListAllV3ResourcesByApplication(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(resource, nil)
	test.db.EXPECT().ListAllAzureResourcesForApplication(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]db.AzureResource{}, nil)

	response, err := test.rp.GetApplication(ctx, id)
	require.NoError(t, err)

	expected := ApplicationResource{
		ID:   testID,
		Type: id.Type(),
		Name: applicationName,
		Tags: map[string]string{
			"tag": "value",
		},
		Location: testLocation,
		Properties: map[string]interface{}{
			"status": rest.ApplicationStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_UpdateApplication_Success(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(applicationID(applicationName))
	input := ApplicationResource{
		Tags: map[string]string{
			"tag": "value",
		},
		Location:   testLocation,
		Properties: map[string]interface{}{},
	}
	b, err := json.Marshal(&input)
	require.NoError(t, err)

	test.db.EXPECT().UpdateV3ApplicationDefinition(gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(ctx context.Context, application db.ApplicationResource) (bool, error) {
			expected := db.ApplicationResource{
				ID:              id.ID,
				Type:            id.Type(),
				SubscriptionID:  subscriptionID,
				ResourceGroup:   resourceGroup,
				ApplicationName: applicationName,
				Tags: map[string]string{
					"tag": "value",
				},
				Location: testLocation,
			}
			require.Equal(t, expected, application)
			return false, nil
		})

	response, err := test.rp.UpdateApplication(ctx, id, b)
	require.NoError(t, err)

	expected := ApplicationResource{
		ID:   id.ID,
		Type: id.Type(),
		Name: applicationName,
		Tags: map[string]string{
			"tag": "value",
		},
		Location:   testLocation,
		Properties: map[string]interface{}{},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_UpdateApplication_InvalidPayload(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(applicationID(applicationName))
	response, err := test.rp.UpdateApplication(ctx, id, []byte{})
	require.Error(t, err)
	require.Nil(t, response)
}

func Test_DeleteApplication_Success(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(applicationID(applicationName))
	test.db.EXPECT().DeleteV3Application(gomock.Any(), gomock.Any()).Times(1).Return(nil)

	response, err := test.rp.DeleteApplication(ctx, id)
	require.NoError(t, err)

	require.Equal(t, rest.NewNoContentResponse(), response)
}

func Test_DeleteApplication_Conflict(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(applicationID(applicationName))
	test.db.EXPECT().DeleteV3Application(gomock.Any(), gomock.Any()).Times(1).Return(db.ErrConflict)

	response, err := test.rp.DeleteApplication(ctx, id)
	require.NoError(t, err)

	require.Equal(t, rest.NewConflictResponse(db.ErrConflict.Error()), response)
}

func Test_ListAllResources_Success(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	requestID := parseOrPanic(fmt.Sprintf("%s/%s", applicationID(applicationName), schema.GenericResourceType))
	azureConnectionID := "/subscriptions/az-resource-subscription/resourceGroups/az-resource-rg/providers/Microsoft.ServiceBus/namespaces/az-resource-name"
	expectedRadiusResources := []db.RadiusResource{
		{
			ID:                testID,
			Type:              requestID.Type(),
			SubscriptionID:    subscriptionID,
			ResourceGroup:     resourceGroup,
			ApplicationName:   applicationName,
			ResourceName:      resourceName,
			ProvisioningState: string(rest.SuccededStatus),
			Status:            db.RadiusResourceStatus{},
			Definition: map[string]interface{}{
				"data": true,
			},
		},
	}
	expectedAzureResources := []db.AzureResource{
		{
			ID:                        azureConnectionID,
			SubscriptionID:            "az-resource-subscription",
			ResourceGroup:             "az-resource-rg",
			ResourceName:              "az-resource-name",
			ResourceKind:              resourcekinds.Azure,
			Type:                      "Microsoft.ServiceBus/namespaces",
			ApplicationName:           applicationName,
			ApplicationSubscriptionID: subscriptionID,
			ApplicationResourceGroup:  resourceGroup,
			RadiusConnectionIDs:       []string{testID},
		},
	}

	test.db.EXPECT().GetV3Application(gomock.Any(), gomock.Any()).Times(1).Return(db.ApplicationResource{}, nil)
	test.db.EXPECT().ListAllV3ResourcesByApplication(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(expectedRadiusResources, nil)
	test.db.EXPECT().ListAllAzureResourcesForApplication(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1).Return(expectedAzureResources, nil)

	response, err := test.rp.ListAllV3ResourcesByApplication(ctx, requestID)
	require.NoError(t, err)

	expectedOutput := RadiusResourceList{
		Value: []RadiusResource{
			{
				ID:   testID,
				Type: requestID.Type(),
				Name: resourceName,
				Properties: map[string]interface{}{
					"data":              true,
					"provisioningState": string(rest.SuccededStatus),
					"status": rest.ResourceStatus{
						ProvisioningState: "Provisioned",
						HealthState:       "Healthy",
						OutputResources:   []rest.OutputResource{},
					},
				},
			},
			{
				ID:         azureConnectionID,
				Type:       "Microsoft.ServiceBus/namespaces",
				Name:       "az-resource-name",
				Properties: map[string]interface{}{},
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expectedOutput), response)
}

func Test_ListAllResources_Failure(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(fmt.Sprintf("%s/%s", applicationID(applicationName), schema.GenericResourceType))

	test.db.EXPECT().GetV3Application(gomock.Any(), gomock.Any()).Times(1).Return(db.ApplicationResource{}, nil)
	test.db.EXPECT().ListAllV3ResourcesByApplication(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]db.RadiusResource{}, errors.New("database connection error"))

	_, err := test.rp.ListAllV3ResourcesByApplication(ctx, id)
	require.Error(t, err)
}

func Test_ListResources_Success(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(resourceListID(applicationName, resourceType))
	data := []db.RadiusResource{
		{
			ID:                testID,
			Type:              id.Type(),
			SubscriptionID:    subscriptionID,
			ResourceGroup:     resourceGroup,
			ApplicationName:   applicationName,
			ResourceName:      resourceName,
			ProvisioningState: string(rest.SuccededStatus),
			Status:            db.RadiusResourceStatus{},
			Definition: map[string]interface{}{
				"data": true,
			},
		},
	}
	test.db.EXPECT().ListV3Resources(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)

	response, err := test.rp.ListResources(ctx, id)
	require.NoError(t, err)

	expected := RadiusResourceList{
		Value: []RadiusResource{
			{
				ID:   testID,
				Type: id.Type(),
				Name: resourceName,
				Properties: map[string]interface{}{
					"data":              true,
					"provisioningState": "Succeeded",
					"status": rest.ResourceStatus{
						ProvisioningState: "Provisioned",
						HealthState:       "Healthy",
						OutputResources:   []rest.OutputResource{},
					},
				},
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_GetResource_Success(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(resourceID(applicationName, resourceType, resourceName))
	data := db.RadiusResource{
		ID:                testID,
		Type:              id.Type(),
		SubscriptionID:    subscriptionID,
		ResourceGroup:     resourceGroup,
		ApplicationName:   applicationName,
		ResourceName:      resourceName,
		ProvisioningState: string(rest.SuccededStatus),
		Status:            db.RadiusResourceStatus{},
		Definition: map[string]interface{}{
			"data": true,
		},
	}
	test.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)

	response, err := test.rp.GetResource(ctx, id)
	require.NoError(t, err)

	expected := RadiusResource{
		ID:   testID,
		Type: id.Type(),
		Name: resourceName,
		Properties: map[string]interface{}{
			"data":              true,
			"provisioningState": "Succeeded",
			"status": rest.ResourceStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
				OutputResources:   []rest.OutputResource{},
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_UpdateResource_Success(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(resourceID(applicationName, resourceType, resourceName))
	input := RadiusResource{
		Properties: map[string]interface{}{
			"data": true,
		},
	}
	b, err := json.Marshal(&input)
	require.NoError(t, err)

	test.db.EXPECT().UpdateV3ResourceDefinition(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) (bool, error) {
			expected := db.RadiusResource{
				ID:                id.ID,
				Type:              id.Type(),
				SubscriptionID:    subscriptionID,
				ResourceGroup:     resourceGroup,
				ApplicationName:   applicationName,
				ResourceName:      resourceName,
				ProvisioningState: string(rest.DeployingStatus),
				Definition: map[string]interface{}{
					"data": true,
				},
			}
			require.Equal(t, expected, resource)
			return false, nil
		})

	oid := azresources.ResourceID{}
	test.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(ctx context.Context, id azresources.ResourceID, operation *db.Operation) (bool, error) {
			// Operations have some generated things like the time. Don't validate deeply.
			require.Equal(t, db.OperationKindUpdate, operation.OperationKind)
			require.Equal(t, string(rest.DeployingStatus), operation.Status)
			oid = id
			return true, nil
		})

	// There's a race condition here due to goroutines. This may or not be called before the test ends.
	test.deploy.EXPECT().Deploy(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	response, err := test.rp.UpdateResource(ctx, id, b)
	require.NoError(t, err)

	expected := RadiusResource{
		ID:   id.ID,
		Type: id.Type(),
		Name: resourceName,
		Properties: map[string]interface{}{
			"data":              true,
			"provisioningState": string(rest.DeployingStatus),
			"status": rest.ResourceStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
				OutputResources:   []rest.OutputResource{},
			},
		},
	}
	require.Equal(t, rest.NewAcceptedAsyncResponse(expected, oid.ID), response)

	// Drain completion to ensure operation finishes
	<-test.completions
}

func Test_UpdateResource_InvalidPayload(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(resourceID(applicationName, resourceType, resourceName))
	response, err := test.rp.UpdateResource(ctx, id, []byte{})
	require.Error(t, err)
	require.Nil(t, response)
}

func Test_DeleteResource_Success(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(resourceID(applicationName, resourceType, resourceName))

	data := db.RadiusResource{
		ID:                id.ID,
		Type:              id.Type(),
		SubscriptionID:    subscriptionID,
		ResourceGroup:     resourceGroup,
		ApplicationName:   applicationName,
		ResourceName:      resourceName,
		ProvisioningState: string(rest.SuccededStatus),
		Status:            db.RadiusResourceStatus{},
		Definition: map[string]interface{}{
			"data": true,
		},
	}
	test.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)

	test.db.EXPECT().UpdateV3ResourceDefinition(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) (bool, error) {
			data.ProvisioningState = string(rest.DeletingStatus)
			require.Equal(t, data, resource)
			return false, nil
		})

	oid := azresources.ResourceID{}
	test.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(ctx context.Context, id azresources.ResourceID, operation *db.Operation) (bool, error) {
			// Operations have some generated things like the time. Don't validate deeply.
			require.Equal(t, db.OperationKindDelete, operation.OperationKind)
			require.Equal(t, string(rest.DeletingStatus), operation.Status)
			oid = id
			return true, nil
		})

	// There's a race condition here due to goroutines. This may or not be called before the test ends.
	test.deploy.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	response, err := test.rp.DeleteResource(ctx, id)
	require.NoError(t, err)

	expected := RadiusResource{
		ID:   id.ID,
		Type: id.Type(),
		Name: resourceName,
		Properties: map[string]interface{}{
			"data":              true,
			"provisioningState": string(rest.DeletingStatus),
			"status": rest.ResourceStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
				OutputResources:   []rest.OutputResource{},
			},
		},
	}
	require.Equal(t, rest.NewAcceptedAsyncResponse(expected, oid.ID), response)

	// Drain completion to ensure operation finishes
	<-test.completions
}

func Test_GetOperation_BadRequest(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(operationID(applicationName, resourceType, resourceName, operationName))
	data := &db.Operation{
		Error: &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: "bad data",
		},
	}
	test.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)

	response, err := test.rp.GetOperation(ctx, id)
	require.NoError(t, err)

	expected := armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: "bad data",
		},
	}
	require.Equal(t, rest.NewBadRequestARMResponse(expected), response)
}

func Test_GetOperation_InternalError(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(operationID(applicationName, resourceType, resourceName, operationName))
	data := &db.Operation{
		Error: &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: "failed, sorry",
		},
	}
	test.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)

	response, err := test.rp.GetOperation(ctx, id)
	require.NoError(t, err)

	expected := armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: "failed, sorry",
		},
	}
	require.Equal(t, rest.NewInternalServerErrorARMResponse(expected), response)
}

func Test_GetOperation_SuccessfulDelete(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	operation := &db.Operation{
		OperationKind: db.OperationKindDelete,
		Status:        string(rest.SuccededStatus),
	}
	id := parseOrPanic(operationID(applicationName, resourceType, resourceName, operationName))
	test.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(operation, nil)
	test.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, db.ErrNotFound)

	response, err := test.rp.GetOperation(ctx, id)
	require.NoError(t, err)
	require.Equal(t, rest.NewNoContentResponse(), response)
}

func Test_GetOperation_SuccessfulDeploy(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	operation := &db.Operation{
		OperationKind: db.OperationKindUpdate,
		Status:        string(rest.SuccededStatus),
	}
	id := parseOrPanic(operationID(applicationName, resourceType, resourceName, operationName))
	test.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(operation, nil)

	data := db.RadiusResource{
		ID:                id.Truncate().ID,
		Type:              id.Truncate().Type(),
		SubscriptionID:    subscriptionID,
		ResourceGroup:     resourceGroup,
		ApplicationName:   applicationName,
		ResourceName:      resourceName,
		ProvisioningState: string(rest.SuccededStatus),
		Status:            db.RadiusResourceStatus{},
		Definition: map[string]interface{}{
			"data": true,
		},
	}
	test.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)

	response, err := test.rp.GetOperation(ctx, id)
	require.NoError(t, err)

	expected := RadiusResource{
		ID:   id.Truncate().ID,
		Type: id.Truncate().Type(),
		Name: resourceName,
		Properties: map[string]interface{}{
			"data":              true,
			"provisioningState": string(rest.SuccededStatus),
			"status": rest.ResourceStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
				OutputResources:   []rest.OutputResource{},
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

func Test_GetOperation_DeployInProgress(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(operationID(applicationName, resourceType, resourceName, operationName))
	test.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)

	data := db.RadiusResource{
		ID:                id.Truncate().ID,
		Type:              id.Truncate().Type(),
		SubscriptionID:    subscriptionID,
		ResourceGroup:     resourceGroup,
		ApplicationName:   applicationName,
		ResourceName:      resourceName,
		ProvisioningState: string(rest.DeployingStatus),
		Status:            db.RadiusResourceStatus{},
		Definition: map[string]interface{}{
			"data": true,
		},
	}
	test.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)

	response, err := test.rp.GetOperation(ctx, id)
	require.NoError(t, err)

	expected := RadiusResource{
		ID:   id.Truncate().ID,
		Type: id.Truncate().Type(),
		Name: resourceName,
		Properties: map[string]interface{}{
			"data":              true,
			"provisioningState": string(rest.DeployingStatus),
			"status": rest.ResourceStatus{
				ProvisioningState: "Provisioned",
				HealthState:       "Healthy",
				OutputResources:   []rest.OutputResource{},
			},
		},
	}
	require.Equal(t, rest.NewAcceptedAsyncResponse(expected, id.ID), response)
}

func Test_OutputResponseWithHealthStatus(t *testing.T) {
	ctx := createContext(t)
	test := createRPTest(t)

	id := parseOrPanic(operationID(applicationName, resourceType, resourceName, operationName))
	test.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)

	data := db.RadiusResource{
		ID:                id.Truncate().ID,
		Type:              id.Truncate().Type(),
		SubscriptionID:    subscriptionID,
		ResourceGroup:     resourceGroup,
		ApplicationName:   applicationName,
		ResourceName:      resourceName,
		ProvisioningState: string(rest.SuccededStatus),
		Status: db.RadiusResourceStatus{
			OutputResources: []db.OutputResource{
				{
					LocalID:      outputresource.LocalIDDeployment,
					ResourceKind: resourcekinds.Kubernetes,
					Status: db.OutputResourceStatus{
						HealthState: healthcontract.HealthStateHealthy,
					},
				},
				{
					LocalID:      outputresource.LocalIDSecret,
					ResourceKind: resourcekinds.Kubernetes,
					Status: db.OutputResourceStatus{
						HealthState: healthcontract.HealthStateNotApplicable,
					},
				},
			},
		},
		Definition: map[string]interface{}{
			"data": true,
		},
	}

	test.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(data, nil)
	response, err := test.rp.GetOperation(ctx, id)
	require.NoError(t, err)

	expected := RadiusResource{
		ID:   id.Truncate().ID,
		Type: id.Truncate().Type(),
		Name: resourceName,
		Properties: map[string]interface{}{
			"data":              true,
			"provisioningState": string(rest.SuccededStatus),
			"status": rest.ResourceStatus{
				ProvisioningState: rest.ProvisioningStateProvisioned,
				HealthState:       healthcontract.HealthStateHealthy, // Aggregation should show Healthy
				OutputResources: []rest.OutputResource{
					{
						LocalID:      outputresource.LocalIDDeployment,
						ResourceKind: resourcekinds.Kubernetes,
						Status: rest.OutputResourceStatus{
							HealthState: healthcontract.HealthStateHealthy,
						},
					},
					{
						LocalID:      outputresource.LocalIDSecret,
						ResourceKind: resourcekinds.Kubernetes,
						Status: rest.OutputResourceStatus{
							HealthState: healthcontract.HealthStateHealthy, // NotApplicable should be shown as Healthy
						},
					},
				},
			},
		},
	}
	require.Equal(t, rest.NewOKResponse(expected), response)
}

type test struct {
	rp          ResourceProvider
	db          *db.MockRadrpDB
	deploy      *deployment.MockDeploymentProcessor
	completions <-chan struct{}
}

func createRPTest(t *testing.T) test {
	ctrl := gomock.NewController(t)
	db := db.NewMockRadrpDB(ctrl)
	deploy := deployment.NewMockDeploymentProcessor(ctrl)
	completions := make(chan struct{})
	rp := NewResourceProvider(db, deploy, completions)
	return test{rp: rp, db: db, deploy: deploy, completions: completions}
}

func parseOrPanic(resourceID string) azresources.ResourceID {
	id, err := azresources.Parse(resourceID)
	if err != nil {
		panic(err)
	}

	return id
}

func applicationListID() string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/%s/%s/%s",
		subscriptionID,
		resourceGroup,
		azresources.CustomProvidersResourceProviders,
		providerName,
		azresources.ApplicationResourceType)
}

func applicationID(applicationName string) string {
	return fmt.Sprintf("%s/%s", applicationListID(), applicationName)
}

func resourceListID(applicationName string, resourceType string) string {
	return fmt.Sprintf("%s/%s", applicationID(applicationName), resourceType)
}

func resourceID(applicationName string, resourceType string, resourceName string) string {
	return fmt.Sprintf("%s/%s", resourceListID(applicationName, resourceType), resourceName)
}

func operationID(applicationName string, resourceType string, resourceName string, operationName string) string {
	return fmt.Sprintf("%s/%s/%s", resourceID(applicationName, resourceType, resourceName), azresources.OperationResourceType, operationName)
}

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}
