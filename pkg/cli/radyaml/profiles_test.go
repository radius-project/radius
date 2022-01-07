// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/stretchr/testify/require"
)

func Test_ApplyProfile_Valid(t *testing.T) {
	cases := []struct {
		Description string
		Main        Stage
		Expected    Stage
	}{
		{
			Description: "EmptyStage",
			Main: Stage{
				Name: "test-stage",
			},
			Expected: Stage{
				Name: "test-stage",
			},
		},
		{
			Description: "NoOverrides",
			Main: Stage{
				Name: "test-stage",
				Bicep: &BicepStage{
					Template: to.StringPtr("test.bicep"),
				},
			},
			Expected: Stage{
				Name: "test-stage",
				Bicep: &BicepStage{
					Template: to.StringPtr("test.bicep"),
				},
			},
		},
		{
			Description: "NoMatchingOverride",
			Main: Stage{
				Name: "test-stage",
				Bicep: &BicepStage{
					Template: to.StringPtr("test.bicep"),
				},
				Profiles: map[string]Profile{
					"nope": {
						Bicep: &BicepStage{
							Template: to.StringPtr("another.bicep"),
						},
					},
				},
			},
			Expected: Stage{
				Name: "test-stage",
				Bicep: &BicepStage{
					Template: to.StringPtr("test.bicep"),
				},
			},
		},
		{
			Description: "EmptyOverride",
			Main: Stage{
				Name: "test-stage",
				Bicep: &BicepStage{
					Template: to.StringPtr("test.bicep"),
				},
				Profiles: map[string]Profile{
					"test": {
						Bicep: nil,
					},
				},
			},
			Expected: Stage{
				Name: "test-stage",
				Bicep: &BicepStage{
					Template: to.StringPtr("test.bicep"),
				},
			},
		},
		{
			Description: "OverrideTemplate",
			Main: Stage{
				Name: "test-stage",
				Bicep: &BicepStage{
					Template: to.StringPtr("test.bicep"),
				},
				Profiles: map[string]Profile{
					"test": {
						Bicep: &BicepStage{
							Template: to.StringPtr("override.bicep"),
						},
					},
				},
			},
			Expected: Stage{
				Name: "test-stage",
				Bicep: &BicepStage{
					Template: to.StringPtr("override.bicep"),
				},
			},
		},
		{
			Description: "EmptyMain",
			Main: Stage{
				Name:  "test-stage",
				Bicep: nil,
				Profiles: map[string]Profile{
					"test": {
						Bicep: &BicepStage{
							Template: to.StringPtr("override.bicep"),
						},
					},
				},
			},
			Expected: Stage{
				Name: "test-stage",
				Bicep: &BicepStage{
					Template: to.StringPtr("override.bicep"),
				},
			},
		},
		{
			Description: "OverrideBuild",
			Main: Stage{
				Name:  "test-stage",
				Bicep: nil,
				Build: map[string]*BuildTarget{
					"project1": { // Profile uses a different builder, will be replaced
						Builder: "build1",
						Values: map[string]interface{}{
							"key1": "value1",
						},
					},
					"project2": { // Profile uses the same builder, will be merged
						Builder: "build2",
						Values: map[string]interface{}{
							"key2": "value2",
						},
					},
					"project3": { // Profile does not contain this, will be unchanged
						Builder: "build3",
						Values: map[string]interface{}{
							"key3": "value3",
						},
					},
				},
				Profiles: map[string]Profile{
					"test": {
						Build: map[string]*BuildTarget{
							"project1": {
								Builder: "overridebuild1",
								Values: map[string]interface{}{
									"overridekey1": "overridevalue1",
								},
							},
							"project2": {
								Builder: "build2",
								Values: map[string]interface{}{
									"additionalkey2": "additionalvalue2",
								},
							},
						},
					},
				},
			},
			Expected: Stage{
				Name: "test-stage",
				Build: map[string]*BuildTarget{
					"project1": {
						Builder: "overridebuild1",
						Values: map[string]interface{}{
							"overridekey1": "overridevalue1",
						},
					},
					"project2": {
						Builder: "build2",
						Values: map[string]interface{}{
							"key2":           "value2",
							"additionalkey2": "additionalvalue2",
						},
					},
					"project3": {
						Builder: "build3",
						Values: map[string]interface{}{
							"key3": "value3",
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.Description, func(t *testing.T) {
			actual, err := c.Main.ApplyProfile("test")
			require.NoError(t, err)

			// our 'expected' results don't declare the set of profiles
			expected := c.Expected
			expected.Profiles = c.Main.Profiles

			require.Equal(t, expected, actual)
		})
	}
}
