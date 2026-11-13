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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// caInjectorConfigPin identifies immutable mounted input, not a local file or a
// caller assertion. No configuration contents or Secret data escape discovery.
type caInjectorConfigPin struct {
	Kind, Namespace, Name, Key string
	UID                        types.UID
	ResourceVersion, SHA256    string
}

// caInjectorInstance is observed configuration/readiness evidence, not proof of
// RBAC authorization, successful injection, or eventual exact-set convergence.
type caInjectorInstance struct {
	Namespace, Name, ServiceAccount, Image string
	UID                                    types.UID
	Generation                             int64
	PodUIDs                                []types.UID
	Config                                 *caInjectorConfigPin
	ConfigurationSHA256                    string
	WatchNamespace                         string
	IgnoreNamespaces                       []string
	APIServices, MergeCAs, Eligible        bool
	Exclusion                              string
}

type caInjectorConfiguration struct {
	APIVersion       string   `json:"apiVersion"`
	Kind             string   `json:"kind"`
	KubeConfig       string   `json:"kubeConfig,omitempty"`
	Namespace        string   `json:"namespace,omitempty"`
	IgnoreNamespaces []string `json:"ignoreNamespaces,omitempty"`
	EnableInjectable struct {
		APIServices                     *bool `json:"apiServices,omitempty"`
		ValidatingWebhookConfigurations *bool `json:"validatingWebhookConfigurations,omitempty"`
		MutatingWebhookConfigurations   *bool `json:"mutatingWebhookConfigurations,omitempty"`
		CustomResourceDefinitions       *bool `json:"customResourceDefinitions,omitempty"`
	} `json:"enableInjectableConfig,omitempty"`
	EnableDataSource struct {
		Certificates *bool `json:"certificates,omitempty"`
	} `json:"enableDataSourceConfig,omitempty"`
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
	Leader       struct {
		Enabled       *bool  `json:"enabled,omitempty"`
		Namespace     string `json:"namespace,omitempty"`
		LeaseDuration string `json:"leaseDuration,omitempty"`
		RenewDeadline string `json:"renewDeadline,omitempty"`
		RetryPeriod   string `json:"retryPeriod,omitempty"`
	} `json:"leaderElectionConfig,omitempty"`
	EnablePprof          bool   `json:"enablePprof,omitempty"`
	PprofAddress         string `json:"pprofAddress,omitempty"`
	MetricsListenAddress string `json:"metricsListenAddress,omitempty"`
}

var caInjectorImage = regexp.MustCompile(`(^|/)cert-manager-cainjector:v1\.(20|21)\.[0-9]+$`)

// inspectCAInjectorPrerequisites uses GET/LIST only. Unknown configuration,
// partial rollouts, orphan writers and multiple eligible Deployments fail closed.
// The supported config subset follows cert-manager v1.20/v1.21; CLI flags override
// a --config file, as in upstream loadConfigFromFile.
func inspectCAInjectorPrerequisites(ctx context.Context, client kubernetes.Interface, radiusNamespace string) ([]caInjectorInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("cainjector inspection canceled: %w", err)
	}
	if radiusNamespace == "" {
		return nil, fmt.Errorf("cainjector inspection requires the Radius namespace")
	}
	deployments, err := client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("cainjector inspection: list all Deployments: %w", err)
	}
	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("cainjector inspection: list all Pods: %w", err)
	}
	replicaSets, err := client.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("cainjector inspection: list all ReplicaSets: %w", err)
	}
	rsOwners := map[types.UID]types.UID{}
	for _, rs := range replicaSets.Items {
		if owner := metav1.GetControllerOf(&rs); owner != nil && owner.Kind == "Deployment" {
			rsOwners[rs.UID] = owner.UID
		}
	}
	var result []caInjectorInstance
	knownPods := map[types.UID]bool{}
	eligible := 0
	for _, d := range deployments.Items {
		if !caInjectorCandidate(d.Name, d.Labels, d.Spec.Template.Spec) {
			continue
		}
		instance, err := inspectCAInjectorDeployment(ctx, client, &d, pods.Items, rsOwners, knownPods, radiusNamespace)
		if err != nil {
			return nil, fmt.Errorf("cainjector Deployment %s/%s: %w; use an inspectable supported controller configuration and retry", d.Namespace, d.Name, err)
		}
		result = append(result, instance)
		if instance.Eligible {
			eligible++
		}
	}
	for _, pod := range pods.Items {
		if caInjectorCandidate(pod.Name, pod.Labels, pod.Spec) && !knownPods[pod.UID] {
			return nil, fmt.Errorf("cainjector Pod %s/%s is not pinned to a verified Deployment rollout; remove ambiguity before retrying", pod.Namespace, pod.Name)
		}
	}
	if eligible != 1 {
		return nil, fmt.Errorf("cainjector inspection found %d eligible Deployments among %d instances for Secret injection into the Radius APIService in namespace %q; exactly one verified writer configuration is required", eligible, len(result), radiusNamespace)
	}
	slices.SortFunc(result, func(a, b caInjectorInstance) int {
		return strings.Compare(a.Namespace+"/"+a.Name, b.Namespace+"/"+b.Name)
	})
	return result, nil
}

