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
	"io"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"

	helm "helm.sh/helm/v4/pkg/action"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"

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
// the CA bundle itself, and this only removes the trust that authorization on left, as
// ReconcileUCPOffAPIServerTrust describes. declared holds the objects of the applied release, as manifestObjects
// returns them, which that cleanup keeps. fresh reports that the release is at its first revision.
func (i *Impl) waitForUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string, mode authz.Mode, declared manifestObjectSet, fresh bool) error {
	if mode == authz.ModeOff {
		return i.reconcileUCPOffAPIServerTrust(ctx, kubeContext, radiusNamespace, declared, fresh)
	}
	if err := i.reconcileUCPAPIServerTrust(ctx, kubeContext, radiusNamespace, fresh); err != nil {
		return err
	}
	return i.waitForAvailableUCPAPIService(ctx, kubeContext, radiusNamespace)
}

// waitForAppliedUCPAPIService waits for the UCP APIService after Helm applied rel, in the mode in which rel serves UCP's
// API server, as servedAuthzMode decides from its manifest rather than from its values: a chart that accepts the
// values of authorization without implementing them serves the certificate of authorization off.
func (i *Impl) waitForAppliedUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string, rel *releasev1.Release) error {
	mode, err := servedAuthzMode(rel)
	if err != nil {
		return err
	}
	declared, err := manifestObjects(rel.Manifest)
	if err != nil {
		return fmt.Errorf("failed to read the manifest of revision %d: %w", rel.Version, err)
	}
	return i.waitForUCPAPIService(ctx, kubeContext, radiusNamespace, mode, declared, false)
}

