// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/linkrp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "20230415preview_requestheaders.json"

func getTestModels20230415preview() (input *v20230415preview.SQLDatabaseResource, dataModel *datamodel.SqlDatabase, output *v20230415preview.SQLDatabaseResource) {
	rawInput := testutil.ReadFixture("20230415preview_input.json")
	input = &v20230415preview.SQLDatabaseResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := testutil.ReadFixture("20230415preview_datamodel.json")
	dataModel = &datamodel.SqlDatabase{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := testutil.ReadFixture("20230415preview_output.json")
	output = &v20230415preview.SQLDatabaseResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}
