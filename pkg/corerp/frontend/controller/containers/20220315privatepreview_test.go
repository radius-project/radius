// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"encoding/json"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
)

const testHeaderfile = "20220315privatepreview_requestheaders.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.ContainerResource, *datamodel.ContainerResource, *v20220315privatepreview.ContainerResource) {
	rawInput := radiustesting.ReadFixture("20220315privatepreview_input.json")
	containerVersioned := &v20220315privatepreview.ContainerResource{}
	_ = json.Unmarshal(rawInput, containerVersioned)

	rawDataModel := radiustesting.ReadFixture("20220315privatepreview_datamodel.json")
	containerDataModel := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, containerDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.ContainerResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return containerVersioned, containerDataModel, expectedOutput
}
