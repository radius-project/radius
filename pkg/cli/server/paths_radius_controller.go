// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

const radiusControllerEnvVar = "RAD_RADIUS_CONTROLLER"

func GetLocalRadiusControllerFilepath() (string, error) {
	return getLocalToolFilepath("radius-controller", radiusControllerEnvVar)
}
