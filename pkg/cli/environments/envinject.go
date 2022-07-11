// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
)

func InjectEnvironmentParam(parameters map[string]map[string]interface{}, context context.Context, env Environment) error {
	client, err := CreateApplicationsManagementClient(context, env)
	if err != nil {
		return err
	}

	envResource, err := client.GetEnvDetails(context, env.GetName())

	if err != nil {
		return err
	}

	if _, ok := parameters["environment"]; !ok {
		parameters["environment"] = map[string]interface{}{
			"value": envResource.ID,
		}
	}

	return nil
}
