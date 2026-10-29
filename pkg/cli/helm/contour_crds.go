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
	"slices"
	"strings"
	"text/template"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	clikubernetes "github.com/radius-project/radius/pkg/cli/kubernetes"
	"helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	gatewayAPIGroup = "gateway.networking.k8s.io"

	// Helm adopts an existing resource into a release only when it has this ownership metadata.
	helmManagedByLabel             = "app.kubernetes.io/managed-by"
	helmManagedByValue             = "Helm"
	helmReleaseNameAnnotation      = "meta.helm.sh/release-name"
	helmReleaseNamespaceAnnotation = "meta.helm.sh/release-namespace"

	crdListPageSize = 100

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

// contourGatewayAPISupport describes how a Contour version uses Gateway API CRDs.
type contourGatewayAPISupport struct {
	// bundledCRDs are the Gateway API CRDs that the Contour chart installs when gatewayAPI.manageCRDs is true.
	bundledCRDs []string
	// requirements are the resources that Contour watches when a Gateway is configured. Contour exits when one of
	// them is missing.
	requirements []gatewayAPIRequirement
}

// contourGatewayAPISupportByVersion is keyed by Contour version, which is the Contour chart's appVersion. When a new
// Contour version is added, take bundledCRDs from
// https://github.com/projectcontour/contour/blob/v<version>/examples/gateway/00-crds.yaml, which the chart installs,
// and requirements from setupGatewayAPI in
// https://github.com/projectcontour/contour/blob/v<version>/cmd/contour/serve.go.
var contourGatewayAPISupportByVersion = map[string]contourGatewayAPISupport{
	// Contour 1.32.0 bundles the experimental channel of Gateway API v1.2.1.
	"1.32.0": {
		bundledCRDs: []string{
			"backendlbpolicies.gateway.networking.k8s.io",
			"backendtlspolicies.gateway.networking.k8s.io",
			"gatewayclasses.gateway.networking.k8s.io",
			"gateways.gateway.networking.k8s.io",
			"grpcroutes.gateway.networking.k8s.io",
			"httproutes.gateway.networking.k8s.io",
			"referencegrants.gateway.networking.k8s.io",
			"tcproutes.gateway.networking.k8s.io",
			"tlsroutes.gateway.networking.k8s.io",
			"udproutes.gateway.networking.k8s.io",
		},
		requirements: []gatewayAPIRequirement{
			{crd: "gatewayclasses.gateway.networking.k8s.io", version: "v1", kind: "GatewayClass"},
			{crd: "gateways.gateway.networking.k8s.io", version: "v1", kind: "Gateway"},
			{crd: "httproutes.gateway.networking.k8s.io", version: "v1", kind: "HTTPRoute"},
			{crd: "referencegrants.gateway.networking.k8s.io", version: "v1beta1", kind: "ReferenceGrant"},
			{crd: "grpcroutes.gateway.networking.k8s.io", version: "v1", kind: "GRPCRoute", feature: "grpcroutes"},
			{crd: "tlsroutes.gateway.networking.k8s.io", version: "v1alpha2", kind: "TLSRoute", feature: "tlsroutes"},
			{crd: "tcproutes.gateway.networking.k8s.io", version: "v1alpha2", kind: "TCPRoute", feature: "tcproutes"},
			{crd: "backendtlspolicies.gateway.networking.k8s.io", version: "v1alpha3", kind: "BackendTLSPolicy", feature: "backendtlspolicies"},
		},
	},
}

// gatewayAPICRD is a Gateway API CRD in the cluster.
type gatewayAPICRD struct {
	name           string
	labels         map[string]string
	annotations    map[string]string
	servedVersions []string
}

// ownedBy reports whether Helm can adopt the CRD into the given release.
func (c gatewayAPICRD) ownedBy(releaseName string, releaseNamespace string) bool {
	return c.labels[helmManagedByLabel] == helmManagedByValue &&
		c.annotations[helmReleaseNameAnnotation] == releaseName &&
		c.annotations[helmReleaseNamespaceAnnotation] == releaseNamespace
}

// owner describes who manages the CRD.
func (c gatewayAPICRD) owner() string {
	if release := c.annotations[helmReleaseNameAnnotation]; release != "" {
		return fmt.Sprintf("Helm release %q in namespace %q", release, c.annotations[helmReleaseNamespaceAnnotation])
	}
	if managedBy := c.labels[helmManagedByLabel]; managedBy != "" {
		return fmt.Sprintf("%q", managedBy)
	}

	return "an installer other than Helm"
}

