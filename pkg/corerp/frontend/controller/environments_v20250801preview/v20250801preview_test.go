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

package environments_v20250801preview

import (
	"encoding/json"

	v20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/test/testutil"
)

const testHeaderfilev20250801preview = "requestheadersv20250801preview.json"

func getTestModelsv20250801preview() (*v20250801preview.EnvironmentResource, *datamodel.Environment_v20250801preview, *v20250801preview.EnvironmentResource) {
	rawInput := testutil.ReadFixture("environmentresourcev20250801preview_input.json")
	envInput := &v20250801preview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environmentresourcev20250801preview_datamodel.json")
	envDataModel := &datamodel.Environment_v20250801preview{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentresourcev20250801preview_output.json")
	expectedOutput := &v20250801preview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}