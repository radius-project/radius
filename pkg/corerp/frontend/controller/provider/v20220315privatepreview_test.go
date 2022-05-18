// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
)

const testHeaderfile = "operationstatus_requestheaders.json"

func getOperationStatusTestModels20220315privatepreview() (*armrpcv1.AsyncOperationStatus, *datamodel.AsyncOperationStatus) {
	rawDataModel := radiustesting.ReadFixture("operationstatus_datamodel.json")
	osDataModel := &armrpcv1.AsyncOperationStatus{}
	_ = json.Unmarshal(rawDataModel, osDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("operationstatus_output.json")
	expectedOutput := &datamodel.AsyncOperationStatus{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return osDataModel, expectedOutput
}
