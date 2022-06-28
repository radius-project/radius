// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

const (
	EnvironmentsResource = "environments"
	ApplicationsResource = "applications"
	HttpRoutesResource   = "httpRoutes"
	ContainersResource   = "containers"
)

type Resource struct {
	Type string
	Name string
}

func VerifyResources() {
}

func DeleteResources() {

}
