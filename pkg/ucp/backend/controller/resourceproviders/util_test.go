/*
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

package resourceproviders

import (
	"context"
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/ucp/database"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/util/etag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestResourceProviderSummaryIDFromRequest(t *testing.T) {
	tests := []struct {
		name              string
		request           *ctrl.Request
		expectedID        resources.ID
		expectedSummaryID resources.ID
		expectedErr       bool
	}{
		{
			name: "valid resource provider ID",
			request: &ctrl.Request{
				ResourceID: "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test",
			},
			expectedID:        resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test"),
			expectedSummaryID: resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviderSummaries/Applications.Test"),
			expectedErr:       false,
		},
		{
			name: "valid resource provider child ID",
			request: &ctrl.Request{
				ResourceID: "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources/apiVersions/2025-01-01",
			},
			expectedID:        resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources/apiVersions/2025-01-01"),
			expectedSummaryID: resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviderSummaries/Applications.Test"),
			expectedErr:       false,
		},
		{
			name: "invalid resource provider ID",
			request: &ctrl.Request{
				ResourceID: "/subscriptions/123/resourceGroups/rg/providers/Microsoft.Provider/invalidType/rp",
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, summaryID, err := resourceProviderSummaryIDFromRequest(tt.request)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedID, id)
				assert.Equal(t, tt.expectedSummaryID, summaryID)
			}
		})
	}
}

func Test_UpdateResourceProviderSummaryWithETag(t *testing.T) {
	summaryID := resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviderSummaries/Applications.Test")
	tests := []struct {
		name         string
		summaryID    resources.ID
		policy       summaryNotFoundPolicy
		updateFunc   func(summary *datamodel.ResourceProviderSummary) error
		expectedErr  bool
		expectedSave bool
		existing     *datamodel.ResourceProviderSummary
	}{
		{
			name:      "create new summary",
			summaryID: summaryID,
			policy:    summaryNotFoundCreate,
			updateFunc: func(summary *datamodel.ResourceProviderSummary) error {
				summary.Properties = datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {},
					},
				}
				return nil
			},
			expectedErr:  false,
			expectedSave: true,
			existing:     nil,
		},
		{
			name:      "ignore not found summary",
			summaryID: summaryID,
			policy:    summaryNotFoundIgnore,
			updateFunc: func(summary *datamodel.ResourceProviderSummary) error {
				panic("Should not be called!")
			},
			expectedErr:  false,
			expectedSave: false,
			existing:     nil,
		},
		{
			name:      "fail on not found summary",
			summaryID: summaryID,
			policy:    summaryNotFoundFail,
			updateFunc: func(summary *datamodel.ResourceProviderSummary) error {
				panic("Should not be called!")
			},
			expectedErr:  true,
			expectedSave: false,
			existing:     nil,
		},
		{
			name:      "update existing summary",
			summaryID: summaryID,
			policy:    summaryNotFoundFail,
			updateFunc: func(summary *datamodel.ResourceProviderSummary) error {
				summary.Properties = datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {},
					},
				}
				return nil
			},
			expectedErr:  false,
			expectedSave: true,
			existing: &datamodel.ResourceProviderSummary{
				BaseResource: v1.BaseResource{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := database.NewMockClient(ctrl)

			expectedETag := ""
			if tt.existing == nil {
				client.EXPECT().Get(gomock.Any(), tt.summaryID.String()).Return(nil, &database.ErrNotFound{})
			} else {
				bs, err := json.Marshal(tt.existing)
				require.NoError(t, err)

				converted := map[string]any{}
				err = json.Unmarshal(bs, &converted)
				require.NoError(t, err)

				expectedETag = etag.New(bs)
				obj := database.Object{
					Metadata: database.Metadata{
						ID:   tt.summaryID.String(),
						ETag: expectedETag,
					},
					Data: converted,
				}
				client.EXPECT().Get(gomock.Any(), tt.summaryID.String()).Return(&obj, nil)
			}

			if tt.expectedSave {
				client.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, o *database.Object, so ...database.SaveOptions) error {

					config := database.NewSaveConfig(so...)
					require.Equal(t, expectedETag, config.ETag)

					expectedResourceTypes := map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {},
					}

					summary := o.Data.(*datamodel.ResourceProviderSummary)
					require.Equal(t, expectedResourceTypes, summary.Properties.ResourceTypes)

					return nil
				})
			}

			err := updateResourceProviderSummaryWithETag(context.Background(), client, tt.summaryID, tt.policy, tt.updateFunc)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
