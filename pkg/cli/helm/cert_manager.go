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
	"strings"
	"text/template"

	"github.com/Masterminds/semver/v3"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/kubeutil"
)

// cert-manager versions. Bump both as upstream releases move: the default is the latest stable release and
// the minimum is the oldest release still supported upstream. See https://cert-manager.io/docs/releases/.
const (
	certManagerHelmRepo            = "https://charts.jetstack.io"
	certManagerReleaseName         = "cert-manager"
	CertManagerNamespace           = "cert-manager"
	CertManagerChartDefaultVersion = "v1.21.2"
	CertManagerMinimumVersion      = "1.20.0"

	certManagerCRDName            = "certificates.cert-manager.io"
	certManagerControllerSelector = "app.kubernetes.io/name=cert-manager,app.kubernetes.io/component=controller"
	certManagerVersionLabel       = "app.kubernetes.io/version"
)

// radManagedByLabels are the Helm release labels that mark a cert-manager release as installed by rad.
func radManagedByLabels() map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "rad"}
}

type CertManagerChartOptions struct {
	ChartOptions
}

// certManagerInstallation describes the cert-manager found on a cluster.
type certManagerInstallation struct {
	Installed bool
	Version   string
	Namespace string
}

type certManagerClientsFunc func(kubeContext string) (kubernetes.Interface, apiextclient.Interface, error)

func newCertManagerClients(kubeContext string) (kubernetes.Interface, apiextclient.Interface, error) {
	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: kubeContext,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
	if err != nil {
		return nil, nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	apiextClient, err := apiextclient.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return kubeClient, apiextClient, nil
}

// resolveAuthzMode returns the authorization mode the Radius chart will render from global.rbac. Values from the
// current invocation are merged over the stored release values with Helm's own CoalesceTables, the merge Helm
// upgrade applies for ResetThenReuseValues, so null or non-map replacements clear stored flags like Helm does.
func resolveAuthzMode(stored, current map[string]any) (authz.Mode, error) {
	// CoalesceTables mutates both arguments, so merge copies.
	merged := valuesutil.CoalesceTables(copyValues(current), copyValues(stored))
	enabled, dryRun := rbacFlag(merged, "enabled"), rbacFlag(merged, "dryRun")

	switch {
	case enabled && dryRun:
		return "", fmt.Errorf("global.rbac.dryRun=true cannot be combined with global.rbac.enabled=true: dry run is a preflight check for installations that are not yet enforcing. Set global.rbac.enabled=false to run the dry run")
	case dryRun:
		return authz.ModeDryRun, nil
	case enabled:
		return authz.ModeEnforce, nil
	default:
		return authz.ModeOff, nil
	}
}

func rbacFlag(values map[string]any, key string) bool {
	global, _ := values["global"].(map[string]any)
	rbac, _ := global["rbac"].(map[string]any)
	return isTrue(rbac[key])
}

// copyValues deep-copies the nested maps of a values tree; leaf values are shared.
func copyValues(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for k, v := range values {
		if m, ok := v.(map[string]any); ok {
			v = copyValues(m)
		}
		out[k] = v
	}
	return out
}

// isTrue applies Go template truthiness, matching how the chart's radius.authz.mode helper reads the flags.
func isTrue(v any) bool {
	truth, _ := template.IsTrue(v)
	return truth
}

// detectCertManager finds cert-manager by its Certificate CRD and controller Deployment in any namespace.
func detectCertManager(ctx context.Context, kubeClient kubernetes.Interface, apiextClient apiextclient.Interface) (certManagerInstallation, error) {
	crdFound := true
	_, err := apiextClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, certManagerCRDName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		crdFound = false
	} else if err != nil {
		return certManagerInstallation{}, fmt.Errorf("failed to check for the %s CRD: %w", certManagerCRDName, err)
	}

	deployments, err := kubeClient.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: certManagerControllerSelector})
	if err != nil {
		return certManagerInstallation{}, fmt.Errorf("failed to list cert-manager deployments: %w", err)
	}

	switch {
	case !crdFound && len(deployments.Items) == 0:
		return certManagerInstallation{}, nil
	case len(deployments.Items) == 0:
		return certManagerInstallation{}, fmt.Errorf("the %s CRD exists but no cert-manager controller is running. Repair or remove the existing cert-manager installation and retry", certManagerCRDName)
	case !crdFound:
		return certManagerInstallation{}, fmt.Errorf("a cert-manager controller is running in namespace %q but the %s CRD is missing. Repair the existing cert-manager installation and retry", deployments.Items[0].Namespace, certManagerCRDName)
	}

	controller := deployments.Items[0]
	raw := controller.Labels[certManagerVersionLabel]
	if raw == "" {
		for _, c := range controller.Spec.Template.Spec.Containers {
			if tag := imageTag(c.Image); tag != "" {
				raw = tag
				break
			}
		}
	}

	version, err := semver.NewVersion(raw)
	if err != nil {
		return certManagerInstallation{}, fmt.Errorf("could not determine the cert-manager version in namespace %q from label %s or the controller image", controller.Namespace, certManagerVersionLabel)
	}

	return certManagerInstallation{Installed: true, Version: version.String(), Namespace: controller.Namespace}, nil
}

