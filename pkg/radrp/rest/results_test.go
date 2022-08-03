package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func Test_AggregateResourceHealth_HealthyAndNotApplicableIsHealthy(t *testing.T) {

	outputResources := []OutputResource{
		{
			LocalID: outputresource.LocalIDDeployment,
			ResourceType: ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID: outputresource.LocalIDSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.Secret,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateNotApplicable,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingResourceHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID: outputresource.LocalIDDeployment,
			ResourceType: ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID: outputresource.LocalIDSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.Secret,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy, // NotApplicable should be shown as Healthy
			},
		},
	}
	require.Equal(t, expected, outputResources)
	require.Equal(t, HealthStateHealthy, aggregateHealthState)
	require.Equal(t, "", aggregateHealthStateErrorDetails)
}

func Test_AggregateResourceHealth_SingleNotSupportedOutputResource_IsEmpty(t *testing.T) {
	outputResources := []OutputResource{
		{
			LocalID: outputresource.LocalIDSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.Secret,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateNotSupported,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingResourceHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID: outputresource.LocalIDSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.Secret,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: "",
			},
		},
	}
	require.Equal(t, expected, outputResources)
	require.Equal(t, "", aggregateHealthState)
	require.Equal(t, "", aggregateHealthStateErrorDetails)
}

func Test_AggregateResourceHealth_NotSupportedAndNotApplicableIsEmpty(t *testing.T) {
	outputResources := []OutputResource{
		{
			LocalID: outputresource.LocalIDKeyVault,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateNotSupported,
			},
		},
		{
			LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateNotApplicable,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingResourceHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID: outputresource.LocalIDKeyVault,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				HealthState: "",
			},
		},
		{
			LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
	}
	require.Equal(t, expected, outputResources)
	require.Equal(t, "", aggregateHealthState)
	require.Equal(t, "", aggregateHealthStateErrorDetails)
}

// We do not expect to see a Radius Resource to have a combination of some output resources as Healthy/Unhealthy and some as NotSupported
func Test_AggregateResourceHealth_NotSupportedAndHealthyIsError(t *testing.T) {
	outputResources := []OutputResource{
		{
			LocalID: outputresource.LocalIDKeyVault,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID: outputresource.LocalIDKeyVaultSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateNotSupported,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingResourceHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID: outputresource.LocalIDKeyVault,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID: outputresource.LocalIDKeyVaultSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				HealthState: "", // NotSupported should show as ""
			},
		},
	}

	require.Equal(t, expected, outputResources)
	require.Equal(t, HealthStateUnhealthy, aggregateHealthState)
	require.Equal(t, "Health aggregation error", aggregateHealthStateErrorDetails)
}

func Test_AggregateResourceHealth_HealthyUnhealthyAndNotApplicable_IsUnhealthy(t *testing.T) {
	outputResources := []OutputResource{
		{
			LocalID: outputresource.LocalIDService,
			ResourceType: ResourceType{
				Type:     resourcekinds.Service,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID: outputresource.LocalIDSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.Secret,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateNotApplicable,
			},
		},
		{
			LocalID: outputresource.LocalIDDeployment,
			ResourceType: ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateUnhealthy,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingResourceHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID: outputresource.LocalIDService,
			ResourceType: ResourceType{
				Type:     resourcekinds.Service,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID: outputresource.LocalIDSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.Secret,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy, // NotApplicable should show as Healthy
			},
		},
		{
			LocalID: outputresource.LocalIDDeployment,
			ResourceType: ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: providers.ProviderKubernetes,
			},
			Status: OutputResourceStatus{
				HealthState: HealthStateUnhealthy,
			},
		},
	}
	require.Equal(t, expected, outputResources)
	require.Equal(t, HealthStateUnhealthy, aggregateHealthState)
	require.Equal(t, "", aggregateHealthStateErrorDetails)
}

func Test_AggregateResourceHealth_FailedAndProvisioningIsFailed(t *testing.T) {
	outputResources := []OutputResource{
		{
			LocalID: outputresource.LocalIDKeyVault,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioning,
			},
		},
		{
			LocalID: outputresource.LocalIDKeyVaultSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateFailed,
			},
		},
	}

	aggregateProvisiongState := GetUserFacingResourceProvisioningState(outputResources)

	expected := []OutputResource{
		{
			LocalID: outputresource.LocalIDKeyVault,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioning,
			},
		},
		{
			LocalID: outputresource.LocalIDKeyVaultSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateFailed,
			},
		},
	}

	require.Equal(t, expected, outputResources)
	require.Equal(t, ProvisioningStateFailed, aggregateProvisiongState)
}

