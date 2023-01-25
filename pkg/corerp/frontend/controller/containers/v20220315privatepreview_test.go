// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"encoding/json"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.ContainerResource, *datamodel.ContainerResource, *v20220315privatepreview.ContainerResource) {
	rawInput := testutil.ReadFixture("container20220315privatepreview_input.json")
	containerInput := &v20220315privatepreview.ContainerResource{}
	_ = json.Unmarshal(rawInput, containerInput)

	rawDataModel := testutil.ReadFixture("container20220315privatepreview_datamodel.json")
	containerDataModel := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, containerDataModel)

	rawExpectedOutput := testutil.ReadFixture("container20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.ContainerResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return containerInput, containerDataModel, expectedOutput
}