// listClusterGatewayAPICRDs lists the Gateway API CRDs in the cluster that kubeContext selects.
func listClusterGatewayAPICRDs(ctx context.Context, kubeContext string) ([]gatewayAPICRD, error) {
	config, err := clikubernetes.NewCLIClientConfig(kubeContext)
	if err != nil {
		return nil, err
	}

	client, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return listGatewayAPICRDsWithClient(ctx, client)
}

// listGatewayAPICRDsWithClient lists CRDs in pages so that clusters with many large CRDs are not loaded at once.
func listGatewayAPICRDsWithClient(ctx context.Context, client apiextensionsclient.Interface) ([]gatewayAPICRD, error) {
	var crds []gatewayAPICRD
	options := metav1.ListOptions{Limit: crdListPageSize}
	for {
		list, err := client.ApiextensionsV1().CustomResourceDefinitions().List(ctx, options)
		if err != nil {
			return nil, err
		}

		for _, item := range list.Items {
			if item.Spec.Group != gatewayAPIGroup {
				continue
			}

			crd := gatewayAPICRD{name: item.Name, labels: item.Labels, annotations: item.Annotations}
			for _, version := range item.Spec.Versions {
				if version.Served {
					crd.servedVersions = append(crd.servedVersions, version.Name)
				}
			}
			crds = append(crds, crd)
		}

		if list.Continue == "" {
			return crds, nil
		}
		options.Continue = list.Continue
	}
}

// gatewayAPICRDsInManifest returns the Gateway API CRDs in a Helm release manifest.
func gatewayAPICRDsInManifest(manifest string) ([]string, error) {
	var names []string
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var object struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				Group string `json:"group"`
			} `json:"spec"`
		}
		err := decoder.Decode(&object)
		if errors.Is(err, io.EOF) {
			return names, nil
		}
		if err != nil {
			return nil, err
		}

		if object.Kind == "CustomResourceDefinition" && object.Spec.Group == gatewayAPIGroup {
			names = append(names, object.Metadata.Name)
		}
	}
}

// gatewayAPICRDCheck decides whether a Contour install or upgrade may manage the Gateway API CRDs in a cluster.
type gatewayAPICRDCheck struct {
	releaseName      string
	releaseNamespace string

	// contourVersion is the version of Contour that the chart deploys.
	contourVersion string

	// support is nil when Radius does not know how contourVersion uses Gateway API CRDs.
	support *contourGatewayAPISupport

	// crds are the Gateway API CRDs in the cluster.
	crds []gatewayAPICRD

	// releaseCRDs are the Gateway API CRDs in the manifest of the existing Contour release. Helm deletes them when an
	// upgrade stops rendering them.
	releaseCRDs []string

	// command is the rad command that installs or upgrades Contour, which the errors tell the user to run again.
	command string

	// resetValues is true when an upgrade discards the values stored on the Contour release.
	resetValues bool

	// manageCRDsFromValues is true when the Contour values, not Radius, decide gatewayAPI.manageCRDs.
	manageCRDsFromValues bool
}

// setManageCRDsDefault sets gatewayAPI.manageCRDs in values to the Radius default unless the Contour values already
// decide it, and records which of them decides.
func (c *gatewayAPICRDCheck) setManageCRDsDefault(values map[string]any) {
	c.manageCRDsFromValues = !setDefaultValue(values, c.defaultManageCRDs(), "gatewayAPI", "manageCRDs")
}

// contourCRDs returns the Gateway API CRDs that the Contour release installed or that Helm can adopt into it.
func (c gatewayAPICRDCheck) contourCRDs() []string {
	names := slices.Clone(c.releaseCRDs)
	for _, crd := range c.crds {
		if crd.ownedBy(c.releaseName, c.releaseNamespace) && !slices.Contains(names, crd.name) {
			names = append(names, crd.name)
		}
	}
	slices.Sort(names)

	return names
}

// conflicts returns the Gateway API CRDs that the chart would install or update but that belong to something else.
// Helm leaves other Gateway API CRDs alone.
func (c gatewayAPICRDCheck) conflicts() []gatewayAPICRD {
	var conflicts []gatewayAPICRD
	for _, crd := range c.crds {
		if crd.ownedBy(c.releaseName, c.releaseNamespace) {
			continue
		}

		// Without the list of CRDs that the chart installs, any Gateway API CRD could conflict.
		if c.support == nil || slices.Contains(c.support.bundledCRDs, crd.name) || slices.Contains(c.releaseCRDs, crd.name) {
			conflicts = append(conflicts, crd)
		}
	}

	return conflicts
}

