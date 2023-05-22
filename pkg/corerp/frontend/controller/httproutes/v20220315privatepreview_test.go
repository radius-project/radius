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

package httproutes

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.HTTPRouteResource, *datamodel.HTTPRoute, *v20220315privatepreview.HTTPRouteResource) {
	rawInput := testutil.ReadFixture("httproute20220315privatepreview_input.json")
	hrtInput := &v20220315privatepreview.HTTPRouteResource{}
	_ = json.Unmarshal(rawInput, hrtInput)

	rawDataModel := testutil.ReadFixture("httproute20220315privatepreview_datamodel.json")
	hrtDataModel := &datamodel.HTTPRoute{}
	_ = json.Unmarshal(rawDataModel, hrtDataModel)

	rawExpectedOutput := testutil.ReadFixture("httproute20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.HTTPRouteResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return hrtInput, hrtDataModel, expectedOutput
}
