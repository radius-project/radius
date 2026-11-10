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
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	appsv1 "k8s.io/api/apps/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/authz"
	clikubernetes "github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/output"
)

var errCertManagerControllerMissing = errors.New("no cert-manager controller is running")

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
	config, err := clikubernetes.NewCLIClientConfig(kubeContext)
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
		return certManagerInstallation{}, fmt.Errorf("the %s CRD exists but %w. Repair or remove the existing cert-manager installation and retry", certManagerCRDName, errCertManagerControllerMissing)
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

// certManagerComponents maps each cert-manager component Radius needs to its app.kubernetes.io/name label. The
// cainjector writes the CA of UCP's API server certificate into the Radius APIService, so the Kubernetes API
// server can reach UCP.
var certManagerComponents = []struct{ component, name string }{
	{component: "controller", name: "cert-manager"},
	{component: "webhook", name: "webhook"},
	{component: "cainjector", name: "cainjector"},
}

// checkCertManagerReady fails unless the cert-manager controller, webhook and cainjector in namespace are rolled out
// and available, and the cainjector can inject the CA of a Certificate in radiusNamespace into an APIService.
func checkCertManagerReady(ctx context.Context, client kubernetes.Interface, namespace, radiusNamespace string) error {
	for _, c := range certManagerComponents {
		selector := "app.kubernetes.io/name=" + c.name + ",app.kubernetes.io/component=" + c.component
		deployments, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return fmt.Errorf("failed to check cert-manager %s readiness in namespace %q: %w", c.component, namespace, err)
		}
		if len(deployments.Items) != 1 {
			return fmt.Errorf("cert-manager %s in namespace %q is not ready: expected one Deployment, found %d. Repair cert-manager and retry", c.component, namespace, len(deployments.Items))
		}
		deployment := deployments.Items[0]
		replicas := int32(1)
		if deployment.Spec.Replicas != nil {
			replicas = *deployment.Spec.Replicas
		}
		if replicas == 0 || deployment.Status.ObservedGeneration < deployment.Generation ||
			deployment.Status.UpdatedReplicas < replicas || deployment.Status.AvailableReplicas < replicas {
			return fmt.Errorf("cert-manager %s Deployment %s/%s is not ready. Wait for its rollout or repair cert-manager and retry", c.component, namespace, deployment.Name)
		}
		if c.component == "cainjector" {
			if problem := caInjectorProblem(&deployment, radiusNamespace); problem != "" {
				return fmt.Errorf("cert-manager cainjector Deployment %s/%s %s. Radius needs it to inject the CA of UCP's API server certificate into the Radius APIService. Remove that setting from cert-manager and retry", namespace, deployment.Name, problem)
			}
		}
	}
	return nil
}

