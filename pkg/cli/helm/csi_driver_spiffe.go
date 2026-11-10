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
	"reflect"
	"slices"
	"strings"

	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/output"
)

// csi-driver-spiffe issues the Radius server certificates. Radius installs its own instance, limited to the Radius
// issuer, when authorization is enabled. Bump the version as upstream releases move; see
// https://github.com/cert-manager/csi-driver-spiffe/releases.
const (
	csiDriverSPIFFEHelmRepo            = certManagerHelmRepo
	csiDriverSPIFFEReleaseName         = "cert-manager-csi-driver-spiffe"
	CSIDriverSPIFFEChartDefaultVersion = "v0.15.1"

	// csiDriverSPIFFEDaemonSetSelector matches the node DaemonSet of any csi-driver-spiffe installation. Every
	// installation registers its kubelet plugin at the same fixed host path, so a cluster can run only one.
	csiDriverSPIFFEDaemonSetSelector = "app.kubernetes.io/name=cert-manager-csi-driver-spiffe"

	// RadiusCAIssuerName is the cert-manager Issuer in the Radius namespace that signs Radius certificates. The
	// Radius chart creates it in deploy/Chart/templates/global/certificates.yaml.
	RadiusCAIssuerName = "radius-ca-issuer"

	// RadiusAPIServerIssuerName is the cert-manager Issuer in the Radius namespace that signs only UCP's aggregated
	// API certificate, from its own CA in the selfSigned certificate mode. cert-manager's default approver approves
	// its requests.
	RadiusAPIServerIssuerName = "radius-apiserver-issuer"

	// RadiusBootstrapIssuerName is the self-signed Issuer in the Radius namespace that issues the Radius CA and the
	// API server CA. cert-manager's default approver approves its requests.
	RadiusBootstrapIssuerName = "radius-selfsigned-bootstrap"

	// SPIFFETrustDomainDefault and SPIFFECSIDriverNameDefault are the defaults of the Radius chart values
	// global.rbac.spiffe.trustDomain and global.rbac.spiffe.csiDriverName. Keep them in sync with
	// deploy/Chart/values.yaml.
	SPIFFETrustDomainDefault   = "radius.internal"
	SPIFFECSIDriverNameDefault = "spiffe.csi.radapp.io"

	// spiffeCertificateDuration is the lifetime of server certificates. The csi-driver-spiffe approver denies
	// requests for any other duration.
	spiffeCertificateDuration = "24h"
)

type CSIDriverSPIFFEChartOptions struct {
	ChartOptions
}

// spiffeSettings are the Radius chart values that the csi-driver-spiffe instance must match.
type spiffeSettings struct {
	TrustDomain   string
	CSIDriverName string
}

// resolveSPIFFESettings returns the SPIFFE settings the Radius chart will render, merging the current invocation's
// values over the stored release values like resolveAuthzMode.
func resolveSPIFFESettings(stored, current map[string]any) spiffeSettings {
	merged := valuesutil.CoalesceTables(copyValues(current), copyValues(stored))
	global, _ := merged["global"].(map[string]any)
	rbac, _ := global["rbac"].(map[string]any)
	spiffe, _ := rbac["spiffe"].(map[string]any)

	settings := spiffeSettings{TrustDomain: SPIFFETrustDomainDefault, CSIDriverName: SPIFFECSIDriverNameDefault}
	if v, ok := spiffe["trustDomain"].(string); ok && v != "" {
		settings.TrustDomain = v
	}
	if v, ok := spiffe["csiDriverName"].(string); ok && v != "" {
		settings.CSIDriverName = v
	}
	return settings
}

// radiusCAIssuerSignerName is the cert-manager signer name of the Radius CA issuer in radiusNamespace.
func radiusCAIssuerSignerName(radiusNamespace string) string {
	return issuerSignerName(radiusNamespace, RadiusCAIssuerName)
}

// issuerSignerName returns the cert-manager signer name of the namespaced Issuer name in namespace.
func issuerSignerName(namespace, name string) string {
	return "issuers.cert-manager.io/" + namespace + "." + name
}

