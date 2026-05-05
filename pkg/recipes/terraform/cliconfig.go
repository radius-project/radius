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
	"sort"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
)

const (
	// terraformCLIConfigFileName is the file name written into the working directory and
	// referenced by the Terraform CLI via TF_CLI_CONFIG_FILE.
	terraformCLIConfigFileName = ".terraformrc"

	// terraformCLIConfigFileMode is the file mode for the generated .terraformrc.
	// 0600 is appropriate because the file may carry credential tokens written
	// inline by the credentials block renderer.
	terraformCLIConfigFileMode os.FileMode = 0600

	// envTFCLIConfigFile is the Terraform CLI environment variable that points to the
	// CLI configuration file. See https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_cli_config_file
	envTFCLIConfigFile = "TF_CLI_CONFIG_FILE"

	// TerraformCredentialsTokenKey is the secret store key that holds the auth
	// token written into a credentials "host" {} block in the generated
	// .terraformrc. Documented on TerraformCredentialConfig.secret in TypeSpec.
	// Exported so the Terraform driver can request the same key when populating
	// the secret-store resolver map.
	TerraformCredentialsTokenKey = "token"
)

// writeTerraformCLIConfig renders a .terraformrc file in workingDir from the given
// provider_installation and credentials configuration and returns the absolute path
// to it.
//
// secrets is the map of fetched secret data keyed by secret store ID, as supplied
// in Options.Secrets. Credential entries that reference secret stores not present
// in this map (or missing the `token` key) cause the call to fail; this prevents
// silently rendering a .terraformrc without credentials the user has configured.
//
// Returns ("", nil) when neither input has any content to render.
func writeTerraformCLIConfig(
	workingDir string,
	pi *datamodel.TerraformProviderInstallation,
	credentials map[string]datamodel.TerraformCredentialConfig,
	secrets map[string]recipes.SecretData,
) (string, error) {
	piHasContent := pi != nil && (pi.NetworkMirror != nil || pi.Direct != nil)
	if !piHasContent && len(credentials) == 0 {
		return "", nil
	}

	body, err := renderTerraformrcHCL(pi, credentials, secrets)
	if err != nil {
		return "", err
	}
	if body == "" {
		return "", nil
	}

	path := filepath.Join(workingDir, terraformCLIConfigFileName)
	if err := os.WriteFile(path, []byte(body), terraformCLIConfigFileMode); err != nil {
		return "", fmt.Errorf("error writing %s: %w", terraformCLIConfigFileName, err)
	}
	return path, nil
}

// renderTerraformrcHCL composes the full .terraformrc body from the optional
// provider_installation block and the optional credentials map. Hostname keys are
// emitted in deterministic order so the generated file is stable across runs.
func renderTerraformrcHCL(
	pi *datamodel.TerraformProviderInstallation,
	credentials map[string]datamodel.TerraformCredentialConfig,
	secrets map[string]recipes.SecretData,
) (string, error) {
	var b strings.Builder

	if pi != nil {
		piBody := renderProviderInstallationHCL(pi)
		b.WriteString(piBody)
	}

	if len(credentials) > 0 {
		hosts := make([]string, 0, len(credentials))
		for host := range credentials {
			hosts = append(hosts, host)
		}
		sort.Strings(hosts)

		for _, host := range hosts {
			cred := credentials[host]
			if cred.Secret == "" {
				return "", fmt.Errorf("terraform credentials entry for host %q has no secret reference", host)
			}
			secretData, ok := secrets[cred.Secret]
			if !ok {
				return "", fmt.Errorf("terraform credentials entry for host %q references secret store %q, but no secret data was fetched", host, cred.Secret)
			}
			token, ok := secretData.Data[TerraformCredentialsTokenKey]
			if !ok {
				return "", fmt.Errorf("terraform credentials entry for host %q: secret store %q is missing the %q key", host, cred.Secret, TerraformCredentialsTokenKey)
			}
			b.WriteString(fmt.Sprintf("credentials %s {\n", quote(host)))
			b.WriteString(fmt.Sprintf("  token = %s\n", quote(token)))
			b.WriteString("}\n")
		}
	}

	return b.String(), nil
}

// renderProviderInstallationHCL formats a Terraform CLI provider_installation block.
// See https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation
func renderProviderInstallationHCL(pi *datamodel.TerraformProviderInstallation) string {
	if pi == nil {
		return ""
	}

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
// network mirror URLs, registry hostnames, provider source patterns, and tokens is
// small, so a minimal escape of backslashes and double quotes is sufficient.
func quote(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
