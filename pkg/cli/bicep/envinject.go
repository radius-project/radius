// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

// InjectEnvironmentParam injects an argument for environment into the parameters if required.
//
// - parameters.environment exists && param not passed in -> inject environmentId
// - parameters.environment does not exist -> noop
// - input parameters already include environment -> noop.
//
// # Function Explanation
// 
//	InjectEnvironmentParam injects a parameter into a deployment template based on an environment ID. It returns an error if
//	 the parameter injection fails.
func InjectEnvironmentParam(deploymentTemplate map[string]any, parameters map[string]map[string]any, environmentId string) error {
	return injectParam(deploymentTemplate, parameters, "environment", environmentId)
}

// InjectApplicationParam injects an argument for application into the parameters if required.
//
// - parameters.application exists && param not passed in -> inject environmentId
// - parameters.application does not exist -> noop
// - input parameters already include application -> noop.
//
// # Function Explanation
// 
//	InjectApplicationParam injects application-specific parameters into a deployment template, returning an error if any of 
//	the parameters are invalid.
func InjectApplicationParam(deploymentTemplate map[string]any, parameters map[string]map[string]any, applicationId string) error {
	return injectParam(deploymentTemplate, parameters, "application", applicationId)
}

func injectParam(deploymentTemplate map[string]any, parameters map[string]map[string]any, parameter string, value string) error {
	if deploymentTemplate["parameters"] == nil {
		return nil
	}

	innerParameters := deploymentTemplate["parameters"].(map[string]any)
	if innerParameters[parameter] == nil {
		return nil
	}

	// If we got here, it means 'parameter' is a formal parameter of the template.

	// Set the value if it wasn't set at the command line by the user.
	if _, ok := parameters[parameter]; !ok {
		parameters[parameter] = map[string]any{
			"value": value,
		}
	}

	return nil
}
