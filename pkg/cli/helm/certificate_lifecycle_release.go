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

	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type certificateReleaseCAS struct {
	typedcorev1.SecretInterface
	before *corev1.Secret
	check  func(context.Context) error
	ctx    context.Context
}

func (c certificateReleaseCAS) Get(_ context.Context, name string, options metav1.GetOptions) (*corev1.Secret, error) {
	if err := c.check(c.ctx); err != nil {
		return nil, err
	}
	live, err := c.SecretInterface.Get(c.ctx, name, options)
	if err != nil {
		return nil, err
	}
	if name != c.before.Name || live.UID != c.before.UID || live.ResourceVersion != c.before.ResourceVersion ||
		!reflect.DeepEqual(live.Data, c.before.Data) {
		return nil, errors.New("pending certificate Helm revision changed; refusing recovery")
	}
	return live, nil
}

func (c certificateReleaseCAS) Update(_ context.Context, secret *corev1.Secret, options metav1.UpdateOptions) (*corev1.Secret, error) {
	if _, err := c.Get(c.ctx, secret.Name, metav1.GetOptions{}); err != nil {
		return nil, err
	}
	secret.UID, secret.ResourceVersion = c.before.UID, c.before.ResourceVersion
	return c.SecretInterface.Update(c.ctx, secret, options)
}

// Only this transaction's exact pending upgrade is recoverable. Reacquiring
// the shared Lease fences the interrupted CLI; raw Secret CAS fences history.
func (w *certificateExecutionWriter) recoverPendingFinalRelease(ctx context.Context, previous *releasev1.Release) error {
	if w.state.Execution == nil || w.state.Phase != rotationComplete ||
		previous == nil || revisionStatus(previous) != releasecommon.StatusPendingUpgrade ||
		previous.Name != w.store.release || previous.Namespace != w.store.namespace ||
		previous.Version <= w.state.Execution.BaselineVersion || previous.Labels[certificateExecutionOwner] != w.state.ID {
		return errors.New("only this CA execution's pending final upgrade can be recovered")
	}
	chartHash, err := bootstrapDigest(previous.Chart)
	if err != nil {
		return err
	}
	valuesHash, err := bootstrapDigest(previous.Config)
	if err != nil || chartHash != w.state.Execution.ChartSHA256 || valuesHash != w.state.Execution.TargetValuesSHA256 {
		return errors.New("pending final upgrade differs from the pinned chart or values")
	}
	if err := w.check(ctx); err != nil {
		return err
	}
	name := fmt.Sprintf("sh.helm.release.v1.%s.v%d", w.store.release, previous.Version)
	client := w.store.kube.CoreV1().Secrets(w.store.namespace)
	before, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if before.UID == "" || before.ResourceVersion == "" || before.DeletionTimestamp != nil {
		return errors.New("pending final upgrade has no stable storage identity")
	}
	storage := driver.NewSecrets(certificateReleaseCAS{SecretInterface: client, before: before, check: w.check, ctx: ctx})
	recorded, err := storage.Get(name)
	if err != nil {
		return err
	}
	expected, err := bootstrapDigest(previous)
	if err != nil {
		return err
	}
	actual, err := bootstrapDigest(recorded)
	if err != nil || actual != expected {
		return errors.New("pending final upgrade changed before recovery")
	}
	recovered := *previous
	info := *previous.Info
	info.Status = releasecommon.StatusFailed
	info.Description = "Interrupted CA lifecycle final reconciliation; retry under the reacquired identity Lease."
	recovered.Info = &info
	return storage.Update(name, &recovered)
}
