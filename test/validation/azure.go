// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type AzureResourceSet struct {
	Resources []ExpectedResource
}

type ExpectedResource struct {
	Children    []ExpectedChildResource
	Type        string
	Tags        map[string]string
	UserManaged bool
}

type ExpectedChildResource struct {
	Type        string
	Name        string
	UserManaged bool
}

// Alias so we can define methods
type ActualResource resources.GenericResource

// Alias so we can define methods - YES the SDK has two types for generic resource
type ActualResourceExpanded resources.GenericResourceExpanded

type AzureResourceValidator struct {
	Authorizer     autorest.Authorizer
	ResourceGroup  string
	SubscriptionID string
	T              *testing.T
}

func ValidateAzureResourcesCreated(ctx context.Context, t *testing.T, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string, application string, set AzureResourceSet) {
	v := &AzureResourceValidator{
		Authorizer:     authorizer,
		ResourceGroup:  resourceGroup,
		SubscriptionID: subscriptionID,
		T:              t,
	}

	t.Logf("Validating creation of resources in resource group %s...", resourceGroup)
	t.Logf("Expected resources: ")
	for _, r := range set.Resources {
		t.Logf("\t%s", r.String())
	}
	t.Logf("")

	t.Logf("Listing Azure resources in resource group %s...", resourceGroup)
	all := v.listResources(ctx)
	t.Logf("Finished listing Azure resources in resource group %s, found:", resourceGroup)
	for _, r := range all {
		t.Logf("\t%s", r.String())
	}
	t.Logf("")

	// Now we have the set of resources, so we can diff them against what's expected. We'll make copies
	// of the expected and actual resources so we can 'check off' things as we match them.
	expected := set.Resources
	actual := all

	// Iterating in reverse allows us to remove things without throwing off indexing
	for actualIndex := len(actual) - 1; actualIndex >= 0; actualIndex-- {
		for expectedIndex := len(expected) - 1; expectedIndex >= 0; expectedIndex-- {
			actualResource := actual[actualIndex]
			expectedResource := expected[expectedIndex]

			if !expectedResource.IsMatch(actualResource) {
				continue // not a match, skip
			}

			for _, expectedChild := range expectedResource.Children {
				actualChild := v.findChildResource(ctx, actualResource, expectedChild)
				assert.NotNilf(
					v.T,
					actualChild,
					"failed to locate child resource %s of %s",
					expectedChild.String(),
					expectedResource.String())
			}

			t.Logf("found a match for expected resource %s", expectedResource.String())

			// We found a match, remove from both lists
			actual = append(actual[:actualIndex], actual[actualIndex+1:]...)
			expected = append(expected[:expectedIndex], expected[expectedIndex+1:]...)
			break
		}
	}

	// We'll also find resources that are tagged for a different application or no application at all
	// if these weren't matched by another predicate then remove them here.
	//
	// We don't want to have crosstalk between tests
	actual = v.removeNonApplicationResources(application, actual)

	failed := false
	if len(actual) > 0 {
		// If we get here then it means there are resources we found for this application
		// that don't match the expected resources. This is a failure.
		failed = true
		for _, actualResource := range actual {
			assert.Failf(t, "validation failed", "no match was found for actual resource %s", actualResource.String())
		}

	}

	if len(expected) > 0 {
		// If we get here then it means there are resources we were looking for but could not be
		// found. This is a failure.
		failed = true
		for _, expectedResource := range expected {
			assert.Failf(t, "validation failed", "no match was found for expected resource %s", expectedResource.String())
		}
	}

	if failed {
		// Extra call to require.Fail to stop testing
		require.Fail(t, "failed resource validation")
	}
}