// buildCSIDriverSPIFFEValues configures csi-driver-spiffe to request certificates from the Radius CA issuer and
// limits its approver to that issuer. The approver only processes requests that carry the csi-driver-spiffe
// identity annotation, so it neither approves nor denies other requests to the issuer.
//
// app.driver.sourceCABundle is deliberately unset. The driver then writes no ca.crt into the CSI volume, and the
// Radius chart mounts ca.crt from the radius-ca Secret into every component instead. The driver reads the source
// bundle at startup and fails when it is missing, and rad installs the driver before the Radius chart creates
// radius-ca, so a source bundle would stop the driver from starting.
func buildCSIDriverSPIFFEValues(radiusNamespace string, settings spiffeSettings) map[string]any {
	return map[string]any{
		"app": map[string]any{
			"trustDomain":                settings.TrustDomain,
			"name":                       settings.CSIDriverName,
			"certificateRequestDuration": spiffeCertificateDuration,
			"issuer": map[string]any{
				"name":  RadiusCAIssuerName,
				"kind":  "Issuer",
				"group": "cert-manager.io",
			},
			"approver": map[string]any{
				"signerName":           radiusCAIssuerSignerName(radiusNamespace),
				"autoApproveNonSPIFFE": false,
			},
		},
	}
}

// getReleaseOwnership returns the latest Helm release for options and whether it carries rad's ownership labels. It
// returns a nil release when the release does not exist.
func (i *Impl) getReleaseOwnership(options ChartOptions, kubeContext string) (*releasev1.Release, bool, error) {
	rel, err := NewHelmAction(i.Helm).getRelease(kubeContext, options.ReleaseName, options.Namespace)
	if err != nil || rel == nil {
		return nil, false, err
	}
	for k, v := range radManagedByLabels() {
		if rel.Labels[k] != v {
			return rel, false, nil
		}
	}
	return rel, true, nil
}

// ensureCSIDriverSPIFFE installs or upgrades Radius's csi-driver-spiffe instance when Radius authorization is
// enabled. It runs after ensureCertManager, because the chart uses cert-manager's CRDs. It upgrades a release rad
// installed when upgrade is true, when the release is not deployed, or when its values no longer match the Radius
// values. It fails when another csi-driver-spiffe runs on the cluster, because only one can register on each node.
// With authorization off it returns without contacting the cluster.
//
// It returns the users the cert-manager controllers run as, which the Radius chart's certificate issuance policy
// allows to request certificates from the Radius bootstrap and API server issuers. It returns none with
// authorization off or when it finds no cert-manager controller.
func (i *Impl) ensureCSIDriverSPIFFE(ctx context.Context, options ClusterOptions, kubeContext string, storedValues map[string]any, upgrade bool) ([]string, error) {
	currentValues, err := parseUserValuesFromCLI(&options.Radius)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Radius values: %w", err)
	}
	mode, err := resolveAuthzMode(storedValues, currentValues)
	if err != nil {
		return nil, err
	}
	if mode == authz.ModeOff {
		return nil, nil
	}

	chartOptions := options.CSIDriverSPIFFE.ChartOptions
	values := buildCSIDriverSPIFFEValues(options.Radius.Namespace, resolveSPIFFESettings(storedValues, currentValues))

	rel, owned, err := i.getReleaseOwnership(chartOptions, kubeContext)
	if err != nil {
		return nil, err
	}
	if rel != nil && !owned {
		return nil, clierrors.Message("Radius stopped before changing the cluster because Helm release %q in namespace %q was not installed by rad. Radius installs and manages its own csi-driver-spiffe. Remove that release, for example with %s, and then run the command again.",
			chartOptions.ReleaseName, chartOptions.Namespace, helmCommand(kubeContext, chartOptions.Namespace, "uninstall "+chartOptions.ReleaseName))
	}

	newClient := i.newCSIDriverSPIFFEClient
	if newClient == nil {
		newClient = func(kubeContext string) (kubernetes.Interface, error) {
			kubeClient, _, err := newCertManagerClients(kubeContext)
			return kubeClient, err
		}
	}
	kubeClient, err := newClient(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	if err := checkForeignCSIDriverSPIFFE(ctx, kubeClient, chartOptions, kubeContext); err != nil {
		return nil, err
	}

	// Only Radius's csi-driver-spiffe approver may approve requests for the Radius CA issuer. A cert-manager default
	// approver that can approve them would sign any SPIFFE ID, so rad stops before changing the cluster.
	var conflict *defaultApproverConflictError
	if err := checkCertManagerDefaultApprover(ctx, kubeClient, options.Radius.Namespace, kubeContext); errors.As(err, &conflict) {
		return nil, clierrors.Message("Radius stopped before changing the cluster because %s", conflict.Error())
	} else if err != nil {
		return nil, err
	}
	if err := checkValidatingAdmissionPolicyServed(kubeClient); err != nil {
		return nil, err
	}
	requesters, err := certManagerControllerUsers(ctx, kubeClient)
	if err != nil {
		return nil, err
	}
	if missing, err := missingApprovableRadiusSigners(ctx, kubeClient, options.Radius.Namespace); err != nil {
		i.logInfo("Warning: %v", err)
	} else if len(missing) > 0 {
		i.logInfo("Warning: cert-manager's default approver cannot approve CertificateRequests for %s. The Radius CAs and UCP's API server certificate are issued only if another approver approves them.",
			strings.Join(missing, ", "))
	}

	helmAction := NewHelmAction(i.Helm)
	if rel != nil {
		deployed := rel.Info != nil && rel.Info.Status == releasecommon.StatusDeployed
		if !upgrade && deployed && reflect.DeepEqual(rel.Config, values) {
			output.LogInfo("Using csi-driver-spiffe installed by rad in namespace %s", chartOptions.Namespace)
			return requesters, nil
		}
		output.LogInfo("Upgrading csi-driver-spiffe...")
		helmChart, helmConf, err := loadChart(ctx, helmAction, chartOptions, kubeContext)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare csi-driver-spiffe Helm chart, err: %w", err)
		}
		if _, err := i.Helm.RunHelmUpgrade(helmConf, helmChart, values, chartOptions.ReleaseName, chartOptions.Namespace, chartOptions.Wait, false, chartOptions.Timeout); err != nil {
			return nil, fmt.Errorf("failed to upgrade csi-driver-spiffe, err: %w", err)
		}
		return requesters, nil
	}

	output.LogInfo("Installing csi-driver-spiffe %s...", chartOptions.ChartVersion)
	helmChart, helmConf, err := loadChart(ctx, helmAction, chartOptions, kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare csi-driver-spiffe Helm chart, err: %w", err)
	}
	if _, err := i.Helm.RunHelmInstall(helmConf, helmChart, values, chartOptions.ReleaseName, chartOptions.Namespace, chartOptions.Wait, chartOptions.Timeout, radManagedByLabels()); err != nil {
		return nil, fmt.Errorf("failed to install csi-driver-spiffe, err: %w", err)
	}
	return requesters, nil
}