func Test_AggregateResourceHealth_ProvisionedAndProvisioning_IsProvisioning(t *testing.T) {
	outputResources := []OutputResource{
		{
			LocalID: outputresource.LocalIDKeyVault,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioning,
			},
		},
		{
			LocalID: outputresource.LocalIDKeyVaultSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioned,
			},
		},
	}

	aggregateProvisiongState := GetUserFacingResourceProvisioningState(outputResources)

	expected := []OutputResource{
		{
			LocalID: outputresource.LocalIDKeyVault,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioning,
			},
		},
		{
			LocalID: outputresource.LocalIDKeyVaultSecret,
			ResourceType: ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioned,
			},
		},
	}

	require.Equal(t, expected, outputResources)
	require.Equal(t, ProvisioningStateProvisioning, aggregateProvisiongState)
}

func Test_AggregateApplicationHealth_HealthyAndUnhealthyIsUnHealthy(t *testing.T) {

	resourceStatuses := map[string]ResourceStatus{
		"a": {
			HealthState: HealthStateHealthy,
		},
		"b": {
			HealthState: HealthStateUnhealthy,
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingAppHealthState(resourceStatuses)

	require.Equal(t, HealthStateUnhealthy, aggregateHealthState)
	require.Equal(t, "Resource b is unhealthy", aggregateHealthStateErrorDetails)
}

func Test_AggregateApplicationHealth_HealthyAndNotSupportedIsHealthy(t *testing.T) {

	resourceStatuses := map[string]ResourceStatus{
		"a": {
			HealthState: HealthStateHealthy,
		},
		"b": {
			HealthState: HealthStateNotSupported,
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingAppHealthState(resourceStatuses)

	require.Equal(t, HealthStateHealthy, aggregateHealthState)
	require.Equal(t, "", aggregateHealthStateErrorDetails)
}

func Test_AggregateApplicationHealth_UnhealthyAndNotSupportedIsUnhealthy(t *testing.T) {

	resourceStatuses := map[string]ResourceStatus{
		"a": {
			HealthState: HealthStateUnhealthy,
		},
		"b": {
			HealthState: HealthStateNotSupported,
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingAppHealthState(resourceStatuses)

	require.Equal(t, HealthStateUnhealthy, aggregateHealthState)
	require.Equal(t, "Resource a is unhealthy", aggregateHealthStateErrorDetails)
}

func Test_AggregateApplicationHealth_UnknownAndHealthyIsUnhealthy(t *testing.T) {

	resourceStatuses := map[string]ResourceStatus{
		"a": {
			HealthState: HealthStateHealthy,
		},
		"b": {
			HealthState: HealthStateUnknown,
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingAppHealthState(resourceStatuses)

	require.Equal(t, HealthStateUnhealthy, aggregateHealthState)
	require.Equal(t, "Resource b has unknown health state", aggregateHealthStateErrorDetails)
}

func Test_AggregateApplicationProvisioningState_ProvisioningAndProvisionedIsProvisioning(t *testing.T) {

	resourceStatuses := map[string]ResourceStatus{
		"a": {
			ProvisioningState: ProvisioningStateProvisioned,
		},
		"b": {
			ProvisioningState: ProvisioningStateProvisioning,
		},
	}

	aggregateProvisioningState, aggregateProvisioningStateErrorDetails := GetUserFacingAppProvisioningState(resourceStatuses)

	require.Equal(t, ProvisioningStateProvisioning, aggregateProvisioningState)
	require.Equal(t, "Resource b is in Provisioning state", aggregateProvisioningStateErrorDetails)
}

func Test_NewLinkedResourceUpdateErrorResponse(t *testing.T) {
	errTests := []struct {
		desc     string
		oldAppID string
		oldEnvID string
		newAppID string
		newEnvID string
		msg      string
	}{
		{
			desc:     "application_and_environment_ids",
			oldAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application",
			oldEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			newAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/updated-application",
			newEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'updated-application' application and 'test-environment' environment to create a new resource, or use '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application' application and '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment' environment to update the existing resource 'test-container-0'.",
		}, {
			desc:     "only_application_id_in_existing_resource",
			oldAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application",
			oldEnvID: "",
			newAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/updated-application",
			newEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'updated-application' application and 'test-environment' environment to create a new resource, or use '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application' application and '' environment to update the existing resource 'test-container-0'.",
		}, {
			desc:     "only_application_id",
			oldAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application",
			oldEnvID: "",
			newAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/updated-application",
			newEnvID: "",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'updated-application' application to create a new resource, or use '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application' application and '' environment to update the existing resource 'test-container-0'.",
		}, {
			desc:     "only_environment",
			oldAppID: "",
			oldEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			newAppID: "",
			newEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/updated-environment",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'updated-environment' environment to create a new resource, or use '' application and '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment' environment to update the existing resource 'test-container-0'.",
		}, {
			desc:     "invalid_id",
			oldAppID: "",
			oldEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			newAppID: "",
			newEnvID: "invalid_id",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'invalid_id' environment to create a new resource, or use '' application and '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment' environment to update the existing resource 'test-container-0'.",
		},
	}

	resource, err := resources.Parse("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/containers/test-container-0")
	require.NoError(t, err)

	for _, tt := range errTests {
		t.Run(tt.desc, func(t *testing.T) {
			expctedResp := &BadRequestResponse{
				Body: armerrors.ErrorResponse{
					Error: armerrors.ErrorDetails{
						Code:    armerrors.Invalid,
						Message: tt.msg,
						Target:  resource.String(),
					},
				},
			}
			oldResourceProp := &v1.BasicResourceProperties{
				Application: tt.oldAppID,
				Environment: tt.oldEnvID,
			}
			newResourceProp := &v1.BasicResourceProperties{
				Application: tt.newAppID,
				Environment: tt.newEnvID,
			}
			resp := NewLinkedResourceUpdateErrorResponse(resource, oldResourceProp, newResourceProp)

			require.Equal(t, expctedResp, resp)
		})
	}

}

func Test_AggregateApplicationProvisioningState_NotProvisionedAndProvisionedIsProvisioning(t *testing.T) {

	resourceStatuses := map[string]ResourceStatus{
		"a": {
			ProvisioningState: ProvisioningStateProvisioned,
		},
		"b": {
			ProvisioningState: ProvisioningStateNotProvisioned,
		},
	}

	aggregateProvisioningState, aggregateProvisioningStateErrorDetails := GetUserFacingAppProvisioningState(resourceStatuses)

	require.Equal(t, ProvisioningStateProvisioning, aggregateProvisioningState)
	require.Equal(t, "Resource b is in NotProvisioned state", aggregateProvisioningStateErrorDetails)
}
func Test_OKResponse_Empty(t *testing.T) {
	response := NewOKResponse(nil)

	req := httptest.NewRequest("GET", "http://example.com", nil)
	w := httptest.NewRecorder()

	err := response.Apply(context.TODO(), w, req)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string(nil), w.Header()["Content-Type"])
	require.Empty(t, w.Body.Bytes())
}

func Test_OKResponse_WithBody(t *testing.T) {
	payload := map[string]string{
		"message": "hi there!",
	}
	response := NewOKResponse(payload)

	req := httptest.NewRequest("GET", "http://example.com", nil)
	w := httptest.NewRecorder()

	err := response.Apply(context.TODO(), w, req)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"application/json"}, w.Header()["Content-Type"])

	body := map[string]string{}
	err = json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	require.Equal(t, payload, body)
}

func TestGetAsyncLocationPath(t *testing.T) {
	operationID := uuid.New()

	testCases := []struct {
		desc string
		base string
		rID  string
		loc  string
		opID uuid.UUID
		av   string
		or   string
		os   string
	}{
		{
			"ucp-test-headers",
			"https://ucp.dev",
			"/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			"global",
			operationID,
			"2022-03-15-privatepreview",
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationResults/%s", operationID.String()),
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationStatuses/%s", operationID.String()),
		},
		{
			"arm-test-headers",
			"https://azure.dev",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			"global",
			operationID,
			"2022-03-15-privatepreview",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/providers/Applications.Core/locations/global/operationResults/%s", operationID.String()),
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/providers/Applications.Core/locations/global/operationStatuses/%s", operationID.String()),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			resourceID, err := resources.Parse(tt.rID)
			require.NoError(t, err)

			body := &datamodel.ContainerResource{}
			r := NewAsyncOperationResponse(body, tt.loc, http.StatusAccepted, resourceID, tt.opID, tt.av)

			req := httptest.NewRequest("GET", tt.base, nil)
			w := httptest.NewRecorder()
			err = r.Apply(context.Background(), w, req)
			require.NoError(t, err)

			require.NotNil(t, w.Header().Get("Location"))
			require.Equal(t, tt.base+tt.or+"?api-version="+tt.av, w.Header().Get("Location"))

			require.NotNil(t, w.Header().Get("Azure-AsyncHeader"))
			require.Equal(t, tt.base+tt.os+"?api-version="+tt.av, w.Header().Get("Azure-AsyncOperation"))
		})
	}
}
