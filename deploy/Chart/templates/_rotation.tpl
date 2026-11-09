{{/*
The public journal is authoritative after completion. Active transactions may
only be advanced by the phase driver, not by normal Helm rendering.
*/}}
{{- define "radius.rotationJournal" -}}
{{- $journal := .journal -}}
{{- $root := .root -}}
{{- $state := dict -}}
{{- if $journal -}}
{{- if or (ne (dig "metadata" "labels" "radius.io/trust-owner" "" $journal) $root.Release.Name) (dig "metadata" "ownerReferences" (list) $journal) (dig "metadata" "deletionTimestamp" "" $journal) (dig "metadata" "annotations" "meta.helm.sh/release-name" "" $journal) -}}
{{- fail "Radius CA rotation journal has unexpected ownership." -}}
{{- end -}}
{{- $state = mustFromJson (dig "data" "state.json" "" $journal) -}}
{{- if or (not (kindIs "float64" $state.schemaVersion)) (not (has (toString $state.schemaVersion) (list "1" "2" "3"))) (ne $state.namespace $root.Release.Namespace) (ne $state.release $root.Release.Name) -}}
{{- fail "Radius CA rotation journal does not match this installation." -}}
{{- end -}}
{{- $plan := $state.plan | default dict -}}
{{- if eq (toString $state.schemaVersion) "3" -}}
{{- if or (ne $state.bootstrap true) (ne $state.phase "complete") $state.oldCA $state.previousRevision $state.priorRevisions $state.recovery $plan.oldComponentCA -}}
{{- fail "Initial bootstrap completion must not fabricate a predecessor CA or rotation phase." -}}
{{- end -}}
{{- else if $state.bootstrap -}}
{{- fail "Bootstrap completion requires its explicit lifecycle schema." -}}
{{- end -}}
{{- if eq (toString $state.schemaVersion) "1" -}}
{{- if or $plan.scope $plan.aggregation $plan.externalIssuer -}}
{{- fail "Legacy CA rotation journals cannot contain version 2 scope or issuer fields." -}}
{{- end -}}
{{- else -}}
{{- if or (not (has $plan.scope (list "componentsAndAggregation" "aggregationOnly"))) (not $plan.aggregation) (not $plan.aggregation.deploymentUID) (not (kindIs "float64" $plan.aggregation.proofPort)) (lt (int $plan.aggregation.proofPort) 1) (gt (int $plan.aggregation.proofPort) 65535) (ne (float64 (int $plan.aggregation.proofPort)) $plan.aggregation.proofPort) -}}
{{- fail "Version 2 CA rotation requires a valid scope and separate aggregation workload." -}}
{{- end -}}
{{- if eq $plan.scope "aggregationOnly" -}}
{{- if or $plan.consumers $plan.oldComponentCA -}}
{{- fail "Aggregation-only CA rotation must not contain component PKI." -}}
{{- end -}}
{{- else -}}
{{- if or (not (kindIs "slice" $plan.consumers)) (ne (len $plan.consumers) 5) (and (ne (toString $state.schemaVersion) "3") (not $plan.oldComponentCA)) -}}
{{- fail "Managed CA rotation requires all five component identities and public trust." -}}
{{- end -}}
{{- $seen := dict -}}
{{- range $consumer := $plan.consumers -}}
{{- if or (not (has $consumer.name (list "ucp" "applications-rp" "dynamic-rp" "controller" "bicep-de"))) (hasKey $seen $consumer.name) (not $consumer.uid) (and (eq $consumer.name "ucp") (ne $consumer.uid $plan.aggregation.deploymentUID)) -}}
{{- fail "Managed CA rotation contains inconsistent component identities." -}}
{{- end -}}
{{- $_ := set $seen $consumer.name true -}}
{{- end -}}
{{- end -}}
{{- if eq $plan.targetMode "issuerRef" -}}
{{- $issuer := $plan.externalIssuer | default dict -}}
{{- if or (ne $issuer.group "cert-manager.io") (not (has $issuer.kind (list "Issuer" "ClusterIssuer"))) (not $issuer.name) (ne $issuer.name $plan.issuer) (not $issuer.uid) (not $issuer.specSHA256) (not $issuer.requestPolicy) $plan.signingSecret $plan.signingUID -}}
{{- fail "External CA rotation requires a pinned public issuer without a signing Secret." -}}
{{- end -}}
{{- else if $plan.externalIssuer -}}
{{- fail "Local CA rotation must not contain an external issuer." -}}
{{- end -}}
{{- end -}}
{{- if ne $state.phase "complete" -}}
{{- fail "Radius CA rotation is unfinished. Resume its explicit phase before changing the Helm release." -}}
{{- end -}}
{{- end -}}
{{- toJson $state -}}
{{- end -}}

