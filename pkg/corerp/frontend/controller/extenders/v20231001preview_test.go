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

package extenders

import (
	"encoding/json"

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/test/testutil"
)

const testHeaderfile = "20231001preview_requestheaders.json"

func getTestModels20231001preview() (input *v20231001preview.ExtenderResource, dataModel *datamodel.Extender, output *v20231001preview.ExtenderResource) {
	rawInput := testutil.ReadFixture("20231001preview_input.json")
	input = &v20231001preview.ExtenderResource{}
	err := json.Unmarshal(rawInput, input)
	if err != nil {
		return nil, nil, nil
	}
	rawDataModel := testutil.ReadFixture("20231001preview_datamodel.json")
	dataModel = &datamodel.Extender{}
	err = json.Unmarshal(rawDataModel, dataModel)
	if err != nil {
		return nil, nil, nil
	}
	rawExpectedOutput := testutil.ReadFixture("20231001preview_output.json")
	output = &v20231001preview.ExtenderResource{}
	err = json.Unmarshal(rawExpectedOutput, output)
	if err != nil {
		return nil, nil, nil
	}

	return input, dataModel, output
}
