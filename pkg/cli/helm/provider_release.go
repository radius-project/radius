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
	"io"
	"net/http"
	"slices"
	"time"

	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

func providerChartDigest(selected *chart.Chart) (string, error) {
	if selected == nil || selected.Metadata == nil {
		return "", errors.New("provider transition requires the complete installed chart")
	}
	children := make([]string, 0, len(selected.Dependencies()))
	for _, dependency := range selected.Dependencies() {
		digest, err := providerChartDigest(dependency)
		if err != nil {
			return "", err
		}
		children = append(children, digest)
	}
	return providerHash(struct {
		Chart        *chart.Chart
		Dependencies []string
	}{selected, children})
}

func newProviderHelmIntent(source *releasev1.Release, target map[string]any, configurations []string) (*providerHelmIntent, error) {
	if source == nil || source.Version < 1 || source.Info == nil || source.Info.Status != releasecommon.StatusDeployed {
		return nil, errors.New("provider transition requires a deployed source Helm revision")
	}
	h := &providerHelmIntent{SourceRevision: source.Version, TargetConfigurations: slices.Clone(configurations)}
	var err error
	if h.ChartSHA256, err = providerChartDigest(source.Chart); err != nil {
		return nil, err
	}
	if h.SourceValuesSHA256, err = providerHash(source.Config); err != nil {
		return nil, err
	}
	if h.SourceManifestSHA256, err = providerHash(source.Manifest); err != nil {
		return nil, err
	}
	if h.TargetValuesSHA256, err = providerHash(target); err != nil {
		return nil, err
	}
	return h, nil
}

// Only this transaction's exact target revisions may follow its source. A
// pending revision with matching values alone is not proof of our ownership.
func providerReleaseHistory(configuration *helm.Configuration, plan providerTransitionPlan) (*releasev1.Release, *releasev1.Release, error) {
	if plan.Helm == nil || configuration == nil || configuration.Releases == nil {
		return nil, nil, errors.New("provider transition is missing pinned Helm history")
	}
	history, err := configuration.Releases.History(plan.Release)
	if err != nil {
		return nil, nil, err
	}
	var source, latest *releasev1.Release
	seen := map[int]bool{}
	for _, entry := range history {
		revision, err := asRelease(entry)
		if err != nil || revision == nil || revision.Name != plan.Release || revision.Namespace != plan.Namespace ||
			revision.Info == nil || revision.Version < 1 || seen[revision.Version] {
			return nil, nil, errors.New("provider transition has invalid or duplicate Helm history")
		}
		seen[revision.Version] = true
		if latest == nil || revision.Version > latest.Version {
			latest = revision
		}
		if revision.Version < plan.Helm.SourceRevision {
			continue
		}
		chartDigest, err := providerChartDigest(revision.Chart)
		if err != nil {
			return nil, nil, err
		}
		valuesDigest, err := providerHash(revision.Config)
		if err != nil {
			return nil, nil, err
		}
		if chartDigest != plan.Helm.ChartSHA256 {
			return nil, nil, errors.New("provider transition Helm chart changed")
		}
		if revision.Version == plan.Helm.SourceRevision {
			manifestDigest, err := providerHash(revision.Manifest)
			if err != nil {
				return nil, nil, err
			}
			if valuesDigest != plan.Helm.SourceValuesSHA256 || manifestDigest != plan.Helm.SourceManifestSHA256 ||
				!slices.Contains([]releasecommon.Status{releasecommon.StatusDeployed, releasecommon.StatusSuperseded}, revision.Info.Status) {
				return nil, nil, errors.New("provider transition source Helm revision changed")
			}
			source = revision
			continue
		}
		if revision.Labels[providerTransitionMarker] != plan.ID || valuesDigest != plan.Helm.TargetValuesSHA256 ||
			!slices.Contains([]releasecommon.Status{releasecommon.StatusDeployed, releasecommon.StatusFailed,
				releasecommon.StatusPendingUpgrade, releasecommon.StatusSuperseded}, revision.Info.Status) {
			return nil, nil, errors.New("an unrelated Helm operation followed the provider transition source")
		}
	}
	if source == nil || latest == nil {
		return nil, nil, errors.New("provider transition source Helm revision is no longer available")
	}
	for version := source.Version; version <= latest.Version; version++ {
		if !seen[version] {
			return nil, nil, errors.New("provider transition Helm history has a revision gap")
		}
	}
	return source, latest, nil
}

type providerGuardedGetter struct {
	genericclioptions.RESTClientGetter
	ctx   context.Context
	fence func(context.Context) error
}

