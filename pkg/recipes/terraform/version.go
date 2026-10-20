/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package terraform

import (
	"os"
	"strings"
)

// VersionEnvVar overrides the Terraform version Radius downloads. The Helm chart sets it from
// global.terraform.version. An environment variable is used rather than a config file field so
// that newer Helm charts remain compatible with older images, which reject unknown config fields.
const VersionEnvVar = "RADIUS_TERRAFORM_VERSION"

// PremountedOnlyEnvVar, when set to "true", stops Radius from downloading Terraform at runtime, so it
// only uses the binary installed by the terraform-init init container. The Helm chart sets it when
// global.terraform.downloadUrl is used, so recipes never fall back to a different Terraform build.
const PremountedOnlyEnvVar = "RADIUS_TERRAFORM_PREMOUNTED_ONLY"

// terraformVersion is the version of Terraform that Radius downloads and
// uses to execute Terraform recipes.
//
// The canonical source of truth is the build/tools.yaml manifest. The updater
// synchronizes the .terraform-version compatibility file, chart default, and
// this fallback with that manifest. The Makefile also overrides this value at
// build time via the -X linker flag. The hard-coded default below is used by
// `go test`, `go run`, and other invocations that do not go through the
// Makefile; the TestTerraformVersionMatchesFile test guarantees it stays in
// sync with the compatibility file.
var terraformVersion = "1.16.5"

// TerraformVersion returns the Terraform version Radius will install: the value of
// VersionEnvVar when set, otherwise the built-in default.
func TerraformVersion() string {
	if v := strings.TrimSpace(os.Getenv(VersionEnvVar)); v != "" {
		return v
	}
	return terraformVersion
}

// premountedOnly reports whether Radius must only use the pre-mounted Terraform binary.
func premountedOnly() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(PremountedOnlyEnvVar)), "true")
}
