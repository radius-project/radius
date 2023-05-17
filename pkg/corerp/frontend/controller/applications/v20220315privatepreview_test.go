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
package applications

import (
	"encoding/json"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.ApplicationResource, *datamodel.Application, *v20220315privatepreview.ApplicationResource) {
	rawInput := testutil.ReadFixture("application20220315privatepreview_input.json")
	appInput := &v20220315privatepreview.ApplicationResource{}
	_ = json.Unmarshal(rawInput, appInput)

	rawDataModel := testutil.ReadFixture("application20220315privatepreview_datamodel.json")
	appDataModel := &datamodel.Application{}
	_ = json.Unmarshal(rawDataModel, appDataModel)

	rawExpectedOutput := testutil.ReadFixture("application20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.ApplicationResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)
	return appInput, appDataModel, expectedOutput
}