// checkForeignCSIDriverSPIFFE fails when a csi-driver-spiffe that is not Radius's release runs on the cluster.
func checkForeignCSIDriverSPIFFE(ctx context.Context, kubeClient kubernetes.Interface, options ChartOptions, kubeContext string) error {
	daemonSets, err := kubeClient.AppsV1().DaemonSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: csiDriverSPIFFEDaemonSetSelector})
	if err != nil {
		return fmt.Errorf("failed to list csi-driver-spiffe DaemonSets: %w", err)
	}
	for _, ds := range daemonSets.Items {
		if ds.Namespace == options.Namespace && ds.Labels["app.kubernetes.io/instance"] == options.ReleaseName {
			continue
		}
		return clierrors.Message("Radius stopped before changing the cluster because another csi-driver-spiffe (DaemonSet %s/%s) runs on it. Radius installs its own csi-driver-spiffe, limited to the Radius issuer, and a cluster can run only one because each registers the same kubelet plugin path. Remove the other installation, for example with %s, and then run the command again.",
			ds.Namespace, ds.Name, helmCommand(kubeContext, ds.Namespace, "uninstall "+releaseNameOf(&ds)))
	}
	return nil
}

func releaseNameOf(ds *appsv1.DaemonSet) string {
	if name := ds.Labels["app.kubernetes.io/instance"]; name != "" {
		return name
	}
	return csiDriverSPIFFEReleaseName
}

// certManagerApproverDisabled reports whether the cert-manager controller runs without its default approver, either
// with --controllers=...,-certificaterequests-approver or with the chart value disableAutoApproval.
func certManagerApproverDisabled(deployment *appsv1.Deployment) bool {
	for _, c := range deployment.Spec.Template.Spec.Containers {
		for idx, arg := range c.Args {
			var value string
			switch {
			case strings.HasPrefix(arg, "--controllers="):
				value = strings.TrimPrefix(arg, "--controllers=")
			case arg == "--controllers" && idx+1 < len(c.Args):
				value = c.Args[idx+1]
			default:
				continue
			}
			if slices.Contains(strings.Split(value, ","), "-certificaterequests-approver") {
				return true
			}
		}
	}
	return false
}

