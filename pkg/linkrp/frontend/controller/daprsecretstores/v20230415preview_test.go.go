// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstores

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/linkrp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "20230415preview_requestheaders.json"

func getTestModels20230415preview() (input *v20230415preview.DaprSecretStoreResource, dataModel *datamodel.DaprSecretStore, output *v20230415preview.DaprSecretStoreResource) {
	rawInput := testutil.ReadFixture("20230415preview_input.json")
	input = &v20230415preview.DaprSecretStoreResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := testutil.ReadFixture("20230415preview_datamodel.json")
	dataModel = &datamodel.DaprSecretStore{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := testutil.ReadFixture("20230415preview_output.json")
	output = &v20230415preview.DaprSecretStoreResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}
