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

package defaultoperation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
)

const (
	resourceTestHeaderFile        = "resource_requestheaders.json"
	operationStatusTestHeaderFile = "operationstatus_requestheaders.json"
	testAPIVersion                = "2022-03-15-privatepreview"
)

// TestResourceDataModel represents test resource.
type TestResourceDataModel struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties *TestResourceDataModelProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (r *TestResourceDataModel) ResourceTypeName() string {
	return "Applications.Core/resources"
}

// TestResourceDataModelProperties represents the properties of TestResourceDataModel.
type TestResourceDataModelProperties struct {
	Application string `json:"application"`
	Environment string `json:"environment"`
	PropertyA   string `json:"propertyA,omitempty"`
	PropertyB   string `json:"propertyB,omitempty"`
}

// TestResource represents test resource for api version.
type TestResource struct {
	ID         *string                 `json:"id,omitempty"`
	Name       *string                 `json:"name,omitempty"`
	SystemData *v1.SystemData          `json:"systemData,omitempty"`
	Type       *string                 `json:"type,omitempty"`
	Location   *string                 `json:"location,omitempty"`
	Properties *TestResourceProperties `json:"properties,omitempty"`
	Tags       map[string]*string      `json:"tags,omitempty"`
}

// TestResourceProperties - HTTP Route properties
type TestResourceProperties struct {
	ProvisioningState *v1.ProvisioningState `json:"provisioningState,omitempty"`
	Environment       *string               `json:"environment,omitempty"`
	Application       *string               `json:"application,omitempty"`
	PropertyA         *string               `json:"propertyA,omitempty"`
	PropertyB         *string               `json:"propertyB,omitempty"`
}

// ConvertTo converts a version specific TestResource into a version-agnostic resource, TestResourceDataModel.
func (src *TestResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &TestResourceDataModel{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      testAPIVersion,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: &TestResourceDataModelProperties{
			Application: to.String(src.Properties.Application),
			Environment: to.String(src.Properties.Environment),
			PropertyA:   to.String(src.Properties.PropertyA),
			PropertyB:   to.String(src.Properties.PropertyB),
		},
	}
	return converted, nil
}

// ConvertFrom converts src version agnostic model to versioned model, TestResource.
func (dst *TestResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*TestResourceDataModel)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(dm.ID)
	dst.Name = to.Ptr(dm.Name)
	dst.Type = to.Ptr(dm.Type)
	dst.SystemData = &dm.SystemData
	dst.Location = to.Ptr(dm.Location)
	dst.Tags = *to.StringMapPtr(dm.Tags)
	dst.Properties = &TestResourceProperties{
		ProvisioningState: fromProvisioningStateDataModel(dm.InternalMetadata.AsyncProvisioningState),
		Environment:       to.Ptr(dm.Properties.Environment),
		Application:       to.Ptr(dm.Properties.Application),
		PropertyA:         to.Ptr(dm.Properties.PropertyA),
		PropertyB:         to.Ptr(dm.Properties.PropertyB),
	}

	return nil
}

func toProvisioningStateDataModel(state *v1.ProvisioningState) v1.ProvisioningState {
	if state == nil {
		return v1.ProvisioningStateAccepted
	}
	return *state
}

func fromProvisioningStateDataModel(state v1.ProvisioningState) *v1.ProvisioningState {
	converted := v1.ProvisioningStateAccepted
	if state != "" {
		converted = state
	}

	return &converted
}

func testResourceDataModelToVersioned(model *TestResourceDataModel, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case testAPIVersion:
		versioned := &TestResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

func testResourceDataModelFromVersioned(content []byte, version string) (*TestResourceDataModel, error) {
	switch version {
	case testAPIVersion:
		am := &TestResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*TestResourceDataModel), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// testValidateRequest is an example resource filter.
//
// In this case we're validating that the application of an existing resource can't change. This is one of our scenarios
// for the corerp and portable resource providers. However we're avoiding calling into that code directly from here to avoid coupling.
func testValidateRequest(ctx context.Context, newResource *TestResourceDataModel, oldResource *TestResourceDataModel, options *controller.Options) (rest.Response, error) {
	if oldResource == nil {
		return nil, nil
	}

	if newResource.Properties.Application != oldResource.Properties.Application {
		return rest.NewBadRequestResponse("Oh no!"), nil
	}

	return nil, nil
}

func loadTestResurce() (*TestResource, *TestResourceDataModel, *TestResource) {
	reqBody := testutil.ReadFixture("resource-request.json")
	reqModel := &TestResource{}
	_ = json.Unmarshal(reqBody, reqModel)

	rawDataModel := testutil.ReadFixture("resource-datamodel.json")
	datamodel := &TestResourceDataModel{}
	_ = json.Unmarshal(rawDataModel, datamodel)

	respBody := testutil.ReadFixture("resource-response.json")
	respModel := &TestResource{}
	_ = json.Unmarshal(respBody, respModel)

	return reqModel, datamodel, respModel
}

func setupTest(tb testing.TB) (func(testing.TB), *store.MockStorageClient, *statusmanager.MockStatusManager) {
	mctrl := gomock.NewController(tb)
	mds := store.NewMockStorageClient(mctrl)
	msm := statusmanager.NewMockStatusManager(mctrl)

	return func(tb testing.TB) {
		mctrl.Finish()
	}, mds, msm
}

// TODO: Use Referer header instead of X-Forwarded-Proto by following ARM RPC spec - https://github.com/project-radius/radius/issues/3068
func getAsyncLocationPath(sCtx *v1.ARMRequestContext, location string, resourceType string, req *http.Request) string {
	dest := url.URL{
		Host:   req.Host,
		Scheme: req.URL.Scheme,
		Path: fmt.Sprintf("%s/providers/%s/locations/%s/%s/%s", sCtx.ResourceID.PlaneScope(),
			sCtx.ResourceID.ProviderNamespace(), location, resourceType, sCtx.OperationID.String()),
	}

	query := url.Values{}
	query.Add("api-version", sCtx.APIVersion)
	dest.RawQuery = query.Encode()

	protocol := req.Header.Get("X-Forwarded-Proto")
	if protocol != "" {
		dest.Scheme = protocol
	}

	if dest.Scheme == "" {
		dest.Scheme = "http"
	}

	return dest.String()
}
