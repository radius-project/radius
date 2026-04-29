/*
Copyright 2023 The Radius Authors.

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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

const (
	// terraformCLIConfigFileName is the file name written into the working directory and
	// referenced by the Terraform CLI via TF_CLI_CONFIG_FILE.
	terraformCLIConfigFileName = ".terraformrc"

	// terraformCLIConfigFileMode is the file mode for the generated .terraformrc.
	// 0600 is appropriate even though the current schema doesn't carry secrets, because
	// future credentials block support will write tokens here.
	terraformCLIConfigFileMode os.FileMode = 0600

	// envTFCLIConfigFile is the Terraform CLI environment variable that points to the
	// CLI configuration file. See https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_cli_config_file
	envTFCLIConfigFile = "TF_CLI_CONFIG_FILE"
)

// writeTerraformCLIConfig renders a .terraformrc file in workingDir from the given
// provider_installation configuration and returns the absolute path to it.
//
// Returns ("", nil) when the input is nil or has no installation methods configured,
// signalling that no CLI configuration file should be set for the Terraform invocation.
func writeTerraformCLIConfig(workingDir string, pi *datamodel.TerraformProviderInstallation) (string, error) {
	if pi == nil || (pi.NetworkMirror == nil && pi.Direct == nil) {
		return "", nil
	}

	body := renderProviderInstallationHCL(pi)
	if body == "" {
		return "", nil
	}

	path := filepath.Join(workingDir, terraformCLIConfigFileName)
	if err := os.WriteFile(path, []byte(body), terraformCLIConfigFileMode); err != nil {
		return "", fmt.Errorf("error writing %s: %w", terraformCLIConfigFileName, err)
	}
	return path, nil
}

// renderProviderInstallationHCL formats a Terraform CLI provider_installation block.
// See https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation
func renderProviderInstallationHCL(pi *datamodel.TerraformProviderInstallation) string {
	var b strings.Builder
	b.WriteString("provider_installation {\n")

	wrote := false
	if pi.NetworkMirror != nil && pi.NetworkMirror.URL != "" {
		b.WriteString("  network_mirror {\n")
		b.WriteString(fmt.Sprintf("    url = %s\n", quote(pi.NetworkMirror.URL)))
		writePatternList(&b, "include", pi.NetworkMirror.Include)
		writePatternList(&b, "exclude", pi.NetworkMirror.Exclude)
		b.WriteString("  }\n")
		wrote = true
	}

	if pi.Direct != nil && (len(pi.Direct.Include) > 0 || len(pi.Direct.Exclude) > 0) {
		b.WriteString("  direct {\n")
		writePatternList(&b, "include", pi.Direct.Include)
		writePatternList(&b, "exclude", pi.Direct.Exclude)
		b.WriteString("  }\n")
		wrote = true
	}

	b.WriteString("}\n")

	if !wrote {
		return ""
	}
	return b.String()
}

func writePatternList(b *strings.Builder, label string, patterns []string) {
	if len(patterns) == 0 {
		return
	}
	quoted := make([]string, len(patterns))
	for i, p := range patterns {
		quoted[i] = quote(p)
	}
	b.WriteString(fmt.Sprintf("    %s = [%s]\n", label, strings.Join(quoted, ", ")))
}

// quote produces an HCL string literal. The set of characters that need escaping in
// network mirror URLs and provider source patterns is small, so a minimal escape of
// backslashes and double quotes is sufficient.
func quote(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
