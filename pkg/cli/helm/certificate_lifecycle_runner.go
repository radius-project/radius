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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"time"

	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type certificateLifecycleExecutor struct {
	writer      *certificateExecutionWriter
	impl        *Impl
	options     ClusterOptions
	selected    *chart.Chart
	target      *certificateTarget
	imports     *certificatePlan
	history     []*releasev1.Release
	kubeContext string

	// Host fixtures replace operations, not observation authority in production.
	applyPhaseFixture func(context.Context) error
	commitFixture     func(context.Context) (int, error)
}

func (e *certificateLifecycleExecutor) run(ctx context.Context, requested rotationPhase) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w := e.writer
	if w == nil || w.observeLoaded == nil || w.observeServing == nil {
		return errCertificateObserverUnavailable
	}
	if !slices.Contains([]rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationRetire}, requested) {
		return errors.New("token/CSI lifecycle supports publish, adopt, issue and explicit retire; legacy outage recovery is not a token/CSI protocol")
	}
	ctx, cancel := context.WithTimeout(ctx, effectiveTimeout(e.options.Radius.Timeout))
	defer cancel()
	return withIdentityOperation(ctx, w.store.kube, w.store.namespace, func(operation *identityOperation) error {
		w.operation = operation
		if w.journal == nil {
			if requested != rotationPublish {
				return errors.New("begin a new certificate lifecycle with publish")
			}
			if err := w.save(ctx, w.state); err != nil {
				return err
			}
		} else {
			recorded, err := decodeCertificateLifecycle(w.journal, w.store.namespace, w.store.release, w.store.now())
			if err != nil {
				return err
			}
			if recorded.ID != w.state.ID {
				if requested != rotationPublish || recorded.Phase != rotationComplete ||
					recorded.Execution == nil || recorded.Execution.FinalReleaseVersion == 0 {
					return errors.New("only a fully reconciled lifecycle can be replaced by a new publish intent")
				}
				if err := w.store.checkCertificateLifecycleRecovery(ctx, w.journal); err != nil {
					return err
				}
				if err := w.save(ctx, w.state); err != nil {
					return err
				}
			}
		}
		if err := w.check(ctx); err != nil {
			return err
		}
		if w.state.Phase == rotationComplete {
			return e.finish(ctx)
		}
		if slices.Index(rotationPhases, w.state.Phase) > slices.Index(rotationPhases, requested) {
			return w.check(ctx)
		}
		if requested == rotationPublish && w.state.Phase == certificatePrepare {
			if err := w.prepareTarget(ctx, e.target, e.imports, e.impl); err != nil {
				return err
			}
			for _, name := range []string{"internal", "aggregation"} {
				if w.state.Planes[name].Affected {
					if err := w.publishSource(ctx, name, w.state.ID+"/publish"); err != nil {
						return err
					}
				}
			}
			if err := e.advance(ctx, rotationPublish); err != nil {
				return err
			}
		}
		if requested == rotationRetire && w.state.Phase == rotationReady {
			if err := e.advance(ctx, rotationRetire); err != nil {
				return err
			}
		}
		if w.state.Phase != requested {
			return fmt.Errorf("certificate lifecycle is in %s; requested %s cannot skip or replay a different phase", w.state.Phase, requested)
		}
		switch requested {
		case rotationPublish:
			if err := w.reconcileAggregationTrust(ctx); err != nil {
				return err
			}
			if err := e.wait(ctx, "APIService public-trust convergence", w.checkAggregationTrust); err != nil {
				return err
			}
			return e.advance(ctx, rotationAdopt)
		case rotationAdopt:
			if err := e.applyPhase(ctx); err != nil {
				return err
			}
			if err := e.wait(ctx, "actual outbound and aggregation trust adoption", func(ctx context.Context) error {
				return w.observeLoaded(ctx, w.state)
			}); err != nil {
				return err
			}
			return e.advance(ctx, rotationIssue)
		case rotationIssue:
			for index, intent := range w.state.Execution.Resources {
				if intent.Kind == "Issuer" && intent.Spec["ca"] != nil {
					if err := w.applyIssuer(ctx, index); err != nil {
						return err
					}
				}
			}
			if err := e.applyPhase(ctx); err != nil {
				return err
			}
			if err := e.waitForEvidence(ctx); err != nil {
				return err
			}
			return e.advance(ctx, rotationReady)
		case rotationRetire:
			for _, name := range []string{"internal", "aggregation"} {
				if w.state.Planes[name].Affected {
					if err := w.publishSource(ctx, name, w.state.ID+"/complete"); err != nil {
						return err
					}
					if err := w.reconcileAggregationTrust(ctx); err != nil {
						return err
					}
				}
			}
			if err := e.wait(ctx, "APIService old-anchor removal", w.checkAggregationTrust); err != nil {
				return err
			}
			if err := e.waitForEvidence(ctx); err != nil {
				return err
			}
			if err := e.advance(ctx, rotationComplete); err != nil {
				return err
			}
			return e.finish(ctx)
		}
		return errors.New("unsupported certificate execution phase")
	})
}

