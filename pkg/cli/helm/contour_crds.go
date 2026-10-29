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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"text/template"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	clikubernetes "github.com/radius-project/radius/pkg/cli/kubernetes"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/kube"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	"helm.sh/helm/v4/pkg/strvals"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	// gatewayAPIGroup and gatewayAPIExperimentalGroup are the API groups of the Gateway API CRDs. The Contour chart
	// installs CRDs of these groups when gatewayAPI.manageCRDs is true.
	gatewayAPIGroup             = "gateway.networking.k8s.io"
	gatewayAPIExperimentalGroup = "gateway.networking.x-k8s.io"

	// Helm adopts an existing resource into a release only when it has this ownership metadata.
	helmManagedByLabel             = "app.kubernetes.io/managed-by"
	helmManagedByValue             = "Helm"
	helmReleaseNameAnnotation      = "meta.helm.sh/release-name"
	helmReleaseNamespaceAnnotation = "meta.helm.sh/release-namespace"

	// installCommand and upgradeCommand are the rad commands that install and upgrade Contour.
	installCommand = "rad install kubernetes"
	upgradeCommand = "rad upgrade kubernetes"
)

// gatewayAPIRequirement is a Gateway API resource version that Contour watches.
type gatewayAPIRequirement struct {
	// crd is the name of the CRD that defines the resource.
	crd string
	// version is the API version that Contour watches.
	version string
	// kind is the resource kind.
	kind string
	// feature is the name that Contour's --disable-feature flag accepts to stop watching the resource. Contour
	// always watches resources without a feature.
	feature string
}

// contourGatewayAPIRequirements lists, by Contour version, the Gateway API resources that Contour watches when a
// Gateway is configured. Contour exits when one of them is missing. The key is the Contour chart's appVersion. When a
// new Contour version is added, take its requirements from setupGatewayAPI in
// https://github.com/projectcontour/contour/blob/v<version>/cmd/contour/serve.go.
var contourGatewayAPIRequirements = map[string][]gatewayAPIRequirement{
	"1.32.0": {
		{crd: "gatewayclasses.gateway.networking.k8s.io", version: "v1", kind: "GatewayClass"},
		{crd: "gateways.gateway.networking.k8s.io", version: "v1", kind: "Gateway"},
		{crd: "httproutes.gateway.networking.k8s.io", version: "v1", kind: "HTTPRoute"},
		{crd: "referencegrants.gateway.networking.k8s.io", version: "v1beta1", kind: "ReferenceGrant"},
		{crd: "grpcroutes.gateway.networking.k8s.io", version: "v1", kind: "GRPCRoute", feature: "grpcroutes"},
		{crd: "tlsroutes.gateway.networking.k8s.io", version: "v1alpha2", kind: "TLSRoute", feature: "tlsroutes"},
		{crd: "tcproutes.gateway.networking.k8s.io", version: "v1alpha2", kind: "TCPRoute", feature: "tcproutes"},
		{crd: "backendtlspolicies.gateway.networking.k8s.io", version: "v1alpha3", kind: "BackendTLSPolicy", feature: "backendtlspolicies"},
	},
}

// crdInfo describes a CRD in the cluster or in a chart.
type crdInfo struct {
	name  string
	group string

	// versions are the names of all the versions of the CRD, and servedVersions are the ones that the API serves.
	versions       []string
	servedVersions []string

	// storedVersions are the versions that a CRD in the cluster has stored objects in.
	storedVersions []string

	// labels and annotations hold only the Helm ownership and resource policy metadata of a CRD in the cluster.
	labels      map[string]string
	annotations map[string]string
}

// isGatewayAPI reports whether the CRD belongs to the Gateway API.
func (c crdInfo) isGatewayAPI() bool {
	return c.group == gatewayAPIGroup || c.group == gatewayAPIExperimentalGroup
}

// ownedBy reports whether Helm can adopt the CRD into the given release.
func (c crdInfo) ownedBy(releaseName string, releaseNamespace string) bool {
	return c.labels[helmManagedByLabel] == helmManagedByValue &&
		c.annotations[helmReleaseNameAnnotation] == releaseName &&
		c.annotations[helmReleaseNamespaceAnnotation] == releaseNamespace
}

// owner describes who manages the CRD.
func (c crdInfo) owner() string {
	if releaseName := c.annotations[helmReleaseNameAnnotation]; releaseName != "" {
		return fmt.Sprintf("Helm release %q in namespace %q", releaseName, c.annotations[helmReleaseNamespaceAnnotation])
	}
	if managedBy := c.labels[helmManagedByLabel]; managedBy != "" {
		return fmt.Sprintf("%q", managedBy)
	}

	return "an installer other than Helm"
}

// kept reports whether Helm keeps the CRD when a release that installed it no longer renders it.
func (c crdInfo) kept() bool {
	return c.annotations[kube.ResourcePolicyAnno] == kube.KeepPolicy
}

// findCRD returns the CRD with the given name.
func findCRD(crds []crdInfo, name string) (crdInfo, bool) {
	index := slices.IndexFunc(crds, func(crd crdInfo) bool { return crd.name == name })
	if index < 0 {
		return crdInfo{}, false
	}

	return crds[index], true
}

// getClusterCRDs reads the CRDs with the given names from the cluster that kubeContext selects. It skips the names of
// CRDs that do not exist.
func getClusterCRDs(ctx context.Context, kubeContext string, names []string) ([]crdInfo, error) {
	config, err := clikubernetes.NewCLIClientConfig(kubeContext)
	if err != nil {
		return nil, err
	}

	client, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return getCRDsWithClient(ctx, client, names)
}

// getCRDsWithClient reads the CRDs with the given names, skipping the ones that do not exist. It reads only these CRDs
// because a cluster can have many large CRDs, and it keeps only the data that the Contour checks use.
func getCRDsWithClient(ctx context.Context, client apiextensionsclient.Interface, names []string) ([]crdInfo, error) {
	var crds []crdInfo
	for _, name := range names {
		item, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, err
		}

		crd := crdInfo{
			name:           item.Name,
			group:          item.Spec.Group,
			storedVersions: item.Status.StoredVersions,
			labels:         pickKeys(item.Labels, helmManagedByLabel),
			annotations:    pickKeys(item.Annotations, helmReleaseNameAnnotation, helmReleaseNamespaceAnnotation, kube.ResourcePolicyAnno),
		}
		for _, version := range item.Spec.Versions {
			crd.versions = append(crd.versions, version.Name)
			if version.Served {
				crd.servedVersions = append(crd.servedVersions, version.Name)
			}
		}
		crds = append(crds, crd)
	}

	return crds, nil
}

// crdNames returns the names of crds.
func crdNames(crds []crdInfo) []string {
	names := make([]string, 0, len(crds))
	for _, crd := range crds {
		names = append(names, crd.name)
	}

	return names
}