// caInjectorProblem describes a cainjector argument that stops it from injecting the CA of a Certificate in
// radiusNamespace into an APIService, or returns "" when there is none.
func caInjectorProblem(deployment *appsv1.Deployment, radiusNamespace string) string {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		for idx, arg := range container.Args {
			if !strings.HasPrefix(arg, "-") {
				continue
			}
			name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			switch name {
			case "enable-apiservices-injectable", "enable-certificates-data-source":
				// Boolean flags take their value only after "=".
				if enabled, err := strconv.ParseBool(value); hasValue && err == nil && !enabled {
					return "sets " + arg
				}
			case "namespace":
				if !hasValue && idx+1 < len(container.Args) {
					value = container.Args[idx+1]
				}
				if value != "" && value != radiusNamespace {
					return "is limited to namespace " + strconv.Quote(value) + " by --namespace"
				}
			}
		}
	}
	return ""
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
// version when none exists, upgrades a release rad installed when upgrade is true, when that release is
// not deployed or its controller is missing, or when it lacks the values rad sets (an older rad installed it
// without approveSignerNames), reuses a supported
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
	values := buildCertManagerValues(options.Radius.Namespace)
	ownedRelease, err := i.radOwnedCertManagerRelease(chartOptions, kubeContext)
	if err != nil {
		return err
	}

	newClients := i.newCertManagerClients
	if newClients == nil {
		newClients = newCertManagerClients
	}
	var kubeClient kubernetes.Interface
	detect := func() (certManagerInstallation, error) {
		var apiextClient apiextclient.Interface
		var err error
		kubeClient, apiextClient, err = newClients(kubeContext)
		if err != nil {
			return certManagerInstallation{}, fmt.Errorf("failed to create Kubernetes clients: %w", err)
		}
		return detectCertManager(ctx, kubeClient, apiextClient)
	}

	if ownedRelease != nil {
		if ownedRelease.Chart == nil || ownedRelease.Chart.Metadata == nil {
			return fmt.Errorf("could not determine the rad-owned cert-manager chart version; repair its Helm release metadata and retry")
		}
		installedVersion, err := semver.NewVersion(ownedRelease.Chart.Metadata.Version)
		if err != nil {
			return fmt.Errorf("invalid rad-owned cert-manager chart version: %w", err)
		}
		targetVersion, err := semver.NewVersion(chartOptions.ChartVersion)
		if err != nil {
			return fmt.Errorf("invalid cert-manager target chart version: %w", err)
		}
		newer := installedVersion.GreaterThan(targetVersion)
		if !upgrade || newer {
			healthy := ownedRelease.Info != nil && ownedRelease.Info.Status == releasecommon.StatusDeployed
			if healthy {
				existing, err := detect()
				if err != nil && !errors.Is(err, errCertManagerControllerMissing) {
					return err
				}
				healthy = err == nil && existing.Installed
				if healthy {
					if err := checkCertManagerReady(ctx, kubeClient, existing.Namespace, options.Radius.Namespace); err != nil {
						return err
					}
				}
			}
			configured := releaseHasValues(ownedRelease.Config, values)
			if healthy && (configured || newer) {
				output.LogInfo("Using cert-manager installed by rad in namespace %s", chartOptions.Namespace)
				return nil
			}
			if newer {
				return fmt.Errorf("rad-owned cert-manager v%s is newer than target v%s but is not healthy. Repair it at its current version and retry; rad will not downgrade it", installedVersion, targetVersion)
			}
			if healthy {
				output.LogInfo("Configuring cert-manager installed by rad...")
			} else {
				output.LogInfo("Repairing cert-manager installed by rad...")
			}
		} else {
			output.LogInfo("Upgrading cert-manager...")
		}
		helmChart, helmConf, err := loadChart(ctx, helmAction, chartOptions, kubeContext)
		if err != nil {
			return fmt.Errorf("failed to prepare cert-manager Helm chart, err: %w", err)
		}
		if _, err := i.Helm.RunHelmUpgrade(helmConf, helmChart, values, chartOptions.ReleaseName, chartOptions.Namespace, chartOptions.Wait, true, chartOptions.Timeout); err != nil {
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
		if err := checkCertManagerReady(ctx, kubeClient, existing.Namespace, options.Radius.Namespace); err != nil {
			return err
		}
		output.LogInfo("Using existing cert-manager v%s in namespace %s", existing.Version, existing.Namespace)
		return nil
	}

	output.LogInfo("Installing cert-manager %s...", chartOptions.ChartVersion)
	helmChart, helmConf, err := loadChart(ctx, helmAction, chartOptions, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to prepare cert-manager Helm chart, err: %w", err)
	}
	if _, err := i.Helm.RunHelmInstall(helmConf, helmChart, values, chartOptions.ReleaseName, chartOptions.Namespace, chartOptions.Wait, chartOptions.Timeout, radManagedByLabels()); err != nil {
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
	{Group: "acme.cert-manager.io", Version: "v1", Resource: "orders"},
	{Group: "acme.cert-manager.io", Version: "v1", Resource: "challenges"},
}

func newCertManagerDynamicClient(kubeContext string) (dynamic.Interface, error) {
	config, err := clikubernetes.NewCLIClientConfig(kubeContext)
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
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			continue
		}
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
	if err != nil {
		i.logInfo("Keeping cert-manager: could not check whether rad installed it: %v", err)
		return nil
	}
	if !owned {
		return nil
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
		i.logInfo("Keeping cert-manager: could not check whether other workloads use it: %v", err)
		return nil
	}
	if len(foreign) > 0 {
		i.logInfo("Keeping cert-manager: other workloads use it (%d resources, e.g. %s)", len(foreign), foreign[0])
		return nil
	}
	return i.uninstallHelmRelease("cert-manager", chartOptions.ReleaseName, chartOptions.Namespace, kubeContext)
}

// UninstallCertManager removes csi-driver-spiffe and cert-manager on their own, with the same ownership and
// other-workload checks that UninstallRadius applies. It does not touch the Radius or Contour releases.
func (i *Impl) UninstallCertManager(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error {
	if err := i.uninstallCSIDriverSPIFFE(clusterOptions, kubeContext); err != nil {
		return err
	}
	return i.uninstallCertManager(ctx, clusterOptions, kubeContext)
}

// releaseHasValues reports whether every value in want is set to the same value in a release's stored config.
func releaseHasValues(config map[string]any, want map[string]any) bool {
	for key, value := range want {
		if !reflect.DeepEqual(config[key], value) {
			return false
		}
	}
	return true
}

// buildCertManagerValues installs the CRDs with the release and removes them on uninstall, so a later
// install does not find CRDs without a controller. It also limits cert-manager's default approver to the signers
// in certManagerApproveSignerNames, so it never approves requests for the Radius CA issuer.
func buildCertManagerValues(radiusNamespace string) map[string]any {
	// []any matches the release config Helm stores, so releaseHasValues can compare them.
	var approveSignerNames []any
	for _, name := range certManagerApproveSignerNames(radiusNamespace) {
		approveSignerNames = append(approveSignerNames, name)
	}
	return map[string]any{
		"crds":               map[string]any{"enabled": true, "keep": false},
		"approveSignerNames": approveSignerNames,
	}
}

// certManagerApproveSignerNames returns the signers cert-manager's default approver may approve when rad installs
// cert-manager: the Radius bootstrap and API server issuers, and every ClusterIssuer. The chart turns the list into
// the resourceNames of the approver's ClusterRole; cert-manager has no --approve-signers flag. The Radius CA issuer
// is left out so that only Radius's csi-driver-spiffe approver approves its requests. Namespaced Issuers in other
// namespaces are left out too, because a wildcard would include the Radius CA issuer.
func certManagerApproveSignerNames(radiusNamespace string) []string {
	return []string{
		issuerSignerName(radiusNamespace, RadiusBootstrapIssuerName),
		issuerSignerName(radiusNamespace, RadiusAPIServerIssuerName),
		"clusterissuers.cert-manager.io/*",
	}
}
