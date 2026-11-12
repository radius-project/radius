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
	"path"
	"reflect"
	"slices"
	"strings"
	"time"

	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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

	// csiDriverSPIFFEPluginDir is the kubelet plugin directory that every csi-driver-spiffe installation mounts and
	// registers its socket in. The upstream chart hardcodes it, even with nameOverride, so a cluster can run only one
	// installation.
	csiDriverSPIFFEPluginDir = "/var/lib/kubelet/plugins/cert-manager-csi-driver-spiffe"
	// csiDriverSPIFFEChartName is the upstream chart name, which its default app.kubernetes.io/name label carries.
	csiDriverSPIFFEChartName = "cert-manager-csi-driver-spiffe"

	// csiDriverSPIFFEPodWaitTimeoutDefault bounds how long uninstall waits for Pods that mount a csi-driver-spiffe
	// volume to go away before removing the driver.
	csiDriverSPIFFEPodWaitTimeoutDefault  = 3 * time.Minute
	csiDriverSPIFFEPodPollIntervalDefault = 2 * time.Second

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

// defaultApprovedRadiusIssuers returns the Radius issuers whose requests cert-manager's default approver approves.
func defaultApprovedRadiusIssuers() []string {
	return []string{RadiusBootstrapIssuerName, RadiusAPIServerIssuerName}
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

// csiDriverSPIFFEPlan is the checked state that applyCSIDriverSPIFFE installs or upgrades from.
type csiDriverSPIFFEPlan struct {
	chartOptions ChartOptions
	values       map[string]any
	release      *releasev1.Release
	kubeClient   kubernetes.Interface
	namespace    string
}

// planCSIDriverSPIFFE checks, without changing the cluster, that Radius's csi-driver-spiffe instance can be
// installed or upgraded. It fails when the release exists but rad did not install it, or when another
// csi-driver-spiffe runs on the cluster, because only one can register on each node. Install and upgrade call it
// before any dependency changes the cluster. It also fails when the cluster does not serve ValidatingAdmissionPolicy,
// which the Radius chart's certificate issuance policy needs. It returns nil, without contacting the cluster, when
// authorization is off.
func (i *Impl) planCSIDriverSPIFFE(ctx context.Context, options ClusterOptions, kubeContext string, storedValues map[string]any) (*csiDriverSPIFFEPlan, error) {
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
	settings := resolveSPIFFESettings(storedValues, currentValues)

	rel, owned, err := i.getReleaseOwnership(chartOptions, kubeContext)
	if err != nil {
		return nil, err
	}
	if rel != nil && !owned {
		return nil, clierrors.Message("Radius stopped before changing the cluster because Helm release %q in namespace %q was not installed by rad. Radius installs and manages its own csi-driver-spiffe. Remove that release, for example with %s, and then run the command again.",
			chartOptions.ReleaseName, chartOptions.Namespace, helmCommand(kubeContext, chartOptions.Namespace, "uninstall "+chartOptions.ReleaseName))
	}

	kubeClient, err := i.csiDriverSPIFFEClient(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}
	if err := checkForeignCSIDriverSPIFFE(ctx, kubeClient, chartOptions, settings.CSIDriverName, kubeContext); err != nil {
		return nil, err
	}
	if err := checkValidatingAdmissionPolicyServed(kubeClient); err != nil {
		return nil, err
	}

	return &csiDriverSPIFFEPlan{
		chartOptions: chartOptions,
		values:       buildCSIDriverSPIFFEValues(options.Radius.Namespace, settings),
		release:      rel,
		kubeClient:   kubeClient,
		namespace:    options.Radius.Namespace,
	}, nil
}

// applyCSIDriverSPIFFE installs or upgrades Radius's csi-driver-spiffe instance from a plan made by
// planCSIDriverSPIFFE. It runs after cert-manager is ready. A nil plan does nothing.
//
// It returns the users the cert-manager controllers run as, which the Radius chart's certificate issuance policy
// allows to request certificates from the Radius bootstrap and API server issuers. It returns none for a nil plan
// or when it finds no cert-manager controller.
func (i *Impl) applyCSIDriverSPIFFE(ctx context.Context, plan *csiDriverSPIFFEPlan, kubeContext string, upgrade bool) ([]string, error) {
	if plan == nil {
		return nil, nil
	}
	chartOptions := plan.chartOptions

	// Only Radius's csi-driver-spiffe approver may approve requests for the Radius CA issuer. A cert-manager default
	// approver that can approve them would sign any SPIFFE ID. At this point rad has changed at most cert-manager, so
	// it stops before it installs or upgrades csi-driver-spiffe and Radius.
	var conflict *defaultApproverConflictError
	if err := checkCertManagerDefaultApprover(ctx, plan.kubeClient, plan.namespace, kubeContext); errors.As(err, &conflict) {
		return nil, clierrors.Message("Radius stopped before it installed or upgraded csi-driver-spiffe and Radius because %s", conflict.Error())
	} else if err != nil {
		return nil, err
	}
	requesters, err := certManagerControllerUsers(ctx, plan.kubeClient)
	if err != nil {
		return nil, err
	}
	if missing, err := missingApprovableRadiusSigners(ctx, plan.kubeClient, plan.namespace); err != nil {
		i.logInfo("Warning: %v", err)
	} else if len(missing) > 0 {
		i.logInfo("Warning: cert-manager's default approver cannot approve CertificateRequests for %s. The Radius CAs and UCP's API server certificate are issued only if another approver approves them.",
			strings.Join(missing, ", "))
	}

	helmAction := NewHelmAction(i.Helm)
	if rel := plan.release; rel != nil {
		deployed := rel.Info != nil && rel.Info.Status == releasecommon.StatusDeployed
		if !upgrade && deployed && reflect.DeepEqual(rel.Config, plan.values) {
			output.LogInfo("Using csi-driver-spiffe installed by rad in namespace %s", chartOptions.Namespace)
			return requesters, nil
		}
		output.LogInfo("Upgrading csi-driver-spiffe...")
		helmChart, helmConf, err := loadChart(ctx, helmAction, chartOptions, kubeContext)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare csi-driver-spiffe Helm chart, err: %w", err)
		}
		if _, err := i.Helm.RunHelmUpgrade(helmConf, helmChart, plan.values, chartOptions.ReleaseName, chartOptions.Namespace, chartOptions.Wait, false, chartOptions.Timeout); err != nil {
			return nil, fmt.Errorf("failed to upgrade csi-driver-spiffe, err: %w", err)
		}
		return requesters, nil
	}

	output.LogInfo("Installing csi-driver-spiffe %s...", chartOptions.ChartVersion)
	helmChart, helmConf, err := loadChart(ctx, helmAction, chartOptions, kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare csi-driver-spiffe Helm chart, err: %w", err)
	}
	if _, err := i.Helm.RunHelmInstall(helmConf, helmChart, plan.values, chartOptions.ReleaseName, chartOptions.Namespace, chartOptions.Wait, chartOptions.Timeout, radManagedByLabels()); err != nil {
		return nil, fmt.Errorf("failed to install csi-driver-spiffe, err: %w", err)
	}
	return requesters, nil
}