// pickKeys returns the entries of values with the given keys, or nil when it has none of them.
func pickKeys(values map[string]string, keys ...string) map[string]string {
	var picked map[string]string
	for _, key := range keys {
		if value, found := values[key]; found {
			if picked == nil {
				picked = map[string]string{}
			}
			picked[key] = value
		}
	}

	return picked
}

// crdsInManifest returns the CRDs in a manifest of YAML documents. Helm applies the items of a list instead of the
// list, so crdsInManifest reads lists the way that Helm's Kubernetes client reads them: a document with items is a
// list, whatever its kind, for example a List or a CustomResourceDefinitionList. See appendObjectCRDs.
func crdsInManifest(manifest string) ([]crdInfo, error) {
	var crds []crdInfo
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var document map[string]any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return crds, nil
		}
		if err != nil {
			return nil, err
		}

		crds, err = appendObjectCRDs(crds, document, true)
		if err != nil {
			return nil, err
		}
	}
}

// appendObjectCRDs appends the CRDs in object, a document of a manifest or an item of a list, to crds. Helm's
// Kubernetes client decodes a document with items as a list, and an item of a list as a list only when its items are
// a list, and it applies the items of every list instead of the list. An item without kind and apiVersion gets the
// kind of its list without the List suffix and the apiVersion of the list, so the items of a
// CustomResourceDefinitionList are CRDs even without a kind.
func appendObjectCRDs(crds []crdInfo, object map[string]any, document bool) ([]crdInfo, error) {
	items, hasItems := object["items"]
	list, isList := items.([]any)
	if document && hasItems && items != nil && !isList {
		return nil, fmt.Errorf("the items of a %s are not a list", kindOf(object))
	}
	if !isList && !(document && hasItems) {
		if kindOf(object) != "CustomResourceDefinition" {
			return crds, nil
		}
		crd, err := crdFromObject(object)
		if err != nil {
			return nil, err
		}
		return append(crds, crd), nil
	}

	for _, item := range list {
		itemObject, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("an item of a %s is not an object", kindOf(object))
		}
		if kindOf(itemObject) == "" && apiVersionOf(itemObject) == "" {
			itemObject = maps.Clone(itemObject)
			itemObject["kind"] = strings.TrimSuffix(kindOf(object), "List")
			itemObject["apiVersion"] = apiVersionOf(object)
		}

		var err error
		crds, err = appendObjectCRDs(crds, itemObject, false)
		if err != nil {
			return nil, err
		}
	}

	return crds, nil
}

// kindOf and apiVersionOf return the kind and the apiVersion of a Kubernetes object, or empty strings when they are
// not set to strings.
func kindOf(object map[string]any) string {
	kind, _ := object["kind"].(string)
	return kind
}

func apiVersionOf(object map[string]any) string {
	apiVersion, _ := object["apiVersion"].(string)
	return apiVersion
}

