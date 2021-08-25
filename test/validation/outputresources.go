// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ComponentSet struct {
	Components []Component
}

type Component struct {
	ComponentName   string
	ApplicationName string
	OutputResources map[string]ExpectedOutputResource
}

type ExpectedOutputResourceStatus struct {
	HealthState       string
	ProvisioningState string
}

type ExpectedOutputResource struct {
	LocalID            string
	OutputResourceType string
	ResourceKind       string
	Managed            bool
	Status             ExpectedOutputResourceStatus
	verifyStatus       bool
}
type ActualOutputResource struct {
	LocalID            string                    `json:"localId"`
	Managed            bool                      `json:"managed"`
	ResourceKind       string                    `json:"resourceKind"`
	OutputResourceType string                    `json:"outputResourceType"`
	OutputResourceInfo interface{}               `json:"outputResourceInfo"`
	Status             rest.OutputResourceStatus `json:"status"`
}

func NewOutputResource(localID, outputResourceType, resourceKind string, managed bool, verifyStatus bool, status ExpectedOutputResourceStatus) ExpectedOutputResource {
	return ExpectedOutputResource{
		LocalID:            localID,
		OutputResourceType: outputResourceType,
		ResourceKind:       resourceKind,
		Managed:            managed,
		Status:             status,
		verifyStatus:       verifyStatus,
	}
}

func ValidateOutputResources(t *testing.T, armConnection *armcore.Connection, subscriptionID string, resourceGroup string, expected ComponentSet) {
	componentsClient := radclient.NewComponentClient(armConnection, subscriptionID)
	failed := false

	for _, c := range expected.Components {
		t.Logf("Validating output resources for component %s...", c.ComponentName)

		t.Logf("Reading component %s...", c.ComponentName)
		component, err := componentsClient.Get(context.Background(), resourceGroup, c.ApplicationName, c.ComponentName, nil)
		require.NoError(t, err)
		t.Logf("Finished reading component %s", c.ComponentName)

		expected := []ExpectedOutputResource{}
		t.Logf("Expected resources: ")
		for _, r := range c.OutputResources {
			t.Logf("\t%+v", r)
			expected = append(expected, r)
		}
		t.Logf("")

		all := []ActualOutputResource{}
		t.Logf("Actual resources: ")
		for _, v := range component.ComponentResource.Properties.Status.OutputResources {
			actual, err := convertToActualOutputResource(v)
			require.NoError(t, err, "failed to convert output resource")
			all = append(all, actual)

			t.Logf("\t%+v", actual)
		}
		t.Logf("")

		// Now we have the set of resources, so we can diff them against what's expected. We'll make copies
		// of the expected and actual resources so we can 'check off' things as we match them.
		actual := all

		// Iterating in reverse allows us to remove things without throwing off indexing
		for actualIndex := len(actual) - 1; actualIndex >= 0; actualIndex-- {
			for expectedIndex := len(expected) - 1; expectedIndex >= 0; expectedIndex-- {
				actualResource := actual[actualIndex]
				expectedResource := expected[expectedIndex]

				if !expectedResource.IsMatch(actualResource) {
					continue // not a match, skip
				}

				// TODO: Remove this check once health checks are implemented for all kinds of output resources
				// https://github.com/Azure/radius/issues/827.
				// Till then, we will selectively verify the health/provisioning state for output resources that
				// have the functionality implemented.
				if expectedResource.verifyStatus {
					if !expectedResource.IsMatchStatus(actualResource) {
						continue // not a match, skip
					}
				}

				t.Logf("found a match for expected resource %+v", expectedResource)

				// We found a match, remove from both lists
				actual = append(actual[:actualIndex], actual[actualIndex+1:]...)
				expected = append(expected[:expectedIndex], expected[expectedIndex+1:]...)
				break
			}
		}

		if len(actual) > 0 {
			// If we get here then it means there are resources we found for this application
			// that don't match the expected resources. This is a failure.
			failed = true
			for _, actualResource := range actual {
				assert.Failf(t, "validation failed", "no match was found for actual resource %+v", actualResource)
			}

		}

		if len(expected) > 0 {
			// If we get here then it means there are resources we were looking for but could not be
			// found. This is a failure.
			failed = true
			for _, expectedResource := range expected {
				assert.Failf(t, "validation failed", "no match was found for expected resource %+v", expectedResource)
			}
		}
	}

	if failed {
		// Extra call to require.Fail to stop testing
		require.Fail(t, "failed resource validation")
	}
}

func convertToActualOutputResource(obj interface{}) (ActualOutputResource, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return ActualOutputResource{}, err
	}

	result := ActualOutputResource{}
	err = json.Unmarshal(b, &result)
	if err != nil {
		return ActualOutputResource{}, err
	}

	return result, nil
}

func (e ExpectedOutputResource) IsMatch(a ActualOutputResource) bool {
	return e.LocalID == a.LocalID &&
		e.OutputResourceType == a.OutputResourceType &&
		e.ResourceKind == a.ResourceKind &&
		e.Managed == a.Managed
}

func (e ExpectedOutputResource) IsMatchStatus(a ActualOutputResource) bool {
	return e.LocalID == a.LocalID &&
		e.Status.HealthState == a.Status.HealthState &&
		e.Status.ProvisioningState == a.Status.ProvisioningState
}
