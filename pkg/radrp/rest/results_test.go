package rest

import (
	"testing"

	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
)

func Test_AggregateResourceHealth_HealthyAndNotApplicableIsHealthy(t *testing.T) {

	outputResources := []OutputResource{
		{
			LocalID:      outputresource.LocalIDDeployment,
			ResourceKind: resourcekinds.Kubernetes,
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID:      outputresource.LocalIDSecret,
			ResourceKind: resourcekinds.Kubernetes,
			Status: OutputResourceStatus{
				HealthState: HealthStateNotApplicable,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID:      outputresource.LocalIDDeployment,
			ResourceKind: resourcekinds.Kubernetes,
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID:      outputresource.LocalIDSecret,
			ResourceKind: resourcekinds.Kubernetes,
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
			LocalID:      outputresource.LocalIDSecret,
			ResourceKind: resourcekinds.Kubernetes,
			Status: OutputResourceStatus{
				HealthState: HealthStateNotSupported,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID:      outputresource.LocalIDSecret,
			ResourceKind: resourcekinds.Kubernetes,
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
			LocalID:      outputresource.LocalIDKeyVault,
			ResourceKind: resourcekinds.AzureKeyVault,
			Status: OutputResourceStatus{
				HealthState: HealthStateNotSupported,
			},
		},
		{
			LocalID:      outputresource.LocalIDUserAssignedManagedIdentity,
			ResourceKind: resourcekinds.AzureUserAssignedManagedIdentity,
			Status: OutputResourceStatus{
				HealthState: HealthStateNotApplicable,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID:      outputresource.LocalIDKeyVault,
			ResourceKind: resourcekinds.AzureKeyVault,
			Status: OutputResourceStatus{
				HealthState: "",
			},
		},
		{
			LocalID:      outputresource.LocalIDUserAssignedManagedIdentity,
			ResourceKind: resourcekinds.AzureUserAssignedManagedIdentity,
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
			LocalID:      outputresource.LocalIDKeyVault,
			ResourceKind: resourcekinds.AzureKeyVault,
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID:      outputresource.LocalIDKeyVaultSecret,
			ResourceKind: resourcekinds.AzureKeyVaultSecret,
			Status: OutputResourceStatus{
				HealthState: HealthStateNotSupported,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID:      outputresource.LocalIDKeyVault,
			ResourceKind: resourcekinds.AzureKeyVault,
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID:      outputresource.LocalIDKeyVaultSecret,
			ResourceKind: resourcekinds.AzureKeyVaultSecret,
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
			LocalID:      outputresource.LocalIDService,
			ResourceKind: resourcekinds.Kubernetes,
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID:      outputresource.LocalIDSecret,
			ResourceKind: resourcekinds.Kubernetes,
			Status: OutputResourceStatus{
				HealthState: HealthStateNotApplicable,
			},
		},
		{
			LocalID:      outputresource.LocalIDDeployment,
			ResourceKind: resourcekinds.Kubernetes,
			Status: OutputResourceStatus{
				HealthState: HealthStateUnhealthy,
			},
		},
	}

	aggregateHealthState, aggregateHealthStateErrorDetails := GetUserFacingHealthState(outputResources)

	expected := []OutputResource{
		{
			LocalID:      outputresource.LocalIDService,
			ResourceKind: resourcekinds.Kubernetes,
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy,
			},
		},
		{
			LocalID:      outputresource.LocalIDSecret,
			ResourceKind: resourcekinds.Kubernetes,
			Status: OutputResourceStatus{
				HealthState: HealthStateHealthy, // NotApplicable should show as Healthy
			},
		},
		{
			LocalID:      outputresource.LocalIDDeployment,
			ResourceKind: resourcekinds.Kubernetes,
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
			LocalID:      outputresource.LocalIDKeyVault,
			ResourceKind: resourcekinds.AzureKeyVault,
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioning,
			},
		},
		{
			LocalID:      outputresource.LocalIDKeyVaultSecret,
			ResourceKind: resourcekinds.AzureKeyVaultSecret,
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateFailed,
			},
		},
	}

	aggregateProvisiongState := GetUserFacingProvisioningState(outputResources)

	expected := []OutputResource{
		{
			LocalID:      outputresource.LocalIDKeyVault,
			ResourceKind: resourcekinds.AzureKeyVault,
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioning,
			},
		},
		{
			LocalID:      outputresource.LocalIDKeyVaultSecret,
			ResourceKind: resourcekinds.AzureKeyVaultSecret,
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
			LocalID:      outputresource.LocalIDKeyVault,
			ResourceKind: resourcekinds.AzureKeyVault,
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioning,
			},
		},
		{
			LocalID:      outputresource.LocalIDKeyVaultSecret,
			ResourceKind: resourcekinds.AzureKeyVaultSecret,
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioned,
			},
		},
	}

	aggregateProvisiongState := GetUserFacingProvisioningState(outputResources)

	expected := []OutputResource{
		{
			LocalID:      outputresource.LocalIDKeyVault,
			ResourceKind: resourcekinds.AzureKeyVault,
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioning,
			},
		},
		{
			LocalID:      outputresource.LocalIDKeyVaultSecret,
			ResourceKind: resourcekinds.AzureKeyVaultSecret,
			Status: OutputResourceStatus{
				ProvisioningState: ProvisioningStateProvisioned,
			},
		},
	}

	require.Equal(t, expected, outputResources)
	require.Equal(t, ProvisioningStateProvisioning, aggregateProvisiongState)
}