// csiDriverSPIFFEClient returns the Kubernetes client for the csi-driver-spiffe checks.
func (i *Impl) csiDriverSPIFFEClient(kubeContext string) (kubernetes.Interface, error) {
	if i.newCSIDriverSPIFFEClient != nil {
		return i.newCSIDriverSPIFFEClient(kubeContext)
	}
	kubeClient, _, err := newCertManagerClients(kubeContext)
	return kubeClient, err
}

// checkForeignCSIDriverSPIFFE fails when a csi-driver-spiffe that is not Radius's release runs on the cluster, or when
// another installation owns the CSIDriver name Radius's release would register. It recognizes a csi-driver-spiffe
// DaemonSet by the fixed kubelet plugin directory it mounts or registers, so renamed installations (nameOverride or
// custom labels) are found too.
func checkForeignCSIDriverSPIFFE(ctx context.Context, kubeClient kubernetes.Interface, options ChartOptions, csiDriverName string, kubeContext string) error {
	daemonSets, err := kubeClient.AppsV1().DaemonSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list DaemonSets to find csi-driver-spiffe installations: %w", err)
	}
	for idx := range daemonSets.Items {
		ds := &daemonSets.Items[idx]
		if !isCSIDriverSPIFFEDaemonSet(ds) || isOwnedByRelease(ds.ObjectMeta, options) {
			continue
		}
		return clierrors.Message("Radius stopped before changing the cluster because another csi-driver-spiffe (DaemonSet %s/%s) runs on it. Radius installs its own csi-driver-spiffe, limited to the Radius issuer, and a cluster can run only one because each registers the same kubelet plugin path (%s). Remove the other installation, for example with %s, and then run the command again.",
			ds.Namespace, ds.Name, csiDriverSPIFFEPluginDir, helmCommand(kubeContext, ds.Namespace, "uninstall "+releaseNameOf(ds.ObjectMeta)))
	}

	csiDriver, err := kubeClient.StorageV1().CSIDrivers().Get(ctx, csiDriverName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check CSIDriver %s: %w", csiDriverName, err)
	}
	if isOwnedByRelease(csiDriver.ObjectMeta, options) {
		return nil
	}
	return clierrors.Message("Radius stopped before changing the cluster because CSIDriver %q already exists and Radius's csi-driver-spiffe release did not create it. Remove it, for example with %s, or set global.rbac.spiffe.csiDriverName to another name, and then run the command again.",
		csiDriverName, kubectlCommand(kubeContext, "", "delete csidriver "+csiDriverName))
}

