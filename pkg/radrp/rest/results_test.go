package rest

import (
	"testing"

	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
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