func caInjectorCandidate(name string, labels map[string]string, spec corev1.PodSpec) bool {
	if strings.Contains(strings.ToLower(name), "cainjector") || labels["app.kubernetes.io/component"] == "cainjector" {
		return true
	}
	for _, c := range append(slices.Clone(spec.Containers), spec.InitContainers...) {
		if strings.Contains(strings.ToLower(c.Image+" "+c.Name+" "+strings.Join(c.Command, " ")), "cainjector") {
			return true
		}
	}
	return false
}

func inspectCAInjectorDeployment(ctx context.Context, client kubernetes.Interface, d *appsv1.Deployment, pods []corev1.Pod, rsOwners map[types.UID]types.UID, knownPods map[types.UID]bool, namespace string) (caInjectorInstance, error) {
	result := caInjectorInstance{Namespace: d.Namespace, Name: d.Name, UID: d.UID, Generation: d.Generation}
	spec := d.Spec.Template.Spec
	if d.UID == "" || d.DeletionTimestamp != nil || len(spec.Containers) != 1 || len(spec.InitContainers) != 0 || spec.ServiceAccountName == "" {
		return result, fmt.Errorf("requires a live Deployment UID, explicit ServiceAccount and one unmodified container")
	}
	c := spec.Containers[0]
	if !caInjectorImage.MatchString(c.Image) || len(c.Command) > 1 || len(c.Command) == 1 && path.Base(c.Command[0]) != "cainjector" || len(c.EnvFrom) != 0 {
		return result, fmt.Errorf("unsupported image, command or environment; expected cert-manager-cainjector v1.20/v1.21 with no wrapper")
	}
	for _, e := range c.Env {
		if e.Name != "POD_NAMESPACE" || e.Value != "" || e.ValueFrom == nil || e.ValueFrom.FieldRef == nil || e.ValueFrom.FieldRef.FieldPath != "metadata.namespace" {
			return result, fmt.Errorf("unsupported environment override %q", e.Name)
		}
	}
	flags, err := caInjectorFlags(c.Args)
	if err != nil {
		return result, err
	}
	cfg := caInjectorConfiguration{}
	if filename := flags["config"]; filename != "" {
		data, pin, err := caInjectorConfigFile(ctx, client, d.Namespace, spec, c, filename)
		if err != nil {
			return result, err
		}
		if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
			return result, fmt.Errorf("unsupported --config format: %w", err)
		}
		decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
		var document json.RawMessage
		if err := decoder.Decode(&document); err != nil {
			return result, fmt.Errorf("invalid --config document: %w", err)
		}
		if err := decoder.Decode(&document); err != io.EOF {
			return result, fmt.Errorf("--config must contain exactly one configuration document")
		}
		if cfg.APIVersion != "cainjector.config.cert-manager.io/v1alpha1" || cfg.Kind != "CAInjectorConfiguration" {
			return result, fmt.Errorf("--config must be cainjector.config.cert-manager.io/v1alpha1 CAInjectorConfiguration")
		}
		result.Config = pin
	}
	if v, ok := flags["namespace"]; ok {
		cfg.Namespace = v
	}
	if v, ok := flags["ignore-namespaces"]; ok {
		if v == "" {
			cfg.IgnoreNamespaces = nil
		} else {
			cfg.IgnoreNamespaces, err = csv.NewReader(strings.NewReader(v)).Read()
			if err != nil {
				return result, fmt.Errorf("invalid --ignore-namespaces CSV: %w", err)
			}
		}
	}
	apiServices := cfg.EnableInjectable.APIServices == nil || *cfg.EnableInjectable.APIServices
	if v, ok := flags["enable-apiservices-injectable"]; ok {
		apiServices, err = strconv.ParseBool(v)
		if err != nil {
			return result, fmt.Errorf("invalid --enable-apiservices-injectable: %w", err)
		}
	}
	if cfg.KubeConfig != "" || flags["kubeconfig"] != "" {
		return result, fmt.Errorf("out-of-cluster kubeconfig makes the target cluster unverifiable")
	}
	if cfg.Namespace != "" && len(cfg.IgnoreNamespaces) != 0 {
		return result, fmt.Errorf("--namespace and ignoreNamespaces cannot both be set")
	}
	merge := true
	for name, value := range cfg.FeatureGates {
		switch name {
		case "CAInjectorMerging":
			merge = value
		case "ServerSideApply":
		default:
			return result, fmt.Errorf("unknown config feature gate %q", name)
		}
	}
	if v, ok := flags["feature-gates"]; ok {
		seen := map[string]bool{}
		for _, gate := range strings.Split(v, ",") {
			name, value, found := strings.Cut(gate, "=")
			enabled, err := strconv.ParseBool(value)
			if !found || err != nil || seen[name] || (name != "CAInjectorMerging" && name != "ServerSideApply") {
				return result, fmt.Errorf("unknown or malformed feature gate %q", gate)
			}
			seen[name] = true
			if name == "CAInjectorMerging" {
				merge = enabled
			}
		}
	}
	replicas := int32(1)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	if replicas < 1 || d.Status.ObservedGeneration != d.Generation || d.Status.Replicas != replicas || d.Status.UpdatedReplicas != replicas || d.Status.ReadyReplicas != replicas || d.Status.AvailableReplicas != replicas {
		return result, fmt.Errorf("Deployment must have a complete current Ready rollout")
	}
	for _, pod := range pods {
		owner := metav1.GetControllerOf(&pod)
		if owner == nil || owner.Kind != "ReplicaSet" || rsOwners[owner.UID] != d.UID || pod.Namespace != d.Namespace {
			continue
		}
		knownPods[pod.UID] = true
		ready := false
		for _, condition := range pod.Status.Conditions {
			ready = ready || condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue
		}
		if pod.UID == "" || !ready || pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning ||
			!caInjectorPodMatches(spec, pod.Spec) {
			return result, fmt.Errorf("Pod %s is not Running/Ready with the pinned template configuration", pod.Name)
		}
		result.PodUIDs = append(result.PodUIDs, pod.UID)
	}
	if int32(len(result.PodUIDs)) != replicas {
		return result, fmt.Errorf("Ready owned Pod inventory does not match the Deployment")
	}
	slices.Sort(result.PodUIDs)
	pin, err := json.Marshal(struct {
		Container corev1.Container
		Config    *caInjectorConfigPin
	}{c, result.Config})
	if err != nil {
		return result, fmt.Errorf("encode configuration pin: %w", err)
	}
	result.ConfigurationSHA256 = fmt.Sprintf("%x", sha256.Sum256(pin))
	result.ServiceAccount, result.Image = spec.ServiceAccountName, c.Image
	result.WatchNamespace, result.IgnoreNamespaces = cfg.Namespace, slices.Clone(cfg.IgnoreNamespaces)
	result.APIServices, result.MergeCAs = apiServices, merge
	switch {
	case !apiServices:
		result.Exclusion = "APIService injection is disabled"
	case cfg.Namespace != "" && cfg.Namespace != namespace:
		result.Exclusion = "watches another namespace"
	case slices.Contains(cfg.IgnoreNamespaces, namespace):
		result.Exclusion = "ignores the Radius namespace"
	default:
		result.Eligible = true
	}
	return result, nil
}

