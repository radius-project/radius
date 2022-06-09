// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
)

const testHeaderfile = "20220315privatepreview_requestheaders.json"

func getTestModels20220315privatepreview() (input *v20220315privatepreview.DaprPubSubBrokerResource, dataModel *datamodel.DaprPubSubBroker, output *v20220315privatepreview.DaprPubSubBrokerResource) {
	rawInput := radiustesting.ReadFixture("20220315privatepreview_input.json")
	input = &v20220315privatepreview.DaprPubSubBrokerResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := radiustesting.ReadFixture("20220315privatepreview_datamodel.json")
	dataModel = &datamodel.DaprPubSubBroker{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := radiustesting.ReadFixture("20220315privatepreview_output.json")
	output = &v20220315privatepreview.DaprPubSubBrokerResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}