// isCSIDriverSPIFFEDaemonSet reports whether ds runs a csi-driver-spiffe node plugin: it mounts or registers the
// fixed csi-driver-spiffe kubelet plugin directory, or carries the upstream chart's name label.
func isCSIDriverSPIFFEDaemonSet(ds *appsv1.DaemonSet) bool {
	if ds.Labels["app.kubernetes.io/name"] == csiDriverSPIFFEChartName {
		return true
	}
	spec := ds.Spec.Template.Spec
	for _, volume := range spec.Volumes {
		if volume.HostPath != nil && path.Clean(volume.HostPath.Path) == csiDriverSPIFFEPluginDir {
			return true
		}
	}
	for _, containers := range [][]corev1.Container{spec.InitContainers, spec.Containers} {
		for _, c := range containers {
			for idx, arg := range c.Args {
				value, ok := strings.CutPrefix(arg, "--kubelet-registration-path=")
				if !ok && arg == "--kubelet-registration-path" && idx+1 < len(c.Args) {
					value, ok = c.Args[idx+1], true
				}
				if ok && path.Dir(path.Clean(value)) == csiDriverSPIFFEPluginDir {
					return true
				}
			}
		}
	}
	return false
}

// isOwnedByRelease reports whether Helm release options created the object.
func isOwnedByRelease(meta metav1.ObjectMeta, options ChartOptions) bool {
	if meta.Annotations["meta.helm.sh/release-name"] == options.ReleaseName && meta.Annotations["meta.helm.sh/release-namespace"] == options.Namespace {
		return true
	}
	return meta.Namespace == options.Namespace && meta.Labels["app.kubernetes.io/instance"] == options.ReleaseName
}

