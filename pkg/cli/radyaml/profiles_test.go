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
