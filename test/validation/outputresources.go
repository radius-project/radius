// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ResourceSet struct {
	Resources []RadiusResource
}

// For now we mostly need the same data regardless of the the appmodel version.
// We can rename this to `RadiusResourceSet` in the future.
type RadiusResource struct {
	ResourceName    string
	ApplicationName string
	ResourceType    string
	OutputResources map[string]ExpectedOutputResource
}

type ExpectedOutputResource struct {
	LocalID      string
	ResourceType rest.ResourceType
	Status       rest.OutputResourceStatus
	VerifyStatus bool

	// SkipLocalIDWhenMatching instructs the test system to ignore the Local ID when matching
	// the expected output resource against the actual output resources.
	//
	// This is useful when the LocalID is generated from information that's not available for the test.
	SkipLocalIDWhenMatching bool
}

func NewOutputResource(localID string, resourceType rest.ResourceType, verifyStatus bool, status rest.OutputResourceStatus) ExpectedOutputResource {
	return ExpectedOutputResource{
		LocalID:      localID,
		ResourceType: resourceType,
		Status:       status,
		VerifyStatus: verifyStatus,
	}
}

func ValidateOutputResources(t *testing.T, authorizer autorest.Authorizer, baseURL string, sender autorest.Sender, subscriptionID string, resourceGroup string, expected ResourceSet) {
	genericClient := clients.NewGenericResourceClient(subscriptionID, authorizer)
	genericClient.BaseURI = baseURL
	genericClient.Sender = sender

	failed := false

	for _, c := range expected.Resources {
		t.Logf("Validating output resources for Radius resource %s...", c.ResourceName)

		all := []rest.OutputResource{}
		require.NotEmpty(t, c.ResourceType, "ResourceType must be set for v3")

		id := azresources.MakeID(
			subscriptionID,
			resourceGroup,
			azresources.ResourceType{
				Type: azresources.CustomProvidersResourceProviders,
				Name: azresources.CustomRPV3Name,
			},
			azresources.ResourceType{
				Type: "Application",
				Name: c.ApplicationName,
			},
			azresources.ResourceType{
				Type: c.ResourceType,
				Name: c.ResourceName,
			})

		t.Logf("Reading resource %s %s...", c.ResourceType, c.ResourceName)
		resource, err := genericClient.GetByID(context.Background(), strings.TrimPrefix(id, "/"), azresources.CustomRPApiVersion)
		require.NoError(t, err)
		t.Logf("Finished resource %s %s...", c.ResourceType, c.ResourceName)

		actual, err := convertFromGenericToRestOutputResource(resource)
		require.NoError(t, err)

		all = append(all, actual...)

		expected := []ExpectedOutputResource{}
		t.Logf("Expected resources: ")
		for _, r := range c.OutputResources {
			t.Logf("\t%+v", r)
			expected = append(expected, r)
		}
		t.Logf("")

		t.Logf("Actual resources: ")
		for _, actual := range all {
			t.Logf("\t%+v", actual)
		}
		t.Logf("")

		// Now we have the set of resources, so we can diff them against what's expected. We'll make copies
		// of the expected and actual resources so we can 'check off' things as we match them.
		actual = all

		// Iterating in reverse allows us to remove things without throwing off indexing
		for actualIndex := len(actual) - 1; actualIndex >= 0; actualIndex-- {
			for expectedIndex := len(expected) - 1; expectedIndex >= 0; expectedIndex-- {
				actualResource := actual[actualIndex]
				expectedResource := expected[expectedIndex]

				if !expectedResource.IsMatch(actualResource) {
					continue // not a match, skip
				}

				t.Logf("found a match for expected resource %+v", expectedResource)

				// TODO: Remove this check once health checks are implemented for all kinds of output resources
				// https://github.com/project-radius/radius/issues/827.
				// Till then, we will selectively verify the health/provisioning state for output resources that
				// have the functionality implemented.
				if expectedResource.VerifyStatus {
					assert.Equal(t, expectedResource.Status.ProvisioningState, actualResource.Status.ProvisioningState)
					assert.Equal(t, expectedResource.Status.HealthState, actualResource.Status.HealthState)
				}

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

func convertFromGenericToRestOutputResource(obj resources.GenericResource) ([]rest.OutputResource, error) {
	b, err := json.Marshal(obj.Properties)
	if err != nil {
		return nil, err
	}

	properties := RadiusResourceProperties{}
	err = json.Unmarshal(b, &properties)
	if err != nil {
		return nil, err
	}

	b, err = json.Marshal(properties.Status.OutputResources)
	if err != nil {
		return nil, err
	}

	result := []rest.OutputResource{}
	err = json.Unmarshal(b, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (e ExpectedOutputResource) IsMatch(a rest.OutputResource) bool {
	match := e.ResourceType.Type == a.ResourceType.Type &&
		e.ResourceType.Provider == a.ResourceType.Provider

	if !e.SkipLocalIDWhenMatching {
		match = match && e.LocalID == a.LocalID
	}

	return match
}

type RadiusResourceProperties struct {
	Status RadiusResourceStatus `json:"status,omitempty"`
}

type RadiusResourceStatus struct {
	OutputResources []map[string]interface{} `json:"outputResources,omitempty"`
}