func caInjectorPodMatches(template, pod corev1.PodSpec) bool {
	if pod.ServiceAccountName != template.ServiceAccountName || len(pod.Containers) != 1 || len(pod.InitContainers) != 0 {
		return false
	}
	actual := pod.DeepCopy()
	// The ServiceAccount admission plugin adds a token mount to Pods, not Deployments.
	for _, v := range pod.Volumes {
		if !strings.HasPrefix(v.Name, "kube-api-access-") || v.Projected == nil || len(v.Projected.Sources) != 3 ||
			v.Projected.Sources[0].ServiceAccountToken == nil || v.Projected.Sources[1].ConfigMap == nil ||
			v.Projected.Sources[1].ConfigMap.Name != "kube-root-ca.crt" || v.Projected.Sources[2].DownwardAPI == nil {
			continue
		}
		actual.Volumes = slices.DeleteFunc(actual.Volumes, func(volume corev1.Volume) bool { return volume.Name == v.Name })
		actual.Containers[0].VolumeMounts = slices.DeleteFunc(actual.Containers[0].VolumeMounts, func(m corev1.VolumeMount) bool {
			return m.Name == v.Name && m.MountPath == "/var/run/secrets/kubernetes.io/serviceaccount" && m.ReadOnly && m.SubPath == "" && m.SubPathExpr == ""
		})
	}
	return equality.Semantic.DeepEqual(actual.Containers, template.Containers) && equality.Semantic.DeepEqual(actual.Volumes, template.Volumes)
}