// defaultManageCRDs returns the gatewayAPI.manageCRDs value to use when the user does not set it. Contour keeps
// managing the CRDs that it manages, reuses CRDs that belong to something else, and installs them in clusters that have
// none.
func (c gatewayAPICRDCheck) defaultManageCRDs() bool {
	return len(c.contourCRDs()) > 0 || len(c.conflicts()) == 0
}

// validate returns an error when installing or upgrading Contour with values would take over or delete Gateway API
// CRDs, or when Contour cannot use the Gateway API CRDs that it does not install. Callers validate before they change
// the cluster, and the errors say so.
func (c gatewayAPICRDCheck) validate(values map[string]any) (warning string, err error) {
	if isTrue(nestedValue(values, "gatewayAPI", "manageCRDs")) {
		if conflicts := c.conflicts(); len(conflicts) > 0 {
			return "", c.conflictError(conflicts)
		}

		return "", nil
	}

	if len(c.releaseCRDs) > 0 {
		return "", c.deleteError()
	}

	if c.support == nil {
		if c.contourVersion == "" {
			return "The Contour chart does not set appVersion, so Radius did not check that Contour can use the Gateway API CRDs in the cluster.", nil
		}
		return fmt.Sprintf("Radius does not know which Gateway API CRD versions Contour %s needs, so it did not check the Gateway API CRDs in the cluster.", c.contourVersion), nil
	}

	args := contourArgsFromValues(values)
	if unmet := c.unmetRequirements(args.disabledFeatures()); len(unmet) > 0 {
		return "", c.requirementsError(unmet, args)
	}

	return "", nil
}

// unmetRequirement is a resource version that Contour watches but that no CRD in the cluster serves.
type unmetRequirement struct {
	gatewayAPIRequirement

	// installed is the CRD that defines the resource, or nil when the CRD is not installed.
	installed *gatewayAPICRD
}

func (c gatewayAPICRDCheck) unmetRequirements(disabledFeatures []string) []unmetRequirement {
	var unmet []unmetRequirement
	for _, requirement := range c.support.requirements {
		if requirement.feature != "" && slices.Contains(disabledFeatures, requirement.feature) {
			continue
		}

		index := slices.IndexFunc(c.crds, func(crd gatewayAPICRD) bool { return crd.name == requirement.crd })
		if index < 0 {
			unmet = append(unmet, unmetRequirement{gatewayAPIRequirement: requirement})
			continue
		}

		if !slices.Contains(c.crds[index].servedVersions, requirement.version) {
			unmet = append(unmet, unmetRequirement{gatewayAPIRequirement: requirement, installed: &c.crds[index]})
		}
	}

	return unmet
}

func (c gatewayAPICRDCheck) conflictError(conflicts []gatewayAPICRD) error {
	var message strings.Builder
	contourCRDs := c.contourCRDs()
	if len(contourCRDs) > 0 {
		fmt.Fprintf(&message, "Radius stopped before changing the cluster because the Gateway API CRDs in this cluster have more than one owner.\n")
		fmt.Fprintf(&message, "Contour release %q in namespace %q manages these CRDs:\n", c.releaseName, c.releaseNamespace)
		for _, name := range contourCRDs {
			fmt.Fprintf(&message, "  - %s\n", name)
		}
		fmt.Fprintf(&message, "These CRDs belong to something else:\n")
	} else {
		fmt.Fprintf(&message, "Radius stopped before changing the cluster because these Gateway API CRDs already exist and belong to something else, so Contour cannot install them:\n")
	}

	for _, crd := range conflicts {
		fmt.Fprintf(&message, "  - %s (owned by %s)\n", crd.name, crd.owner())
	}

	if len(contourCRDs) > 0 {
		message.WriteString("Radius does not take over CRDs that belong to something else, and it does not stop managing the CRDs that Contour installed because Helm would delete them. Make a single installer responsible for these CRDs, then run the command again.")
	} else {
		message.WriteString("Radius does not take over CRDs that belong to something else. Remove the gatewayAPI.manageCRDs=true Contour value so that Contour uses the existing CRDs.")
	}

	return clierrors.Message("%s", message.String())
}