// imageTag returns the tag of an image reference, ignoring any digest.
func imageTag(image string) string {
	image, _, _ = strings.Cut(image, "@")
	lastSlash := strings.LastIndex(image, "/")
	if i := strings.LastIndex(image, ":"); i > lastSlash {
		return image[i+1:]
	}
	return ""
}

// ensureCertManager makes cert-manager available when Radius authorization is enabled. It installs a pinned
// version when none exists, upgrades a release rad installed when upgrade is true or when that release is
// not deployed or its controller is missing, reuses a supported
// installation it did not install, and fails when that installation is older than CertManagerMinimumVersion.
// With authorization off it returns without contacting the cluster.
func (i *Impl) ensureCertManager(ctx context.Context, options ClusterOptions, kubeContext string, storedValues map[string]any, upgrade bool) error {
	currentValues, err := parseUserValuesFromCLI(&options.Radius)
	if err != nil {
		return fmt.Errorf("failed to parse Radius values: %w", err)
	}
	mode, err := resolveAuthzMode(storedValues, currentValues)
	if err != nil {
		return err
	}
	if mode == authz.ModeOff {
		return nil
	}

	helmAction := NewHelmAction(i.Helm)
	chartOptions := options.CertManager.ChartOptions
	ownedRelease, err := i.radOwnedCertManagerRelease(chartOptions, kubeContext)
	if err != nil {
		return err
	}

	newClients := i.newCertManagerClients
	if newClients == nil {
		newClients = newCertManagerClients
	}
	detect := func() (certManagerInstallation, error) {
		kubeClient, apiextClient, err := newClients(kubeContext)
		if err != nil {
			return certManagerInstallation{}, fmt.Errorf("failed to create Kubernetes clients: %w", err)
		}
		return detectCertManager(ctx, kubeClient, apiextClient)
	}

	if ownedRelease != nil {
		if !upgrade {
			healthy := ownedRelease.Info != nil && ownedRelease.Info.Status == releasecommon.StatusDeployed
			if healthy {
				existing, err := detect()
				healthy = err == nil && existing.Installed
			}
			if healthy {
				output.LogInfo("Using cert-manager installed by rad in namespace %s", chartOptions.Namespace)
				return nil
			}
			output.LogInfo("Repairing cert-manager installed by rad...")
		} else {
			output.LogInfo("Upgrading cert-manager...")
		}
		helmChart, helmConf, err := loadChart(ctx, helmAction, chartOptions, kubeContext)
		if err != nil {
			return fmt.Errorf("failed to prepare cert-manager Helm chart, err: %w", err)
		}
		if _, err := i.Helm.RunHelmUpgrade(helmConf, helmChart, buildCertManagerValues(), chartOptions.ReleaseName, chartOptions.Namespace, chartOptions.Wait, true, chartOptions.Timeout); err != nil {
			return fmt.Errorf("failed to upgrade cert-manager, err: %w", err)
		}
		return nil
	}

	existing, err := detect()
	if err != nil {
		return err
	}

	if existing.Installed {
		minimum := semver.MustParse(CertManagerMinimumVersion)
		if semver.MustParse(existing.Version).LessThan(minimum) {
			return fmt.Errorf("cert-manager v%s in namespace %q is older than the minimum supported version v%s. Upgrade cert-manager to v%s or later and retry; rad does not upgrade a cert-manager it did not install", existing.Version, existing.Namespace, minimum, minimum)
		}
		output.LogInfo("Using existing cert-manager v%s in namespace %s", existing.Version, existing.Namespace)
		return nil
	}

	output.LogInfo("Installing cert-manager %s...", chartOptions.ChartVersion)
	helmChart, helmConf, err := loadChart(ctx, helmAction, chartOptions, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to prepare cert-manager Helm chart, err: %w", err)
	}
	if _, err := i.Helm.RunHelmInstall(helmConf, helmChart, buildCertManagerValues(), chartOptions.ReleaseName, chartOptions.Namespace, chartOptions.Wait, chartOptions.Timeout, radManagedByLabels()); err != nil {
		return fmt.Errorf("failed to install cert-manager, err: %w", err)
	}
	return nil
}

