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

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"

	releasev1 "helm.sh/helm/v4/pkg/release/v1"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/cli/output"
)

const (
	// ucpAPIServiceName is the APIService through which the Kubernetes API server reaches UCP.
	ucpAPIServiceName = "v1alpha3.api.ucp.dev"

	// ucpAPIServerCertificateName is the cert-manager Certificate of UCP's API server when Radius authorization is
	// on.
	ucpAPIServerCertificateName = "ucp-cert"
)

var (
	apiServicesResource = schema.GroupVersionResource{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"}

	// ucpAPIServiceTimeout and ucpAPIServicePollInterval bound the wait for the UCP APIService. Tests shorten them.
	ucpAPIServiceTimeout      = 5 * time.Minute
	ucpAPIServicePollInterval = 2 * time.Second
)

// waitForUCPAPIService, when Radius authorization is on, reconciles the CAs the Kubernetes API server trusts for UCP
// and waits until the UCP APIService has a CA bundle and the API server reports it Available. Helm waits only for
// workloads, and with authorization on the APIService trusts UCP only after cert-manager issues
// radius-apiserver-ca and its cainjector copies the CA into the APIService. With authorization off, the chart sets
// the CA bundle itself and this returns at once.
func (i *Impl) waitForUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string, mode authz.Mode) error {
	if mode == authz.ModeOff {
		return nil
	}
	if err := i.reconcileUCPAPIServerTrust(ctx, kubeContext, radiusNamespace); err != nil {
		return err
	}
	return i.waitForAvailableUCPAPIService(ctx, kubeContext, radiusNamespace)
}

// waitForAvailableUCPAPIService waits until the UCP APIService has a CA bundle and the API server reports it
// Available.
func (i *Impl) waitForAvailableUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string) error {
	newClient := i.newCertManagerDynamicClient
	if newClient == nil {
		newClient = newCertManagerDynamicClient
	}
	client, err := newClient(kubeContext)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	output.LogInfo("Waiting for the Kubernetes API server to trust and reach UCP...")
	var problem string
	err = wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		problem, err = ucpAPIServiceProblem(ctx, client)
		if err != nil {
			return false, err
		}
		return problem == "", nil
	})
	if err == nil {
		return nil
	}
	if !wait.Interrupted(err) {
		return fmt.Errorf("failed to check APIService %s: %w", ucpAPIServiceName, err)
	}

	return fmt.Errorf("APIService %s is not usable after %s: %s. %s", ucpAPIServiceName, ucpAPIServiceTimeout, problem,
		ucpAPIServiceDiagnostics(kubeContext, radiusNamespace, problem == ucpAPIServiceNoCABundle))
}

// waitForInstalledUCPAPIService waits for the UCP APIService after rad installs or upgrades Radius with the values of
// options merged over storedValues.
func (i *Impl) waitForInstalledUCPAPIService(ctx context.Context, options ClusterOptions, kubeContext string, storedValues map[string]any) error {
	mode, err := installedAuthzMode(options, storedValues)
	if err != nil {
		return err
	}
	return i.waitForUCPAPIService(ctx, kubeContext, options.Radius.Namespace, mode)
}

// seedInstalledUCPAPIServerTrust seeds the UCP API server trust before rad installs or upgrades Radius with the
// values of options merged over storedValues.
func (i *Impl) seedInstalledUCPAPIServerTrust(ctx context.Context, options ClusterOptions, kubeContext string, storedValues map[string]any) error {
	mode, err := installedAuthzMode(options, storedValues)
	if err != nil {
		return err
	}
	return i.seedUCPAPIServerTrust(ctx, kubeContext, options.Radius.Namespace, mode)
}

func installedAuthzMode(options ClusterOptions, storedValues map[string]any) (authz.Mode, error) {
	currentValues, err := parseUserValuesFromCLI(&options.Radius)
	if err != nil {
		return "", fmt.Errorf("failed to parse Radius values: %w", err)
	}
	return resolveAuthzMode(storedValues, currentValues)
}

// waitForRolledBackUCPAPIService waits for the UCP APIService after rad rolls Radius back to revision of history.
func (i *Impl) waitForRolledBackUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string, history []*releasev1.Release, revision int) error {
	mode, found, err := rolledBackAuthzMode(history, revision)
	if err != nil || !found {
		return err
	}
	return i.waitForUCPAPIService(ctx, kubeContext, radiusNamespace, mode)
}

// seedRolledBackUCPAPIServerTrust seeds the UCP API server trust before rad rolls Radius back to revision of history.
func (i *Impl) seedRolledBackUCPAPIServerTrust(ctx context.Context, kubeContext, radiusNamespace string, history []*releasev1.Release, revision int) error {
	mode, found, err := rolledBackAuthzMode(history, revision)
	if err != nil || !found {
		return err
	}
	return i.seedUCPAPIServerTrust(ctx, kubeContext, radiusNamespace, mode)
}

func rolledBackAuthzMode(history []*releasev1.Release, revision int) (authz.Mode, bool, error) {
	for _, rel := range history {
		if rel == nil || rel.Version != revision {
			continue
		}
		mode, err := resolveAuthzMode(rel.Config, nil)
		return mode, err == nil, err
	}
	return "", false, nil
}

const ucpAPIServiceNoCABundle = "it has no CA bundle"

// ucpAPIServiceProblem describes why the UCP APIService is not usable yet, or returns "" when it has a CA bundle and
// is Available.
func ucpAPIServiceProblem(ctx context.Context, client dynamic.Interface) (string, error) {
	apiService, err := client.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "it does not exist", nil
	}
	if err != nil {
		return "", err
	}
	if caBundle, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle"); caBundle == "" {
		return ucpAPIServiceNoCABundle, nil
	}

	available, found := condition(apiService, "Available")
	if !found {
		return "the Kubernetes API server has not reported whether it is Available", nil
	}
	if available.status != "True" {
		return fmt.Sprintf("it is not Available (%s: %s)", available.reason, available.message), nil
	}
	return "", nil
}

// ucpAPIServiceDiagnostics tells the user where to look when the UCP APIService does not become usable.
func ucpAPIServiceDiagnostics(kubeContext, radiusNamespace string, noCABundle bool) string {
	if !noCABundle {
		return fmt.Sprintf("Check that the ucp pods are ready and that the API server can reach Service ucp in namespace %s, for example with %s",
			radiusNamespace, kubectlCommand(kubeContext, "", "get apiservice "+ucpAPIServiceName+" -o yaml"))
	}

	return fmt.Sprintf("cert-manager's cainjector copies ca.crt of Secret %s/%s into the APIService. Check the Secret with %s, and find the cainjector to check its logs with %s",
		radiusNamespace, ucpAPIServerCASecretName,
		kubectlCommand(kubeContext, radiusNamespace, "get secret "+ucpAPIServerCASecretName+" -o yaml"),
		kubectlCommand(kubeContext, "", "get pods -A -l app.kubernetes.io/component=cainjector"))
}

type objectCondition struct {
	status, reason, message string
}

// condition returns the status condition of conditionType of object.
func condition(object *unstructured.Unstructured, conditionType string) (objectCondition, bool) {
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, item := range conditions {
		c, ok := item.(map[string]any)
		if !ok || c["type"] != conditionType {
			continue
		}
		status, _ := c["status"].(string)
		reason, _ := c["reason"].(string)
		message, _ := c["message"].(string)
		return objectCondition{status: status, reason: reason, message: message}, true
	}
	return objectCondition{}, false
}