// crdFromObject reads the name, the group, and the versions of a CustomResourceDefinition object.
func crdFromObject(object map[string]any) (crdInfo, error) {
	data, err := json.Marshal(object)
	if err != nil {
		return crdInfo{}, err
	}
	var definition struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Group    string `json:"group"`
			Versions []struct {
				Name   string `json:"name"`
				Served bool   `json:"served"`
			} `json:"versions"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(data, &definition); err != nil {
		return crdInfo{}, err
	}

	crd := crdInfo{name: definition.Metadata.Name, group: definition.Spec.Group}
	for _, version := range definition.Spec.Versions {
		crd.versions = append(crd.versions, version.Name)
		if version.Served {
			crd.servedVersions = append(crd.servedVersions, version.Name)
		}
	}

	return crd, nil
}

// renderChartCRDs renders a chart with configuration as Helm renders it when it installs or upgrades the chart, and
// returns the CRDs that Helm applies with the release, and the CRDs in the crds/ directories of the chart and of the
// subcharts that the values enable. It renders with a server-side dry run, so that templates see the capabilities and
// the lookup results that the real install or upgrade sees, and .Release.IsUpgrade and .Release.IsInstall match it.
// configuration decides where those come from, and its Kubernetes client and release storage keep the dry run from
// changing the cluster. It does not change helmChart.
//
// One difference remains: the dry run is an install, so templates see .Release.Revision 1, while an upgrade renders
// the revision after the latest one. A chart that renders a CRD only for some revisions could still lose the CRD in an
// upgrade that the check allows. Charts tell an install from an upgrade with .Release.IsUpgrade or .Release.IsInstall
// rather than with the revision, so the check accepts this difference instead of rendering with Helm's upgrade action,
// which needs the release history in the configuration.
func renderChartCRDs(ctx context.Context, configuration *helm.Configuration, helmChart *chart.Chart, releaseName string, namespace string, upgrade bool, values map[string]any) ([]crdInfo, []crdInfo, error) {
	renderedChart := cloneChart(helmChart)

	install := helm.NewInstall(configuration)
	install.DryRunStrategy = helm.DryRunServer
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.IsUpgrade = upgrade
	rendered, err := install.RunWithContext(ctx, renderedChart, copyValues(values))
	if err != nil {
		return nil, nil, err
	}

	accessor, err := release.NewAccessor(rendered)
	if err != nil {
		return nil, nil, err
	}

	manifestCRDs, err := crdsInManifest(accessor.Manifest())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read the rendered manifest: %w", err)
	}

	// The render removed the subcharts that the values disable, as Helm does before it installs CRDs.
	var directoryCRDs []crdInfo
	for _, object := range renderedChart.CRDObjects() {
		crds, err := crdsInManifest(string(object.File.Data))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read %s: %w", object.Filename, err)
		}
		directoryCRDs = append(directoryCRDs, crds...)
	}

	return manifestCRDs, directoryCRDs, nil
}

// RenderConfiguration returns the Helm configuration with which the Contour checks render a chart for the cluster
// that kubeContext selects. Like the configuration of the real install or upgrade, it reads the capabilities of that
// cluster and lets template lookups read it. Its Kubernetes client applies nothing, and its release storage is in
// memory, so a dry run with it never changes the cluster.
func (liveContourCluster) RenderConfiguration(kubeContext string, namespace string) (*helm.Configuration, error) {
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}
	configuration, err := initHelmConfig(&flags)
	if err != nil {
		return nil, err
	}

	configuration.KubeClient = &kubefake.PrintingKubeClient{Out: io.Discard}
	memory := driver.NewMemory()
	memory.SetNamespace(namespace)
	configuration.Releases = storage.Init(memory)

	return configuration, nil
}

// cloneChart copies a chart and its subcharts so that rendering the copy does not change the original. Helm changes
// the chart that it renders: it removes the subcharts that the values disable and rewrites the dependency metadata
// and the values.
func cloneChart(original *chart.Chart) *chart.Chart {
	copied := *original
	if original.Metadata != nil {
		metadata := *original.Metadata
		if original.Metadata.Dependencies != nil {
			metadata.Dependencies = make([]*chart.Dependency, len(original.Metadata.Dependencies))
			for i, dependency := range original.Metadata.Dependencies {
				if dependency != nil {
					copiedDependency := *dependency
					metadata.Dependencies[i] = &copiedDependency
				}
			}
		}
		copied.Metadata = &metadata
	}

	subcharts := make([]*chart.Chart, 0, len(original.Dependencies()))
	for _, subchart := range original.Dependencies() {
		subcharts = append(subcharts, cloneChart(subchart))
	}
	copied.SetDependencies(subcharts...)

	return &copied
}

// contourUpgradeBaseline returns the revision of the Contour release that a Helm upgrade starts from, which Helm's
// upgrade action chooses this way: the latest revision when it is deployed, otherwise the latest deployed revision,
// otherwise the latest revision when it failed or was superseded. Helm reuses the values of that revision and deletes
// the resources in its manifest that the new chart no longer renders. Like Helm, it refuses a release with a pending
// operation, and its error suggests commands for the cluster that kubeContext selects.
func contourUpgradeBaseline(history []*releasev1.Release, releaseName string, releaseNamespace string, kubeContext string) (*releasev1.Release, error) {
	var latest, deployed *releasev1.Release
	for _, revision := range history {
		if revision == nil {
			continue
		}
		if latest == nil || revision.Version > latest.Version {
			latest = revision
		}
		if revisionStatus(revision) == releasecommon.StatusDeployed && (deployed == nil || revision.Version > deployed.Version) {
			deployed = revision
		}
	}
	if latest == nil {
		return nil, driver.NewErrNoDeployedReleases(releaseName)
	}

	status := revisionStatus(latest)
	switch {
	case status.IsPending():
		// Without a deployed revision there is nothing to roll back to. Helm stores each revision in a Secret, and
		// deleting the Secret of the pending revision makes Helm forget it without changing the cluster.
		recovery := fmt.Sprintf("delete the record of the pending revision with %s, which does not change the resources in the cluster",
			kubectlCommand(kubeContext, releaseNamespace, fmt.Sprintf("delete secret sh.helm.release.v1.%s.v%d", releaseName, latest.Version)))
		if deployed != nil {
			recovery = fmt.Sprintf("roll the release back to the last revision that worked, for example with %s",
				helmCommand(kubeContext, releaseNamespace, fmt.Sprintf("rollback %s %d", releaseName, deployed.Version)))
		}
		return nil, clierrors.Message("Radius stopped before changing the cluster because another operation is in progress on Contour release %q in namespace %q: revision %d is %s. "+
			"Wait for that operation to finish, and then run the command again. If no operation is running, for example because an earlier command was interrupted, "+
			"list the revisions with %s, %s, and then run the command again.",
			releaseName, releaseNamespace, latest.Version, status, helmCommand(kubeContext, releaseNamespace, "history "+releaseName), recovery)
	case status == releasecommon.StatusDeployed:
		return latest, nil
	case deployed != nil:
		return deployed, nil
	case status == releasecommon.StatusFailed || status == releasecommon.StatusSuperseded:
		return latest, nil
	}

	return nil, driver.NewErrNoDeployedReleases(releaseName)
}

// revisionStatus returns the status of a release revision.
func revisionStatus(revision *releasev1.Release) releasecommon.Status {
	if revision.Info == nil {
		return ""
	}

	return revision.Info.Status
}

// gatewayAPICRDCheck decides how a Contour install or upgrade treats the Gateway API CRDs in a cluster. It checks
// that the change neither deletes CRDs nor takes over Gateway API CRDs that belong to something else, and that
// Contour can use the Gateway API CRDs that the cluster has afterwards.
type gatewayAPICRDCheck struct {
	releaseName      string
	releaseNamespace string

	// kubeContext selects the cluster that the commands in the errors act on.
	kubeContext string

	// command is the rad command that installs or upgrades Contour, which the errors tell the user to run again.
	command string

	// setArgs and setFileArgs are the --contour-set and --contour-set-file values of this invocation, which the
	// remedies tell the user how to change.
	setArgs     []string
	setFileArgs []string

	// getCRDs reads the CRDs with the given names from the cluster. It skips the names of CRDs that do not exist.
	getCRDs func(ctx context.Context, names []string) ([]crdInfo, error)

	// renderConfiguration returns the Helm configuration with which the check renders the chart as Helm renders it in
	// the cluster.
	renderConfiguration func() (*helm.Configuration, error)

	// The fields below describe the change. run sets them.

	// clusterCRDs are the CRDs in the cluster that the change involves: the CRDs in the manifest of the release, the
	// CRDs that the chart renders, and the Gateway API CRDs that Contour watches. readNames are the names of the CRDs
	// that run read, including the ones that do not exist.
	clusterCRDs []crdInfo
	readNames   []string

	// contourVersion is the version of Contour that the chart deploys.
	contourVersion string

	// requirements are the Gateway API resources that contourVersion watches, or nil when Radius does not know them.
	requirements []gatewayAPIRequirement

	// upgrade is true when Helm upgrades an existing Contour release instead of installing one.
	upgrade bool

	// resetValues is true when the upgrade discards the values stored on the Contour release.
	resetValues bool

	// releaseCRDs are the CRDs in the manifest of the release revision that the upgrade starts from. Helm deletes the
	// ones that the chart no longer renders.
	releaseCRDs []crdInfo

	// manageableCRDs are the CRDs that the chart renders when gatewayAPI.manageCRDs is true.
	manageableCRDs []crdInfo

	// renderedCRDs are the CRDs that the chart renders with the final values. Helm creates or updates them.
	renderedCRDs []crdInfo

	// directoryCRDs are the CRDs in the crds/ directories of the chart and its enabled subcharts. Helm creates the
	// missing ones when it installs the chart, and never updates or deletes them.
	directoryCRDs []crdInfo

	// manageCRDsFromValues is true when the Contour values, not Radius, decide gatewayAPI.manageCRDs.
	manageCRDsFromValues bool
}

// run checks installing helmChart, or upgrading baseline to it, with values, which are the Contour values of this
// invocation. It sets the Radius default for gatewayAPI.manageCRDs in values unless they already decide it. baseline is
// the release revision that the upgrade starts from, or nil for an install. reuseValues is true when Helm merges the
// values stored on baseline beneath values. It returns a warning when it cannot check whether Contour can use the
// Gateway API CRDs.
func (c *gatewayAPICRDCheck) run(ctx context.Context, helmChart *chart.Chart, values map[string]any, baseline *releasev1.Release, reuseValues bool) (string, error) {
	c.contourVersion = strings.TrimPrefix(helmChart.Metadata.AppVersion, "v")
	c.requirements = contourGatewayAPIRequirements[c.contourVersion]
	c.upgrade = baseline != nil
	c.resetValues = c.upgrade && !reuseValues

	var reusedValues map[string]any
	if baseline != nil {
		releaseCRDs, err := crdsInManifest(baseline.Manifest)
		if err != nil {
			return "", fmt.Errorf("failed to read the Contour release manifest, err: %w", err)
		}
		c.releaseCRDs = releaseCRDs

		if reuseValues {
			reusedValues = baseline.Config
		}
	}

	manageable := helmValues(values, reusedValues)
	setValue(manageable, true, "gatewayAPI", "manageCRDs")
	// One configuration for both renders discovers the capabilities of the cluster once.
	configuration, err := c.renderConfiguration()
	if err != nil {
		return "", fmt.Errorf("failed to prepare the render of the Contour Helm chart, err: %w", err)
	}
	manageableCRDs, _, err := renderChartCRDs(ctx, configuration, helmChart, c.releaseName, c.releaseNamespace, c.upgrade, manageable)
	if err != nil {
		return "", fmt.Errorf("failed to render the Contour Helm chart, err: %w", err)
	}
	c.manageableCRDs = manageableCRDs

	requiredCRDs := make([]string, 0, len(c.requirements))
	for _, requirement := range c.requirements {
		requiredCRDs = append(requiredCRDs, requirement.crd)
	}
	if err := c.readCRDs(ctx, crdNames(c.releaseCRDs), crdNames(c.manageableCRDs), requiredCRDs); err != nil {
		return "", err
	}

	// The user's --contour-set and --contour-set-file values take precedence over this default.
	c.manageCRDsFromValues = !setDefaultValue(values, c.defaultManageCRDs(), "gatewayAPI", "manageCRDs")

	c.renderedCRDs, c.directoryCRDs, err = renderChartCRDs(ctx, configuration, helmChart, c.releaseName, c.releaseNamespace, c.upgrade, helmValues(values, reusedValues))
	if err != nil {
		return "", fmt.Errorf("failed to render the Contour Helm chart, err: %w", err)
	}
	if err := c.readCRDs(ctx, crdNames(c.renderedCRDs)); err != nil {
		return "", err
	}

	effectiveValues, err := effectiveContourValues(helmChart, values, reusedValues)
	if err != nil {
		return "", fmt.Errorf("failed to compute Contour values, err: %w", err)
	}

	return c.validate(effectiveValues)
}

// readCRDs reads the CRDs with the given names from the cluster, except the ones that it read already.
func (c *gatewayAPICRDCheck) readCRDs(ctx context.Context, nameLists ...[]string) error {
	var names []string
	for _, list := range nameLists {
		for _, name := range list {
			if !slices.Contains(c.readNames, name) && !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 {
		return nil
	}

	crds, err := c.getCRDs(ctx, names)
	if err != nil {
		return fmt.Errorf("failed to read CRDs, err: %w", err)
	}
	c.readNames = append(c.readNames, names...)
	c.clusterCRDs = append(c.clusterCRDs, crds...)

	return nil
}

// installedGatewayAPICRDs returns the Gateway API CRDs that Helm would delete if the Contour release stopped rendering
// them: the ones in the manifest of the release that exist and lack the helm.sh/resource-policy: keep annotation.
func (c gatewayAPICRDCheck) installedGatewayAPICRDs() []crdInfo {
	var installed []crdInfo
	for _, releaseCRD := range c.releaseCRDs {
		if !releaseCRD.isGatewayAPI() {
			continue
		}
		clusterCRD, found := findCRD(c.clusterCRDs, releaseCRD.name)
		if found && !clusterCRD.kept() && !slices.ContainsFunc(installed, func(crd crdInfo) bool { return crd.name == clusterCRD.name }) {
			installed = append(installed, clusterCRD)
		}
	}
	slices.SortFunc(installed, func(a, b crdInfo) int { return strings.Compare(a.name, b.name) })

	return installed
}

// foreignCRDs returns the Gateway API CRDs in the cluster that the chart CRDs would replace but that belong to
// something else than the Contour release.
func (c gatewayAPICRDCheck) foreignCRDs(chartCRDs []crdInfo) []crdInfo {
	var foreign []crdInfo
	for _, crd := range c.clusterCRDs {
		if !crd.isGatewayAPI() || crd.ownedBy(c.releaseName, c.releaseNamespace) {
			continue
		}
		if _, found := findCRD(chartCRDs, crd.name); found {
			foreign = append(foreign, crd)
		}
	}

	return foreign
}

// defaultManageCRDs returns the gatewayAPI.manageCRDs value to use when the user does not set it. Contour keeps
// managing the Gateway API CRDs that Helm would otherwise delete, reuses CRDs that belong to something else, and
// installs them in clusters that have none. CRDs that only carry the ownership metadata of the Contour release, for
// example because they were kept when the release stopped managing them, do not count, because Helm never deletes
// them.
func (c gatewayAPICRDCheck) defaultManageCRDs() bool {
	return len(c.installedGatewayAPICRDs()) > 0 || len(c.foreignCRDs(c.manageableCRDs)) == 0
}

// deletedCRDs returns the CRDs that Helm would delete because the release installed them and the chart no longer
// renders them. Like Helm, it skips CRDs that no longer exist and CRDs with the helm.sh/resource-policy: keep
// annotation.
func (c gatewayAPICRDCheck) deletedCRDs() []crdInfo {
	var deleted []crdInfo
	for _, releaseCRD := range c.releaseCRDs {
		if _, rendered := findCRD(c.renderedCRDs, releaseCRD.name); rendered {
			continue
		}
		clusterCRD, found := findCRD(c.clusterCRDs, releaseCRD.name)
		if found && !clusterCRD.kept() && !slices.ContainsFunc(deleted, func(crd crdInfo) bool { return crd.name == clusterCRD.name }) {
			deleted = append(deleted, clusterCRD)
		}
	}

	return deleted
}

// storedVersionRemoval is a CRD that Helm would update to a definition without versions that the cluster stores
// objects of the CRD in.
type storedVersionRemoval struct {
	name string

	// removed are the stored versions that the chart's definition lacks, and versions are the versions that it has.
	removed  []string
	versions []string
}

// removedStoredVersions returns the CRDs that Helm would update to remove versions that the cluster stores objects in.
// The API server rejects such an update, so Helm would fail partway through applying the chart.
func (c gatewayAPICRDCheck) removedStoredVersions() []storedVersionRemoval {
	var removals []storedVersionRemoval
	for _, rendered := range c.renderedCRDs {
		clusterCRD, found := findCRD(c.clusterCRDs, rendered.name)
		if !found {
			continue
		}

		var removed []string
		for _, version := range clusterCRD.storedVersions {
			if !slices.Contains(rendered.versions, version) {
				removed = append(removed, version)
			}
		}
		if len(removed) > 0 {
			removals = append(removals, storedVersionRemoval{name: rendered.name, removed: removed, versions: rendered.versions})
		}
	}

	return removals
}

func storedVersionsError(removals []storedVersionRemoval) error {
	var message strings.Builder
	message.WriteString("Radius stopped before changing the cluster because the Contour chart removes versions of these CRDs that the cluster stores objects in:\n")
	for _, removal := range removals {
		versions := "no versions"
		if len(removal.versions) > 0 {
			versions = strings.Join(removal.versions, ", ")
		}
		fmt.Fprintf(&message, "  - %s stores objects in %s, but the chart's definition has %s\n", removal.name, strings.Join(removal.removed, ", "), versions)
	}
	message.WriteString("The Kubernetes API server rejects a CRD update that removes a stored version, so Helm would fail to apply the Contour chart.\n")
	message.WriteString("To continue, do one of the following:\n")
	message.WriteString("  - Use a Contour chart whose CRDs keep these versions.\n")
	message.WriteString("  - Migrate the stored objects to a version that the chart keeps, and then remove the old versions from status.storedVersions of these CRDs. " +
		"See https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/#upgrade-existing-objects-to-a-new-stored-version.")

	return clierrors.Message("%s", message.String())
}

// validate returns an error when the change would delete CRDs, take over Gateway API CRDs that belong to something
// else, or remove CRD versions that the cluster stores objects in, or when Contour cannot use the Gateway API CRDs that
// the cluster has afterwards. values are the values that Helm renders the chart with. Callers validate before they
// change the cluster, and the errors say so.
func (c gatewayAPICRDCheck) validate(values map[string]any) (string, error) {
	manageCRDs := isTrue(nestedValue(values, "gatewayAPI", "manageCRDs"))
	if deleted := c.deletedCRDs(); len(deleted) > 0 {
		return "", c.deleteError(deleted, manageCRDs)
	}

	if conflicts := c.foreignCRDs(c.renderedCRDs); len(conflicts) > 0 {
		return "", c.conflictError(conflicts, manageCRDs)
	}

	if removals := c.removedStoredVersions(); len(removals) > 0 {
		return "", storedVersionsError(removals)
	}

	if c.requirements == nil {
		if c.contourVersion == "" {
			return "The Contour chart does not set appVersion, so Radius did not check that Contour can use the Gateway API CRDs in the cluster.", nil
		}
		return fmt.Sprintf("Radius does not know which Gateway API CRD versions Contour %s needs, so it did not check the Gateway API CRDs in the cluster.", c.contourVersion), nil
	}

	args := contourArgsFromValues(values)
	if unmet := c.unmetRequirements(c.renderedCRDs, args.disabledFeatures()); len(unmet) > 0 {
		return "", c.requirementsError(unmet, args, manageCRDs)
	}

	return "", nil
}

// unmetRequirement is a resource version that Contour watches but that no CRD serves after the change.
type unmetRequirement struct {
	gatewayAPIRequirement

	// definition is the CRD that defines the resource after the change, or nil when there is none.
	definition *crdInfo

	// fromChart is true when the Contour chart provides definition.
	fromChart bool
}

// unmetRequirements returns the requirements that the Gateway API CRDs do not meet after Helm applies chartCRDs,
// skipping the features that Contour's arguments turn off.
func (c gatewayAPICRDCheck) unmetRequirements(chartCRDs []crdInfo, disabledFeatures []string) []unmetRequirement {
	var unmet []unmetRequirement
	for _, requirement := range c.requirements {
		if requirement.feature != "" && slices.Contains(disabledFeatures, requirement.feature) {
			continue
		}

		crd, fromChart, found := c.crdAfterChange(chartCRDs, requirement.crd)
		if !found {
			unmet = append(unmet, unmetRequirement{gatewayAPIRequirement: requirement})
		} else if !slices.Contains(crd.servedVersions, requirement.version) {
			unmet = append(unmet, unmetRequirement{gatewayAPIRequirement: requirement, definition: &crd, fromChart: fromChart})
		}
	}

	return unmet
}

// crdAfterChange returns the CRD with the given name that the cluster has after Helm applies chartCRDs, and whether
// the chart provides it. Helm creates or updates the CRDs that the chart renders. When it installs the chart, it also
// creates the CRDs in the chart's crds/ directories that do not exist yet.
func (c gatewayAPICRDCheck) crdAfterChange(chartCRDs []crdInfo, name string) (crdInfo, bool, bool) {
	if crd, found := findCRD(chartCRDs, name); found {
		return crd, true, true
	}
	if crd, found := findCRD(c.clusterCRDs, name); found {
		return crd, false, true
	}
	if !c.upgrade {
		if crd, found := findCRD(c.directoryCRDs, name); found {
			return crd, true, true
		}
	}

	return crdInfo{}, false, false
}

func (c gatewayAPICRDCheck) deleteError(deleted []crdInfo, manageCRDs bool) error {
	var message strings.Builder
	fmt.Fprintf(&message, "Radius stopped before changing the cluster because the Contour chart no longer includes these CRDs, which Contour release %q in namespace %q installed:\n", c.releaseName, c.releaseNamespace)
	names := make([]string, 0, len(deleted))
	gatewayAPI := false
	for _, crd := range deleted {
		fmt.Fprintf(&message, "  - %s", crd.name)
		if !crd.ownedBy(c.releaseName, c.releaseNamespace) {
			fmt.Fprintf(&message, " (now owned by %s)", crd.owner())
		}
		message.WriteString("\n")
		names = append(names, crd.name)
		gatewayAPI = gatewayAPI || crd.isGatewayAPI()
	}
	message.WriteString("Helm would delete these CRDs, and every resource of these kinds in the cluster with them.\nTo continue, do one of the following:\n")

	if gatewayAPI && !manageCRDs && c.manageCRDsFromValues {
		message.WriteString("  - Remove the Contour value that sets gatewayAPI.manageCRDs to false.\n")
	} else {
		message.WriteString("  - Change the Contour chart or values so that the chart still includes these CRDs.\n")
	}
	fmt.Fprintf(&message, "  - Keep these CRDs when Helm stops managing them by annotating them first:\n      %s", c.keepCRDsCommand(names))

	return clierrors.Message("%s", message.String())
}

func (c gatewayAPICRDCheck) conflictError(conflicts []crdInfo, manageCRDs bool) error {
	var message strings.Builder
	installed := c.installedGatewayAPICRDs()
	if !manageCRDs || len(installed) == 0 {
		message.WriteString("Radius stopped before changing the cluster because these Gateway API CRDs already exist and belong to something else, so Contour cannot install them:\n")
		for _, crd := range conflicts {
			fmt.Fprintf(&message, "  - %s (owned by %s)\n", crd.name, crd.owner())
		}
		message.WriteString("Radius does not take over CRDs that belong to something else")
		switch {
		case !manageCRDs:
			message.WriteString(". The Contour chart includes these CRDs even though gatewayAPI.manageCRDs is false, so use a Contour chart that does not.")
		case c.manageCRDsFromValues:
			message.WriteString(". Remove the gatewayAPI.manageCRDs=true Contour value so that Contour uses the existing CRDs.")
		default:
			message.WriteString(".")
		}
		if remedy, ok := c.skipContourInstall("To continue without Contour, install Radius", " "); ok {
			message.WriteString("\n" + remedy)
		}
		return clierrors.Message("%s", message.String())
	}

	// Contour cannot manage the Gateway API CRDs, because some belong to something else, and it cannot stop managing
	// them, because Helm would delete the ones that the release installed.
	fmt.Fprintf(&message, "Radius stopped before changing the cluster because the Gateway API CRDs in this cluster have more than one owner. Contour release %q in namespace %q installed these CRDs, so Helm would delete them if Contour stopped managing them:\n", c.releaseName, c.releaseNamespace)
	names := make([]string, 0, len(installed))
	for _, crd := range installed {
		fmt.Fprintf(&message, "  - %s", crd.name)
		if !crd.ownedBy(c.releaseName, c.releaseNamespace) {
			fmt.Fprintf(&message, " (now owned by %s)", crd.owner())
		}
		message.WriteString("\n")
		names = append(names, crd.name)
	}
	var others []crdInfo
	for _, crd := range conflicts {
		if !slices.Contains(names, crd.name) {
			others = append(others, crd)
		}
	}
	if len(others) > 0 {
		message.WriteString("These CRDs belong to something else:\n")
		for _, crd := range others {
			fmt.Fprintf(&message, "  - %s (owned by %s)\n", crd.name, crd.owner())
		}
	}
	message.WriteString("Radius neither takes over CRDs that belong to something else nor lets Helm delete the CRDs that the Contour release installed.\n")
	fmt.Fprintf(&message, "To continue, keep the CRDs that the Contour release installed when Helm stops managing them, and then run the command again. Contour then uses the Gateway API CRDs in the cluster instead of managing them:\n      %s", c.keepCRDsCommand(names))
	if remedy, ok := c.skipContourInstall("To continue without Contour, install Radius", " "); ok {
		message.WriteString("\n" + remedy)
	}

	return clierrors.Message("%s", message.String())
}

// keepCRDsCommand returns the kubectl command that annotates the CRDs with the given names so that Helm keeps them.
func (c gatewayAPICRDCheck) keepCRDsCommand(names []string) string {
	return kubectlCommand(c.kubeContext, "", fmt.Sprintf("annotate crd %s %s=%s", strings.Join(names, " "), kube.ResourcePolicyAnno, kube.KeepPolicy))
}

func (c gatewayAPICRDCheck) requirementsError(unmet []unmetRequirement, args contourArgs, manageCRDs bool) error {
	var message strings.Builder
	fmt.Fprintf(&message, "Radius stopped before changing the cluster because Contour %s cannot use the Gateway API CRDs in this cluster. Contour needs these resource versions:\n", c.contourVersion)
	var features, kinds []string
	for _, requirement := range unmet {
		status := "the CRD is not installed"
		if requirement.definition != nil {
			subject := "the installed CRD"
			if requirement.fromChart {
				subject = "the CRD that the Contour chart installs"
			}
			status = subject + " serves no versions"
			if served := requirement.definition.servedVersions; len(served) > 0 {
				status = subject + " serves " + strings.Join(served, ", ")
			}
		}
		fmt.Fprintf(&message, "  - %s %s (%s)\n", requirement.crd, requirement.version, status)

		if requirement.feature != "" {
			features = append(features, requirement.feature)
			kinds = append(kinds, requirement.kind)
		}
	}

	owners := c.foreignOwners()
	switch {
	case manageCRDs:
		message.WriteString("The Contour chart does not install Gateway API CRDs that serve all of these versions.\n")
	case c.manageCRDsFromValues:
		message.WriteString("Contour does not install Gateway API CRDs because the Contour values set gatewayAPI.manageCRDs to false.\n")
		if len(owners) > 0 {
			fmt.Fprintf(&message, "The Gateway API CRDs in this cluster belong to %s.\n", joinWords(owners))
		}
	default:
		// Radius chose to reuse the Gateway API CRDs because some of them belong to something else.
		fmt.Fprintf(&message, "The Gateway API CRDs in this cluster belong to %s, so Contour uses them instead of installing its own.\n", joinWords(owners))
	}

	var remedies []string
	switch {
	case manageCRDs:
		remedies = append(remedies, "Use a Contour chart that installs Gateway API CRDs that serve these versions.")
	case c.manageCRDsFromValues && c.canManageCRDs(args.disabledFeatures()):
		remedies = append(remedies,
			"Remove the Contour value that sets gatewayAPI.manageCRDs to false, so that Contour installs the Gateway API CRDs that it needs.",
			"Install Gateway API CRDs that serve these versions.")
	default:
		remedies = append(remedies, "Install Gateway API CRDs that serve these versions.")
	}
	if len(features) == len(unmet) {
		remedies = append(remedies, c.turnOffFeaturesRemedy(features, kinds, args))
	}
	if !manageCRDs && !c.manageCRDsFromValues {
		remedies = append(remedies, `Use a cluster without preinstalled Gateway API CRDs. For example, create the k3d cluster with --k3s-arg "--disable=traefik@server:*".`)
	}
	if remedy, ok := c.skipContourInstall("Install Radius without Contour", "\n    "); ok {
		remedies = append(remedies, remedy)
	}

	message.WriteString("To continue, do one of the following:")
	for _, remedy := range remedies {
		message.WriteString("\n  - " + remedy)
	}

	return clierrors.Message("%s", message.String())
}

// canManageCRDs reports whether Contour could install the Gateway API CRDs that it needs if gatewayAPI.manageCRDs
// were true: the chart would take over no CRDs, and its CRDs would meet the requirements.
func (c gatewayAPICRDCheck) canManageCRDs(disabledFeatures []string) bool {
	return len(c.foreignCRDs(c.manageableCRDs)) == 0 && len(c.unmetRequirements(c.manageableCRDs, disabledFeatures)) == 0
}

// turnOffFeaturesRemedy explains how to turn off the Contour features that use resources without a matching CRD.
func (c gatewayAPICRDCheck) turnOffFeaturesRemedy(features []string, kinds []string, args contourArgs) string {
	var remedy strings.Builder
	fmt.Fprintf(&remedy, "Turn off the Contour features that use these resources. Contour then ignores %s resources.", joinWords(kinds))

	// A value that Radius cannot read as a list cannot be extended safely, for example without dropping the serve
	// argument from contour.args.
	if args.unreadable {
		fmt.Fprintf(&remedy, " Radius cannot read your %s value as a list, so add these arguments to it:", args.key)
		for _, feature := range features {
			remedy.WriteString("\n      --disable-feature=" + feature)
		}
		return remedy.String()
	}

	flag := "--contour-set " + QuoteFlagValue(args.withDisabledFeatures(features))
	source, found := c.valueSource(args.key)
	switch {
	case !found:
		fmt.Fprintf(&remedy, " Add this flag to the %s command:\n      %s", c.command, flag)
		if c.resetValues {
			remedy.WriteString("\n    This upgrade uses --reset-values, which discards the values stored on the Contour release. If you set this value when you installed Radius, you can instead run the upgrade without --reset-values.")
		}
		if c.command == installCommand {
			remedy.WriteString("\n    If you used rad init, run rad install kubernetes with this flag first, and then run rad init again.")
		}
	case source.fromFile:
		fmt.Fprintf(&remedy, " The %s value comes from --contour-set-file %s, and Radius applies --contour-set-file values after --contour-set values, so another --contour-set flag does not change it.", args.key, QuoteFlagValue(source.value))
		fileFlags := make([]string, 0, len(source.fileFlags))
		for _, fileFlag := range source.fileFlags {
			fileFlags = append(fileFlags, "--contour-set-file "+QuoteFlagValue(fileFlag))
		}
		switch {
		case source.file != "":
			fmt.Fprintf(&remedy, " Add these entries to the list in %s:", source.file)
			for _, feature := range features {
				remedy.WriteString("\n      - --disable-feature=" + feature)
			}
			if !source.otherValues {
				fmt.Fprintf(&remedy, "\n    Or replace %s in the %s command with this flag, which keeps the current arguments:\n      %s", joinWords(fileFlags), c.command, flag)
			}
		case !source.otherValues:
			fmt.Fprintf(&remedy, " Replace %s in the %s command with this flag, which keeps the current arguments:\n      %s", joinWords(fileFlags), c.command, flag)
		default:
			fmt.Fprintf(&remedy, " Remove the %s value from %s, and add this flag to the %s command:\n      %s", args.key, joinWords(fileFlags), c.command, flag)
		}
	case source.otherValues:
		fmt.Fprintf(&remedy, " The %s value comes from --contour-set %s, which also sets other values. Add this flag to the %s command after that flag, so that it replaces the %s value and keeps its arguments:\n      %s", args.key, QuoteFlagValue(source.value), c.command, args.key, flag)
	default:
		fmt.Fprintf(&remedy, " Replace --contour-set %s in the %s command with this flag, which keeps its arguments:\n      %s", QuoteFlagValue(source.value), c.command, flag)
	}

	return remedy.String()
}

// contourValueSource is how the --contour-set and --contour-set-file flags of this invocation set a Contour value.
type contourValueSource struct {
	// fromFile is true when --contour-set-file flags set the value as a whole. value is the flag that decides the
	// value: the last --contour-set-file flag that sets it as a whole, or else the last --contour-set flag that sets it.
	fromFile bool
	value    string

	// file is the file from which the deciding --contour-set-file flag reads the whole value, or empty when the flag
	// reads it from several files or sets a value that contains it.
	file string

	// fileFlags are the --contour-set-file flags that set the value as a whole, in order. Radius applies every one of
	// them after the --contour-set flags, so a remedy that replaces the deciding flag must replace all of them.
	fileFlags []string

	// otherValues is true when a flag that a remedy would replace sets other values too.
	otherValues bool
}

// valueSource returns how the flags of this invocation set the Contour value at key. Like Helm, Radius applies every
// --contour-set flag in order and then every --contour-set-file flag, so the last --contour-set-file flag that sets the
// value as a whole decides it, or else the last --contour-set flag that sets the value or part of it. A
// --contour-set-file flag that sets only items of the list keeps the other items, so it does not decide the list. It
// returns false when no flag of this invocation decides the value.
func (c gatewayAPICRDCheck) valueSource(key string) (contourValueSource, bool) {
	path := strings.Split(key, ".")
	var source contourValueSource
	found := false
	for _, arg := range c.setArgs {
		parsed := map[string]any{}
		if strvals.ParseInto(arg, parsed) != nil || !setsValue(parsed, path) {
			continue
		}
		source, found = contourValueSource{value: arg, otherValues: setsOtherValues(parsed, path)}, true
	}

	fileSource := contourValueSource{fromFile: true}
	for _, arg := range c.setFileArgs {
		file, whole, otherValues := fileFlagSetting(arg, path)
		if !whole {
			continue
		}
		fileSource.value, fileSource.file = arg, file
		fileSource.fileFlags = append(fileSource.fileFlags, arg)
		fileSource.otherValues = fileSource.otherValues || otherValues
	}
	if len(fileSource.fileFlags) > 0 {
		return fileSource, true
	}

	return source, found
}

// fileFlagSetting reports how a --set-file style flag sets the list at path. whole is true when the flag sets the
// list as a whole, or a value that contains it, and false when it sets only items of the list, or nothing there. file
// is the file from which the flag reads the whole list, or empty when it reads the list from several files, item by
// item, or sets a value that contains it. otherValues is true when the flag sets other values too.
func fileFlagSetting(arg string, path []string) (file string, whole bool, otherValues bool) {
	// Parse the flag over a list of placeholder items, as earlier flags or defaults can set a list there, so that a flag
	// that sets items of the list keeps some placeholders, and a flag that replaces the list keeps none. The names of the
	// files stand for their contents.
	placeholders := make([]any, 16)
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("\x00item %d", i)
	}
	values := map[string]any{}
	setValue(values, slices.Clone(placeholders), path...)
	readName := func(name []rune) (any, error) { return string(name), nil }
	if strvals.ParseIntoFile(setFileArg(arg), values, readName) != nil {
		return "", false, false
	}
	otherValues = setsOtherValues(values, path)

	value := nestedValue(values, path...)
	list, ok := value.([]any)
	if !ok {
		// The flag reads the list from one file, or replaces a value that contains it.
		file, _ = value.(string)
		return file, true, otherValues
	}

	kept := 0
	for i, placeholder := range placeholders {
		if i < len(list) && list[i] == placeholder {
			kept++
		}
	}
	if kept == len(placeholders) && len(list) == len(placeholders) {
		return "", false, otherValues
	}

	return "", kept == 0, otherValues
}

// setsValue reports whether values, which a --set style flag sets, include the value at path, part of it, or a value
// that contains it.
func setsValue(values map[string]any, path []string) bool {
	var current any = values
	for _, key := range path {
		table, ok := current.(map[string]any)
		if !ok {
			// The flag replaces a value that contains path.
			return true
		}
		if current, ok = table[key]; !ok {
			return false
		}
	}

	return true
}

// setsOtherValues reports whether values, which a --set style flag sets, include values outside path.
func setsOtherValues(values map[string]any, path []string) bool {
	table := values
	for _, key := range path {
		if len(table) != 1 {
			return true
		}
		next, ok := table[key].(map[string]any)
		if !ok {
			return false
		}
		table = next
	}

	return false
}

// skipContourInstall explains, for rad install kubernetes, how to install Radius without Contour. The explanation
// starts with lead, and noteSeparator comes before the note for rad init. It returns false for commands that cannot
// skip Contour.
func (c gatewayAPICRDCheck) skipContourInstall(lead string, noteSeparator string) (string, bool) {
	if c.command != installCommand {
		return "", false
	}

	return lead + " with --skip-contour-install, for example when Traefik is your ingress controller. Radius gateways use Contour, so they do not work without it." +
		noteSeparator + "If you used rad init, run rad install kubernetes --skip-contour-install first, and then run rad init again.", true
}

// foreignOwners describes who owns the Gateway API CRDs that the change involves and that do not belong to the Contour
// release.
func (c gatewayAPICRDCheck) foreignOwners() []string {
	var owners []string
	for _, crd := range c.clusterCRDs {
		if !crd.isGatewayAPI() || crd.ownedBy(c.releaseName, c.releaseNamespace) {
			continue
		}
		if owner := crd.owner(); !slices.Contains(owners, owner) {
			owners = append(owners, owner)
		}
	}

	return owners
}

// contourArgs are the extra Contour command-line arguments in the chart values.
type contourArgs struct {
	// key is the chart value that holds args.
	key  string
	args []any

	// unreadable is true when the value is set but Radius cannot read it as a list of arguments.
	unreadable bool
}

// contourArgsFromValues returns the Contour arguments that the chart renders. The chart uses contour.args instead of
// its default arguments when contour.args is set, and otherwise appends contour.extraArgs to the default arguments.
func contourArgsFromValues(values map[string]any) contourArgs {
	key, value := "contour.extraArgs", nestedValue(values, "contour", "extraArgs")
	if args := nestedValue(values, "contour", "args"); isTrue(args) {
		key, value = "contour.args", args
	}

	list, ok := argumentList(value)
	return contourArgs{key: key, args: list, unreadable: !ok}
}

// argumentList returns the list of arguments in value. The chart renders a string value as YAML, so a string that
// holds a YAML list is a list of arguments too. It returns false when value is set but is not a list.
func argumentList(value any) ([]any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, true
	case []any:
		return typed, true
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, true
		}

		var list []any
		if err := yaml.Unmarshal([]byte(typed), &list); err != nil {
			return nil, false
		}
		return list, true
	default:
		return nil, false
	}
}

// disabledFeatures returns the features that --disable-feature=<name> or --disable-feature <name> turns off.
func (a contourArgs) disabledFeatures() []string {
	var features []string
	for i := 0; i < len(a.args); i++ {
		arg, _ := a.args[i].(string)
		if feature, found := strings.CutPrefix(arg, "--disable-feature="); found {
			features = append(features, feature)
		} else if arg == "--disable-feature" && i+1 < len(a.args) {
			i++
			feature, _ := a.args[i].(string)
			features = append(features, feature)
		}
	}

	return features
}

// withDisabledFeatures returns a value in --set syntax that keeps the existing arguments and turns off features.
func (a contourArgs) withDisabledFeatures(features []string) string {
	items := make([]string, 0, len(a.args)+len(features))
	for _, arg := range a.args {
		items = append(items, setListItemEscaper.Replace(fmt.Sprint(arg)))
	}
	for _, feature := range features {
		items = append(items, "--disable-feature="+feature)
	}

	return fmt.Sprintf("%s={%s}", a.key, strings.Join(items, ","))
}

// setListItemEscaper escapes the characters that end an item of a list in --set syntax.
var setListItemEscaper = strings.NewReplacer(`\`, `\\`, `,`, `\,`, `}`, `\}`)

// helmValues returns the values that Helm renders the chart with on top of the chart's defaults: the values of this
// invocation, merged over the values stored on the release when Helm reuses them. reusedValues is nil otherwise.
func helmValues(values map[string]any, reusedValues map[string]any) map[string]any {
	merged := copyValues(values)
	if reusedValues != nil {
		// Helm's upgrade merges the stored values beneath the new values this way for ResetThenReuseValues.
		merged = util.CoalesceTables(merged, copyValues(reusedValues))
	}

	return merged
}

// effectiveContourValues returns the values that the chart's templates see, including the chart's defaults.
// reusedValues are the values stored on the existing release when Helm reuses them, or nil.
func effectiveContourValues(helmChart *chart.Chart, values map[string]any, reusedValues map[string]any) (map[string]any, error) {
	return util.CoalesceValues(helmChart, helmValues(values, reusedValues))
}

// copyValues deep-copies values, because Helm's CoalesceTables modifies both of its arguments.
func copyValues(values map[string]any) map[string]any {
	copied := make(map[string]any, len(values))
	for key, value := range values {
		copied[key] = copyValue(value)
	}

	return copied
}

func copyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return copyValues(typed)
	case []any:
		copied := make([]any, len(typed))
		for i, item := range typed {
			copied[i] = copyValue(item)
		}
		return copied
	default:
		return value
	}
}

// setDefaultValue sets the value at path unless values already set it or replaced one of its parent tables, for
// example with null. It reports whether it set the value.
func setDefaultValue(values map[string]any, value any, path ...string) bool {
	parent := values
	for _, key := range path[:len(path)-1] {
		child, found := parent[key]
		if !found {
			table := map[string]any{}
			parent[key] = table
			parent = table
			continue
		}

		table, ok := child.(map[string]any)
		if !ok {
			return false
		}
		parent = table
	}

	if _, found := parent[path[len(path)-1]]; found {
		return false
	}

	parent[path[len(path)-1]] = value
	return true
}

// setValue sets the value at path, replacing any value on the way that is not a table.
func setValue(values map[string]any, value any, path ...string) {
	parent := values
	for _, key := range path[:len(path)-1] {
		child, ok := parent[key].(map[string]any)
		if !ok {
			child = map[string]any{}
			parent[key] = child
		}
		parent = child
	}

	parent[path[len(path)-1]] = value
}

// nestedValue returns the value at path, or nil when it is not set.
func nestedValue(values map[string]any, path ...string) any {
	var value any = values
	for _, key := range path {
		table, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = table[key]
	}

	return value
}

// isTrue reports whether a chart template's if action treats value as true.
func isTrue(value any) bool {
	truth, _ := template.IsTrue(value)
	return truth
}

// joinWords joins words into an English list, for example "A, B, and C".
func joinWords(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	default:
		return strings.Join(words[:len(words)-1], ", ") + ", and " + words[len(words)-1]
	}
}