// waitForAvailableUCPAPIService waits until the UCP APIService has a CA bundle and the API server reports it
// Available.
func (i *Impl) waitForAvailableUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string) error {
	client, err := i.certManagerDynamicClient(kubeContext)
	if err != nil {
		return err
	}

	output.LogInfo("Waiting for the Kubernetes API server to trust and reach UCP...")
	problem, err := pollUCPAPIServer(ctx, func(ctx context.Context) (string, error) {
		return ucpAPIServiceProblem(ctx, client)
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

// waitForInstalledUCPAPIService waits for the UCP APIService in radiusNamespace after rad installs or upgrades Radius to
// applied, the release Helm returned, in the mode in which applied serves UCP's API server. userValues, the values that
// prepareRadiusChart read from the command line and Helm applied, merged over storedValues and chartDefaults, the
// values.yaml of the chart rad applied, select that mode, but the manifest of applied
// decides between on and off: a chart that accepts those values without implementing authorization, such as v0.62.0,
// serves UCP from the Secret ucp-cert of authorization off whatever they say, and a manifest with the cert-manager
// Certificate ucp-cert serves the certificate of authorization on.
func (i *Impl) waitForInstalledUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string, applied *releasev1.Release, chartDefaults, storedValues, userValues map[string]any) error {
	mode, err := resolveAuthzMode(chartDefaults, storedValues, userValues)
	if err != nil {
		return err
	}
	var declared manifestObjectSet
	if applied != nil {
		kind, err := ucpServingCertificateKind(applied.Manifest)
		if err != nil {
			return fmt.Errorf("failed to read the manifest of revision %d: %w", applied.Version, err)
		}
		mode = servingAuthzMode(kind, mode)
		if declared, err = manifestObjects(applied.Manifest); err != nil {
			return fmt.Errorf("failed to read the manifest of revision %d: %w", applied.Version, err)
		}
	}
	return i.waitForUCPAPIService(ctx, kubeContext, radiusNamespace, mode, declared, applied != nil && applied.Version == 1)
}

// servingAuthzMode returns the mode in which a manifest whose ucp-cert has kind, as ucpServingCertificateKind returns
// it, serves UCP's API server, given mode, the mode its values select. The kind decides between on and off; the trust
// rad reconciles depends only on that. A Certificate whose values select off serves as authorization on in
// authz.ModeEnforce, and a manifest without ucp-cert keeps mode.
func servingAuthzMode(kind string, mode authz.Mode) authz.Mode {
	switch {
	case kind == "Secret":
		return authz.ModeOff
	case kind == "Certificate" && mode == authz.ModeOff:
		return authz.ModeEnforce
	}
	return mode
}

// stageInstalledUCPAPIServerCA stages radius-apiserver-ca before rad installs or upgrades Radius with userValues, the
// values prepareRadiusChart read from the command line, merged over storedValues and chartDefaults. render returns the manifest that Helm applies. upgrade is true when Helm upgrades
// an installed release, as rad upgrade and rad install --reinstall do; otherwise the install is fresh or leaves the
// installed release unchanged.
func (i *Impl) stageInstalledUCPAPIServerCA(ctx context.Context, options ClusterOptions, kubeContext string, helmConf *helm.Configuration, chartDefaults, storedValues, userValues map[string]any, upgrade bool, render renderRadiusManifest) error {
	mode, err := resolveAuthzMode(chartDefaults, storedValues, userValues)
	if err != nil {
		return err
	}
	deployed := func() (authz.Mode, error) {
		if !upgrade {
			return authz.ModeOff, nil
		}
		// Decide from the newest revision, not from the stored values of the deployed one: a newer failed or
		// interrupted upgrade may have moved UCP to the certificate of authorization on while those values still turn
		// it off. deployedAuthzMode then returns "", so the stage checks which certificate UCP serves.
		history, err := i.radiusReleaseHistory(helmConf, options.Radius.ReleaseName)
		if err != nil {
			return "", err
		}
		return deployedAuthzMode(history)
	}
	return i.stageUCPAPIServerCA(ctx, kubeContext, options.Radius.Namespace, options.Radius.ReleaseName, deployed, mode, render)
}

// radiusReleaseHistory returns the history of release releaseName, or none when the release does not exist.
func (i *Impl) radiusReleaseHistory(helmConf *helm.Configuration, releaseName string) ([]*releasev1.Release, error) {
	history, err := i.Helm.RunHelmHistory(helmConf, releaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the history of release %s: %w", releaseName, err)
	}
	return history, nil
}

// latestRevision returns the newest revision of history.
func latestRevision(history []*releasev1.Release) *releasev1.Release {
	var latest *releasev1.Release
	for _, rel := range history {
		if rel != nil && (latest == nil || rel.Version > latest.Version) {
			latest = rel
		}
	}
	return latest
}

// certManagerDynamicClient returns the dynamic client for kubeContext.
func (i *Impl) certManagerDynamicClient(kubeContext string) (dynamic.Interface, error) {
	newClient := i.newCertManagerDynamicClient
	if newClient == nil {
		newClient = newCertManagerDynamicClient
	}
	client, err := newClient(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	return client, nil
}

// waitForRolledBackUCPAPIService waits for the UCP APIService after rad rolls Radius back to revision of history, in the
// mode in which that revision serves UCP's API server.
func (i *Impl) waitForRolledBackUCPAPIService(ctx context.Context, kubeContext, radiusNamespace string, history []*releasev1.Release, revision int) error {
	target := releaseRevision(history, revision)
	if target == nil {
		return nil
	}
	return i.waitForAppliedUCPAPIService(ctx, kubeContext, radiusNamespace, target)
}

// stageRolledBackUCPAPIServerCA stages UCP's API server certificate for revision of history before rad rolls Radius
// back to it, from what that rollback renders. When that revision serves UCP with authorization on, it first checks
// that cert-manager can issue and inject radius-apiserver-ca, as an install or upgrade does.
func (i *Impl) stageRolledBackUCPAPIServerCA(ctx context.Context, kubeContext, radiusNamespace string, history []*releasev1.Release, revision int) error {
	target := releaseRevision(history, revision)
	if target == nil {
		return nil
	}
	mode, err := servedAuthzMode(target)
	if err != nil {
		return err
	}
	if mode != authz.ModeOff {
		if err := i.checkInstalledCertManagerReady(ctx, kubeContext, radiusNamespace); err != nil {
			return err
		}
	}
	deployed := func() (authz.Mode, error) { return deployedAuthzMode(history) }
	return i.stageUCPAPIServerCA(ctx, kubeContext, radiusNamespace, radiusReleaseName, deployed, mode, i.renderRadiusRollback(kubeContext, radiusNamespace, history, revision))
}

// deployedAuthzMode returns the mode in which the newest revision of history serves UCP's API server, which is the
// release Helm rolls back from, or authz.ModeOff when history is empty. It returns "" when that revision is not
// deployed: a failed or interrupted upgrade may have changed UCP only partly, so the cluster, not history, shows which
// certificate UCP serves.
func deployedAuthzMode(history []*releasev1.Release) (authz.Mode, error) {
	latest := latestRevision(history)
	if latest == nil {
		return authz.ModeOff, nil
	}
	if latest.Info == nil || latest.Info.Status != releasecommon.StatusDeployed {
		return "", nil
	}
	return servedAuthzMode(latest)
}

// servedAuthzMode returns the mode in which rel serves UCP's API server. Its values, merged over the values.yaml of its
// chart, select the mode only in a chart that implements authorization, whose manifest has the cert-manager Certificate
// ucp-cert with authorization on. A chart that accepts the values without implementing them, such as v0.62.0, renders
// the Helm-generated Secret ucp-cert of authorization off whatever they say. A manifest has the same kind of ucp-cert
// as a rollback to rel renders, because the chart selects it from values alone, so the kind decides between on and
// off, as servingAuthzMode describes.
//
// It reads the kind before the values: a chart that ignores the values may have stored a combination that a chart
// implementing them rejects, such as global.rbac.enabled and global.rbac.dryRun both true, and its Secret ucp-cert
// still decides that it serves with authorization off.
func servedAuthzMode(rel *releasev1.Release) (authz.Mode, error) {
	kind, err := ucpServingCertificateKind(rel.Manifest)
	if err != nil {
		return "", fmt.Errorf("failed to read the manifest of revision %d: %w", rel.Version, err)
	}
	if kind == "Secret" {
		return authz.ModeOff, nil
	}
	var chartDefaults map[string]any
	if rel.Chart != nil {
		chartDefaults = rel.Chart.Values
	}
	mode, err := resolveAuthzMode(chartDefaults, rel.Config, nil)
	if err != nil {
		return "", err
	}
	return servingAuthzMode(kind, mode), nil
}

// manifestObjectSet holds the objects of a manifest by API group, kind and name.
type manifestObjectSet map[string]bool

func manifestObjectKey(group, kind, name string) string {
	return group + "/" + kind + "/" + name
}

// has reports whether the set holds object, the object of ucpAPIServerCAStageObjects with that name.
func (s manifestObjectSet) has(object stageInventoryObject) bool {
	return s[manifestObjectKey(object.resource.Group, object.kind, object.name)]
}

// manifestObjects returns the objects of manifest.
func manifestObjects(manifest string) (manifestObjectSet, error) {
	objects := manifestObjectSet{}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var document map[string]any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return objects, nil
		}
		if err != nil {
			return nil, err
		}
		if document == nil {
			continue
		}
		object := &unstructured.Unstructured{Object: document}
		objects[manifestObjectKey(object.GroupVersionKind().Group, object.GetKind(), object.GetName())] = true
	}
}

// ucpServingCertificateKind returns the kind of the ucp-cert object in manifest: Certificate with authorization on,
// Secret with it off, or "" when manifest has neither.
func ucpServingCertificateKind(manifest string) (string, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var document map[string]any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		object := &unstructured.Unstructured{Object: document}
		if document == nil || object.GetName() != ucpAPIServerCertificateName {
			continue
		}
		switch {
		case object.GetAPIVersion() == "cert-manager.io/v1" && object.GetKind() == "Certificate":
			return "Certificate", nil
		case object.GetAPIVersion() == "v1" && object.GetKind() == "Secret":
			return "Secret", nil
		}
	}
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

	// The Secret also holds the CA's private key, so the suggested commands read only the Certificate's status and
	// the public ca.crt.
	return fmt.Sprintf("cert-manager's cainjector copies ca.crt of Secret %s/%s into the APIService. Check that cert-manager issued the CA with %s and that the Secret has a ca.crt with %s, and find the cainjector to check its logs with %s",
		radiusNamespace, ucpAPIServerCASecretName,
		kubectlCommand(kubeContext, radiusNamespace, "describe certificate "+ucpAPIServerCASecretName),
		kubectlCommand(kubeContext, radiusNamespace, "get secret "+ucpAPIServerCASecretName+` -o jsonpath='{.data.ca\.crt}'`),
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

// ucpAPIServerUnchecked is the problem pollUCPAPIServer reports when no check finished before the wait ended.
const ucpAPIServerUnchecked = "rad could not finish a check before the wait ended"

// pollUCPAPIServer calls check every ucpAPIServicePollInterval until it reports no problem, an error, or
// ucpAPIServiceTimeout passes. It returns the problem of the last check that finished, or ucpAPIServerUnchecked if
// none did, with the error of the wait, which wait.Interrupted reports for the timeout. A check that fails because the
// wait ended reports nothing about Radius, so it does not replace the problem of the check before it.
func pollUCPAPIServer(ctx context.Context, check func(context.Context) (string, error)) (string, error) {
	problem := ucpAPIServerUnchecked
	err := wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		p, err := check(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, err
		}
		problem = p
		return p == "", nil
	})
	return problem, err
}