func ValidateAzureResourcesDeleted(ctx context.Context, t *testing.T, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string, application string, set AzureResourceSet) {
	v := &AzureResourceValidator{
		Authorizer:     authorizer,
		ResourceGroup:  resourceGroup,
		SubscriptionID: subscriptionID,
		T:              t,
	}

	t.Logf("Validating deletion of resources in resource group %s...", resourceGroup)
	t.Logf("Expected resources: ")
	for _, r := range set.Resources {
		// We only expect to find user-managed resources
		if !r.UserManaged {
			continue
		}

		t.Logf("\t%s", r.String())
	}
	t.Logf("")

	t.Logf("Listing Azure resources in resource group %s...", resourceGroup)
	all := v.listResources(ctx)
	t.Logf("Finished listing Azure resources in resource group %s, found:", resourceGroup)
	for _, r := range all {
		t.Logf("\t%s", r.String())
	}
	t.Logf("")

	// Now we have the set of resources, so we can diff them against what's expected. We'll make copies
	// of the expected and actual resources so we can 'check off' things as we match them.
	expected := set.Resources
	actual := all

	// NOTE: this step is different from creation validation. Only resources that are USER-MANAGED should still be around.
	// A resource that matches the predicate and is radius-managed is a failure.

	// Iterating in reverse allows us to remove things without throwing off indexing
	for actualIndex := len(actual) - 1; actualIndex >= 0; actualIndex-- {
		for expectedIndex := len(expected) - 1; expectedIndex >= 0; expectedIndex-- {
			actualResource := actual[actualIndex]
			expectedResource := expected[expectedIndex]

			if !expectedResource.IsMatch(actualResource) {
				continue // not a match, skip
			}

			if !expectedResource.UserManaged {
				assert.Failf(t, "validation failed", "found a resource that should have been deleted %s", actualResource.String())
				continue
			}

			for _, expectedChild := range expectedResource.Children {
				actualChild := v.findChildResource(ctx, actualResource, expectedChild)

				// A user-managed child of a user-managed parent should still be here
				if expectedChild.UserManaged {
					assert.NotNil(
						v.T,
						actualChild,
						"failed to locate child resource %s of %s",
						expectedChild.String(),
						expectedResource.String())
				} else {
					// A user-managed child of a radius-managed parent should be gone
					assert.Nil(
						v.T,
						actualChild,
						"found unexpected child resource %s of %s",
						expectedChild.String(),
						expectedResource.String())
				}
			}

			t.Logf("found a match for user-managed expected resource %s", expectedResource.String())

			// We found a match, remove from both lists
			actual = append(actual[:actualIndex], actual[actualIndex+1:]...)
			expected = append(expected[:expectedIndex], expected[expectedIndex+1:]...)
			break
		}
	}

	// We'll also find resources that are tagged for a different application or no application at all
	// if these weren't matched by another predicate then remove them here.
	//
	// We don't want to have crosstalk between tests
	actual = v.removeNonApplicationResources(application, actual)

	// We only care about the expected resources if they are user-managed. We can ignore any radius-managed
	// resources because if we got this far then it means that they were deleted successfully.
	for expectedIndex := len(expected) - 1; expectedIndex >= 0; expectedIndex-- {
		expectedResource := expected[expectedIndex]
		if !expectedResource.UserManaged {
			expected = append(expected[:expectedIndex], expected[expectedIndex+1:]...)
		}
	}

	failed := false
	if len(actual) > 0 {
		// If we get here then it means there are resources we found for this application
		// that don't match the expected resources. This is a failure.
		failed = true
		for _, actualResource := range actual {
			assert.Failf(t, "validation failed", "no match was found for actual resource %s", actualResource.String())
		}

	}

	// We only care about the expected
	if len(expected) > 0 {
		// If we get here then it means there are resources we were looking for but could not be
		// found. This is a failure.
		failed = true
		for _, expectedResource := range expected {
			assert.Failf(t, "validation failed", "no match was found for expected resource %s", expectedResource.String())
		}
	}

	if failed {
		// Extra call to require.Fail to stop testing
		require.Fail(t, "failed resource validation")
	}
}

