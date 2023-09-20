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

package sqldatabases

import (
	"encoding/json"

	"github.com/radius-project/radius/pkg/datastoresrp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	"github.com/radius-project/radius/test/testutil"
)

const testHeaderfile = "20231001preview_requestheaders.json"

func getTestModels20231001preview() (input *v20231001preview.SQLDatabaseResource, dataModel *datamodel.SqlDatabase, output *v20231001preview.SQLDatabaseResource) {
	rawInput := testutil.ReadFixture("20231001preview_input.json")
	input = &v20231001preview.SQLDatabaseResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := testutil.ReadFixture("20231001preview_datamodel.json")
	dataModel = &datamodel.SqlDatabase{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := testutil.ReadFixture("20231001preview_output.json")
	output = &v20231001preview.SQLDatabaseResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}
