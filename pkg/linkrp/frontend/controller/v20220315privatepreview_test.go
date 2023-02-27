// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"encoding/json"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	radiustesting "github.com/project-radius/radius/test/testutil"
)

func getTestModels20220315privatepreview[P interface {
	*D
	v1.ResourceDataModel
}, I any, D any, O any](input *I, dataModel P, output *O, useDiff bool) (testHeaderFileName string) {
	var rawInput []byte
	var rawExpectedOutput []byte

	resourceType := strings.ToLower(dataModel.ResourceTypeName())
	folder := strings.ToLower(strings.Split(resourceType, "/")[1]) + "/"

	if useDiff {
		rawInput = radiustesting.ReadFixture(folder + "20220315privatepreview_input_diff.json")

	} else {
		rawInput = radiustesting.ReadFixture(folder + "20220315privatepreview_input.json")
	}

	rawExpectedOutput = radiustesting.ReadFixture(folder + "20220315privatepreview_output.json")
	testHeaderFileName = getTestHeaderFileName(resourceType)

	_ = json.Unmarshal(rawInput, input)

	getTestDataModel20220315privatepreview(dataModel)
	_ = json.Unmarshal(rawExpectedOutput, output)

	return testHeaderFileName
}

func getTestDataModel20220315privatepreview[P interface {
	*D
	v1.ResourceDataModel
}, D any](dataModel P) {
	var rawDataModel []byte

	resourceType := strings.ToLower(dataModel.ResourceTypeName())
	folder := strings.ToLower(strings.Split(resourceType, "/")[1]) + "/"

	rawDataModel = radiustesting.ReadFixture(folder + "20220315privatepreview_datamodel.json")
	_ = json.Unmarshal(rawDataModel, dataModel)
}

func getTestHeaderFileName(resourceType string) string {
	resourceType = strings.ToLower(strings.Split(resourceType, "/")[1])
	return resourceType + "/20220315privatepreview_requestheaders.json"
}
func createDataModelForLinkType(resourceType string) (dataModel any) {
	switch resourceType {
	case strings.ToLower(linkrp.DaprInvokeHttpRoutesResourceType):
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		getTestDataModel20220315privatepreview(dataModel)
		return dataModel
	case strings.ToLower(linkrp.DaprPubSubBrokersResourceType):
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		getTestDataModel20220315privatepreview(dataModel)
		return dataModel
	case strings.ToLower(linkrp.DaprSecretStoresResourceType):
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		getTestDataModel20220315privatepreview(dataModel)
		return dataModel
	case strings.ToLower(linkrp.DaprStateStoresResourceType):
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		getTestDataModel20220315privatepreview(dataModel)
		return dataModel
	case strings.ToLower(linkrp.ExtendersResourceType):
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		getTestDataModel20220315privatepreview(dataModel)
		return dataModel
	case strings.ToLower(linkrp.MongoDatabasesResourceType):
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		getTestDataModel20220315privatepreview(dataModel)
		return dataModel
	case strings.ToLower(linkrp.RabbitMQMessageQueuesResourceType):
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		getTestDataModel20220315privatepreview(dataModel)
		return dataModel
	case strings.ToLower(linkrp.RedisCachesResourceType):
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		getTestDataModel20220315privatepreview(dataModel)
		return dataModel
	case strings.ToLower(linkrp.SqlDatabasesResourceType):
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		getTestDataModel20220315privatepreview(dataModel)
		return dataModel
	}

	return
}

