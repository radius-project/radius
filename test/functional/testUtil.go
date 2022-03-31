package functional

import (
	"fmt"
	"os"
)

var defaultDockerReg, imageTag string

func GetMagpieImage() string {
	setDefault()
	magpieImage := "magpieimage=" + defaultDockerReg + "/magpiego:" + imageTag
	fmt.Println("magpieImage:", magpieImage)
	return magpieImage
}

func GetMagpieTag() string {
	setDefault()
	magpietag := "magpietag=" + imageTag
	fmt.Println("magpietag:", magpietag)
	return magpietag
}

func setDefault() {
	defaultDockerReg = os.Getenv("DOCKER_REGISTRY")
	imageTag = os.Getenv("REL_VERsion")
	if defaultDockerReg == "" {
		defaultDockerReg = "radiusdev.azurecr.io"
	}
	if imageTag == "" {
		imageTag = "latest"
	}
}
