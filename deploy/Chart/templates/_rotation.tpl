{{/*
The public journal is authoritative after completion. Active transactions may
only be advanced by the phase driver, not by normal Helm rendering.
*/}}
{{- define "radius.rotationState" -}}
{{- $journal := lookup "v1" "ConfigMap" .Release.Namespace "radius-ca-rotation" -}}
{{- $state := dict -}}
{{- if $journal -}}
{{- if or (ne (dig "metadata" "labels" "radius.io/trust-owner" "" $journal) .Release.Name) (dig "metadata" "ownerReferences" (list) $journal) (dig "metadata" "deletionTimestamp" "" $journal) (dig "metadata" "annotations" "meta.helm.sh/release-name" "" $journal) -}}
{{- fail "Radius CA rotation journal has unexpected ownership." -}}
{{- end -}}
{{- $state = mustFromJson (dig "data" "state.json" "" $journal) -}}
{{- if or (not (kindIs "float64" $state.schemaVersion)) (ne (toString $state.schemaVersion) "1") (ne $state.namespace .Release.Namespace) (ne $state.release .Release.Name) -}}
{{- fail "Radius CA rotation journal does not match this installation." -}}
{{- end -}}
{{- if ne $state.phase "complete" -}}
{{- fail "Radius CA rotation is unfinished. Resume its explicit phase before changing the Helm release." -}}
{{- end -}}
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
{{- $state := include "radius.rotationState" .root | mustFromJson -}}
{{- if and $object $state -}}
{{- if or (ne (dig "metadata" "annotations" "meta.helm.sh/release-name" "" $object) .root.Release.Name) (ne (dig "metadata" "annotations" "meta.helm.sh/release-namespace" "" $object) .root.Release.Namespace) (dig "metadata" "deletionTimestamp" "" $object) -}}
{{- fail "A managed CA rotation resource changed ownership or is terminating." -}}
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
  radius.io/trust-revision: {{ printf "%s/complete" .state.id | quote }}
  {{- $annotations := dig "metadata" "annotations" (dict) .object }}
  {{- range $key := list "radius.io/ca-rotation" "radius.io/aggregation-route" "radius.io/aggregation-old-hash" "radius.io/aggregation-new-hash" }}
  {{- if hasKey $annotations $key }}
  {{ $key }}: {{ index $annotations $key | quote }}
  {{- end }}
  {{- end }}
{{- end -}}
{{- end -}}
