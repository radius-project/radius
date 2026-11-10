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
	"errors"
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

	// ucpAPIServerCertificateName is the cert-manager Certificate of UCP's API server, whose CA cert-manager's
	// cainjector writes into the APIService when Radius authorization is on.
	ucpAPIServerCertificateName = "ucp-cert"
)

var (
	apiServicesResource  = schema.GroupVersionResource{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"}
	certificatesResource = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}

	// ucpAPIServiceTimeout and ucpAPIServicePollInterval bound the wait for the UCP APIService. Tests shorten them.
	ucpAPIServiceTimeout      = 5 * time.Minute
	ucpAPIServicePollInterval = 2 * time.Second
)

// waitForUCPAPIService waits, when Radius authorization is on, until the UCP APIService has a CA bundle and the
// Kubernetes API server reports it Available. Helm waits only for workloads, and with authorization on the
// APIService trusts UCP only after cert-manager issues UCP's API server certificate and its cainjector copies the CA
// into the APIService. With authorization off, the chart sets the CA bundle itself and this returns at once.
func (i *Impl) waitForUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string, mode authz.Mode) error {
	if mode == authz.ModeOff {
		return nil
	}

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
		ucpAPIServiceDiagnostics(ctx, client, kubeContext, radiusNamespace, problem == ucpAPIServiceNoCABundle))
}

// waitForInstalledUCPAPIService waits for the UCP APIService after rad installs or upgrades Radius with the values of
// options merged over storedValues.
func (i *Impl) waitForInstalledUCPAPIService(ctx context.Context, options ClusterOptions, kubeContext string, storedValues map[string]any) error {
	currentValues, err := parseUserValuesFromCLI(&options.Radius)
	if err != nil {
		return fmt.Errorf("failed to parse Radius values: %w", err)
	}
	mode, err := resolveAuthzMode(storedValues, currentValues)
	if err != nil {
		return err
	}
	return i.waitForUCPAPIService(ctx, kubeContext, options.Radius.Namespace, mode)
}

// waitForRolledBackUCPAPIService waits for the UCP APIService after rad rolls Radius back to revision of history.
func (i *Impl) waitForRolledBackUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string, history []*releasev1.Release, revision int) error {
	for _, rel := range history {
		if rel == nil || rel.Version != revision {
			continue
		}
		mode, err := resolveAuthzMode(rel.Config, nil)
		if err != nil {
			return err
		}
		return i.waitForUCPAPIService(ctx, kubeContext, radiusNamespace, mode)
	}
	return nil
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
func ucpAPIServiceDiagnostics(ctx context.Context, client dynamic.Interface, kubeContext, radiusNamespace string, noCABundle bool) string {
	if !noCABundle {
		return fmt.Sprintf("Check that the ucp pods are ready and that the API server can reach Service ucp in namespace %s, for example with %s",
			radiusNamespace, kubectlCommand(kubeContext, "", "get apiservice "+ucpAPIServiceName+" -o yaml"))
	}

	certificate := "Certificate " + radiusNamespace + "/" + ucpAPIServerCertificateName
	ready := "its status is unknown"
	if object, err := client.Resource(certificatesResource).Namespace(radiusNamespace).Get(ctx, ucpAPIServerCertificateName, metav1.GetOptions{}); err == nil {
		if c, found := condition(object, "Ready"); found {
			ready = fmt.Sprintf("it is Ready=%s (%s: %s)", c.status, c.reason, c.message)
		} else {
			ready = "it has no Ready condition"
		}
	} else if apierrors.IsNotFound(err) {
		ready = "it does not exist"
	} else if !errors.Is(err, context.Canceled) {
		ready = "it could not be read: " + err.Error()
	}

	return fmt.Sprintf("cert-manager's cainjector copies the CA of %s into the APIService once the certificate is issued, and %s. Check the certificate with %s, and find the cainjector to check its logs with %s",
		certificate, ready,
		kubectlCommand(kubeContext, radiusNamespace, "describe certificate "+ucpAPIServerCertificateName),
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