// checkCertManagerDefaultApprover returns an error when cert-manager's default approver can approve
// CertificateRequests for the Radius CA issuer in radiusNamespace. Only Radius's csi-driver-spiffe approver should
// approve server certificates for that issuer: the default approver approves every request it is allowed to,
// including a request for another service's SPIFFE ID. cert-manager's webhook allows the approval when the approver
// may approve the signer's wildcard name or its exact name, so both are checked.
func checkCertManagerDefaultApprover(ctx context.Context, kubeClient kubernetes.Interface, radiusNamespace string, kubeContext string) error {
	deployments, err := kubeClient.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: certManagerControllerSelector})
	if err != nil {
		return fmt.Errorf("could not check cert-manager's default approver: failed to list cert-manager deployments: %w", err)
	}

	signerName := radiusCAIssuerSignerName(radiusNamespace)
	for idx := range deployments.Items {
		controller := &deployments.Items[idx]
		if certManagerApproverDisabled(controller) {
			continue
		}
		allowed, err := defaultApproverCanApprove(ctx, kubeClient, controller, signerName)
		if err != nil {
			return err
		}
		if allowed {
			return newDefaultApproverConflictError(controller, defaultApproverUser(controller), radiusNamespace, kubeContext)
		}
	}
	return nil
}

// missingApprovableRadiusSigners returns the Radius bootstrap and API server issuer signers that no enabled
// cert-manager default approver may approve. With every default approver turned off it returns both; with no
// cert-manager controller found it returns none, because there is nothing to report on.
func missingApprovableRadiusSigners(ctx context.Context, kubeClient kubernetes.Interface, radiusNamespace string) ([]string, error) {
	deployments, err := kubeClient.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: certManagerControllerSelector})
	if err != nil {
		return nil, fmt.Errorf("could not check cert-manager's default approver: failed to list cert-manager deployments: %w", err)
	}
	if len(deployments.Items) == 0 {
		return nil, nil
	}
	var missing []string
	for _, signerName := range []string{issuerSignerName(radiusNamespace, RadiusBootstrapIssuerName), issuerSignerName(radiusNamespace, RadiusAPIServerIssuerName)} {
		approvable := false
		for idx := range deployments.Items {
			controller := &deployments.Items[idx]
			if certManagerApproverDisabled(controller) {
				continue
			}
			allowed, err := defaultApproverCanApprove(ctx, kubeClient, controller, signerName)
			if err != nil {
				return nil, err
			}
			if allowed {
				approvable = true
				break
			}
		}
		if !approvable {
			missing = append(missing, signerName)
		}
	}
	return missing, nil
}

// certManagerControllerUsers returns the sorted users the cert-manager controller deployments run as. The controller
// creates the CertificateRequests of Certificate resources, such as the Radius CAs and UCP's API server certificate.
func certManagerControllerUsers(ctx context.Context, kubeClient kubernetes.Interface) ([]string, error) {
	deployments, err := kubeClient.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: certManagerControllerSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list cert-manager deployments: %w", err)
	}
	var users []string
	for idx := range deployments.Items {
		users = append(users, defaultApproverUser(&deployments.Items[idx]))
	}
	slices.Sort(users)
	return slices.Compact(users), nil
}

// checkValidatingAdmissionPolicyServed fails when the cluster does not serve ValidatingAdmissionPolicy. The Radius
// chart enforces authorization with these policies, including the policy that reserves the Radius bootstrap and API
// server issuers for cert-manager, and renders none without the API.
func checkValidatingAdmissionPolicyServed(kubeClient kubernetes.Interface) error {
	const groupVersion = "admissionregistration.k8s.io/v1"
	resources, err := kubeClient.Discovery().ServerResourcesForGroupVersion(groupVersion)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("could not check for the ValidatingAdmissionPolicy API: %w", err)
	}
	if resources != nil {
		for _, resource := range resources.APIResources {
			if resource.Name == "validatingadmissionpolicies" {
				return nil
			}
		}
	}
	return clierrors.Message("Radius stopped before changing the cluster because the cluster does not serve ValidatingAdmissionPolicy (%s). Radius authorization uses these policies to restrict Radius workloads and certificate issuance, and needs Kubernetes 1.30 or later. Upgrade the cluster, or install Radius with authorization off.", groupVersion)
}