{{- define "radius.rotationState" -}}
{{- $bootstrap := include "radius.bootstrapState" . | mustFromJson -}}
{{- $journal := lookup "v1" "ConfigMap" .Release.Namespace "radius-ca-rotation" -}}
{{- $state := include "radius.rotationJournal" (dict "journal" $journal "root" .) | mustFromJson -}}
{{- if not $state -}}
{{- $state = include "radius.bootstrapPresentation" . | mustFromJson -}}
{{- end -}}
{{- if $state -}}
{{- if or (not $state.plan) (not $state.plan.targetTrustMap) (not $state.plan.targetTrustSHA256) (not $state.plan.issuer) (not $state.plan.servingSecret) -}}
{{- fail "Radius CA rotation journal is missing its completed resource plan." -}}
{{- end -}}
{{- end -}}
{{- toJson $state -}}
{{- end -}}

{{/*
Read each mutable target before reading the journal. Rendered UID/resourceVersion
preconditions prevent an older Helm apply from undoing a later phase write.
*/}}
{{- define "radius.rotationSnapshot" -}}
{{- $object := lookup .apiVersion .kind .root.Release.Namespace .name -}}
{{- $bootstrap := include "radius.bootstrapState" .root | mustFromJson -}}
{{- if and $bootstrap (eq $bootstrap.phase "preparing") (has .kind (list "Deployment" "Service")) -}}
{{- fail "Aggregation bootstrap has not verified its selected CA leaf; workloads cannot be installed yet." -}}
{{- end -}}
{{- $state := include "radius.rotationState" .root | mustFromJson -}}
{{- if and $object $state -}}
{{- if or (ne (dig "metadata" "annotations" "meta.helm.sh/release-name" "" $object) .root.Release.Name) (ne (dig "metadata" "annotations" "meta.helm.sh/release-namespace" "" $object) .root.Release.Namespace) (dig "metadata" "deletionTimestamp" "" $object) -}}
{{- fail "A managed CA rotation resource changed ownership or is terminating." -}}
{{- end -}}
{{- if and $state.bootstrap (ne $state.phase "complete") (ne (dig "metadata" "annotations" "radius.io/aggregation-bootstrap" "" $object) $state.id) -}}
{{- fail "Bootstrap cannot adopt a resource without its matching creation intent." -}}
{{- end -}}
{{- else if and $object (or (dig "metadata" "annotations" "radius.io/trust-revision" "" $object) (dig "metadata" "annotations" "radius.io/ca-rotation" "" $object)) -}}
{{- fail "A managed CA rotation resource is missing its public journal." -}}
{{- end -}}
{{- toJson (dict "object" $object "state" $state) -}}
{{- end -}}

{{- define "radius.rotationMetadata" -}}
{{- if .object -}}
uid: {{ .object.metadata.uid | quote }}
resourceVersion: {{ .object.metadata.resourceVersion | quote }}
{{- end -}}
{{- if .state }}
annotations:
  {{- if and .state.bootstrap (ne .state.phase "complete") }}
  radius.io/aggregation-bootstrap: {{ .state.id | quote }}
  {{- else }}
  radius.io/trust-revision: {{ printf "%s/complete" .state.id | quote }}
  {{- end }}
  {{- $annotations := dig "metadata" "annotations" (dict) .object }}
  {{- range $key := list "radius.io/ca-rotation" "radius.io/aggregation-route" "radius.io/aggregation-old-hash" "radius.io/aggregation-new-hash" }}
  {{- if hasKey $annotations $key }}
  {{ $key }}: {{ index $annotations $key | quote }}
  {{- end }}
  {{- end }}
{{- end -}}
{{- end -}}
