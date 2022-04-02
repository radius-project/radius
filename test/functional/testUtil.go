// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package functional

import (
	"os"
)

func GetMagpieImage() string {
	setDefault()
	defaultDockerReg, imageTag := setDefault()
	magpieImage := "magpieimage=" + defaultDockerReg + "/magpiego:" + imageTag
	return magpieImage
}

func GetMagpieTag() string {
	setDefault()
	_, imageTag := setDefault()
	magpietag := "magpietag=" + imageTag
	return magpietag
}

func setDefault() (string, string) {
	defaultDockerReg := os.Getenv("DOCKER_REGISTRY")
	imageTag := os.Getenv("REL_VERSION")
	if defaultDockerReg == "" {
		defaultDockerReg = "radiusdev.azurecr.io"
	}
	if imageTag == "" {
		imageTag = "latest"
	}
	return defaultDockerReg, imageTag
}