// defaultApproverUser returns the user the cert-manager controller deployment runs as.
func defaultApproverUser(controller *appsv1.Deployment) string {
	serviceAccount := controller.Spec.Template.Spec.ServiceAccountName
	if serviceAccount == "" {
		serviceAccount = "default"
	}
	return "system:serviceaccount:" + controller.Namespace + ":" + serviceAccount
}

// defaultApproverCanApprove reports whether the cert-manager controller may approve requests for signerName.
// cert-manager's webhook allows the approval when the approver may approve the signer's wildcard name or its exact
// name, and RBAC resourceNames do not match wildcards, so both are checked.
func defaultApproverCanApprove(ctx context.Context, kubeClient kubernetes.Interface, controller *appsv1.Deployment, signerName string) (bool, error) {
	wildcard := signerName[:strings.Index(signerName, "/")+1] + "*"
	for _, name := range []string{wildcard, signerName} {
		review := &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   defaultApproverUser(controller),
			Groups: []string{"system:serviceaccounts", "system:serviceaccounts:" + controller.Namespace, "system:authenticated"},
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Group:    "cert-manager.io",
				Resource: "signers",
				Verb:     "approve",
				Name:     name,
			},
		}}
		result, err := kubeClient.AuthorizationV1().SubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
		if err != nil {
			return false, fmt.Errorf("could not check cert-manager's default approver: %w", err)
		}
		if result.Status.Allowed {
			return true, nil
		}
	}
	return false, nil
}

// defaultApproverConflictError reports that cert-manager's default approver can approve requests for the Radius
// issuer. Its message tells the user how to limit the default approver to other signers.
type defaultApproverConflictError struct {
	User       string
	SignerName string
	Command    string
}

func (e *defaultApproverConflictError) Error() string {
	return fmt.Sprintf("cert-manager's default approver (%s) can approve CertificateRequests for the Radius CA issuer signer %s. Only Radius's csi-driver-spiffe approver may approve them. Limit the default approver to other signers, for example with %s. Add the signer names of your own namespaced Issuers to that list, or approve their requests with a policy approver such as approver-policy, and then run the command again.",
		e.User, e.SignerName, e.Command)
}

func newDefaultApproverConflictError(controller *appsv1.Deployment, user, radiusNamespace, kubeContext string) error {
	release := controller.Labels["app.kubernetes.io/instance"]
	if release == "" {
		release = certManagerReleaseName
	}
	version := controller.Labels[certManagerVersionLabel]
	args := "upgrade " + release + " cert-manager --repo " + certManagerHelmRepo
	if version != "" {
		args += " --version " + version
	}
	signers := strings.Join(certManagerApproveSignerNames(radiusNamespace), ",")
	args += " --reuse-values --set " + QuoteFlagValue("approveSignerNames={"+signers+"}")
	return &defaultApproverConflictError{User: user, SignerName: radiusCAIssuerSignerName(radiusNamespace), Command: helmCommand(kubeContext, controller.Namespace, args)}
}

// CSIDriverSPIFFEInstalledByRad reports whether the default csi-driver-spiffe release exists and rad installed it.
func (i *Impl) CSIDriverSPIFFEInstalledByRad(kubeContext string) (bool, error) {
	_, owned, err := i.getReleaseOwnership(NewDefaultClusterOptions().CSIDriverSPIFFE.ChartOptions, kubeContext)
	return owned, err
}

// uninstallCSIDriverSPIFFE removes csi-driver-spiffe when rad installed it. It keeps the release, without failing,
// when the ownership check fails, like uninstallCertManager.
func (i *Impl) uninstallCSIDriverSPIFFE(options ClusterOptions, kubeContext string) error {
	chartOptions := options.CSIDriverSPIFFE.ChartOptions
	_, owned, err := i.getReleaseOwnership(chartOptions, kubeContext)
	if err != nil {
		i.logInfo("Keeping csi-driver-spiffe: could not check whether rad installed it: %v", err)
		return nil
	}
	if !owned {
		return nil
	}
	return i.uninstallHelmRelease("csi-driver-spiffe", chartOptions.ReleaseName, chartOptions.Namespace, kubeContext)
}