// radOwnedCertManagerRelease returns the latest cert-manager Helm release when it carries rad's ownership
// labels, or nil when the release does not exist or rad did not install it.
func (i *Impl) radOwnedCertManagerRelease(options ChartOptions, kubeContext string) (*releasev1.Release, error) {
	rel, err := NewHelmAction(i.Helm).getRelease(kubeContext, options.ReleaseName, options.Namespace)
	if err != nil || rel == nil {
		return nil, err
	}
	for k, v := range radManagedByLabels() {
		if rel.Labels[k] != v {
			return nil, nil
		}
	}
	return rel, nil
}

// isRadOwnedCertManager reports whether the cert-manager Helm release exists and carries rad's ownership labels.
func (i *Impl) isRadOwnedCertManager(options ChartOptions, kubeContext string) (bool, error) {
	rel, err := i.radOwnedCertManagerRelease(options, kubeContext)
	return rel != nil, err
}

// CertManagerInstalledByRad reports whether the default cert-manager release exists and rad installed it.
func (i *Impl) CertManagerInstalledByRad(kubeContext string) (bool, error) {
	return i.isRadOwnedCertManager(NewDefaultClusterOptions().CertManager.ChartOptions, kubeContext)
}

// certManagerResources are the cert-manager resources that other workloads may depend on. Uninstalling
// cert-manager deletes its CRDs, which deletes every one of these resources in the cluster.
var certManagerResources = []schema.GroupVersionResource{
	{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	{Group: "cert-manager.io", Version: "v1", Resource: "certificaterequests"},
	{Group: "cert-manager.io", Version: "v1", Resource: "issuers"},
	{Group: "cert-manager.io", Version: "v1", Resource: "clusterissuers"},
}

func newCertManagerDynamicClient(kubeContext string) (dynamic.Interface, error) {
	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: kubeContext,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(config)
}

// foreignCertManagerResources lists cert-manager resources outside radiusNamespace. ClusterIssuers always count.
func foreignCertManagerResources(ctx context.Context, client dynamic.Interface, radiusNamespace string) ([]string, error) {
	var found []string
	for _, gvr := range certManagerResources {
		list, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list %s: %w", gvr.Resource, err)
		}
		for _, item := range list.Items {
			switch ns := item.GetNamespace(); ns {
			case radiusNamespace:
			case "":
				found = append(found, item.GetKind()+"/"+item.GetName())
			default:
				found = append(found, ns+"/"+item.GetKind()+"/"+item.GetName())
			}
		}
	}
	return found, nil
}

// uninstallCertManager removes cert-manager only when rad installed it and no other workload uses it. It keeps
// cert-manager, without failing, when other workloads have cert-manager resources or the check itself fails.
func (i *Impl) uninstallCertManager(ctx context.Context, options ClusterOptions, kubeContext string) error {
	chartOptions := options.CertManager.ChartOptions
	owned, err := i.isRadOwnedCertManager(chartOptions, kubeContext)
	if err != nil || !owned {
		return err
	}

	logInfo := i.logInfo
	if logInfo == nil {
		logInfo = output.LogInfo
	}
	newClient := i.newCertManagerDynamicClient
	if newClient == nil {
		newClient = newCertManagerDynamicClient
	}
	client, err := newClient(kubeContext)
	var foreign []string
	if err == nil {
		foreign, err = foreignCertManagerResources(ctx, client, options.Radius.Namespace)
	}
	if err != nil {
		logInfo("Keeping cert-manager: could not check whether other workloads use it: %v", err)
		return nil
	}
	if len(foreign) > 0 {
		logInfo("Keeping cert-manager: other workloads use it (%d resources, e.g. %s)", len(foreign), foreign[0])
		return nil
	}
	return i.uninstallHelmRelease("cert-manager", chartOptions.ReleaseName, chartOptions.Namespace, kubeContext)
}

// UninstallCertManager removes cert-manager on its own, with the same ownership and other-workload checks
// that UninstallRadius applies. It does not touch the Radius or Contour releases.
func (i *Impl) UninstallCertManager(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error {
	return i.uninstallCertManager(ctx, clusterOptions, kubeContext)
}

// buildCertManagerValues installs the CRDs with the release and removes them on uninstall, so a later
// install does not find CRDs without a controller.
func buildCertManagerValues() map[string]any {
	return map[string]any{"crds": map[string]any{"enabled": true, "keep": false}}
}
