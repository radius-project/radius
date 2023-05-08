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
------------------------------------------------------------
*/
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