func (v *AzureResourceValidator) listResources(ctx context.Context) []ActualResourceExpanded {
	resc := azclients.NewResourcesClient(v.SubscriptionID, v.Authorizer)

	all := []ActualResourceExpanded{}
	page, err := resc.ListByResourceGroup(ctx, v.ResourceGroup, "", "", nil)
	require.NoErrorf(v.T, err, "failed to list azure resources in %s", v.ResourceGroup)

	for ; page.NotDone(); err = page.NextWithContext(ctx) {
		require.NoErrorf(v.T, err, "failed to list azure resources in %s", v.ResourceGroup)
		for _, r := range page.Values() {
			all = append(all, ActualResourceExpanded(r))
		}
	}

	return all
}

func (v *AzureResourceValidator) findChildResource(ctx context.Context, parent ActualResourceExpanded, child ExpectedChildResource) *ActualResource {
	resc := azclients.NewResourcesClient(v.SubscriptionID, v.Authorizer)

	parts := strings.Split(*parent.Type, "/")
	require.Len(v.T, parts, 2, "resource type should have a provider and type")

	provider := parts[0]
	parentType := parts[1]
	apiVersion := v.getDefaultAPIVersion(ctx, provider, parentType)

	resource, err := resc.Get(ctx, v.ResourceGroup, provider, parentType+"/"+*parent.Name, child.Type, child.Name, apiVersion)
	if detailed, ok := err.(*autorest.DetailedError); ok && detailed.StatusCode == 404 {
		return nil
	}

	require.NoError(v.T, err, "failed to query resource")
	return (*ActualResource)(&resource)
}

func (v *AzureResourceValidator) getDefaultAPIVersion(ctx context.Context, provider string, resourceType string) string {
	providerc := azclients.NewProvidersClient(v.SubscriptionID, v.Authorizer)

	p, err := providerc.Get(ctx, provider, "")
	require.NoError(v.T, err, "failed to query provider")

	for _, rt := range *p.ResourceTypes {
		if strings.EqualFold(*rt.ResourceType, resourceType) {
			return *rt.DefaultAPIVersion
		}
	}

	require.Fail(v.T, "failed to find resource type "+resourceType)
	return "" // unreachable
}

func (v *AzureResourceValidator) removeNonApplicationResources(application string, actual []ActualResourceExpanded) []ActualResourceExpanded {
	for actualIndex := len(actual) - 1; actualIndex >= 0; actualIndex-- {
		actualResource := actual[actualIndex]
		if keys.HasRadiusApplicationTag(actualResource.Tags, application) {
			continue
		}

		v.T.Logf("ignoring non-application resource %s", actualResource.String())
		actual = append(actual[:actualIndex], actual[actualIndex+1:]...)
	}

	return actual
}

func (e ExpectedResource) String() string {
	return fmt.Sprintf("{ type: %v tags: %v }", e.Type, e.Tags)
}

func (e ExpectedResource) IsMatch(actual ActualResourceExpanded) bool {
	if actual.Type == nil || *actual.Type != e.Type {
		return false
	}

	if !keys.HasTagSet(actual.Tags, e.Tags) {
		return false
	}

	return true
}

func (e ExpectedChildResource) String() string {
	return fmt.Sprintf("{ type: %v name: %v }", e.Type, e.Name)
}

func formatTags(tags map[string]*string) string {
	// The Azure SDK types use map[string]*string which doesn't fmt nicely.
	parts := []string{}
	for k, v := range tags {
		if v == nil {
			parts = append(parts, fmt.Sprintf("%s:(null)", k))
		} else {
			parts = append(parts, fmt.Sprintf("%s:%s", k, *v))
		}
	}

	return "[" + strings.Join(parts, " ") + "]"
}

func (a ActualResource) String() string {
	return fmt.Sprintf("{ type: %v name: %v tags: %v }", *a.Type, *a.Name, formatTags(a.Tags))
}

func (a ActualResourceExpanded) String() string {
	return fmt.Sprintf("{ type: %v name: %v tags: %v }", *a.Type, *a.Name, formatTags(a.Tags))
}
