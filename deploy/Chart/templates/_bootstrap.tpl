{{/*
Fresh bootstrap has its own intent, before any workload or APIService UID exists.
Only the matching, leased install may render an unfinished bootstrap.
*/}}
{{- define "radius.bootstrapState" -}}
{{- $journal := lookup "v1" "ConfigMap" .Release.Namespace "radius-aggregation-bootstrap" -}}
{{- $state := dict -}}
{{- if not $journal -}}
{{- $namespace := lookup "v1" "Namespace" "" .Release.Namespace -}}
{{- if dig "metadata" "annotations" "radius.io/aggregation-bootstrap-intent" "" $namespace -}}
{{- fail "Namespace retains an aggregation bootstrap intent without its journal; resume the matching Radius install before changing the release." -}}
{{- end -}}
{{- end -}}
{{- if $journal -}}
{{- if or (not $journal.metadata.uid) (not $journal.metadata.resourceVersion) (dig "metadata" "deletionTimestamp" "" $journal) (dig "metadata" "ownerReferences" (list) $journal) (dig "metadata" "annotations" "meta.helm.sh/release-name" "" $journal) (ne (dig "metadata" "labels" "radius.io/aggregation-bootstrap" "" $journal) .Release.Name) (ne (len $journal.data) 1) $journal.binaryData (gt (len (dig "data" "state.json" "" $journal)) 65536) -}}
{{- fail "Aggregation bootstrap journal has foreign ownership or no stable identity." -}}
{{- end -}}
{{- $state = mustFromJson (dig "data" "state.json" "" $journal) -}}
{{- if or (ne (toString $state.version) "1") (ne $state.namespace .Release.Namespace) (ne $state.release .Release.Name) (not (has $state.phase (list "preparing" "installing" "verifying" "complete"))) (not (has $state.scope (list "componentsAndAggregation" "aggregationOnly"))) (not (has $state.mode (list "selfSigned" "caSecret" "issuerRef"))) (not (regexMatch `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$` $state.id)) -}}
{{- fail "Aggregation bootstrap identity, scope or phase is invalid." -}}
{{- end -}}
{{- if has $state.phase (list "installing" "verifying") -}}
{{- $lease := lookup "coordination.k8s.io/v1" "Lease" .Release.Namespace "radius-identity-operation" -}}
{{- if or (ne (default "" .Values.global.rbac.aggregationBootstrap) $state.id) (not $lease.metadata.uid) (dig "metadata" "deletionTimestamp" "" $lease) (ne (default "" .Values.global.rbac.aggregationBootstrapInvocation) $lease.metadata.uid) -}}
{{- fail "Aggregation bootstrap is unfinished; resume the matching leased Radius install." -}}
{{- end -}}
{{- $values := deepCopy .Values -}}
{{- $_ := unset $values.global.rbac "aggregationBootstrapInvocation" -}}
{{- if or (not $state.ca) (not (regexMatch `^[0-9a-f]{64}$` $state.trustSHA256)) (ne (sha256sum (toJson $values)) $state.installValuesSHA256) -}}
{{- fail "Bootstrap install settings or public trust differ from the recorded intent." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- toJson $state -}}
{{- end -}}

{{- define "radius.bootstrapPresentation" -}}
{{- $bootstrap := include "radius.bootstrapState" . | mustFromJson -}}
{{- $result := dict -}}
{{- if and $bootstrap (has $bootstrap.phase (list "installing" "verifying")) -}}
{{- $suffix := trunc 16 (replace "-" "" $bootstrap.id) -}}
{{- $issuer := printf "radius-ca-%s" $suffix -}}
{{- $plan := dict "scope" $bootstrap.scope "targetMode" $bootstrap.mode "targetTrustSHA256" $bootstrap.trustSHA256 "targetTrustMap" (printf "radius-mtls-ca-%s" (trunc 32 $bootstrap.trustSHA256)) "servingSecret" (printf "ucp-cert-aggregation-%s" $suffix) "serviceSelector" (dict "app.kubernetes.io/name" "ucp") "aggregation" (dict "proofPort" $bootstrap.proofPort) -}}
{{- if eq $bootstrap.mode "issuerRef" -}}
{{- if or (not $bootstrap.external) $bootstrap.signer $bootstrap.signerUID -}}
{{- fail "Bootstrap external issuer must be a public pin without signing-key ownership." -}}
{{- end -}}
{{- $issuer = $bootstrap.external.name -}}
{{- $_ := set $plan "externalIssuer" $bootstrap.external -}}
{{- else -}}
{{- if or (not $bootstrap.signer) (not $bootstrap.signerUID) $bootstrap.external -}}
{{- fail "Bootstrap local issuer has no observed signing identity." -}}
{{- end -}}
{{- $_ := set $plan "signingSecret" $bootstrap.signer -}}
{{- $_ := set $plan "signingUID" $bootstrap.signerUID -}}
{{- end -}}
{{- $_ := set $plan "issuer" $issuer -}}
{{- $result = dict "bootstrap" true "id" $bootstrap.id "phase" $bootstrap.phase "newCA" $bootstrap.ca "plan" $plan -}}
{{- else if and $bootstrap (eq $bootstrap.phase "complete") -}}
{{- fail "Completed bootstrap is missing its initial CA lifecycle journal; restore it before changing the release." -}}
{{- end -}}
{{- toJson $result -}}
{{- end -}}
