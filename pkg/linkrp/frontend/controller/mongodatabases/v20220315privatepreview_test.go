// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"encoding/json"

	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
)

const testHeaderfile = "20220315privatepreview_requestheaders.json"

func getTestModels20220315privatepreview() (input *v20220315privatepreview.MongoDatabaseResource, dataModel *datamodel.MongoDatabase, output *v20220315privatepreview.MongoDatabaseResource) {
	rawInput := radiustesting.ReadFixture("20220315privatepreview_input.json")
	input = &v20220315privatepreview.MongoDatabaseResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := radiustesting.ReadFixture("20220315privatepreview_datamodel.json")
	dataModel = &datamodel.MongoDatabase{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := radiustesting.ReadFixture("20220315privatepreview_output.json")
	output = &v20220315privatepreview.MongoDatabaseResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}

func getTestModelsForGetAndListApis20220315privatepreview() (input *v20220315privatepreview.MongoDatabaseResource, dataModel *datamodel.MongoDatabase, output *v20220315privatepreview.MongoDatabaseResource) {
	rawInput := radiustesting.ReadFixture("20220315privatepreview_input.json")
	input = &v20220315privatepreview.MongoDatabaseResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := radiustesting.ReadFixture("20220315privatepreview_datamodel.json")
	dataModel = &datamodel.MongoDatabase{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := radiustesting.ReadFixture("20220315privatepreview_output.json")
	output = &v20220315privatepreview.MongoDatabaseResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}
