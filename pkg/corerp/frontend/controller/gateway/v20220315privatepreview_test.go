// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package gateway

import (
	"encoding/json"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.GatewayResource, *datamodel.Gateway, *v20220315privatepreview.GatewayResource) {
	rawInput := radiustesting.ReadFixture("gateway20220315privatepreview_input.json")
	gtwyInput := &v20220315privatepreview.GatewayResource{}
	_ = json.Unmarshal(rawInput, gtwyInput)

	rawDataModel := radiustesting.ReadFixture("gateway20220315privatepreview_datamodel.json")
	gtwyDataModel := &datamodel.Gateway{}
	_ = json.Unmarshal(rawDataModel, gtwyDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("gateway20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.GatewayResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)
	return gtwyInput, gtwyDataModel, expectedOutput
}