func (g providerGuardedGetter) ToRESTConfig() (*rest.Config, error) {
	config, err := g.RESTClientGetter.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	config = rest.CopyConfig(config)
	config.Wrap(func(base http.RoundTripper) http.RoundTripper {
		return providerGuardedTransport{base: base, ctx: g.ctx, fence: g.fence}
	})
	return config, nil
}

type providerGuardedTransport struct {
	base  http.RoundTripper
	ctx   context.Context
	fence func(context.Context) error
}

func (t providerGuardedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(request.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	finish := func() { stop(); cancel() }
	if err := t.ctx.Err(); err != nil {
		finish()
		return nil, err
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead && request.Method != http.MethodOptions {
		if t.fence == nil {
			finish()
			return nil, errors.New("provider Helm write requires a transaction fence")
		}
		if err := t.fence(ctx); err != nil {
			finish()
			return nil, err
		}
	}
	response, err := t.base.RoundTrip(request.Clone(ctx))
	if err != nil {
		finish()
		return nil, err
	}
	if response.Body == nil {
		finish()
	} else {
		response.Body = &providerGuardedBody{ReadCloser: response.Body, finish: finish}
	}
	return response, nil
}

type providerGuardedBody struct {
	io.ReadCloser
	finish func()
}

func (b *providerGuardedBody) Close() error {
	defer b.finish()
	return b.ReadCloser.Close()
}

func recordProviderRelease(ctx context.Context, configuration *helm.Configuration, plan providerTransitionPlan,
	selected *chart.Chart, values map[string]any, timeout time.Duration, fence func(context.Context) error) error {
	if fence == nil || plan.Helm == nil {
		return errors.New("provider Helm recording requires pinned intent and a write fence")
	}
	chartDigest, err := providerChartDigest(selected)
	if err != nil {
		return err
	}
	valuesDigest, err := providerHash(values)
	if err != nil {
		return err
	}
	if chartDigest != plan.Helm.ChartSHA256 || valuesDigest != plan.Helm.TargetValuesSHA256 {
		return errors.New("provider final chart or settings differ from the pinned intent")
	}
	writeFence := func(ctx context.Context) error {
		if _, _, err := providerReleaseHistory(configuration, plan); err != nil {
			return err
		}
		return fence(ctx)
	}
	if err := writeFence(ctx); err != nil {
		return err
	}
	_, latest, err := providerReleaseHistory(configuration, plan)
	if err != nil {
		return err
	}
	if latest.Version > plan.Helm.SourceRevision && latest.Info.Status == releasecommon.StatusDeployed {
		return nil
	}
	guarded := &helm.Configuration{}
	getter, ok := configuration.RESTClientGetter.(genericclioptions.RESTClientGetter)
	if !ok {
		return errors.New("provider Helm recording requires the selected Kubernetes configuration")
	}
	if err := guarded.Init(providerGuardedGetter{RESTClientGetter: getter, ctx: ctx, fence: writeFence},
		plan.Namespace, helmDriverSecret); err != nil {
		return err
	}
	if latest.Info.Status == releasecommon.StatusPendingUpgrade {
		// The shared lease was acquired without takeover; the exact labeled
		// revision belongs to this interrupted transaction, never another upgrade.
		latest.SetStatus(releasecommon.StatusFailed, "Interrupted provider transition; resuming the same target")
		if err := guarded.Releases.Update(latest); err != nil {
			return err
		}
	}
	// Workloads have already converged to the pinned target. The surrounding
	// recorder obtains fresh provider and aggregate evidence again after Helm;
	// generic Helm readiness is neither needed nor a substitute for that proof.
	upgrade := newUpgradeClient(guarded, plan.Namespace, false, false, timeout, usesServerSideApply(configuration, plan.Release))
	upgrade.MaxHistory = 0 // Retain the pinned source throughout arbitrarily many interrupted retries.
	upgrade.Labels = map[string]string{providerTransitionMarker: plan.ID}
	upgrade.Description = "Radius provider transition " + plan.ID
	if _, err := upgrade.RunWithContext(ctx, plan.Release, cloneChart(selected), copyValues(values)); err != nil {
		return err
	}
	_, latest, err = providerReleaseHistory(configuration, plan)
	if err != nil {
		return err
	}
	if latest.Version <= plan.Helm.SourceRevision || latest.Info.Status != releasecommon.StatusDeployed {
		return errors.New("provider target Helm revision has not completed")
	}
	return fence(ctx)
}