func releaseNameOf(meta metav1.ObjectMeta) string {
	if name := meta.Annotations["meta.helm.sh/release-name"]; name != "" {
		return name
	}
	if name := meta.Labels["app.kubernetes.io/instance"]; name != "" {
		return name
	}
	return "<release>"
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
// including a request for another service's SPIFFE ID.
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
	for _, issuer := range defaultApprovedRadiusIssuers() {
		signerName := issuerSignerName(radiusNamespace, issuer)
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

// CSIDriverSPIFFEKeptError reports that an uninstall kept the csi-driver-spiffe that rad installed, and the
// cert-manager it needs, because Pods might still mount certificate volumes from the driver. The Helm releases that
// came before it were removed. Callers must not delete the driver's namespace until those Pods are gone, because the
// kubelet needs the driver to unmount their volumes.
type CSIDriverSPIFFEKeptError struct {
	// Namespace is the namespace the driver runs in.
	Namespace string
	// DriverName is the CSIDriver name, when it is known.
	DriverName string
	// Pods are the namespace/name of Pods that still mount volumes from the driver. Empty when Reason is set.
	Pods []string
	// Reason says why rad could not check for such Pods, when it could not.
	Reason string
}

func (e *CSIDriverSPIFFEKeptError) Error() string {
	if len(e.Pods) > 0 {
		return fmt.Sprintf("kept csi-driver-spiffe in namespace %s and cert-manager because %d Pods still mount volumes from CSI driver %s (e.g. %s)",
			e.Namespace, len(e.Pods), e.DriverName, e.Pods[0])
	}
	return fmt.Sprintf("kept csi-driver-spiffe in namespace %s and cert-manager because rad could not check for Pods that use it: %s", e.Namespace, e.Reason)
}

// uninstallCSIDriverSPIFFE removes csi-driver-spiffe when rad installed it, and returns nil when it removed it or
// rad did not install it.
//
// Helm deletes the Radius Deployments without waiting for their Pods, and the kubelet needs the driver to unmount a
// terminating Pod's certificate volume. So it first waits for every Pod that mounts a volume from the driver to go
// away. When some remain, or it cannot check ownership or Pods, it keeps the driver and returns a
// *CSIDriverSPIFFEKeptError, so that the caller keeps cert-manager, which the driver needs, and does not delete the
// driver's namespace.
func (i *Impl) uninstallCSIDriverSPIFFE(ctx context.Context, options ClusterOptions, kubeContext string) error {
	chartOptions := options.CSIDriverSPIFFE.ChartOptions
	kept := &CSIDriverSPIFFEKeptError{Namespace: chartOptions.Namespace}
	rel, owned, err := i.getReleaseOwnership(chartOptions, kubeContext)
	if err != nil {
		kept.Reason = fmt.Sprintf("could not check whether rad installed it: %v", err)
		i.logInfo("Keeping csi-driver-spiffe and cert-manager: %s", kept.Reason)
		return kept
	}
	if !owned {
		return nil
	}

	kept.DriverName = csiDriverNameOf(rel)
	kubeClient, err := i.csiDriverSPIFFEClient(kubeContext)
	if err == nil {
		kept.Pods, err = i.waitForCSIDriverSPIFFEPods(ctx, kubeClient, kept.DriverName)
	}
	if err != nil {
		kept.Reason = fmt.Sprintf("could not check for Pods that use csi-driver-spiffe: %v", err)
		i.logInfo("Keeping csi-driver-spiffe and cert-manager: %s", kept.Reason)
		return kept
	}
	if len(kept.Pods) > 0 {
		i.logInfo("Keeping csi-driver-spiffe and cert-manager: %d Pods still mount volumes from CSI driver %s (e.g. %s). Rerun the uninstall after they are gone.",
			len(kept.Pods), kept.DriverName, kept.Pods[0])
		return kept
	}
	return i.uninstallHelmRelease("csi-driver-spiffe", chartOptions.ReleaseName, chartOptions.Namespace, kubeContext)
}

// csiDriverNameOf returns the CSIDriver name that rad configured on a csi-driver-spiffe release.
func csiDriverNameOf(rel *releasev1.Release) string {
	if app, ok := rel.Config["app"].(map[string]any); ok {
		if name, ok := app["name"].(string); ok && name != "" {
			return name
		}
	}
	return SPIFFECSIDriverNameDefault
}

// waitForCSIDriverSPIFFEPods waits until no Pod in the cluster mounts a volume from CSI driver driverName, and returns
// the namespace/name of the Pods that still do when the wait times out.
func (i *Impl) waitForCSIDriverSPIFFEPods(ctx context.Context, kubeClient kubernetes.Interface, driverName string) ([]string, error) {
	timeout := i.csiDriverSPIFFEPodWaitTimeout
	if timeout == 0 {
		timeout = csiDriverSPIFFEPodWaitTimeoutDefault
	}
	interval := i.csiDriverSPIFFEPodPollInterval
	if interval == 0 {
		interval = csiDriverSPIFFEPodPollIntervalDefault
	}

	var remaining []string
	logged := false
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		pods, err := kubeClient.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		remaining = remaining[:0]
		for idx := range pods.Items {
			if podUsesCSIDriver(&pods.Items[idx], driverName) {
				remaining = append(remaining, pods.Items[idx].Namespace+"/"+pods.Items[idx].Name)
			}
		}
		if len(remaining) > 0 && !logged {
			output.LogInfo("Waiting for %d Pods that use csi-driver-spiffe to terminate...", len(remaining))
			logged = true
		}
		return len(remaining) == 0, nil
	})
	if err != nil && len(remaining) > 0 && wait.Interrupted(err) {
		return remaining, nil
	}
	return remaining, err
}

func podUsesCSIDriver(pod *corev1.Pod, driverName string) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.CSI != nil && volume.CSI.Driver == driverName {
			return true
		}
	}
	return false
}
