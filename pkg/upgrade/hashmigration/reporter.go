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

package hashmigration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/radius-project/radius/pkg/kubeutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// fieldManager is the server-side field manager used when recording status.
const fieldManager = "radius-sha256-migration"

// Reporter records the status of a migration step.
type Reporter interface {
	// Report records status under key.
	Report(ctx context.Context, key string, status Status) error
}

// ConfigMapReporter records step status as JSON values in the ConfigMap named ConfigMapName. Each component writes only
// its own keys with a merge patch, so components do not overwrite each other.
type ConfigMapReporter struct {
	client    kubernetes.Interface
	namespace string
}

var _ Reporter = (*ConfigMapReporter)(nil)

// NewConfigMapReporter creates a ConfigMapReporter for namespace.
func NewConfigMapReporter(client kubernetes.Interface, namespace string) *ConfigMapReporter {
	return &ConfigMapReporter{client: client, namespace: namespace}
}

// NewKubernetesReporter returns a ConfigMapReporter when the component can build a Kubernetes client from in-cluster or
// local configuration, and nil otherwise so that the migration only logs its progress.
func NewKubernetesReporter() Reporter {
	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{QPS: kubeutil.DefaultServerQPS, Burst: kubeutil.DefaultServerBurst})
	if err != nil {
		return nil
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil
	}

	return NewConfigMapReporter(client, DefaultNamespace)
}

// Report implements Reporter.
func (r *ConfigMapReporter) Report(ctx context.Context, key string, status Status) error {
	value, err := json.Marshal(status)
	if err != nil {
		return err
	}

	patch, err := json.Marshal(map[string]any{"data": map[string]string{key: string(value)}})
	if err != nil {
		return err
	}

	configMaps := r.client.CoreV1().ConfigMaps(r.namespace)
	for range 3 {
		_, err = configMaps.Patch(ctx, ConfigMapName, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: fieldManager})
		if !apierrors.IsNotFound(err) {
			return err
		}

		_, err = configMaps.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConfigMapName,
				Namespace: r.namespace,
				Labels:    map[string]string{"app.kubernetes.io/part-of": "radius"},
			},
			Data: map[string]string{key: string(value)},
		}, metav1.CreateOptions{FieldManager: fieldManager})
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	return fmt.Errorf("unable to record status in ConfigMap %s/%s", r.namespace, ConfigMapName)
}

// ReadStatus returns the recorded status of every step, keyed by StatusKey. It returns an empty map when no status has
// been recorded.
func ReadStatus(ctx context.Context, client kubernetes.Interface, namespace string) (map[string]Status, error) {
	configMap, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, ConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return map[string]Status{}, nil
	} else if err != nil {
		return nil, err
	}

	result := map[string]Status{}
	for key, value := range configMap.Data {
		status := Status{}
		if err := json.Unmarshal([]byte(value), &status); err != nil {
			return nil, fmt.Errorf("invalid status %q in ConfigMap %s/%s: %w", key, namespace, ConfigMapName, err)
		}
		result[key] = status
	}

	return result, nil
}
