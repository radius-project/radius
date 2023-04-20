// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package gateway

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20230415preview.json"

func getTestModels20230415preview() (*v20230415preview.GatewayResource, *datamodel.Gateway, *v20230415preview.GatewayResource) {
	rawInput := testutil.ReadFixture("gateway20230415preview_input.json")
	gtwyInput := &v20230415preview.GatewayResource{}
	_ = json.Unmarshal(rawInput, gtwyInput)

	rawDataModel := testutil.ReadFixture("gateway20230415preview_datamodel.json")
	gtwyDataModel := &datamodel.Gateway{}
	_ = json.Unmarshal(rawDataModel, gtwyDataModel)

	rawExpectedOutput := testutil.ReadFixture("gateway20230415preview_output.json")
	expectedOutput := &v20230415preview.GatewayResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)
	return gtwyInput, gtwyDataModel, expectedOutput
}
