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
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// CheckProviderTransition performs read-only validation and reports whether the
// explicit provider selector starts or resumes an outage-bearing transaction.
func (i *Impl) CheckProviderTransition(ctx context.Context, options ClusterOptions, kubeContext string) (bool, error) {
	return i.runProviderTransition(ctx, options, kubeContext, true)
}

func (i *Impl) runProviderTransition(ctx context.Context, options ClusterOptions, kubeContext string, readOnly bool) (bool, error) {
	if options.MTLS.Provider == "" {
		return false, nil
	}
	if err := options.MTLS.ValidateFlags(nil); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, effectiveTimeout(options.Radius.Timeout))
	defer cancel()
	configuration, err := initHelmConfig(&genericclioptions.ConfigFlags{Namespace: &options.Radius.Namespace, Context: &kubeContext})
	if err != nil {
		return false, err
	}
	source, err := i.Helm.RunHelmGet(configuration, options.Radius.ReleaseName)
	if err != nil {
		return false, err
	}
	config, err := configuration.RESTClientGetter.ToRESTConfig()
	if err != nil {
		return false, err
	}
	caStore, err := i.rotationStoreForContext(kubeContext, options.Radius.Namespace)
	if err != nil {
		return false, err
	}
	kube, dynamicClient := caStore.kube, caStore.dynamic
	store := &providerTransitionStore{kube: kube}
	_, current, journalErr := store.read(ctx, options.Radius.Namespace)
	if journalErr != nil && !apierrors.IsNotFound(journalErr) {
		return false, journalErr
	}
	resuming := journalErr == nil && current.Phase != providerComplete
	if resuming {
		if current.Plan.Target != options.MTLS.Provider || current.Plan.Helm == nil {
			return true, errors.New("resume the existing provider transition with its original explicit --mtls-provider target")
		}
		source, _, err = providerReleaseHistory(configuration, current.Plan)
		if err != nil {
			return true, err
		}
	}
	if source == nil || source.Chart == nil || source.Chart.Metadata == nil {
		return false, errors.New("provider transition requires the installed Radius chart and values")
	}
	effective := valuesutil.CoalesceTables(copyValues(source.Config), copyValues(source.Chart.Values))
	old, _, err := providerValue(effective)
	if err != nil {
		return false, err
	}
	if !resuming && old == options.MTLS.Provider {
		return false, nil
	}
	if source.Chart.Metadata.Annotations["radius.io/provider-transition-protocol"] != "1" {
		return true, errors.New("upgrade the installed chart to the coordinated provider-transition protocol before changing provider")
	}
	if options.Radius.ReleaseName != radiusReleaseName || options.ResetValues || options.Radius.Reinstall ||
		options.Radius.ChartPath != "" || options.Radius.ChartVersion != "" ||
		len(options.Radius.SetFileArgs)+len(options.Contour.SetArgs)+len(options.Contour.SetFileArgs) != 0 ||
		options.Certificates.RotationPhase != "" {
		return true, errors.New("provider changes cannot be combined with chart, version, reset, CA rotation, file values or Contour changes")
	}
	if source.Info == nil {
		return true, errors.New("provider source release has no installation status")
	}
	certificateOptions := options
	certificateOptions.MTLS = MTLSProviderOptions{}
	certificateOptions.RadiusReleaseValues = copyValues(source.Config)
	if _, err := i.prepareCertificates(ctx, certificateOptions, source.Chart.Values, map[string]any{}, kubeContext, true); err != nil {
		return true, err
	}
	_, ca, err := caStore.read(ctx)
	if err != nil {
		return true, err
	}
	snapshot, err := caStore.aggregationTransitionSnapshot(ctx)
	if err != nil || !snapshot.Complete {
		if err != nil {
			return true, err
		}
		return true, errors.New("complete selected-CA aggregation bootstrap or migration before changing provider")
	}
	if resuming && snapshot != current.Plan.Aggregation {
		return true, errors.New("aggregation identity changed since provider transition planning")
	}
	overrides := map[string]any{}
	if err := parseSetValuesInto(overrides, options.Radius.SetArgs, nil); err != nil {
		return true, err
	}
	replicas := map[string]int32{}
	if resuming {
		for _, workload := range current.Plan.Workloads {
			replicas[workload.Name] = workload.Replicas
		}
	} else {
		for _, name := range rotationComponents {
			deployment, err := kube.AppsV1().Deployments(options.Radius.Namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return true, err
			}
			if deployment.Spec.Replicas == nil {
				return true, errors.New("provider transition requires explicit workload replica counts")
			}
			replicas[name] = *deployment.Spec.Replicas
		}
	}
	id := current.Plan.ID
	if !resuming {
		chartDigest, err := providerChartDigest(source.Chart)
		if err != nil {
			return true, err
		}
		seed, err := providerHash(struct {
			Namespace, Chart, Target string
			Revision                 int
			Values, Overrides        map[string]any
			Replicas                 map[string]int32
			Aggregation              providerAggregationSnapshot
		}{options.Radius.Namespace, chartDigest, options.MTLS.Provider, source.Version, source.Config, overrides, replicas, snapshot})
		if err != nil {
			return true, err
		}
		// Preparation interrupted before the public plan can be written reuses
		// the same isolated names, without persisting rendered credentials.
		id = uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)).String()
	}
	values, err := providerTargetValues(source, options.MTLS.Provider, id, overrides, replicas)
	if err != nil {
		return true, err
	}
	policy := func(ctx context.Context) error {
		return i.renderIssuerPolicy(ctx, options, source.Chart, configuration, values, kubeContext, false, nil)
	}
	caStore.verifyIssuerPolicy = func(ctx context.Context, _ rotationState) error { return policy(ctx) }
	prepared, err := i.renderProviderPreparation(ctx, source.Chart, options, values, kubeContext)
	if err != nil {
		return true, err
	}
	targets, err := store.materializeTargets(ctx, options.Radius.Namespace, prepared, replicas)
	if err != nil {
		return true, err
	}
	names := map[string]bool{}
	for _, spec := range targets {
		for _, name := range providerConfigurationNames(spec.Template.Spec) {
			names[name] = true
		}
	}
	var plan providerTransitionPlan
	if resuming {
		plan = current.Plan
		digest, err := providerHash(values)
		if err != nil || digest != plan.Helm.TargetValuesSHA256 {
			return true, errors.New("retry must retain the original provider target flags and settings")
		}
	} else {
		intent, err := newProviderHelmIntent(source, values, slices.Sorted(maps.Keys(names)))
		if err != nil {
			return true, err
		}
		mode, err := resolveAuthzMode(nil, effective)
		if err != nil {
			return true, err
		}
		plan = providerTransitionPlan{ID: id, Namespace: options.Radius.Namespace, Release: radiusReleaseName,
			Source: old, Target: options.MTLS.Provider, Mode: string(mode), Aggregation: snapshot, Helm: intent}
	}
	checks := &providerRuntimeChecks{ca: caStore, config: config, observe: i.observeProviderTransition}
	checks.validate = func(ctx context.Context, _ providerTransitionPlan, _ map[string]appsv1.DeploymentSpec) error {
		if _, _, err := providerReleaseHistory(configuration, plan); err != nil {
			return err
		}
		if err := policy(ctx); err != nil {
			return err
		}
		if plan.Target == "radius" {
			return caStore.checkProviderCertificates(ctx, ca)
		}
		for _, expected := range prepared.discovery {
			if _, err := dynamicClient.Resource(providerDiscoveryResource(expected)).Get(ctx, expected.GetName(), metav1.GetOptions{}); err != nil {
				return err
			}
		}
		return caStore.prepareProviderDiscovery(ctx, prepared, nil, true)
	}
	store.checks = checks
	preparationFence := func(ctx context.Context) error {
		if _, _, err := providerReleaseHistory(configuration, plan); err != nil {
			return err
		}
		if err := caStore.checkAggregationTransitionSnapshot(ctx, snapshot); err != nil {
			return err
		}
		return policy(ctx)
	}
	if err := preparationFence(ctx); err != nil {
		return true, err
	}
	population := plan
	population.Workloads = nil
	for _, name := range rotationComponents {
		population.Workloads = append(population.Workloads, providerTransitionWorkload{Name: name})
	}
	if err := store.checkAutoscalers(ctx, population); err != nil {
		return true, err
	}
	if err := store.prepareConfigurationsMode(ctx, plan.Namespace, prepared, nil, true); err != nil {
		return true, err
	}
	if plan.Target == "radius" {
		err = caStore.prepareProviderCertificates(ctx, ca, nil, true)
	} else {
		err = caStore.prepareProviderDiscovery(ctx, prepared, nil, true)
	}
	if err != nil {
		return true, err
	}
	if !resuming && plan.Target == "external" {
		plan.SourceSecrets, err = caStore.providerSourceSecrets(ctx, ca)
		if err != nil {
			return true, err
		}
	}
	if resuming {
		if err := store.validateTargets(ctx, current, targets); err != nil {
			return true, err
		}
		if err := store.inspect(ctx, current); err != nil {
			return true, err
		}
	}
	if readOnly {
		return true, nil
	}
	if !options.MTLS.OutageConfirmed {
		return true, errors.New("provider change interrupts all Radius components and requires explicit outage confirmation")
	}
	i.logInfo("Changing the internal provider stops all five Radius components before any target starts. Failures retain the journal for an explicit retry; there is no automatic rollback.")
	if plan.Target == "external" {
		i.logInfo("Warning: the operator must protect EVERY connection, including storage and queues. Runtime receipts do not prove operator-provided encryption.")
	}
	if !resuming {
		err = withIdentityOperation(ctx, kube, plan.Namespace, func(operation *identityOperation) error {
			guard := func(ctx context.Context) error {
				if err := preparationFence(ctx); err != nil {
					return err
				}
				return operation.checkProviderJournal(ctx, plan.Release, i.completedProviderJournalValidator(plan.Namespace, plan.Release))
			}
			if err := guard(ctx); err != nil {
				return err
			}
			if err := store.prepareConfigurations(ctx, plan.Namespace, prepared, guard); err != nil {
				return err
			}
			if plan.Target == "radius" {
				if err := caStore.prepareProviderCertificates(ctx, ca, guard, false); err != nil {
					return err
				}
				for {
					materialErr := caStore.checkProviderCertificates(ctx, ca)
					if materialErr == nil {
						break
					}
					if err := providerPause(ctx); err != nil {
						return fmt.Errorf("provider component issuance did not finish: %w", errors.Join(err, materialErr))
					}
					if err := guard(ctx); err != nil {
						return err
					}
				}
				return nil
			}
			return caStore.prepareProviderDiscovery(ctx, prepared, guard, false)
		})
		if err != nil {
			return true, err
		}
		planned, err := store.planWithID(ctx, plan.Namespace, plan.Source, plan.Target, plan.Mode, plan.ID, targets)
		if err != nil {
			return true, err
		}
		planned.Helm = plan.Helm
		planned.SourceSecrets = plan.SourceSecrets
		plan = planned
		if err := store.begin(ctx, plan, targets, true); err != nil {
			return true, err
		}
	}
	for {
		_, state, err := store.read(ctx, plan.Namespace)
		if err != nil {
			return true, err
		}
		if state.Plan.ID != plan.ID {
			return true, errors.New("provider transaction changed while executing")
		}
		if state.Phase == providerComplete {
			return true, nil
		}
		if state.Phase == providerRecording {
			return true, store.record(ctx, plan.Namespace, plan.ID, targets,
				func(ctx context.Context, fence func(context.Context) error) error {
					if err := recordProviderRelease(ctx, configuration, plan, source.Chart, values, options.Radius.Timeout, fence); err != nil {
						return err
					}
					if plan.Target == "external" {
						return caStore.retireProviderSecrets(ctx, plan, fence)
					}
					return nil
				})
		}
		err = store.step(ctx, plan.Namespace, plan.ID, targets)
		if errors.Is(err, errProviderTransitionWaiting) {
			if err := providerPause(ctx); err != nil {
				return true, fmt.Errorf("provider transition remains unfinished; retry the same flags: %w", err)
			}
			continue
		}
		if err != nil {
			return true, fmt.Errorf("provider transition remains unfinished; retry the same flags: %w", err)
		}
	}
}

func providerPause(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
