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

const testHeaderfile = "requestheaders_20220315privatepreview.json"

func getTestModels_20220315privatepreview() (*v20220315privatepreview.ContainerResource, *datamodel.ContainerResource, *v20220315privatepreview.ContainerResource) {
	rawInput := radiustesting.ReadFixture("input_20220315privatepreview.json")
	containerVersioned := &v20220315privatepreview.ContainerResource{}
	_ = json.Unmarshal(rawInput, containerVersioned)

	rawDataModel := radiustesting.ReadFixture("datamodel_20220315privatepreview.json")
	containerDataModel := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, containerDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("output_20220315privatepreview.json")
	expectedOutput := &v20220315privatepreview.ContainerResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return containerVersioned, containerDataModel, expectedOutput
}
