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

package settingsref

import (
	"context"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testTerraformSettingsID = "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/terraformSettings/tf-settings1"
	testBicepSettingsID     = "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/bicepSettings/bicep-settings1"
	testEnvID1              = "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/environments/env1"
	testEnvID2              = "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/environments/env2"
)

func TestAddTerraformSettingsReference(t *testing.T) {
	ctx := context.Background()

	t.Run("success - add reference to empty list", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		settings := &datamodel.TerraformSettings_v20250801preview{
			Properties: datamodel.TerraformSettingsProperties_v20250801preview{
				ReferencedBy: []string{},
			},
		}

		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			Return(&database.Object{
				Metadata: database.Metadata{ID: testTerraformSettingsID, ETag: "etag1"},
				Data:     settings,
			}, nil)

		dbClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
				savedSettings := obj.Data.(*datamodel.TerraformSettings_v20250801preview)
				require.Contains(t, savedSettings.Properties.ReferencedBy, testEnvID1)
				return nil
			})

		err := AddTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.NoError(t, err)
	})

	t.Run("success - idempotent - reference already exists", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		settings := &datamodel.TerraformSettings_v20250801preview{
			Properties: datamodel.TerraformSettingsProperties_v20250801preview{
				ReferencedBy: []string{testEnvID1},
			},
		}

		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			Return(&database.Object{
				Metadata: database.Metadata{ID: testTerraformSettingsID, ETag: "etag1"},
				Data:     settings,
			}, nil)

		// Save should NOT be called since reference already exists
		err := AddTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.NoError(t, err)
	})

	t.Run("success - retry on concurrency error", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		attemptCount := 0
		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			DoAndReturn(func(ctx context.Context, id string, opts ...database.GetOptions) (*database.Object, error) {
				// Return a fresh copy each time to simulate real database behavior
				return &database.Object{
					Metadata: database.Metadata{ID: testTerraformSettingsID, ETag: "etag1"},
					Data: &datamodel.TerraformSettings_v20250801preview{
						Properties: datamodel.TerraformSettingsProperties_v20250801preview{
							ReferencedBy: []string{},
						},
					},
				}, nil
			}).Times(2)

		dbClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
				attemptCount++
				if attemptCount == 1 {
					return &database.ErrConcurrency{}
				}
				return nil
			}).Times(2)

		err := AddTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.NoError(t, err)
		require.Equal(t, 2, attemptCount)
	})

	t.Run("failure - max retries exceeded on concurrency", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			DoAndReturn(func(ctx context.Context, id string, opts ...database.GetOptions) (*database.Object, error) {
				// Return a fresh copy each time to simulate real database behavior
				return &database.Object{
					Metadata: database.Metadata{ID: testTerraformSettingsID, ETag: "etag1"},
					Data: &datamodel.TerraformSettings_v20250801preview{
						Properties: datamodel.TerraformSettingsProperties_v20250801preview{
							ReferencedBy: []string{},
						},
					},
				}, nil
			}).Times(3)

		dbClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&database.ErrConcurrency{}).Times(3)

		err := AddTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.Error(t, err)
		require.True(t, errors.Is(err, &database.ErrConcurrency{}))
	})

	t.Run("failure - settings not found", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			Return(nil, &database.ErrNotFound{ID: testTerraformSettingsID})

		err := AddTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.Error(t, err)
		require.True(t, errors.Is(err, &database.ErrNotFound{}))
	})

	t.Run("failure - non-retryable error on save", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		settings := &datamodel.TerraformSettings_v20250801preview{
			Properties: datamodel.TerraformSettingsProperties_v20250801preview{
				ReferencedBy: []string{},
			},
		}

		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			Return(&database.Object{
				Metadata: database.Metadata{ID: testTerraformSettingsID, ETag: "etag1"},
				Data:     settings,
			}, nil)

		expectedErr := errors.New("database connection error")
		dbClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(expectedErr)

		err := AddTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.Error(t, err)
		require.Equal(t, expectedErr, err)
	})

	t.Run("failure - invalid settings ID", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		err := AddTerraformSettingsReference(ctx, dbClient, "invalid-id", testEnvID1)
		require.Error(t, err)
	})
}

