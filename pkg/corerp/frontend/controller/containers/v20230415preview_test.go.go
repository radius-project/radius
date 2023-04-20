// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"encoding/json"

	v20230415preview "github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20230415preview.json"

func getTestModels20230415preview() (*v20230415preview.ContainerResource, *datamodel.ContainerResource, *v20230415preview.ContainerResource) {
	rawInput := testutil.ReadFixture("container20230415preview_input.json")
	containerInput := &v20230415preview.ContainerResource{}
	_ = json.Unmarshal(rawInput, containerInput)

	rawDataModel := testutil.ReadFixture("container20230415preview_datamodel.json")
	containerDataModel := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, containerDataModel)

	rawExpectedOutput := testutil.ReadFixture("container20230415preview_output.json")
	expectedOutput := &v20230415preview.ContainerResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return containerInput, containerDataModel, expectedOutput
}