func (c gatewayAPICRDCheck) deleteError() error {
	var message strings.Builder
	fmt.Fprintf(&message, "Radius stopped before changing the cluster because Contour release %q in namespace %q installed these Gateway API CRDs:\n", c.releaseName, c.releaseNamespace)
	for _, name := range c.releaseCRDs {
		fmt.Fprintf(&message, "  - %s\n", name)
	}
	message.WriteString("Setting gatewayAPI.manageCRDs to false would make Helm delete these CRDs, and every Gateway API resource in the cluster with them. Remove the gatewayAPI.manageCRDs=false Contour value.")

	return clierrors.Message("%s", message.String())
}

func (c gatewayAPICRDCheck) requirementsError(unmet []unmetRequirement, args contourArgs) error {
	var message strings.Builder
	fmt.Fprintf(&message, "Radius stopped before changing the cluster because Contour %s cannot use the Gateway API CRDs in this cluster. Contour needs these resource versions:\n", c.contourVersion)
	var features, kinds []string
	for _, requirement := range unmet {
		status := "the CRD is not installed"
		if requirement.installed != nil {
			status = "the installed CRD serves no versions"
			if served := requirement.installed.servedVersions; len(served) > 0 {
				status = "the installed CRD serves " + strings.Join(served, ", ")
			}
		}
		fmt.Fprintf(&message, "  - %s %s (%s)\n", requirement.crd, requirement.version, status)

		if requirement.feature != "" {
			features = append(features, requirement.feature)
			kinds = append(kinds, requirement.kind)
		}
	}

	// When Radius chose not to install the CRDs, other CRDs are in the way. Otherwise the Contour values chose it.
	owners := c.foreignOwners()
	if c.manageCRDsFromValues {
		message.WriteString("Contour does not install Gateway API CRDs because the Contour values set gatewayAPI.manageCRDs to false.\n")
		if len(owners) > 0 {
			fmt.Fprintf(&message, "The Gateway API CRDs in this cluster belong to %s.\n", joinWords(owners))
		}
	} else {
		fmt.Fprintf(&message, "The Gateway API CRDs in this cluster belong to %s, so Contour uses them instead of installing its own.\n", joinWords(owners))
	}

	var remedies []string
	if c.manageCRDsFromValues && c.defaultManageCRDs() {
		remedies = append(remedies, "Remove the Contour value that sets gatewayAPI.manageCRDs to false, so that Contour installs the Gateway API CRDs that it needs.")
	}
	remedies = append(remedies, "Install Gateway API CRDs that serve these versions.")
	if len(features) == len(unmet) {
		remedies = append(remedies, c.turnOffFeaturesRemedy(features, kinds, args))
	}
	if !c.manageCRDsFromValues {
		remedies = append(remedies, `Use a cluster without preinstalled Gateway API CRDs. For example, create the k3d cluster with --k3s-arg "--disable=traefik@server:*".`)
	}

	message.WriteString("To continue, do one of the following:")
	for _, remedy := range remedies {
		message.WriteString("\n  - " + remedy)
	}

	return clierrors.Message("%s", message.String())
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

	fmt.Fprintf(&remedy, " Add this flag to the %s command:\n      --contour-set '%s'", c.command, args.withDisabledFeatures(features))
	if c.resetValues {
		remedy.WriteString("\n    This upgrade uses --reset-values, which discards the values stored on the Contour release. If you set this value when you installed Radius, you can instead run the upgrade without --reset-values.")
	}
	if c.command == installCommand {
		remedy.WriteString("\n    If you used rad init, run rad install kubernetes with this flag first, and then run rad init again.")
	}

	return remedy.String()
}

// foreignOwners describes who owns the Gateway API CRDs that do not belong to the Contour release.
func (c gatewayAPICRDCheck) foreignOwners() []string {
	var owners []string
	for _, crd := range c.crds {
		if owner := crd.owner(); !crd.ownedBy(c.releaseName, c.releaseNamespace) && !slices.Contains(owners, owner) {
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

// effectiveContourValues returns the values that Helm renders the chart with. reusedValues are the values stored on
// the existing release when Helm reuses them, or nil.
func effectiveContourValues(helmChart *chart.Chart, values map[string]any, reusedValues map[string]any) (map[string]any, error) {
	merged := copyValues(values)
	if reusedValues != nil {
		// Helm's upgrade merges the stored values beneath the new values this way for ResetThenReuseValues.
		merged = util.CoalesceTables(merged, copyValues(reusedValues))
	}

	return util.CoalesceValues(helmChart, merged)
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