func createDataForLinkType(resourceType string, useDiff bool) (input any, dataModel any, output any, actualOutput any, testHeaderFileName string) {
	switch strings.ToLower(resourceType) {
	case strings.ToLower(linkrp.DaprInvokeHttpRoutesResourceType):
		input := new(v20220315privatepreview.DaprInvokeHTTPRouteResource)
		dataModel := new(datamodel.DaprInvokeHttpRoute)
		expectedOutput := new(v20220315privatepreview.DaprInvokeHTTPRouteResource)
		testHeaderFileName = getTestModels20220315privatepreview(input, dataModel, expectedOutput, useDiff)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		// First time created objects should have the same lastModifiedAt and createdAt
		dataModel.SystemData.CreatedAt = dataModel.SystemData.LastModifiedAt
		actualOutput := new(v20220315privatepreview.DaprInvokeHTTPRouteResource)

		return input, dataModel, expectedOutput, actualOutput, testHeaderFileName
	case strings.ToLower(linkrp.DaprPubSubBrokersResourceType):
		input := new(v20220315privatepreview.DaprPubSubBrokerResource)
		dataModel := new(datamodel.DaprPubSubBroker)
		expectedOutput := new(v20220315privatepreview.DaprPubSubBrokerResource)
		testHeaderFileName = getTestModels20220315privatepreview(input, dataModel, expectedOutput, useDiff)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		// First time created objects should have the same lastModifiedAt and createdAt
		dataModel.SystemData.CreatedAt = dataModel.SystemData.LastModifiedAt
		actualOutput := new(v20220315privatepreview.DaprPubSubBrokerResource)

		return input, dataModel, expectedOutput, actualOutput, testHeaderFileName
	case strings.ToLower(linkrp.DaprSecretStoresResourceType):
		input := new(v20220315privatepreview.DaprSecretStoreResource)
		dataModel := new(datamodel.DaprSecretStore)
		expectedOutput := new(v20220315privatepreview.DaprSecretStoreResource)
		testHeaderFileName = getTestModels20220315privatepreview(input, dataModel, expectedOutput, useDiff)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		// First time created objects should have the same lastModifiedAt and createdAt
		dataModel.SystemData.CreatedAt = dataModel.SystemData.LastModifiedAt
		actualOutput := new(v20220315privatepreview.DaprSecretStoreResource)

		return input, dataModel, expectedOutput, actualOutput, testHeaderFileName
	case strings.ToLower(linkrp.DaprStateStoresResourceType):
		input := new(v20220315privatepreview.DaprStateStoreResource)
		dataModel := new(datamodel.DaprStateStore)
		expectedOutput := new(v20220315privatepreview.DaprStateStoreResource)
		testHeaderFileName = getTestModels20220315privatepreview(input, dataModel, expectedOutput, useDiff)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		// First time created objects should have the same lastModifiedAt and createdAt
		dataModel.SystemData.CreatedAt = dataModel.SystemData.LastModifiedAt
		actualOutput := new(v20220315privatepreview.DaprStateStoreResource)

		return input, dataModel, expectedOutput, actualOutput, testHeaderFileName
	case strings.ToLower(linkrp.ExtendersResourceType):
		input := new(v20220315privatepreview.ExtenderResource)
		dataModel := new(datamodel.Extender)
		expectedOutput := new(v20220315privatepreview.ExtenderResource)
		testHeaderFileName = getTestModels20220315privatepreview(input, dataModel, expectedOutput, useDiff)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		// First time created objects should have the same lastModifiedAt and createdAt
		dataModel.SystemData.CreatedAt = dataModel.SystemData.LastModifiedAt
		actualOutput := new(v20220315privatepreview.ExtenderResource)

		return input, dataModel, expectedOutput, actualOutput, testHeaderFileName
	case strings.ToLower(linkrp.MongoDatabasesResourceType):
		input := new(v20220315privatepreview.MongoDatabaseResource)
		dataModel := new(datamodel.MongoDatabase)
		expectedOutput := new(v20220315privatepreview.MongoDatabaseResource)
		testHeaderFileName = getTestModels20220315privatepreview(input, dataModel, expectedOutput, useDiff)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		// First time created objects should have the same lastModifiedAt and createdAt
		dataModel.SystemData.CreatedAt = dataModel.SystemData.LastModifiedAt
		actualOutput := new(v20220315privatepreview.MongoDatabaseResource)

		return input, dataModel, expectedOutput, actualOutput, testHeaderFileName
	case strings.ToLower(linkrp.RabbitMQMessageQueuesResourceType):
		input := new(v20220315privatepreview.RabbitMQMessageQueueResource)
		dataModel := new(datamodel.RabbitMQMessageQueue)
		expectedOutput := new(v20220315privatepreview.RabbitMQMessageQueueResource)
		testHeaderFileName = getTestModels20220315privatepreview(input, dataModel, expectedOutput, useDiff)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		// First time created objects should have the same lastModifiedAt and createdAt
		dataModel.SystemData.CreatedAt = dataModel.SystemData.LastModifiedAt
		actualOutput := new(v20220315privatepreview.RabbitMQMessageQueueResource)

		return input, dataModel, expectedOutput, actualOutput, testHeaderFileName
	case strings.ToLower(linkrp.RedisCachesResourceType):
		input := new(v20220315privatepreview.RedisCacheResource)
		dataModel := new(datamodel.RedisCache)
		expectedOutput := new(v20220315privatepreview.RedisCacheResource)
		testHeaderFileName = getTestModels20220315privatepreview(input, dataModel, expectedOutput, useDiff)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		// First time created objects should have the same lastModifiedAt and createdAt
		dataModel.SystemData.CreatedAt = dataModel.SystemData.LastModifiedAt
		actualOutput := new(v20220315privatepreview.RedisCacheResource)

		return input, dataModel, expectedOutput, actualOutput, testHeaderFileName
	case strings.ToLower(linkrp.SqlDatabasesResourceType):
		input := new(v20220315privatepreview.SQLDatabaseResource)
		dataModel := new(datamodel.SqlDatabase)
		expectedOutput := new(v20220315privatepreview.SQLDatabaseResource)
		testHeaderFileName = getTestModels20220315privatepreview(input, dataModel, expectedOutput, useDiff)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		// First time created objects should have the same lastModifiedAt and createdAt
		dataModel.SystemData.CreatedAt = dataModel.SystemData.LastModifiedAt
		actualOutput := new(v20220315privatepreview.SQLDatabaseResource)

		return input, dataModel, expectedOutput, actualOutput, testHeaderFileName
	}

	return
}