func caInjectorFlags(args []string) (map[string]string, error) {
	flags := map[string]string{}
	bools := []string{"enable-apiservices-injectable", "enable-certificates-data-source", "enable-validatingwebhookconfigurations-injectable", "enable-mutatingwebhookconfigurations-injectable", "enable-customresourcedefinitions-injectable", "leader-elect", "enable-profiling"}
	stringsOnly := []string{"config", "namespace", "ignore-namespaces", "kubeconfig", "feature-gates", "leader-election-namespace", "leader-election-lease-duration", "leader-election-renew-deadline", "leader-election-retry-period", "v", "logging-format", "metrics-listen-address", "profiler-address"}
	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if strings.HasPrefix(arg, "-v=") {
			arg = "-" + arg
		}
		if !strings.HasPrefix(arg, "--") {
			return nil, fmt.Errorf("unsupported cainjector argument %q", arg)
		}
		name, value, found := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if _, duplicate := flags[name]; duplicate {
			return nil, fmt.Errorf("duplicate cainjector flag --%s", name)
		}
		if slices.Contains(bools, name) {
			if !found {
				value = "true"
			}
			if _, err := strconv.ParseBool(value); err != nil {
				return nil, fmt.Errorf("invalid boolean flag --%s: %w", name, err)
			}
		} else if slices.Contains(stringsOnly, name) {
			if !found && idx+1 < len(args) && !strings.HasPrefix(args[idx+1], "-") {
				idx++
				value, found = args[idx], true
			}
			if !found {
				return nil, fmt.Errorf("missing value for cainjector --%s", name)
			}
		} else {
			return nil, fmt.Errorf("unknown cainjector flag --%s", name)
		}
		if strings.Contains(value, "$") && !(name == "leader-election-namespace" && value == "$(POD_NAMESPACE)") {
			return nil, fmt.Errorf("unverifiable environment expansion in --%s", name)
		}
		flags[name] = value
	}
	return flags, nil
}

