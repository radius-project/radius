// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"context"
)

// Injects an argument for environment into the parameters if required
// parameters.environment exists && param not passed in -> inject environmentId
// parameters.environment does not exist -> noop
// input parameters already include environment -> noop.
func InjectEnvironmentParam(deploymentTemplate map[string]interface{}, parameters map[string]map[string]interface{}, context context.Context, environmentId string) error {
	if deploymentTemplate["parameters"] == nil {
		return nil
	}
	innerParameters := deploymentTemplate["parameters"].(map[string]interface{})

	if innerParameters["environment"] == nil {
		return nil
	}

	// If we got here, it means an environment is an input parameter, inject if it isn't an input param
	if _, ok := parameters["environment"]; !ok {
		parameters["environment"] = map[string]interface{}{
			"value": environmentId,
		}
	}

	return nil
}
