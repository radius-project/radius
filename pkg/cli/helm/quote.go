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

package helm

import "strings"

// QuoteFlagValue quotes a flag value for a command that Radius suggests. It uses double quotes, which bash, zsh,
// PowerShell, and cmd.exe all remove without changing the value, unless the value has a character that one of them
// treats specially inside double quotes. bash changes a backslash only before $, `, ", \, or a newline, so a value
// such as C:\charts\contour keeps its double quotes. Other values get single quotes, which only POSIX shells
// understand.
func QuoteFlagValue(value string) string {
	if !strings.ContainsAny(value, "$`\"!\n") && !strings.Contains(value, `\\`) && !strings.HasSuffix(value, `\`) {
		return `"` + value + `"`
	}

	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// kubectlCommand returns a kubectl command for the cluster that kubeContext selects. It omits the namespace flag when
// namespace is empty, for example for cluster-scoped resources.
func kubectlCommand(kubeContext string, namespace string, arguments string) string {
	return clusterCommand("kubectl", "--context", kubeContext, namespace, arguments)
}

// helmCommand returns a helm command for the cluster that kubeContext selects.
func helmCommand(kubeContext string, namespace string, arguments string) string {
	return clusterCommand("helm", "--kube-context", kubeContext, namespace, arguments)
}

// clusterCommand returns a command for the cluster that Radius checked. With a kubeContext, the command names it with
// contextFlag, so it acts on that cluster even if the current context of the kubeconfig changes. Without one, the
// command omits contextFlag and uses the current context of the kubeconfig, as Radius did.
func clusterCommand(program string, contextFlag string, kubeContext string, namespace string, arguments string) string {
	command := program
	if kubeContext != "" {
		command += " " + contextFlag + " " + QuoteFlagValue(kubeContext)
	}
	if namespace != "" {
		command += " -n " + namespace
	}

	return command + " " + arguments
}