func caInjectorConfigFile(ctx context.Context, client kubernetes.Interface, namespace string, spec corev1.PodSpec, container corev1.Container, filename string) ([]byte, *caInjectorConfigPin, error) {
	if !path.IsAbs(filename) || path.Clean(filename) != filename {
		return nil, nil, fmt.Errorf("--config must be an absolute clean path to an immutable ConfigMap or Secret")
	}
	var selected *corev1.VolumeMount
	for _, m := range container.VolumeMounts {
		if filename == m.MountPath || strings.HasPrefix(filename, strings.TrimSuffix(m.MountPath, "/")+"/") {
			if selected != nil {
				return nil, nil, fmt.Errorf("--config has ambiguous overlapping mounts")
			}
			copy := m
			selected = &copy
		}
	}
	if selected == nil || selected.SubPathExpr != "" {
		return nil, nil, fmt.Errorf("--config is not an inspectable mounted ConfigMap/Secret file")
	}
	keyPath := strings.TrimPrefix(strings.TrimPrefix(filename, selected.MountPath), "/")
	if selected.SubPath != "" {
		if filename != selected.MountPath {
			return nil, nil, fmt.Errorf("--config subPath must mount the exact file")
		}
		keyPath = selected.SubPath
	}
	for _, volume := range spec.Volumes {
		if volume.Name != selected.Name {
			continue
		}
		pin := &caInjectorConfigPin{Namespace: namespace}
		var items []corev1.KeyToPath
		var data map[string][]byte
		switch {
		case volume.ConfigMap != nil:
			cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, volume.ConfigMap.Name, metav1.GetOptions{})
			if err != nil {
				return nil, nil, fmt.Errorf("read cainjector --config ConfigMap: %w", err)
			}
			if cm.Immutable == nil || !*cm.Immutable || cm.UID == "" {
				return nil, nil, fmt.Errorf("--config ConfigMap must be immutable with a live UID to pin the startup bytes")
			}
			pin.Kind, pin.Name, pin.UID, pin.ResourceVersion = "ConfigMap", cm.Name, cm.UID, cm.ResourceVersion
			data = map[string][]byte{}
			for k, v := range cm.Data {
				data[k] = []byte(v)
			}
			for k, v := range cm.BinaryData {
				data[k] = v
			}
			items = volume.ConfigMap.Items
		case volume.Secret != nil:
			secret, err := client.CoreV1().Secrets(namespace).Get(ctx, volume.Secret.SecretName, metav1.GetOptions{})
			if err != nil {
				return nil, nil, fmt.Errorf("read cainjector --config Secret: %w", err)
			}
			if secret.Immutable == nil || !*secret.Immutable || secret.UID == "" {
				return nil, nil, fmt.Errorf("--config Secret must be immutable with a live UID to pin the startup bytes")
			}
			pin.Kind, pin.Name, pin.UID, pin.ResourceVersion = "Secret", secret.Name, secret.UID, secret.ResourceVersion
			data, items = secret.Data, volume.Secret.Items
		default:
			return nil, nil, fmt.Errorf("--config volume %q uses an unsupported source; use an immutable direct ConfigMap/Secret volume", volume.Name)
		}
		key := keyPath
		if len(items) > 0 {
			key = ""
			for _, item := range items {
				if item.Path == keyPath {
					if key != "" {
						return nil, nil, fmt.Errorf("--config has duplicate projected paths")
					}
					key = item.Key
				}
			}
		}
		contents, ok := data[key]
		if key == "" || !ok {
			return nil, nil, fmt.Errorf("--config mounted key %q is missing", keyPath)
		}
		pin.Key, pin.SHA256 = key, fmt.Sprintf("%x", sha256.Sum256(contents))
		return contents, pin, nil
	}
	return nil, nil, fmt.Errorf("--config volume %q is missing", selected.Name)
}
