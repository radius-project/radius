{{/*
The provider journal stays unfinished until the final, pinned Helm revision is
recorded. Preparation renders only its explicit resource subset, not this gate.
*/}}
{{- define "radius.providerState" -}}
{{- $journal := lookup "v1" "ConfigMap" .Release.Namespace "radius-provider-transition" -}}
{{- $state := dict -}}
{{- if $journal -}}
{{- if or (ne (dig "metadata" "labels" "radius.io/provider-transition" "" $journal) .Release.Name) (dig "metadata" "ownerReferences" (list) $journal) (dig "metadata" "deletionTimestamp" "" $journal) (dig "metadata" "annotations" "meta.helm.sh/release-name" "" $journal) (ne (len $journal.data) 1) -}}
{{- fail "Provider transition journal has unexpected ownership or content." -}}
{{- end -}}
{{- $state = mustFromJson (dig "data" "state.json" "" $journal) -}}
{{- $plan := $state.plan | default dict -}}
{{- if or (ne (toString $state.version) "1") (ne $plan.namespace .Release.Namespace) (ne $plan.release .Release.Name) (not (has $plan.source (list "radius" "external"))) (not (has $plan.target (list "radius" "external"))) (eq $plan.source $plan.target) (not (has $plan.mode (list "dryRun" "enforce"))) (not (regexMatch `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$` $plan.id)) (not $plan.helm) -}}
{{- fail "Provider transition journal does not contain a pinned installation and Helm intent." -}}
{{- end -}}
{{- if not (has $state.phase (list "recording" "complete")) -}}
{{- fail "Provider transition is unfinished. Resume its coordinated driver before changing Helm resources." -}}
{{- end -}}
{{- if ne (include "radius.authz.provider" .) $plan.target -}}
{{- fail "Changing the mTLS provider requires a new coordinated provider transition." -}}
{{- end -}}
{{- if eq $state.phase "recording" -}}
{{- if or (ne (include "radius.authz.mode" .) $plan.mode) (ne (sha256sum (toJson .Values)) $plan.helm.targetValuesSHA256) -}}
{{- fail "Final provider Helm settings differ from the verified transaction." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- toJson $state -}}
{{- end -}}

{{- define "radius.providerScopeHandoff" -}}
{{- $provider := include "radius.providerState" .root | mustFromJson -}}
{{- $ca := .state -}}
{{- $journal := lookup "v1" "ConfigMap" .root.Release.Namespace "radius-ca-rotation" -}}
{{- if or (not $provider) (not $journal) (not $provider.plan.aggregation.complete) (ne $provider.plan.aggregation.journalUID (dig "metadata" "uid" "" $journal)) (ne $provider.plan.aggregation.revision (printf "%s/complete" $ca.id)) (ne $provider.plan.aggregation.apiServiceUID $ca.apiServiceUID) (ne $provider.plan.aggregation.trustSHA256 $ca.plan.targetTrustSHA256) (ne $provider.plan.aggregation.scope (default "componentsAndAggregation" $ca.plan.scope)) -}}
{{- fail "Provider scope handoff requires the same completed aggregation identity; CA and provider changes cannot be combined." -}}
{{- end -}}
{{- end -}}