func TestRemoveTerraformSettingsReference(t *testing.T) {
	ctx := context.Background()

	t.Run("success - remove existing reference", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		settings := &datamodel.TerraformSettings_v20250801preview{
			Properties: datamodel.TerraformSettingsProperties_v20250801preview{
				ReferencedBy: []string{testEnvID1, testEnvID2},
			},
		}

		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			Return(&database.Object{
				Metadata: database.Metadata{ID: testTerraformSettingsID, ETag: "etag1"},
				Data:     settings,
			}, nil)

		dbClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
				savedSettings := obj.Data.(*datamodel.TerraformSettings_v20250801preview)
				require.NotContains(t, savedSettings.Properties.ReferencedBy, testEnvID1)
				require.Contains(t, savedSettings.Properties.ReferencedBy, testEnvID2)
				return nil
			})

		err := RemoveTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.NoError(t, err)
	})

	t.Run("success - settings not found returns nil", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			Return(nil, &database.ErrNotFound{ID: testTerraformSettingsID})

		err := RemoveTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.NoError(t, err)
	})

	t.Run("success - idempotent - reference not in list", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		settings := &datamodel.TerraformSettings_v20250801preview{
			Properties: datamodel.TerraformSettingsProperties_v20250801preview{
				ReferencedBy: []string{testEnvID2}, // testEnvID1 not in list
			},
		}

		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			Return(&database.Object{
				Metadata: database.Metadata{ID: testTerraformSettingsID, ETag: "etag1"},
				Data:     settings,
			}, nil)

		dbClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		err := RemoveTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.NoError(t, err)
	})

	t.Run("success - retry on concurrency error", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		settings := &datamodel.TerraformSettings_v20250801preview{
			Properties: datamodel.TerraformSettingsProperties_v20250801preview{
				ReferencedBy: []string{testEnvID1},
			},
		}

		attemptCount := 0
		dbClient.EXPECT().
			Get(gomock.Any(), testTerraformSettingsID).
			Return(&database.Object{
				Metadata: database.Metadata{ID: testTerraformSettingsID, ETag: "etag1"},
				Data:     settings,
			}, nil).Times(2)

		dbClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
				attemptCount++
				if attemptCount == 1 {
					return &database.ErrConcurrency{}
				}
				return nil
			}).Times(2)

		err := RemoveTerraformSettingsReference(ctx, dbClient, testTerraformSettingsID, testEnvID1)
		require.NoError(t, err)
		require.Equal(t, 2, attemptCount)
	})
}

func TestAddBicepSettingsReference(t *testing.T) {
	ctx := context.Background()

	t.Run("success - add reference to empty list", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		settings := &datamodel.BicepSettings_v20250801preview{
			Properties: datamodel.BicepSettingsProperties_v20250801preview{
				ReferencedBy: []string{},
			},
		}

		dbClient.EXPECT().
			Get(gomock.Any(), testBicepSettingsID).
			Return(&database.Object{
				Metadata: database.Metadata{ID: testBicepSettingsID, ETag: "etag1"},
				Data:     settings,
			}, nil)

		dbClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
				savedSettings := obj.Data.(*datamodel.BicepSettings_v20250801preview)
				require.Contains(t, savedSettings.Properties.ReferencedBy, testEnvID1)
				return nil
			})

		err := AddBicepSettingsReference(ctx, dbClient, testBicepSettingsID, testEnvID1)
		require.NoError(t, err)
	})

	t.Run("success - idempotent - reference already exists", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		settings := &datamodel.BicepSettings_v20250801preview{
			Properties: datamodel.BicepSettingsProperties_v20250801preview{
				ReferencedBy: []string{testEnvID1},
			},
		}

		dbClient.EXPECT().
			Get(gomock.Any(), testBicepSettingsID).
			Return(&database.Object{
				Metadata: database.Metadata{ID: testBicepSettingsID, ETag: "etag1"},
				Data:     settings,
			}, nil)

		err := AddBicepSettingsReference(ctx, dbClient, testBicepSettingsID, testEnvID1)
		require.NoError(t, err)
	})
}

func TestRemoveBicepSettingsReference(t *testing.T) {
	ctx := context.Background()

	t.Run("success - remove existing reference", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		settings := &datamodel.BicepSettings_v20250801preview{
			Properties: datamodel.BicepSettingsProperties_v20250801preview{
				ReferencedBy: []string{testEnvID1, testEnvID2},
			},
		}

		dbClient.EXPECT().
			Get(gomock.Any(), testBicepSettingsID).
			Return(&database.Object{
				Metadata: database.Metadata{ID: testBicepSettingsID, ETag: "etag1"},
				Data:     settings,
			}, nil)

		dbClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
				savedSettings := obj.Data.(*datamodel.BicepSettings_v20250801preview)
				require.NotContains(t, savedSettings.Properties.ReferencedBy, testEnvID1)
				require.Contains(t, savedSettings.Properties.ReferencedBy, testEnvID2)
				return nil
			})

		err := RemoveBicepSettingsReference(ctx, dbClient, testBicepSettingsID, testEnvID1)
		require.NoError(t, err)
	})

	t.Run("success - settings not found returns nil", func(t *testing.T) {
		mctrl := gomock.NewController(t)
		defer mctrl.Finish()
		dbClient := database.NewMockClient(mctrl)

		dbClient.EXPECT().
			Get(gomock.Any(), testBicepSettingsID).
			Return(nil, &database.ErrNotFound{ID: testBicepSettingsID})

		err := RemoveBicepSettingsReference(ctx, dbClient, testBicepSettingsID, testEnvID1)
		require.NoError(t, err)
	})
}
