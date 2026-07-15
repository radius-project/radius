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

package v20250801preview

import (
	"context"
	"errors"
	"strings"
	"testing"

	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	tfConfigID      = "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/terraformSettings/tf"
	bicepSettingsID = "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/bicepSettings/bc"
	recipePackID    = "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/pack"
)

// newControllerForValidateConfigRef builds a CreateOrUpdateEnvironmentv20250801preview
// wired with the supplied database client. We construct the controller exactly the
// way NewCreateOrUpdateEnvironmentv20250801preview does so that GetResource
// behaves identically to production.
func newControllerForValidateConfigRef(databaseClient database.Client) *CreateOrUpdateEnvironmentv20250801preview {
	opts := ctrl.Options{DatabaseClient: databaseClient}
	return &CreateOrUpdateEnvironmentv20250801preview{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Environment_v20250801preview]{
				RequestConverter:  converter.Environment20250801DataModelFromVersioned,
				ResponseConverter: converter.Environment20250801DataModelToVersioned,
			},
		),
	}
}

func TestValidateConfigRef_InvalidResourceID(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()
	databaseClient := database.NewMockClient(mctrl)
	// No DB calls expected: parsing fails first.

	e := newControllerForValidateConfigRef(databaseClient)
	resp := validateConfigRef(context.Background(), e, "not a resource id", datamodel.TerraformSettingsResourceType, "terraformSettings")

	br, ok := resp.(*rest.BadRequestResponse)
	require.True(t, ok, "expected BadRequestResponse, got %T", resp)
	require.Contains(t, br.Body.Error.Message, "Invalid terraformSettings resource ID")
}

func TestValidateConfigRef_WrongType(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()
	databaseClient := database.NewMockClient(mctrl)
	// No DB calls: type mismatch is rejected before the lookup.

	e := newControllerForValidateConfigRef(databaseClient)
	// recipePackID has a valid ARM-style structure but the wrong resource type.
	resp := validateConfigRef(context.Background(), e, recipePackID, datamodel.TerraformSettingsResourceType, "terraformSettings")

	br, ok := resp.(*rest.BadRequestResponse)
	require.True(t, ok, "expected BadRequestResponse, got %T", resp)
	msg := br.Body.Error.Message
	require.True(t, strings.Contains(msg, "expected") && strings.Contains(msg, "terraformSettings"),
		"unexpected error message: %s", msg)
}

func TestValidateConfigRef_NotFound_ReturnsBadRequest(t *testing.T) {
	// Regression test: Operation.GetResource swallows ErrNotFound (clears the
	// error and returns out=nil). validateConfigRef must inspect the returned
	// resource — not just err — to detect missing references.
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()
	databaseClient := database.NewMockClient(mctrl)

	databaseClient.EXPECT().
		Get(gomock.Any(), tfConfigID).
		Return(nil, &database.ErrNotFound{ID: tfConfigID})

	e := newControllerForValidateConfigRef(databaseClient)
	resp := validateConfigRef(context.Background(), e, tfConfigID, datamodel.TerraformSettingsResourceType, "terraformSettings")

	br, ok := resp.(*rest.BadRequestResponse)
	require.True(t, ok, "expected BadRequestResponse for missing resource, got %T", resp)
	require.Contains(t, br.Body.Error.Message, "does not exist")
}

func TestValidateConfigRef_DatabaseError_ReturnsInternalServerError(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()
	databaseClient := database.NewMockClient(mctrl)

	databaseClient.EXPECT().
		Get(gomock.Any(), tfConfigID).
		Return(nil, errors.New("database is on fire"))

	e := newControllerForValidateConfigRef(databaseClient)
	resp := validateConfigRef(context.Background(), e, tfConfigID, datamodel.TerraformSettingsResourceType, "terraformSettings")

	ise, ok := resp.(*rest.InternalServerErrorResponse)
	require.True(t, ok, "expected InternalServerErrorResponse for transport failure, got %T", resp)
	require.Contains(t, ise.Body.Error.Message, "database is on fire")
}

func TestValidateConfigRef_HappyPath_TerraformSettings(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()
	databaseClient := database.NewMockClient(mctrl)

	versioned := &v20250801preview.TerraformSettingsResource{
		Properties: &v20250801preview.TerraformSettingsProperties{},
	}
	databaseClient.EXPECT().
		Get(gomock.Any(), tfConfigID).
		Return(&database.Object{
			Metadata: database.Metadata{ID: tfConfigID, ETag: "etag-1"},
			Data:     versioned,
		}, nil)

	e := newControllerForValidateConfigRef(databaseClient)
	resp := validateConfigRef(context.Background(), e, tfConfigID, datamodel.TerraformSettingsResourceType, "terraformSettings")
	require.Nil(t, resp, "expected validateConfigRef to return nil on success")
}

func TestValidateConfigRef_HappyPath_BicepSettings(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()
	databaseClient := database.NewMockClient(mctrl)

	versioned := &v20250801preview.BicepSettingsResource{
		Properties: &v20250801preview.BicepSettingsProperties{},
	}
	databaseClient.EXPECT().
		Get(gomock.Any(), bicepSettingsID).
		Return(&database.Object{
			Metadata: database.Metadata{ID: bicepSettingsID, ETag: "etag-1"},
			Data:     versioned,
		}, nil)

	e := newControllerForValidateConfigRef(databaseClient)
	resp := validateConfigRef(context.Background(), e, bicepSettingsID, datamodel.BicepSettingsResourceType, "bicepSettings")
	require.Nil(t, resp, "expected validateConfigRef to return nil on success")
}
