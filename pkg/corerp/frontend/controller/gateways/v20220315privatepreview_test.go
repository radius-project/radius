// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package gateways

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.GatewayResource, *datamodel.Gateway, *v20220315privatepreview.GatewayResource) {
	rawInput := testutil.ReadFixture("gateway20220315privatepreview_input.json")
	gtwyInput := &v20220315privatepreview.GatewayResource{}
	_ = json.Unmarshal(rawInput, gtwyInput)

	rawDataModel := testutil.ReadFixture("gateway20220315privatepreview_datamodel.json")
	gtwyDataModel := &datamodel.Gateway{}
	_ = json.Unmarshal(rawDataModel, gtwyDataModel)

	rawExpectedOutput := testutil.ReadFixture("gateway20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.GatewayResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)
	return gtwyInput, gtwyDataModel, expectedOutput
}
