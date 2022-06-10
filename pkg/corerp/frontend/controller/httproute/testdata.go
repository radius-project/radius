// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproute

import (
	"encoding/json"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.HTTPRouteResource, *datamodel.HTTPRoute, *v20220315privatepreview.HTTPRouteResource) {
	rawInput := radiustesting.ReadFixture("httproute20220315privatepreview_input.json")
	httprouteInput := &v20220315privatepreview.HTTPRouteResource{}
	_ = json.Unmarshal(rawInput, httprouteInput)

	rawDataModel := radiustesting.ReadFixture("httproute20220315privatepreview_datamodel.json")
	httprouteDataModel := &datamodel.HTTPRoute{}
	_ = json.Unmarshal(rawDataModel, httprouteDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("httproute20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.HTTPRouteResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return httprouteInput, httprouteDataModel, expectedOutput
}