func (e *certificateLifecycleExecutor) advance(ctx context.Context, phase rotationPhase) error {
	next, err := cloneCertificateLifecycle(e.writer.state)
	if err != nil {
		return err
	}
	next.Phase = phase
	return e.writer.save(ctx, next)
}

func (e *certificateLifecycleExecutor) wait(ctx context.Context, description string, check func(context.Context) error) error {
	var last error
	err := wait.PollUntilContextCancel(ctx, 200*time.Millisecond, true, func(ctx context.Context) (bool, error) {
		if err := e.writer.check(ctx); err != nil {
			return false, err
		}
		last = check(ctx)
		return last == nil, nil
	})
	if err != nil {
		return fmt.Errorf("%s: %w", description, errors.Join(err, last))
	}
	return nil
}

func (e *certificateLifecycleExecutor) waitForEvidence(ctx context.Context) error {
	return e.wait(ctx, "loaded trust and separate exact serving-pod evidence", func(ctx context.Context) error {
		if err := e.writer.observeLoaded(ctx, e.writer.state); err != nil {
			return err
		}
		return e.writer.observeServing(ctx, e.writer.state)
	})
}

func (e *certificateLifecycleExecutor) applyPhase(ctx context.Context) error {
	if e.applyPhaseFixture != nil {
		return e.applyPhaseFixture(ctx)
	}
	w := e.writer
	if e.history == nil {
		flags := &genericclioptions.ConfigFlags{Namespace: &w.store.namespace, Context: &e.kubeContext}
		configuration, err := initHelmConfig(flags)
		if err != nil {
			return err
		}
		e.history, err = e.impl.Helm.RunHelmHistory(configuration, w.store.release)
		if err != nil {
			return err
		}
	}
	objects, err := w.renderSelectedPhase(ctx, e.impl, e.options, e.selected, e.target, e.history, e.kubeContext)
	if err != nil {
		return err
	}
	for _, object := range objects {
		desired, err := w.admitSelectedMutation(ctx, object)
		if err != nil {
			return err
		}
		if err := w.applySelectedMutation(ctx, desired); err != nil {
			return err
		}
		if object.GetKind() == "Certificate" {
			if err := e.wait(ctx, "target aggregation leaf issuance", e.checkAggregationLeaf); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *certificateLifecycleExecutor) checkAggregationLeaf(ctx context.Context) error {
	w := e.writer
	plane := w.state.Planes["aggregation"]
	certificate, err := w.store.dynamic.Resource(rotationCertificateResource).Namespace(w.store.namespace).Get(ctx, "ucp-cert", metav1.GetOptions{})
	if err != nil {
		return err
	}
	ready, err := certificateResourceReady(certificate)
	if err != nil {
		return err
	}
	if !ready {
		return errors.New("aggregation Certificate has not observed its target generation")
	}
	name, _, err := unstructured.NestedString(certificate.Object, "spec", "secretName")
	if err != nil || name != w.state.Execution.ServingSecret {
		return errors.New("aggregation Certificate no longer selects the planned serving Secret")
	}
	secret, err := w.store.kube.CoreV1().Secrets(w.store.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if secret.Type != corev1.SecretTypeTLS || secret.UID == "" || secret.DeletionTimestamp != nil ||
		secret.Annotations["cert-manager.io/issuer-name"] != plane.Issuer.Name ||
		secret.Annotations["cert-manager.io/issuer-kind"] != plane.Issuer.Kind ||
		secret.Annotations["cert-manager.io/issuer-group"] != plane.Issuer.Group {
		return errors.New("target aggregation Secret is not bound to the selected issuer")
	}
	for _, owner := range secret.OwnerReferences {
		if owner.Kind != "Certificate" || owner.Name != certificate.GetName() || owner.UID != certificate.GetUID() ||
			owner.APIVersion != "cert-manager.io/v1" {
			return errors.New("target aggregation Secret has foreign ownership")
		}
	}
	pair, err := tls.X509KeyPair(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey])
	if err != nil {
		return errors.New("target aggregation Secret has an invalid certificate/key pair")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return err
	}
	dns := "ucp." + w.store.namespace + ".svc"
	if leaf.IsCA || len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != dns ||
		len(leaf.URIs) != 0 || len(leaf.IPAddresses) != 0 || len(leaf.EmailAddresses) != 0 {
		return errors.New("target aggregation leaf has an unexpected identity")
	}
	roots := x509.NewCertPool()
	ca, err := validatePublicCA(plane.NewCA, w.store.now())
	if err != nil {
		return err
	}
	roots.AddCert(ca)
	intermediates := x509.NewCertPool()
	for _, der := range pair.Certificate[1:] {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return err
		}
		intermediates.AddCert(cert)
	}
	_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: dns,
		CurrentTime: w.store.now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	return err
}

func (e *certificateLifecycleExecutor) finish(ctx context.Context) error {
	w := e.writer
	if w.state.Execution.FinalReleaseVersion != 0 {
		return w.check(ctx)
	}
	var version int
	var err error
	if e.commitFixture != nil {
		version, err = e.commitFixture(ctx)
	} else {
		version, err = e.commitChart(ctx)
	}
	if err != nil {
		return err
	}
	if err := e.waitForEvidence(ctx); err != nil {
		return err
	}
	next, err := cloneCertificateLifecycle(w.state)
	if err != nil {
		return err
	}
	next.Execution.FinalReleaseVersion = version
	return w.save(ctx, next)
}

func (e *certificateLifecycleExecutor) commitChart(ctx context.Context) (int, error) {
	w := e.writer
	if e.selected == nil || e.target == nil || w.state.Phase != rotationComplete {
		return 0, errors.New("final Helm reconciliation requires completed trust and its selected chart")
	}
	for _, entry := range []struct {
		value any
		hash  string
	}{{e.selected, w.state.Execution.ChartSHA256}, {e.target.Values, w.state.Execution.TargetValuesSHA256}} {
		hash, err := bootstrapDigest(entry.value)
		if err != nil || hash != entry.hash {
			return 0, errors.New("final Helm reconciliation changed its chart or values")
		}
	}
	flags := &genericclioptions.ConfigFlags{Namespace: &w.store.namespace, Context: &e.kubeContext}
	configuration := &helm.Configuration{}
	getter := initialCertificateGetter{RESTClientGetter: flags, ctx: ctx, check: w.check}
	if err := configuration.Init(getter, w.store.namespace, helmDriverSecret); err != nil {
		return 0, err
	}
	recorded, err := configuration.Releases.Last(w.store.release)
	if err != nil {
		return 0, err
	}
	previous, ok := recorded.(*releasev1.Release)
	if !ok {
		return 0, errors.New("final reconciliation requires a supported Helm release record")
	}
	if previous.Version != w.state.Execution.BaselineVersion {
		hash, err := bootstrapDigest(previous.Config)
		chartHash, chartErr := bootstrapDigest(previous.Chart)
		if err != nil || chartErr != nil || previous.Labels[certificateExecutionOwner] != w.state.ID ||
			hash != w.state.Execution.TargetValuesSHA256 || chartHash != w.state.Execution.ChartSHA256 {
			return 0, errors.New("Helm history changed outside this certificate execution")
		}
		if revisionStatus(previous) == releasecommon.StatusDeployed {
			return previous.Version, nil
		}
		if revisionStatus(previous) == releasecommon.StatusPendingUpgrade {
			if err := w.recoverPendingFinalRelease(ctx, previous); err != nil {
				return 0, err
			}
		}
	} else {
		hash, err := bootstrapDigest(previous.Config)
		if err != nil || hash != w.state.Execution.BaselineValuesSHA256 {
			return 0, errors.New("installed Helm baseline changed during certificate execution")
		}
	}
	upgrade := newUpgradeClient(configuration, w.store.namespace, true, false, e.options.Radius.Timeout,
		usesServerSideApply(configuration, w.store.release))
	upgrade.Labels = map[string]string{certificateExecutionOwner: w.state.ID}
	result, err := upgrade.RunWithContext(ctx, w.store.release, cloneChart(e.selected), copyValues(e.target.Values))
	if err != nil {
		if apierrors.IsConflict(err) {
			return 0, fmt.Errorf("final Helm reconciliation conflicted; preserve the completed trust journal: %w", err)
		}
		return 0, err
	}
	updated, ok := result.(*releasev1.Release)
	if !ok {
		return 0, errors.New("final Helm reconciliation returned an unsupported release")
	}
	return updated.Version, nil
}
